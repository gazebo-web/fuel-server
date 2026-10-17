package aitools

import (
	"encoding/json"
	"errors"

	"github.com/gazebo-web/fuel-server/mcp"
)

func (t *tools) registerFuel(reg *mcp.Registry) {
	search := schema(nil, map[string]any{
		"query":    str("Keywords, e.g. 'warehouse shelf' or 'forklift'"),
		"owner":    str("Optional owner filter, e.g. 'OpenRobotics'"),
		"per_page": map[string]any{"type": "integer", "description": "Max results (default 20, max 50)"},
		"include_private": map[string]any{"type": "boolean",
			"description": "Also return your private assets. Others can't load a world that includes them. Default false."},
	})
	ownerName := schema([]string{"owner", "name"}, map[string]any{
		"owner": str("Model owner"),
		"name":  str("Model name"),
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "fuel_search_models",
			Description: "Search the public Gazebo Fuel catalog for models. Use concrete keywords. " +
				"Each result has the exact uri to put in <include><uri>...</uri></include>, plus its " +
				"license; credit the ones you use. Non-commercial models are never returned and " +
				"fail validation if included.",
			InputSchema: search,
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        t.fuelSearch(kindModels),
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "fuel_search_worlds",
			Description: "Search the public Gazebo Fuel catalog for worlds.",
			InputSchema: search,
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        t.fuelSearch(kindWorlds),
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

func (t *tools) fuelSearch(kind string) mcp.ToolHandler {
	return func(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
		includePrivate, _ := args["include_private"].(bool)
		items, err := searchCatalogs(tc, t.catalogs, kind, argString(args, "query"), argString(args, "owner"),
			perPage(args["per_page"], 20, 50), includePrivate)
		if err != nil {
			return mcp.ErrorResult("search failed: " + err.Error()), nil
		}
		return jsonResult(items)
	}
}

// firstFound tries each catalog in order, moving on only when an asset is
// missing from one; other errors stop the search.
func (t *tools) firstFound(fn func(Catalog) ([]byte, error)) ([]byte, Catalog, error) {
	var err error
	for _, c := range t.catalogs {
		var b []byte
		if b, err = fn(c); err == nil {
			return b, c, nil
		}
		if !errors.Is(err, errNotInCatalog) {
			return nil, nil, err
		}
	}
	return nil, nil, err
}

func (t *tools) fuelGetModelDetails(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "owner", "name")
	if errRes != nil {
		return *errRes, nil
	}
	b, c, err := t.firstFound(func(c Catalog) ([]byte, error) { return c.Details(tc, kindModels, v[0], v[1]) })
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}
	return jsonResult(map[string]any{
		"uri":     fuelURI(c.Host(), kindModels, v[0], v[1]),
		"details": json.RawMessage(b),
	})
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
	b, _, err := t.firstFound(func(c Catalog) ([]byte, error) {
		return c.File(tc, kindModels, v[0], v[1], "model.sdf", versionOrTip(args))
	})
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}
	return mcp.TextResult(string(b)), nil
}

func (t *tools) fuelGetWorldSDF(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "owner", "name")
	if errRes != nil {
		return *errRes, nil
	}
	owner, name := v[0], v[1]
	// Worlds ship as <name>.sdf, or <name>.world in older uploads
	// (journal Q-MCP-10).
	b, _, err := t.firstFound(func(c Catalog) ([]byte, error) {
		b, err := c.File(tc, kindWorlds, owner, name, name+".sdf", versionOrTip(args))
		if errors.Is(err, errNotInCatalog) {
			b, err = c.File(tc, kindWorlds, owner, name, name+".world", versionOrTip(args))
		}
		return b, err
	})
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}
	return mcp.TextResult(string(b)), nil
}
