package main

import (
	"fmt"
	"path"
	"strings"

	"github.com/gazebo-web/fuel-server/bundles/projects"
	"github.com/gazebo-web/fuel-server/mcp"
	"github.com/gazebo-web/gz-go/v7"
)

// gzMcpError converts a gz.ErrMsg into a tool-level CallToolResult,
// including the wrapped BaseError's message when present. The bare
// ErrMsg.Msg is a human-readable category ("Unable to process
// repository command") that loses the actually-useful detail (e.g.
// stderr from a failed `git reset`); folding BaseError back in turns
// the toast on the frontend into something actionable.
func gzMcpError(em *gz.ErrMsg) mcp.CallToolResult {
	if em == nil {
		return mcp.ErrorResult("unknown error")
	}
	msg := em.Msg
	if em.BaseError != nil {
		baseMsg := em.BaseError.Error()
		if baseMsg != "" {
			msg = msg + ": " + baseMsg
		}
	}
	return mcp.ErrorResult(msg)
}

// registerProjectTools wires the project.* MCP tools. Every handler is
// owner-scoped via the authenticated caller, so no extra RBAC is needed
// at this layer. Publish tools stay on the roadmap (design §9.1) and land
// once the AI Builder UI grows the confirmation flow.
func registerProjectTools(reg *mcp.Registry) {
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "project_list",
			Description: "List the caller's AI Builder projects, most recently updated first.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		RequiredScopes: []string{"project:read"},
		Handler:        projectList,
	})

	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "project_get",
			Description: "Get a single project's metadata by name (owner is the caller).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
				"required": []string{"name"},
			},
		},
		RequiredScopes: []string{"project:read"},
		Handler:        projectGet,
	})

	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "project_list_versions",
			Description: "List the commit history of a project (newest first).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
				"required": []string{"name"},
			},
		},
		RequiredScopes: []string{"project:read"},
		Handler:        projectListVersions,
	})

	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "project_get_world_sdf",
			Description: "Read the project's current world.sdf (or a specific version).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string"},
					"version": map[string]any{"type": "string", "description": "'tip' or 'N' (default 'tip')"},
				},
				"required": []string{"name"},
			},
		},
		RequiredScopes: []string{"project:read"},
		Handler:        projectGetWorldSDF,
	})

	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "project_get_model_sdf",
			Description: "Read a named model's SDF from the project (or a specific version).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":       map[string]any{"type": "string", "description": "Project name"},
					"model_name": map[string]any{"type": "string"},
					"version":    map[string]any{"type": "string"},
				},
				"required": []string{"name", "model_name"},
			},
		},
		RequiredScopes: []string{"project:read"},
		Handler:        projectGetModelSDF,
	})

	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "project_save_world_sdf",
			Description: "Write the project's world.sdf and auto-version. Optional commit_message produces a labeled snapshot instead of an auto-save. Pass squash_autosaves=true (manual Save flow) to collapse the run of recent auto-save commits into this one labeled save.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":             map[string]any{"type": "string"},
					"sdf":              map[string]any{"type": "string"},
					"commit_message":   map[string]any{"type": "string"},
					"squash_autosaves": map[string]any{"type": "boolean"},
				},
				"required": []string{"name", "sdf"},
			},
		},
		RequiredScopes: []string{"project:write"},
		Handler:        projectSaveWorldSDF,
	})

	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "project_replace_in_world_sdf",
			Description: "Targeted edit to the project's world.sdf. Replaces every occurrence of `find` with `replace` and saves. Use this for incremental edits (changing a pose, adding one element, swapping a model) so you don't have to re-emit the whole world. `find` must match exactly (whitespace and indentation matter); pick a unique anchor — e.g. a full <link>...</link> block or a unique attribute value. Returns the new version number, or an error if `find` was not found.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":           map[string]any{"type": "string", "description": "Project name"},
					"find":           map[string]any{"type": "string", "description": "Exact string to find in world.sdf"},
					"replace":        map[string]any{"type": "string", "description": "Replacement string (may be empty to delete the match)"},
					"commit_message": map[string]any{"type": "string"},
				},
				"required": []string{"name", "find", "replace"},
			},
		},
		RequiredScopes: []string{"project:write"},
		Handler:        projectReplaceInWorldSDF,
	})

	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "project_save_model_sdf",
			Description: "Write a named model's SDF under the project's models/ folder and auto-version.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":           map[string]any{"type": "string", "description": "Project name"},
					"model_name":     map[string]any{"type": "string"},
					"sdf":            map[string]any{"type": "string"},
					"commit_message": map[string]any{"type": "string"},
				},
				"required": []string{"name", "model_name", "sdf"},
			},
		},
		RequiredScopes: []string{"project:write"},
		Handler:        projectSaveModelSDF,
	})
}

// -- handlers ---------------------------------------------------------------

func projectList(tc *mcp.ToolContext, _ map[string]any) (mcp.CallToolResult, error) {
	s := &projects.Service{}
	list, em := s.List(tc.DB, tc.User, 100, 0)
	if em != nil {
		return gzMcpError(em), nil
	}
	return jsonResult(list)
}

func projectGet(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return mcp.ErrorResult("name is required"), nil
	}
	s := &projects.Service{}
	p, em := s.GetForCaller(tc.DB, name, tc.User)
	if em != nil {
		return gzMcpError(em), nil
	}
	return jsonResult(p)
}

func projectListVersions(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return mcp.ErrorResult("name is required"), nil
	}
	s := &projects.Service{}
	list, em := s.ListVersions(tc.Ctx, tc.DB, *tc.User.Username, name, tc.User)
	if em != nil {
		return gzMcpError(em), nil
	}
	return jsonResult(list)
}

func projectGetWorldSDF(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return mcp.ErrorResult("name is required"), nil
	}
	version, _ := args["version"].(string)
	if version == "" {
		version = "tip"
	}
	s := &projects.Service{}
	bs, _, em := s.GetFile(tc.Ctx, tc.DB, *tc.User.Username, name, "world.sdf", version, tc.User)
	if em != nil {
		return gzMcpError(em), nil
	}
	return mcp.TextResult(string(*bs)), nil
}

func projectGetModelSDF(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	name, _ := args["name"].(string)
	modelName, _ := args["model_name"].(string)
	if name == "" || modelName == "" {
		return mcp.ErrorResult("name and model_name are required"), nil
	}
	version, _ := args["version"].(string)
	if version == "" {
		version = "tip"
	}
	s := &projects.Service{}
	// Repo layout: models/<model_name>/model.sdf (design §4.2).
	relPath := path.Join("models", modelName, "model.sdf")
	bs, _, em := s.GetFile(tc.Ctx, tc.DB, *tc.User.Username, name, relPath, version, tc.User)
	if em != nil {
		return gzMcpError(em), nil
	}
	return mcp.TextResult(string(*bs)), nil
}

func projectSaveWorldSDF(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	name, _ := args["name"].(string)
	sdfStr, _ := args["sdf"].(string)
	if name == "" || sdfStr == "" {
		return mcp.ErrorResult("name and sdf are required"), nil
	}
	squash, _ := args["squash_autosaves"].(bool)
	commitMsg, _ := args["commit_message"].(string)
	if commitMsg == "" {
		// If no label is provided, this is an auto-save. Use the canonical
		// AutoSaveMessage so the squash path can find the run later.
		commitMsg = projects.AutoSaveMessage
	}
	s := &projects.Service{}
	v, em := s.WriteFileWithOptions(tc.Ctx, tc.DB, *tc.User.Username, name, "world.sdf",
		[]byte(sdfStr), tc.User, projects.WriteFileOptions{
			CommitMessage:   commitMsg,
			SquashAutoSaves: squash,
		})
	if em != nil {
		return gzMcpError(em), nil
	}
	// MCP handlers own the transaction lifecycle — commit so the save
	// becomes durable before the response goes out.
	if err := tc.DB.Commit().Error; err != nil {
		return mcp.ErrorResult("commit: " + err.Error()), nil
	}
	return jsonResult(map[string]any{
		"version":         v,
		"commit_message":  commitMsg,
		"squashed":        squash,
	})
}

// projectReplaceInWorldSDF performs a literal substring replace inside
// the project's world.sdf and saves the result. Designed for incremental
// edits by the chat agent — small targeted changes don't need a full
// world re-emit, which keeps both prompt and tool-output token counts
// down on large worlds.
//
// Errors with a clear message if `find` is not present, so the agent
// can recover (search for a different anchor or fall back to
// project_save_world_sdf).
func projectReplaceInWorldSDF(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	name, _ := args["name"].(string)
	find, _ := args["find"].(string)
	replace, _ := args["replace"].(string)
	if name == "" {
		return mcp.ErrorResult("name is required"), nil
	}
	if find == "" {
		return mcp.ErrorResult("find is required and must be non-empty"), nil
	}
	commitMsg, _ := args["commit_message"].(string)
	if commitMsg == "" {
		commitMsg = projects.AutoSaveMessage
	}

	s := &projects.Service{}
	bs, _, em := s.GetFile(tc.Ctx, tc.DB, *tc.User.Username, name, "world.sdf", "tip", tc.User)
	if em != nil {
		return gzMcpError(em), nil
	}
	current := string(*bs)
	if !strings.Contains(current, find) {
		return mcp.ErrorResult(
			"find string was not found in world.sdf — pick a unique anchor present in the current world",
		), nil
	}
	updated := strings.ReplaceAll(current, find, replace)

	v, em := s.WriteFile(tc.Ctx, tc.DB, *tc.User.Username, name, "world.sdf",
		[]byte(updated), commitMsg, tc.User)
	if em != nil {
		return gzMcpError(em), nil
	}
	if err := tc.DB.Commit().Error; err != nil {
		return mcp.ErrorResult("commit: " + err.Error()), nil
	}
	occurrences := strings.Count(current, find)
	return jsonResult(map[string]any{
		"version":        v,
		"occurrences":    occurrences,
		"commit_message": commitMsg,
	})
}

func projectSaveModelSDF(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	name, _ := args["name"].(string)
	modelName, _ := args["model_name"].(string)
	sdfStr, _ := args["sdf"].(string)
	if name == "" || modelName == "" || sdfStr == "" {
		return mcp.ErrorResult("name, model_name, and sdf are required"), nil
	}
	commitMsg, _ := args["commit_message"].(string)
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("auto-save: model %q updated", modelName)
	}
	relPath := path.Join("models", modelName, "model.sdf")
	s := &projects.Service{}
	v, em := s.WriteFile(tc.Ctx, tc.DB, *tc.User.Username, name, relPath,
		[]byte(sdfStr), commitMsg, tc.User)
	if em != nil {
		return gzMcpError(em), nil
	}
	if err := tc.DB.Commit().Error; err != nil {
		return mcp.ErrorResult("commit: " + err.Error()), nil
	}
	return jsonResult(map[string]any{
		"version":        v,
		"model_name":     modelName,
		"commit_message": commitMsg,
	})
}
