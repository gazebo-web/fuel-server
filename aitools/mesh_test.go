package aitools

import (
	"archive/tar"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gazebo-web/fuel-server/mcp"
)

// makeTar builds a worker-style tarball from name/content pairs.
func makeTar(t *testing.T, files ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for i := 0; i+1 < len(files); i += 2 {
		b := []byte(files[i+1])
		if err := tw.WriteHeader(&tar.Header{Name: files[i], Mode: 0o644, Size: int64(len(b))}); err != nil {
			t.Fatal(err)
		}
		tw.Write(b)
	}
	tw.Close()
	return buf.Bytes()
}

func TestReadModelTar(t *testing.T) {
	good := makeTar(t, "report.json", `{"ok":true}`, "preview.png", "PNG",
		"models/cone/model.sdf", "<sdf/>", "models/cone/meshes/cone.glb", "glTF")
	out, err := readModelTar(good, "cone")
	if err != nil {
		t.Fatal(err)
	}
	if out.report["ok"] != true || string(out.preview) != "PNG" || string(out.files["meshes/cone.glb"]) != "glTF" || len(out.files) != 2 {
		t.Fatalf("%+v", out)
	}
	for name, b := range map[string][]byte{
		"other model": makeTar(t, "report.json", `{}`, "models/crate/model.sdf", "x"),
		"traversal":   makeTar(t, "report.json", `{}`, "models/cone/../../world.sdf", "x"),
		"stray file":  makeTar(t, "report.json", `{}`, "world.sdf", "x"),
		"no report":   makeTar(t, "models/cone/model.sdf", "x"),
		"bad report":  makeTar(t, "report.json", "{"),
	} {
		if _, err := readModelTar(b, "cone"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	r := newRateLimiter(3, 5)
	now := time.Unix(1_000_000, 0)
	for i := 0; i < 3; i++ {
		if _, ok := r.allow("nate", now.Add(time.Duration(i)*time.Second)); !ok {
			t.Fatalf("call %d refused", i)
		}
	}
	wait, ok := r.allow("nate", now.Add(10*time.Second))
	if ok || wait != 50*time.Second {
		t.Fatalf("4th in a minute: ok=%v wait=%s", ok, wait)
	}
	if _, ok := r.allow("someone", now); !ok {
		t.Fatal("limits are per user")
	}
	for i := 0; i < 2; i++ {
		if _, ok := r.allow("nate", now.Add(time.Duration(2+i)*time.Minute)); !ok {
			t.Fatalf("after a minute: call %d refused", i)
		}
	}
	if _, ok := r.allow("nate", now.Add(10*time.Minute)); ok {
		t.Fatal("day limit of 5 not enforced")
	}
	if _, ok := r.allow("nate", now.Add(25*time.Hour)); !ok {
		t.Fatal("day limit should reset")
	}
}

func TestMeshWorkerClient(t *testing.T) {
	tarball := makeTar(t, "report.json", `{"ok":true}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":"missing or wrong token"}`))
			return
		}
		switch {
		case r.URL.Path == "/v1/jobs" && r.URL.Query().Get("wait") != "90":
			w.WriteHeader(400)
		case r.URL.Path == "/v1/jobs" && strings.Contains(readAll(r), "busy"):
			w.Header().Set("Retry-After", "10")
			w.WriteHeader(429)
		case r.URL.Path == "/v1/jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(202)
			w.Write([]byte(`{"job_id":"j1","status":"running"}`))
		case r.URL.Path == "/v1/jobs/j1":
			w.Header().Set("Content-Type", "application/x-tar")
			w.Write(tarball)
		case r.URL.Path == "/v1/jobs/boom":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"the job ran longer than 2m0s and was stopped"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	m := &meshTools{base: srv.URL, token: "tok", http: srv.Client()}
	ctx := context.Background()

	if _, id, err := m.submit(ctx, []byte(`{"name":"x"}`)); err != nil || id != "j1" {
		t.Fatalf("submit: %q %v", id, err)
	}
	if b, running, err := m.poll(ctx, "j1"); err != nil || running || !bytes.Equal(b, tarball) {
		t.Fatalf("poll: %v %v", running, err)
	}
	if _, _, err := m.submit(ctx, []byte(`busy`)); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("429: %v", err)
	}
	if _, _, err := m.poll(ctx, "boom"); err == nil || !strings.Contains(err.Error(), "ran longer") {
		t.Fatalf("500: %v", err)
	}
	if _, _, err := m.poll(ctx, "gone"); err == nil || !strings.Contains(err.Error(), "no longer has this job") {
		t.Fatalf("404: %v", err)
	}
	m.token = "wrong"
	if _, _, err := m.submit(ctx, []byte(`{}`)); err == nil || !strings.Contains(err.Error(), "wrong token") {
		t.Fatalf("401: %v", err)
	}
	m.base = "http://127.0.0.1:1"
	if _, _, err := m.submit(ctx, []byte(`{}`)); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("down: %v", err)
	}
}

func readAll(r *http.Request) string {
	var b bytes.Buffer
	b.ReadFrom(r.Body)
	return b.String()
}

func TestMeshToolsOnlyWithWorker(t *testing.T) {
	names := func(reg *mcp.Registry) string {
		var s []string
		for _, spec := range reg.Specs() {
			s = append(s, spec.Name)
		}
		return strings.Join(s, " ")
	}
	if got := names(NewRegistry(Config{})); strings.Contains(got, "model_generate") || strings.Contains(got, "mesh_") {
		t.Errorf("mesh tools offered without a worker: %s", got)
	}
	got := names(NewRegistry(Config{MeshWorkerURL: "http://meshworker:8020"}))
	for _, n := range []string{"model_generate", "model_generate_status", "mesh_api_reference", "mesh_list_materials"} {
		if !strings.Contains(got, n) {
			t.Errorf("%s missing: %s", n, got)
		}
	}
}

func TestMeshAPIReference(t *testing.T) {
	reg := NewRegistry(Config{MeshWorkerURL: "http://meshworker:8020"})
	tool, _ := reg.Get("mesh_api_reference")
	for topic, want := range map[string]string{
		"":              "def build()",
		"sweep_loft":    "sweep(",
		"materials":     "gz.painted_metal(",
		"../aitools.go": "unknown topic",
	} {
		r, _ := tool.Handler(&mcp.ToolContext{}, map[string]any{"topic": topic})
		if !strings.Contains(r.Content[0].Text, want) {
			t.Errorf("topic %q: missing %q", topic, want)
		}
	}
	if topics := strings.Join(meshTopics(), " "); !strings.Contains(topics, "articulation") || !strings.Contains(topics, "booleans_patterns") {
		t.Errorf("topics %v", topics)
	}
}
