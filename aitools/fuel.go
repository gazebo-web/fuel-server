package aitools

import (
	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/bundles/worlds"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/mcp"
	"github.com/gazebo-web/gz-go/v7"
)

func (t *tools) registerFuel(reg *mcp.Registry) {
	search := schema(nil, map[string]any{
		"query":    str("Keywords, e.g. 'warehouse shelf' or 'forklift'"),
		"owner":    str("Optional owner filter, e.g. 'OpenRobotics'"),
		"per_page": map[string]any{"type": "integer", "description": "Max results (default 20, max 50)"},
	})
	ownerName := schema([]string{"owner", "name"}, map[string]any{
		"owner": str("Model owner"),
		"name":  str("Model name"),
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "fuel_search_models",
			Description: "Search the Gazebo Fuel catalog for models the caller can see. Use concrete " +
				"keywords. Include a result in a world with <include><uri>https://fuel.gazebosim.org/1.0/" +
				"{owner}/models/{name}</uri></include>.",
			InputSchema: search,
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        t.fuelSearchModels,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "fuel_search_worlds",
			Description: "Search the Gazebo Fuel catalog for worlds the caller can see.",
			InputSchema: search,
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        t.fuelSearchWorlds,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "fuel_get_model_details",
			Description: "Get metadata for one Fuel model: description, tags, license, file list.",
			InputSchema: ownerName,
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        t.fuelGetModelDetails,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "fuel_get_model_sdf",
			Description: "Fetch a Fuel model's model.sdf, e.g. to learn its size or links before placing it.",
			InputSchema: schema([]string{"owner", "name"}, map[string]any{
				"owner":   str("Model owner"),
				"name":    str("Model name"),
				"version": str("'tip' (default) or a version number"),
			}),
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        t.fuelGetModelSDF,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "fuel_get_world_sdf",
			Description: "Fetch a Fuel world's SDF file.",
			InputSchema: schema([]string{"owner", "name"}, map[string]any{
				"owner":   str("World owner"),
				"name":    str("World name"),
				"version": str("'tip' (default) or a version number"),
			}),
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        t.fuelGetWorldSDF,
	})
}

func searchArgs(args map[string]any) (*gz.PaginationRequest, *string, string) {
	pr := &gz.PaginationRequest{Page: 1, PerPage: int64(perPage(args["per_page"], 20, 50)), PageRequested: true}
	var owner *string
	if o := argString(args, "owner"); o != "" {
		owner = &o
	}
	return pr, owner, argString(args, "query")
}

func (t *tools) fuelSearchModels(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	pr, owner, q := searchArgs(args)
	ms := &models.Service{Storage: globals.Storage}
	list, _, em := ms.ModelList(pr, tc.DB, owner, "recent", q, nil, tc.User, nil, true)
	if em != nil {
		return gzError(em), nil
	}
	return jsonResult(list)
}

func (t *tools) fuelSearchWorlds(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	pr, owner, q := searchArgs(args)
	ws := &worlds.Service{Storage: globals.Storage}
	list, _, em := ws.WorldList(pr, tc.DB, owner, "recent", q, nil, tc.User)
	if em != nil {
		return gzError(em), nil
	}
	return jsonResult(list)
}

func (t *tools) fuelGetModelDetails(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "owner", "name")
	if errRes != nil {
		return *errRes, nil
	}
	ms := &models.Service{Storage: globals.Storage}
	proto, em := ms.GetModelProto(tc.Ctx, tc.DB, v[0], v[1], tc.User)
	if em != nil {
		return gzError(em), nil
	}
	return jsonResult(proto)
}

func versionOrTip(args map[string]any) string {
	if v := argString(args, "version"); v != "" {
		return v
	}
	return "tip"
}

func (t *tools) fuelGetModelSDF(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "owner", "name")
	if errRes != nil {
		return *errRes, nil
	}
	ms := &models.Service{Storage: globals.Storage}
	bs, _, em := ms.GetFile(tc.Ctx, tc.DB, v[0], v[1], "model.sdf", versionOrTip(args), tc.User)
	if em != nil {
		return gzError(em), nil
	}
	return mcp.TextResult(string(*bs)), nil
}

func (t *tools) fuelGetWorldSDF(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "owner", "name")
	if errRes != nil {
		return *errRes, nil
	}
	owner, name := v[0], v[1]
	ws := &worlds.Service{Storage: globals.Storage}
	// Worlds ship as <name>.sdf, or <name>.world in older uploads
	// (journal Q-MCP-10).
	var em *gz.ErrMsg
	for _, path := range []string{name + ".sdf", name + ".world"} {
		var bs *[]byte
		if bs, _, em = ws.GetFile(tc.Ctx, tc.DB, owner, name, path, versionOrTip(args), tc.User); em == nil {
			return mcp.TextResult(string(*bs)), nil
		}
	}
	return gzError(em), nil
}
