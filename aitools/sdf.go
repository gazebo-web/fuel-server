package aitools

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/mcp"
	"github.com/gazebo-web/fuel-server/sdf"
)

const defaultSDFVersion = "1.9"

func (t *tools) registerSDF(reg *mcp.Registry) {
	elem := schema([]string{"element"}, map[string]any{
		"element": str("Element name, e.g. 'link', 'joint', 'sensor', 'camera'"),
		"version": str("SDF version (default " + defaultSDFVersion + ")"),
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "sdf_get_element_schema",
			Description: "Look up one SDF element's schema: attributes, child elements, types, defaults " +
				"and documentation. Use it instead of guessing element or attribute names.",
			InputSchema: elem,
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        t.sdfGetElementSchema,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name:        "sdf_list_child_elements",
			Description: "List the child elements an SDF element may contain.",
			InputSchema: elem,
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        t.sdfListChildElements,
	})
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "sdf_validate",
			Description: "Validate SDF against the schema for its version, plus semantic and physics " +
				"checks (unique names, joint references, Fuel URIs, mass and inertia). Pass either " +
				"'sdf' (a document) or 'name' and 'path' (a project file). Project writes are " +
				"validated automatically; use this for documents you haven't written yet.",
			InputSchema: schema(nil, map[string]any{
				"sdf":  str("SDF document to validate"),
				"name": str("Project name (with 'path')"),
				"path": str("Project file path, e.g. 'world.sdf'"),
				"rev":  str("Project revision (default HEAD)"),
			}),
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        t.sdfValidate,
	})
}

func (t *tools) sdfGetElementSchema(_ *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "element")
	if errRes != nil {
		return *errRes, nil
	}
	version := argOr(args, "version", defaultSDFVersion)
	e, ok := sdf.MustDefault().Get(version, v[0])
	if !ok {
		return mcp.ErrorResult(fmt.Sprintf("no element %q in SDF %s", v[0], version)), nil
	}
	return jsonResult(e)
}

func (t *tools) sdfListChildElements(_ *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "element")
	if errRes != nil {
		return *errRes, nil
	}
	version := argOr(args, "version", defaultSDFVersion)
	return jsonResult(map[string]any{
		"element":  v[0],
		"version":  version,
		"children": sdf.MustDefault().ChildNames(version, v[0]),
	})
}

func (t *tools) sdfValidate(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	doc, label := argString(args, "sdf"), ""
	if doc == "" {
		v, errRes := requireStrings(args, "name", "path")
		if errRes != nil {
			r := mcp.ErrorResult("pass 'sdf', or 'name' and 'path'")
			return r, nil
		}
		b, _, em := projectService.ReadFile(tc.Ctx, tc.DB, *tc.User.Username, v[0], v[1], argString(args, "rev"), tc.User)
		if em != nil {
			return gzError(em), nil
		}
		doc, label = string(b), v[1]
	}
	r := t.validate(tc, doc)
	return jsonResult(validationReport(r, label))
}

func argOr(args map[string]any, key, def string) string {
	if v := argString(args, key); v != "" {
		return v
	}
	return def
}

// validate runs sdf.ValidateWithOptions with the caller-aware URI resolver.
func (t *tools) validate(tc *mcp.ToolContext, doc string) sdf.ValidationResult {
	return sdf.ValidateWithOptions(doc, sdf.Options{ResolveURI: t.uriResolver(tc)})
}

// validationReport is the compact form returned to the model: counts plus
// one line per problem.
func validationReport(r sdf.ValidationResult, file string) map[string]any {
	return map[string]any{
		"valid":    r.Valid,
		"errors":   len(r.Errors),
		"warnings": len(r.Warnings),
		"summary":  r.Summary(file),
	}
}

// uriResolver checks <include><uri> values. Non-HTTP URIs (model://,
// relative paths) are accepted. HTTP URIs must be Fuel model URIs, and on
// the configured FuelHosts the model must exist and be visible to the
// caller.
func (t *tools) uriResolver(tc *mcp.ToolContext) func(string) error {
	return func(raw string) error {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			return err
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil
		}
		seg := strings.Split(strings.Trim(path.Clean(u.Path), "/"), "/")
		if len(seg) < 4 || seg[0] != "1.0" || seg[2] != "models" {
			return fmt.Errorf("not a Fuel model URI (want https://%s/1.0/<owner>/models/<name>)", u.Host)
		}
		owner, name := seg[1], seg[3]
		if !t.isFuelHost(u.Host) {
			return nil
		}
		ms := &models.Service{Storage: globals.Storage}
		if _, em := ms.GetModel(tc.DB, owner, name, tc.User); em != nil {
			return fmt.Errorf("no model %q owned by %q; find real models with fuel_search_models", name, owner)
		}
		return nil
	}
}

func (t *tools) isFuelHost(host string) bool {
	for _, h := range t.cfg.FuelHosts {
		if strings.EqualFold(h, host) {
			return true
		}
	}
	return false
}
