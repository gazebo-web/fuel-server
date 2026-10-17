package aitools

import (
	"errors"
	"net/http"

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
				"the lines you need.",
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
	return jsonResult(map[string]any{"path": path, "rev": rev, "summary": s})
}
