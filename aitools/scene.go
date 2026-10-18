package aitools

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gazebo-web/fuel-server/bundles/projects"
	"github.com/gazebo-web/fuel-server/mcp"
	"github.com/gazebo-web/fuel-server/sdf"
)

func (t *tools) registerScene(reg *mcp.Registry) {
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "scene_summary",
			Description: "Compact overview of a project's world (or model) file: every model, include, " +
				"light and actor with its name, pose, URI, link/joint counts and primitive shape, plus " +
				"line numbers. Much smaller than the file. Read this first, then read or edit around " +
				"the lines you need. Fuel includes whose size is known have a bbox: their size and " +
				"world_min/world_max, the box they fill where they're placed, to check that they rest " +
				"on the floor and don't overlap.",
			InputSchema: schema([]string{"name"}, map[string]any{
				"name": str("Project name"),
				"path": str("File to summarize (default 'world.sdf')"),
				"rev":  str("Revision (default HEAD)"),
			}),
		},
		RequiredScopes: []string{"project:read"},
		Handler:        t.sceneSummary,
	})
}

func (t *tools) sceneSummary(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "name")
	if errRes != nil {
		return *errRes, nil
	}
	path := argOr(args, "path", "world.sdf")
	b, rev, em := projectService.ReadFile(tc.Ctx, tc.DB, *tc.User.Username, v[0], path, argString(args, "rev"), tc.User)
	if em != nil && em.StatusCode == http.StatusNotFound && errors.Is(em.BaseError, projects.ErrFileNotFound) {
		return mcp.ErrorResult(path + " doesn't exist yet; create it with project_write_file"), nil
	}
	if em != nil {
		return gzError(em), nil
	}
	s, err := sdf.Summarize(string(b))
	if err != nil {
		return mcp.ErrorResult(path + ": " + err.Error()), nil
	}
	if t.sizes != nil {
		t.includeSizes(s.Entities)
	}
	return jsonResult(map[string]any{"path": path, "rev": rev, "summary": s})
}

// includeSizes sets the bbox of top-level Fuel includes whose size is
// stored, and starts measuring the others for next time.
func (t *tools) includeSizes(entities []sdf.Entity) {
	type ref struct {
		i   int
		cat Catalog
		key sizeKey
	}
	var refs []ref
	keys := map[sizeKey]string{}
	for i, e := range entities {
		if e.Kind != "include" {
			continue
		}
		u, err := url.Parse(strings.TrimSpace(e.URI))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		owner, name, ok := modelURIPath(u)
		cat := t.catalogForHost(u.Host)
		if !ok || cat == nil {
			continue
		}
		k := sizeKey{cat.Host(), owner, name}
		refs = append(refs, ref{i, cat, k})
		keys[k] = ""
	}
	have := t.sizes.stored(keys)
	for _, r := range refs {
		rep, ok := have[r.key]
		if !ok {
			_, _ = t.sizes.measure(r.cat, r.key, "", 0)
			continue
		}
		b := rep.bbox()
		if b == nil {
			continue
		}
		out := map[string]any{"size": b.Size, "min": b.Min, "max": b.Max}
		if lo, hi, ok := worldBox(b, entities[r.i].Pose); ok {
			out["world_min"], out["world_max"] = lo, hi
		}
		if b.Note != "" {
			out["note"] = b.Note
		}
		entities[r.i].BBox = out
	}
}
