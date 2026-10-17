package aitools

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

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
	num := func(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "sdf_compute_inertia",
			Description: "Compute the exact <inertial> for a solid primitive of uniform density. Use it " +
				"for every dynamic link instead of estimating the tensor. Give mass, or density in " +
				"kg/m³ (wood 600, plastic 1000, steel 7850, cardboard box contents ~100).",
			InputSchema: schema([]string{"shape"}, map[string]any{
				"shape": map[string]any{"type": "string", "enum": sdf.ShapeKinds(),
					"description": "Primitive; cylinder and capsule are along Z, as in SDF"},
				"size": map[string]any{"type": "array", "items": map[string]any{"type": "number"},
					"minItems": 3, "maxItems": 3, "description": "box: x y z size; ellipsoid: x y z radii (m)"},
				"radius":  num("cylinder, sphere, capsule radius (m)"),
				"length":  num("cylinder length, or a capsule's straight part (m)"),
				"mass":    num("Mass in kg (or give density)"),
				"density": num("Density in kg/m³ (or give mass)"),
				"pose":    str("Optional inertial pose relative to the link, e.g. '0 0 0.5 0 0 0'"),
			}),
		},
		RequiredScopes: []string{"fuel:read"},
		Handler:        t.sdfComputeInertia,
	})
}

func (t *tools) sdfComputeInertia(_ *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	f := func(k string) float64 { x, _ := args[k].(float64); return x }
	shape := sdf.Shape{Kind: argString(args, "shape"), Radius: f("radius"), Length: f("length")}
	if size, ok := args["size"].([]any); ok {
		if len(size) != 3 {
			return mcp.ErrorResult("size needs 3 numbers"), nil
		}
		for i, v := range size {
			shape.Size[i], _ = v.(float64)
		}
	}
	in, err := sdf.PrimitiveInertial(shape, f("mass"), f("density"))
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}
	r6 := func(x float64) float64 { y, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'g', 6, 64), 64); return y }
	return jsonResult(map[string]any{
		"mass": r6(in.Mass), "volume": r6(in.Volume), "density": r6(in.Mass / in.Volume),
		"ixx": r6(in.IXX), "iyy": r6(in.IYY), "izz": r6(in.IZZ),
		"inertial": in.XML(argString(args, "pose")),
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
		b, rev, em := projectService.ReadFile(tc.Ctx, tc.DB, *tc.User.Username, v[0], v[1], argString(args, "rev"), tc.User)
		if em != nil {
			return gzError(em), nil
		}
		return jsonResult(validationReport(t.validateIn(tc, v[0], v[1], rev, string(b)), v[1]))
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
// relative paths) are accepted. HTTP URIs must be Fuel model URIs. On the
// configured FuelHosts the model must exist and be visible to the caller;
// on the upstream Fuel host it must exist there. Non-commercial models are
// errors. An unreachable upstream doesn't fail validation.
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
		cat := t.catalogForHost(u.Host)
		if cat == nil {
			return nil
		}
		b, err := cat.Details(tc, kindModels, owner, name)
		if errors.Is(err, errNotInCatalog) {
			return fmt.Errorf("no model %q owned by %q; find real models with fuel_search_models", name, owner)
		}
		var d struct {
			Private bool   `json:"private"`
			License string `json:"license_name"`
		}
		if err != nil || json.Unmarshal(b, &d) != nil {
			return nil
		}
		if nonCommercial(d.License) {
			return fmt.Errorf("non-commercial license (%s): worlds may be used commercially, so don't use this model; search for another or build one", d.License)
		}
		if d.Private {
			return sdf.Warning("private model: only you can load it, so others opening this world won't see it")
		}
		return nil
	}
}

// catalogForHost returns the catalog that serves URIs on host, if any.
func (t *tools) catalogForHost(host string) Catalog {
	if len(t.catalogs) == 0 {
		return nil
	}
	if t.isFuelHost(host) {
		return t.catalogs[0]
	}
	for _, c := range t.catalogs[1:] {
		if strings.EqualFold(c.Host(), host) {
			return c
		}
	}
	return nil
}

func (t *tools) isFuelHost(host string) bool {
	for _, h := range t.cfg.FuelHosts {
		if strings.EqualFold(h, host) {
			return true
		}
	}
	return false
}
