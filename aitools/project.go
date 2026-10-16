package aitools

import (
	"path"
	"strings"

	"github.com/gazebo-web/fuel-server/bundles/projects"
	"github.com/gazebo-web/fuel-server/mcp"
	"github.com/gazebo-web/gz-go/v7"
)

var projectService = &projects.Service{}

// gazeboReleases maps a Gazebo release to its SDF version and ROS 2
// distro (design §7).
var gazeboReleases = map[string][2]string{
	"Harmonic": {"1.9", "Jazzy"},
	"Fortress": {"1.8", "Humble"},
}

func (t *tools) registerProject(reg *mcp.Registry) {
	nameOnly := schema([]string{"name"}, map[string]any{"name": str("Project name")})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "project_list",
			Description: "List the caller's AI Builder projects.",
			InputSchema: schema(nil, map[string]any{}),
		},
		RequiredScopes: []string{"project:read"},
		Handler:        t.projectList,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "project_get",
			Description: "Get a project's metadata: target Gazebo release, SDF version, ROS 2 distro, " +
				"and head_rev (the current revision).",
			InputSchema: nameOnly,
		},
		RequiredScopes: []string{"project:read"},
		Handler:        t.projectGet,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "project_create",
			Description: "Create a new private project. Its world lives in world.sdf; create it with " +
				"project_write_file.",
			InputSchema: schema([]string{"name"}, map[string]any{
				"name":           str("Project name, unique among the caller's projects"),
				"description":    str("What the project is for"),
				"gazebo_release": str("'Harmonic' (default; SDF 1.9, ROS 2 Jazzy) or 'Fortress' (SDF 1.8, Humble)"),
			}),
		},
		RequiredScopes: []string{"project:write"},
		Handler:        t.projectCreate,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "project_list_files",
			Description: "List the files in a project at a revision (default HEAD).",
			InputSchema: schema([]string{"name"}, map[string]any{
				"name": str("Project name"),
				"rev":  str("Revision (default HEAD)"),
			}),
		},
		RequiredScopes: []string{"project:read"},
		Handler:        t.projectListFiles,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "project_read_file",
			Description: "Read a project file. Returns its content and rev; pass rev as base_rev to " +
				"project_edit_file. For a quick overview of a world, prefer scene_summary.",
			InputSchema: schema([]string{"name", "path"}, map[string]any{
				"name": str("Project name"),
				"path": str("File path, e.g. 'world.sdf' or 'models/cart/model.sdf'"),
				"rev":  str("Revision (default HEAD)"),
			}),
		},
		RequiredScopes: []string{"project:read"},
		Handler:        t.projectReadFile,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "project_write_file",
			Description: "Create or replace a whole project file. Use for new or small files; for " +
				"changes to an existing file prefer project_edit_file. SDF files are validated and the " +
				"result is returned; fix any errors it reports.",
			InputSchema: schema([]string{"name", "path", "content"}, map[string]any{
				"name":     str("Project name"),
				"path":     str("File path, e.g. 'world.sdf'"),
				"content":  str("Full file content"),
				"base_rev": str("If set, fail when the file changed since this revision"),
				"message":  str("Short description of the change"),
			}),
		},
		RequiredScopes: []string{"project:write"},
		Handler:        t.projectWriteFile,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "project_edit_file",
			Description: "Replace one exact occurrence of old_str with new_str in a project file. " +
				"old_str must match exactly once: include enough surrounding lines (e.g. the model's " +
				"name) to make it unique. Fails if the file changed since base_rev; then re-read it. " +
				"SDF files are validated and the result is returned; fix any errors it reports.",
			InputSchema: schema([]string{"name", "path", "old_str", "new_str", "base_rev"}, map[string]any{
				"name":     str("Project name"),
				"path":     str("File path, e.g. 'world.sdf'"),
				"old_str":  str("Exact text to replace (must occur once)"),
				"new_str":  str("Replacement text (empty deletes old_str)"),
				"base_rev": str("The rev returned when you last read or wrote this file"),
				"message":  str("Short description of the change"),
			}),
		},
		RequiredScopes: []string{"project:write"},
		Handler:        t.projectEditFile,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "project_list_versions",
			Description: "List a project's saved versions, newest first.",
			InputSchema: nameOnly,
		},
		RequiredScopes: []string{"project:read"},
		Handler:        t.projectListVersions,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "project_checkpoint",
			Description: "Save a labeled version of the project, folding the automatic saves since the " +
				"last labeled version into it. Call when a coherent piece of work is done.",
			InputSchema: schema([]string{"name", "message"}, map[string]any{
				"name":    str("Project name"),
				"message": str("Label for this version, e.g. 'Add loading dock'"),
			}),
		},
		RequiredScopes: []string{"project:write"},
		Handler:        t.projectCheckpoint,
	})
}

func (t *tools) projectList(tc *mcp.ToolContext, _ map[string]any) (mcp.CallToolResult, error) {
	list, em := projectService.List(tc.DB, tc.User, 100, 0)
	if em != nil {
		return gzError(em), nil
	}
	return jsonResult(list)
}

func (t *tools) projectGet(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "name")
	if errRes != nil {
		return *errRes, nil
	}
	p, em := projectService.GetForCaller(tc.DB, v[0], tc.User)
	if em != nil {
		return gzError(em), nil
	}
	return jsonResult(p)
}

func (t *tools) projectCreate(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "name")
	if errRes != nil {
		return *errRes, nil
	}
	release := argOr(args, "gazebo_release", "Harmonic")
	versions, ok := gazeboReleases[release]
	if !ok {
		return mcp.ErrorResult("gazebo_release must be 'Harmonic' or 'Fortress'"), nil
	}
	name, desc := v[0], argString(args, "description")
	sdfVersion, ros := versions[0], versions[1]
	p, em := projectService.Create(tc.Ctx, tc.DB, tc.User, &name, &desc, &release, &sdfVersion, &ros)
	if em != nil {
		return gzError(em), nil
	}
	return jsonResult(p)
}

func (t *tools) projectListFiles(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "name")
	if errRes != nil {
		return *errRes, nil
	}
	files, rev, em := projectService.ListFiles(tc.Ctx, tc.DB, *tc.User.Username, v[0], argString(args, "rev"), tc.User)
	if em != nil {
		return gzError(em), nil
	}
	return jsonResult(map[string]any{"rev": rev, "files": files})
}

func (t *tools) projectReadFile(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "name", "path")
	if errRes != nil {
		return *errRes, nil
	}
	b, rev, em := projectService.ReadFile(tc.Ctx, tc.DB, *tc.User.Username, v[0], v[1], argString(args, "rev"), tc.User)
	if em != nil {
		return gzError(em), nil
	}
	return jsonResult(map[string]any{"path": v[1], "rev": rev, "content": string(b)})
}

// autoSaveMessage labels agent edits so labeled saves and the idle squash
// fold them (projects.IsAutoSave).
func autoSaveMessage(tool, filePath, msg string) string {
	if msg = strings.TrimSpace(msg); msg != "" {
		return projects.AutoSaveMessage + ": " + msg
	}
	return projects.AutoSaveMessage + ": " + tool + " " + filePath
}

func (t *tools) projectWriteFile(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "name", "path")
	if errRes != nil {
		return *errRes, nil
	}
	content, ok := args["content"].(string)
	if !ok {
		return mcp.ErrorResult("content required"), nil
	}
	res, em := projectService.WriteFileWithOptions(tc.Ctx, tc.DB, *tc.User.Username, v[0], v[1],
		[]byte(content), tc.User, projects.WriteFileOptions{
			CommitMessage: autoSaveMessage("project_write_file", v[1], argString(args, "message")),
			BaseRev:       argString(args, "base_rev"),
		})
	return t.writeResult(tc, v[1], content, res, em)
}

func (t *tools) projectEditFile(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "name", "path", "old_str", "base_rev")
	if errRes != nil {
		return *errRes, nil
	}
	newStr, _ := args["new_str"].(string)
	res, em := projectService.EditFile(tc.Ctx, tc.DB, *tc.User.Username, v[0], v[1], v[2], newStr, v[3],
		autoSaveMessage("project_edit_file", v[1], argString(args, "message")), tc.User)
	if em != nil {
		return gzError(em), nil
	}
	b, _, em := projectService.ReadFile(tc.Ctx, tc.DB, *tc.User.Username, v[0], v[1], res.Rev, tc.User)
	if em != nil {
		return gzError(em), nil
	}
	return t.writeResult(tc, v[1], string(b), res, nil)
}

// writeResult reports a write: the new rev (base_rev for the next edit)
// and, for SDF files, the validation report.
func (t *tools) writeResult(tc *mcp.ToolContext, filePath, content string,
	res projects.WriteResult, em *gz.ErrMsg) (mcp.CallToolResult, error) {
	if em != nil {
		return gzError(em), nil
	}
	out := map[string]any{"path": filePath, "rev": res.Rev, "version": res.Version}
	switch path.Ext(filePath) {
	case ".sdf", ".world":
		out["validation"] = validationReport(t.validate(tc, content), filePath)
	}
	return jsonResult(out)
}

func (t *tools) projectListVersions(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "name")
	if errRes != nil {
		return *errRes, nil
	}
	list, em := projectService.ListVersions(tc.Ctx, tc.DB, *tc.User.Username, v[0], tc.User)
	if em != nil {
		return gzError(em), nil
	}
	return jsonResult(list)
}

func (t *tools) projectCheckpoint(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "name", "message")
	if errRes != nil {
		return *errRes, nil
	}
	res, em := projectService.Checkpoint(tc.Ctx, tc.DB, *tc.User.Username, v[0], v[1], tc.User)
	if em != nil {
		return gzError(em), nil
	}
	return jsonResult(res)
}
