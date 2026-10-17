package aitools

import (
	"archive/tar"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gazebo-web/fuel-server/bundles/projects"
	"github.com/gazebo-web/fuel-server/mcp"
)

// Generated models (plan §5): the agent writes build123d code, the mesh
// worker (cmd/meshworker) turns it into a model folder, and these tools
// commit it to the project as models/<name>/, with the code kept in
// source/model.py so the model is changed by editing it and regenerating.

//go:embed meshref/*.md
var meshRef embed.FS

// generatedModelRe is the worker's rule (gzmesh/run.py NAME_RE): the name is
// also a Python-friendly identifier in reports and file names.
var generatedModelRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

const (
	meshWait        = 90 * time.Second
	meshMaxTar      = 40 << 20
	meshPendingTTL  = 15 * time.Minute
	meshPerMinute   = 10
	meshPerDay      = 200
	meshMaxCodeSize = 200 << 10
)

type meshTools struct {
	base, token string
	http        *http.Client
	limit       *rateLimiter

	mu      sync.Mutex
	pending map[string]pendingMesh // by worker job id
}

// pendingMesh is a job still running when model_generate returned; its
// result is committed by model_generate_status for the same user.
type pendingMesh struct {
	user, project, model, baseRev string
	at                            time.Time
}

func (t *tools) registerMesh(reg *mcp.Registry) {
	t.mesh = &meshTools{
		base:    strings.TrimSuffix(t.cfg.MeshWorkerURL, "/"),
		token:   t.cfg.MeshWorkerToken,
		http:    &http.Client{Timeout: meshWait + 30*time.Second},
		limit:   newRateLimiter(meshPerMinute, meshPerDay),
		pending: map[string]pendingMesh{},
	}
	num := func(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "model_generate",
			Description: "Generate a custom mesh model from build123d Python you write, for objects that Fuel " +
				"doesn't have and primitives can't shape well (curves, fillets, sweeps, holes, text). The " +
				"code defines build() returning gz.part(shape, name, material) parts; the server " +
				"tessellates, textures, adds collision and exact inertia, and commits models/<model>/ with " +
				"your code in source/model.py. Returns a report and a preview image: check it, then fix " +
				"the code and call again with the same model to replace it. Read mesh_api_reference first. " +
				"Include the result with <include><uri>model://<model></uri></include>.",
			InputSchema: schema([]string{"name", "model", "code"}, map[string]any{
				"name":         str("Project name"),
				"model":        str("Model name: lowercase letters, digits and '_', starting with a letter"),
				"code":         str("build123d Python defining build(); see mesh_api_reference"),
				"description":  str("What the model is (model.config)"),
				"mass":         num("Total mass in kg (or give density; default: from the parts' materials)"),
				"density":      num("Density in kg/m³ for every part"),
				"static":       map[string]any{"type": "boolean", "description": "Immovable (shelves, signs). Default false"},
				"tri_budget":   num("Max triangles: small prop 5000, furniture 15000 (default), vehicle 50000"),
				"texture_size": num("Atlas size in px: 512, 1024 (default) or 2048"),
				"collision": map[string]any{"type": "string", "enum": []string{"auto", "primitive", "convex"},
					"description": "auto (default): a primitive when it fits, else one hull, else a convex decomposition"},
				"base_rev": str("Optional: fail if the project changed since this revision"),
			}),
		},
		RequiredScopes: []string{"project:write"},
		Handler:        t.modelGenerate,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "model_generate_status",
			Description: "Finish a model_generate call that returned a job_id: returns running, or the result.",
			InputSchema: schema([]string{"job_id"}, map[string]any{"job_id": str("From model_generate")}),
		},
		RequiredScopes: []string{"project:write"},
		Handler:        t.modelGenerateStatus,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "mesh_api_reference",
			Description: "A short build123d and gzmesh reference with working examples, for model_generate. " +
				"Topics: " + strings.Join(meshTopics(), ", ") + ".",
			InputSchema: schema(nil, map[string]any{"topic": str("Topic (default overview)")}),
		},
		Handler: t.meshAPIReference,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "mesh_list_materials",
			Description: "Materials for model_generate parts: names, parameters, default densities.",
			InputSchema: schema(nil, map[string]any{}),
		},
		Handler: func(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
			return t.meshAPIReference(tc, map[string]any{"topic": "materials"})
		},
	})
}

func meshTopics() []string {
	entries, _ := meshRef.ReadDir("meshref")
	var out []string
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(out)
	return out
}

func (t *tools) meshAPIReference(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	topic := argOr(args, "topic", "overview")
	b, err := meshRef.ReadFile("meshref/" + path.Base(topic) + ".md")
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("unknown topic %q; topics: %s", topic, strings.Join(meshTopics(), ", "))), nil
	}
	return mcp.TextResult(string(b)), nil
}

func (t *tools) modelGenerate(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "name", "model", "code")
	if errRes != nil {
		return *errRes, nil
	}
	project, model, code := v[0], v[1], v[2]
	if !generatedModelRe.MatchString(model) {
		return mcp.ErrorResult("model name must be lowercase letters, digits and '_', starting with a letter"), nil
	}
	if len(code) > meshMaxCodeSize {
		return mcp.ErrorResult(fmt.Sprintf("code is longer than %d KB", meshMaxCodeSize>>10)), nil
	}
	user := *tc.User.Username
	if wait, ok := t.mesh.limit.allow(user, time.Now()); !ok {
		return mcp.ErrorResult(fmt.Sprintf("model_generate rate limit (%d per minute, %d per day); try again in %s",
			meshPerMinute, meshPerDay, wait.Round(time.Second))), nil
	}

	dir := path.Join(modelsDir, model)
	files, _, em := projectService.ListFiles(tc.Ctx, tc.DB, user, project, "", tc.User)
	if em != nil {
		return gzError(em), nil
	}
	pf := newProjectFiles("", files)
	if pf.hasModel(dir) && !contains(files, dir+"/source/model.py") {
		return mcp.ErrorResult(fmt.Sprintf("%s exists and wasn't generated (no source/model.py); choose another "+
			"model name, or edit it by hand", dir)), nil
	}
	p, em := projectService.GetForCaller(tc.DB, project, tc.User)
	if em != nil {
		return gzError(em), nil
	}
	sdfVersion := defaultSDFVersion
	if p.SDFVersion != nil && *p.SDFVersion != "" {
		sdfVersion = *p.SDFVersion
	}

	job := map[string]any{"name": model, "code": code, "sdf_version": sdfVersion}
	for _, k := range []string{"description", "collision"} {
		if s := argString(args, k); s != "" {
			job[k] = s
		}
	}
	for _, k := range []string{"mass", "density"} {
		if x, ok := args[k].(float64); ok {
			job[k] = x
		}
	}
	for _, k := range []string{"tri_budget", "texture_size"} {
		if x, ok := args[k].(float64); ok {
			job[k] = int(x)
		}
	}
	if s, ok := args["static"].(bool); ok {
		job["static"] = s
	}
	body, _ := json.Marshal(job)
	tarball, jobID, err := t.mesh.submit(tc.Ctx, body)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}
	baseRev := argString(args, "base_rev")
	if jobID != "" {
		t.mesh.addPending(jobID, pendingMesh{user: user, project: project, model: model, baseRev: baseRev, at: time.Now()})
		return jsonResult(map[string]any{"status": "running", "job_id": jobID,
			"next": "call model_generate_status with this job_id in a little while"})
	}
	return t.commitGenerated(tc, project, model, baseRev, tarball)
}

func (t *tools) modelGenerateStatus(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	id := argString(args, "job_id")
	p, ok := t.mesh.getPending(id)
	if !ok || p.user != *tc.User.Username {
		return mcp.ErrorResult("no such job (results are kept for 15 minutes)"), nil
	}
	tarball, running, err := t.mesh.poll(tc.Ctx, id)
	if err != nil {
		t.mesh.dropPending(id)
		return mcp.ErrorResult(err.Error()), nil
	}
	if running {
		return jsonResult(map[string]any{"status": "running", "job_id": id})
	}
	t.mesh.dropPending(id)
	return t.commitGenerated(tc, p.project, p.model, p.baseRev, tarball)
}

// commitGenerated commits a worker result as models/<model>/ (replacing the
// folder) and returns the report, the validation and the preview image.
func (t *tools) commitGenerated(tc *mcp.ToolContext, project, model, baseRev string, tarball []byte) (mcp.CallToolResult, error) {
	out, err := readModelTar(tarball, model)
	if err != nil {
		return mcp.ErrorResult("mesh worker: " + err.Error()), nil
	}
	if ok, _ := out.report["ok"].(bool); !ok {
		res := map[string]any{"error": out.report["error"],
			"next": "fix the code and call model_generate again with the same model"}
		for _, k := range []string{"line", "detail"} {
			if out.report[k] != nil {
				res[k] = out.report[k]
			}
		}
		r, err := jsonResult(res)
		r.IsError = true
		return r, err
	}

	dir := path.Join(modelsDir, model)
	files := map[string][]byte{}
	for rel, b := range out.files {
		files[path.Join(dir, rel)] = b
	}
	if out.preview != nil {
		files[path.Join(dir, "previews", "preview.png")] = out.preview
	}
	res, em := projectService.WriteFiles(tc.Ctx, tc.DB, *tc.User.Username, project, files, []string{dir}, tc.User,
		projects.WriteFileOptions{CommitMessage: autoSaveMessage("model_generate", dir, ""), BaseRev: baseRev})
	if em != nil {
		return gzError(em), nil
	}
	modelSDF := string(out.files["model.sdf"])
	validation := validationReport(t.validateIn(tc, project, dir+"/model.sdf", res.Rev, modelSDF), dir+"/model.sdf")

	summary := map[string]any{
		"model": model, "uri": "model://" + model, "rev": res.Rev, "version": res.Version,
		"validation": validation,
		"include": fmt.Sprintf("<include>\n  <uri>model://%s</uri>\n  <name>%s</name>\n  <pose>0 0 0 0 0 0</pose>\n</include>",
			model, model),
		"preview": "attached: three-quarter from the front (+X -Y) and back (-X +Y), front (+X), top; also " +
			path.Join(dir, "previews", "preview.png"),
	}
	for _, k := range []string{"bounding_box_m", "triangles", "collision", "inertial", "parts", "warnings", "tessellation"} {
		if v, ok := out.report[k]; ok {
			summary[k] = v
		}
	}
	r, err := jsonResult(summary)
	if err != nil {
		return r, err
	}
	if out.preview != nil {
		r.Content = append(r.Content, mcp.ImageBlock("image/png", out.preview))
	}
	return r, nil
}

type modelTar struct {
	report  map[string]any
	files   map[string][]byte // relative to models/<model>/
	preview []byte
}

// readModelTar unpacks a worker result, accepting only report.json,
// preview.png and files inside models/<model>/.
func readModelTar(b []byte, model string) (modelTar, error) {
	out := modelTar{files: map[string][]byte{}}
	prefix := "models/" + model + "/"
	tr := tar.NewReader(bytes.NewReader(b))
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, fmt.Errorf("bad result: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			return out, fmt.Errorf("bad result: %s is not a regular file", h.Name)
		}
		total += h.Size
		if total > meshMaxTar {
			return out, errors.New("result is too large")
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return out, fmt.Errorf("bad result: %w", err)
		}
		switch name := path.Clean(h.Name); {
		case name == "report.json":
			if err := json.Unmarshal(data, &out.report); err != nil {
				return out, fmt.Errorf("bad report: %w", err)
			}
		case name == "preview.png":
			out.preview = data
		case strings.HasPrefix(name, prefix) && !strings.Contains(name, ".."):
			out.files[strings.TrimPrefix(name, prefix)] = data
		default:
			return out, fmt.Errorf("bad result: unexpected file %s", h.Name)
		}
	}
	if out.report == nil {
		return out, errors.New("bad result: no report.json")
	}
	return out, nil
}

// -- worker client

// submit posts a job. It returns the result tarball, or a job id when the
// job is still running after meshWait.
func (m *meshTools) submit(ctx context.Context, job []byte) (tarball []byte, jobID string, err error) {
	q := url.Values{"wait": {fmt.Sprint(int(meshWait.Seconds()))}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.base+"/v1/jobs?"+q.Encode(), bytes.NewReader(job))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	return m.do(req)
}

// poll fetches a job started by submit.
func (m *meshTools) poll(ctx context.Context, id string) (tarball []byte, running bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.base+"/v1/jobs/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, false, err
	}
	tarball, jobID, err := m.do(req)
	return tarball, jobID != "", err
}

func (m *meshTools) do(req *http.Request) ([]byte, string, error) {
	if m.token != "" {
		req.Header.Set("Authorization", "Bearer "+m.token)
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("the mesh worker is unavailable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, meshMaxTar+1))
	if err != nil {
		return nil, "", fmt.Errorf("mesh worker: %w", err)
	}
	if resp.Header.Get("Content-Type") == "application/x-tar" {
		return body, "", nil // 200, or 500 with a report of an internal error
	}
	var msg struct {
		Error string `json:"error"`
		JobID string `json:"job_id"`
	}
	_ = json.Unmarshal(body, &msg)
	switch resp.StatusCode {
	case http.StatusAccepted:
		return nil, msg.JobID, nil
	case http.StatusTooManyRequests:
		return nil, "", errors.New("the mesh worker is busy; try again in a minute")
	case http.StatusNotFound:
		return nil, "", errors.New("the mesh worker no longer has this job; run model_generate again")
	}
	if msg.Error == "" {
		msg.Error = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return nil, "", fmt.Errorf("mesh worker: %s", msg.Error)
}

func (m *meshTools) addPending(id string, p pendingMesh) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.pending {
		if time.Since(v.at) > meshPendingTTL {
			delete(m.pending, k)
		}
	}
	m.pending[id] = p
}

func (m *meshTools) getPending(id string) (pendingMesh, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pending[id]
	return p, ok
}

func (m *meshTools) dropPending(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pending, id)
}

// rateLimiter allows perMinute and perDay events per key (plan §5.7). It
// lives in one fuel-server process; with several replicas each enforces
// its own share.
type rateLimiter struct {
	mu                sync.Mutex
	perMinute, perDay int
	hits              map[string][]time.Time
}

func newRateLimiter(perMinute, perDay int) *rateLimiter {
	return &rateLimiter{perMinute: perMinute, perDay: perDay, hits: map[string][]time.Time{}}
}

// allow records an event for key at now if within both limits; otherwise
// it returns how long until the next one would be allowed.
func (r *rateLimiter) allow(key string, now time.Time) (time.Duration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var kept []time.Time
	for _, t := range r.hits[key] {
		if now.Sub(t) < 24*time.Hour {
			kept = append(kept, t)
		}
	}
	var lastMinute []time.Time
	for _, t := range kept {
		if now.Sub(t) < time.Minute {
			lastMinute = append(lastMinute, t)
		}
	}
	switch {
	case len(lastMinute) >= r.perMinute:
		r.hits[key] = kept
		return time.Minute - now.Sub(lastMinute[0]), false
	case len(kept) >= r.perDay:
		r.hits[key] = kept
		return 24*time.Hour - now.Sub(kept[0]), false
	}
	r.hits[key] = append(kept, now)
	return 0, true
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
