package main

import (
	"github.com/gazebo-web/fuel-server/mcp"
	"github.com/gazebo-web/fuel-server/sdf"
)

const defaultSDFVersion = "1.9"

// registerSDFTools wires the sdf_* MCP tools. Schemas come from the
// embedded sdformat bundle; validation is sdf.Validate.
func registerSDFTools(reg *mcp.Registry) {
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "sdf_get_element_schema",
			Description: "Return the parsed schema definition for a single SDF element (attributes + child elements).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"element": map[string]any{"type": "string"},
					"version": map[string]any{"type": "string"},
				},
				"required": []string{"element"},
			},
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        sdfGetElementSchema,
	})

	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "sdf_list_child_elements",
			Description: "List valid direct child element names for an SDF element, including <include>d siblings.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"element": map[string]any{"type": "string"},
					"version": map[string]any{"type": "string"},
				},
				"required": []string{"element"},
			},
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        sdfListChildElements,
	})

	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "sdf_validate_sdf",
			Description: "Structural validation of an SDF document (XML well-formedness, required root/version, common mistakes).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sdf":     map[string]any{"type": "string", "description": "SDF XML document"},
					"version": map[string]any{"type": "string", "description": "SDF version hint (reserved; current validator ignores it)"},
				},
				"required": []string{"sdf"},
			},
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        sdfValidate,
	})
}

// -- handlers ---------------------------------------------------------------

func sdfGetElementSchema(_ *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	elem, _ := args["element"].(string)
	if elem == "" {
		return mcp.ErrorResult("element is required"), nil
	}
	version := versionArg(args, defaultSDFVersion)
	reg := sdf.MustDefault()
	e, ok := reg.Get(version, elem)
	if !ok {
		return mcp.ErrorResult("no schema for element " + elem + " in SDF " + version), nil
	}
	return jsonResult(e)
}

func sdfListChildElements(_ *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	elem, _ := args["element"].(string)
	if elem == "" {
		return mcp.ErrorResult("element is required"), nil
	}
	version := versionArg(args, defaultSDFVersion)
	reg := sdf.MustDefault()
	children := reg.ChildNames(version, elem)
	return jsonResult(map[string]any{
		"element":  elem,
		"version":  version,
		"children": children,
	})
}

func sdfValidate(_ *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	src, _ := args["sdf"].(string)
	if src == "" {
		return mcp.ErrorResult("sdf is required"), nil
	}
	result := sdf.Validate(src)
	return jsonResult(result)
}

func versionArg(args map[string]any, fallback string) string {
	if v, _ := args["version"].(string); v != "" {
		return v
	}
	return fallback
}
