package main

import (
	"encoding/json"
	"fmt"

	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/bundles/worlds"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/mcp"
	"github.com/gazebo-web/gz-go/v7"
)

// registerFuelTools wires every fuel.* tool the MCP endpoint exposes. All
// of them reuse the fuel-server Service layer the REST handlers already
// call, so the same RBAC + private-resource filtering apply.
func registerFuelTools(reg *mcp.Registry) {
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "fuel.search_models",
			Description: "Search Fuel for models the caller can see (public plus owned/private).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":    map[string]any{"type": "string", "description": "Keyword search"},
					"owner":    map[string]any{"type": "string", "description": "Optional owner filter"},
					"per_page": map[string]any{"type": "integer", "description": "Max results (default 20)"},
				},
			},
			// Read-only: satisfied by the fuel:read or project:read scope.
			// JWT callers bypass the gate entirely.
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        fuelSearchModels,
	})

	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "fuel.search_worlds",
			Description: "Search Fuel for worlds the caller can see.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":    map[string]any{"type": "string"},
					"owner":    map[string]any{"type": "string"},
					"per_page": map[string]any{"type": "integer"},
				},
			},
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        fuelSearchWorlds,
	})

	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "fuel.get_model_details",
			Description: "Get metadata for a single Fuel model by (owner, name).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"owner": map[string]any{"type": "string"},
					"name":  map[string]any{"type": "string"},
				},
				"required": []string{"owner", "name"},
			},
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        fuelGetModelDetails,
	})

	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "fuel.get_model_sdf",
			Description: "Fetch the model.sdf of a Fuel model. Optionally pin a version (default: latest).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"owner":   map[string]any{"type": "string"},
					"name":    map[string]any{"type": "string"},
					"version": map[string]any{"type": "string", "description": "'tip', 'N' (default 'tip')"},
				},
				"required": []string{"owner", "name"},
			},
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        fuelGetModelSDF,
	})

	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "fuel.get_world_sdf",
			Description: "Fetch a world's SDF file (<name>.world) by (owner, name). Optionally pin a version.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"owner":   map[string]any{"type": "string"},
					"name":    map[string]any{"type": "string"},
					"version": map[string]any{"type": "string"},
				},
				"required": []string{"owner", "name"},
			},
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        fuelGetWorldSDF,
	})
}

// -- handlers ---------------------------------------------------------------

func fuelSearchModels(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	q, _ := args["query"].(string)
	owner, _ := args["owner"].(string)
	perPage := perPageOrDefault(args["per_page"], 20)

	pr := &gz.PaginationRequest{Page: 1, PerPage: int64(perPage), PageRequested: true}
	ms := &models.Service{Storage: globals.Storage}
	var ownerPtr *string
	if owner != "" {
		ownerPtr = &owner
	}
	list, _, em := ms.ModelList(pr, tc.DB, ownerPtr, "recent", q, nil, tc.User, nil, true)
	if em != nil {
		return gzMcpError(em), nil
	}
	return jsonResult(list)
}

func fuelSearchWorlds(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	q, _ := args["query"].(string)
	owner, _ := args["owner"].(string)
	perPage := perPageOrDefault(args["per_page"], 20)

	pr := &gz.PaginationRequest{Page: 1, PerPage: int64(perPage), PageRequested: true}
	ws := &worlds.Service{Storage: globals.Storage}
	var ownerPtr *string
	if owner != "" {
		ownerPtr = &owner
	}
	list, _, em := ws.WorldList(pr, tc.DB, ownerPtr, "recent", q, nil, tc.User)
	if em != nil {
		return gzMcpError(em), nil
	}
	return jsonResult(list)
}

func fuelGetModelDetails(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	owner, name, em := requireOwnerName(args)
	if em != nil {
		return gzMcpError(em), nil
	}
	ms := &models.Service{Storage: globals.Storage}
	proto, emg := ms.GetModelProto(tc.Ctx, tc.DB, owner, name, tc.User)
	if emg != nil {
		return gzMcpError(emg), nil
	}
	return jsonResult(proto)
}

func fuelGetModelSDF(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	owner, name, em := requireOwnerName(args)
	if em != nil {
		return gzMcpError(em), nil
	}
	version, _ := args["version"].(string)
	if version == "" {
		version = "tip"
	}
	ms := &models.Service{Storage: globals.Storage}
	bs, _, emg := ms.GetFile(tc.Ctx, tc.DB, owner, name, "model.sdf", version, tc.User)
	if emg != nil {
		return gzMcpError(emg), nil
	}
	return mcp.TextResult(string(*bs)), nil
}

func fuelGetWorldSDF(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	owner, name, em := requireOwnerName(args)
	if em != nil {
		return gzMcpError(em), nil
	}
	version, _ := args["version"].(string)
	if version == "" {
		version = "tip"
	}
	ws := &worlds.Service{Storage: globals.Storage}
	// Fuel worlds most commonly ship the world definition as
	// <name>.sdf; older worlds published with <name>.world. Try the
	// canonical .sdf path first and fall back to .world so both shapes
	// resolve. See journal Q-MCP-10.
	candidates := []string{name + ".sdf", name + ".world"}
	var (
		bs  *[]byte
		emg *gz.ErrMsg
	)
	for _, path := range candidates {
		bs, _, emg = ws.GetFile(tc.Ctx, tc.DB, owner, name, path, version, tc.User)
		if emg == nil {
			return mcp.TextResult(string(*bs)), nil
		}
	}
	return gzMcpError(emg), nil
}

// -- small helpers ----------------------------------------------------------

// jsonResult marshals v to pretty JSON and returns it in a text content
// block. MCP content is free-form text; LLMs parse JSON well, so we stream
// structured results that way rather than inventing a custom encoding.
func jsonResult(v interface{}) (mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("marshal: %w", err)
	}
	return mcp.TextResult(string(b)), nil
}

// requireOwnerName extracts `owner` and `name` strings from the args map,
// returning a gz.ErrMsg if either is missing or not a non-empty string.
func requireOwnerName(args map[string]any) (string, string, *gz.ErrMsg) {
	owner, _ := args["owner"].(string)
	name, _ := args["name"].(string)
	if owner == "" {
		return "", "", gz.NewErrorMessage(gz.ErrorOwnerNotInRequest)
	}
	if name == "" {
		return "", "", gz.NewErrorMessage(gz.ErrorNameWrongFormat)
	}
	return owner, name, nil
}

// perPageOrDefault normalizes the per_page arg: JSON numbers land as
// float64, and we want a clamped positive int for pagination.
func perPageOrDefault(v any, def int) int {
	switch x := v.(type) {
	case float64:
		n := int(x)
		if n > 0 {
			return n
		}
	case int:
		if x > 0 {
			return x
		}
	}
	return def
}
