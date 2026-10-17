// Package aitools is the single tool layer behind both AI front ends
// (design §3.1): the public MCP endpoint and the in-app agent call the
// same registry in-process. Tools reuse the fuel-server service layer, so
// the caller's permissions apply exactly as they do for the REST API.
//
// Tool names are [a-z0-9_] (provider tool-name rules reject dots) and
// grouped by prefix: fuel_* (catalog), sdf_* (schema and validation),
// project_* (workspace files and versions), scene_* (SDF-aware views).
package aitools

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gazebo-web/fuel-server/mcp"
	"github.com/gazebo-web/gz-go/v7"
)

// Config holds settings shared by the tools.
type Config struct {
	// FuelHosts lists hosts whose model URIs are checked against this
	// server's database during validation (e.g. "fuel.gazebosim.org").
	// URIs on other hosts only get a format check. Empty disables the
	// database check, which suits local development with an empty catalog.
	// The first one is also the host in include URIs for this server's
	// own assets (default fuel.gazebosim.org).
	FuelHosts []string
	// UpstreamFuel is a public Fuel server (e.g. https://fuel.gazebosim.org)
	// whose catalog is searched after this server's own, so development and
	// staging see real models (design §5.4). Empty in production, where this
	// server is the public catalog.
	UpstreamFuel string
	// MeshWorkerURL is the mesh worker (cmd/meshworker) that runs
	// model_generate jobs, and MeshWorkerToken its shared secret. The mesh
	// tools are only offered when the URL is set.
	MeshWorkerURL, MeshWorkerToken string
}

// ConfigFromEnv reads AI_TOOLS_FUEL_HOSTS (comma-separated),
// AI_TOOLS_UPSTREAM_FUEL, AI_TOOLS_MESHWORKER_URL and
// AI_TOOLS_MESHWORKER_TOKEN.
func ConfigFromEnv() Config {
	var hosts []string
	for _, h := range strings.Split(os.Getenv("AI_TOOLS_FUEL_HOSTS"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			hosts = append(hosts, h)
		}
	}
	return Config{
		FuelHosts:       hosts,
		UpstreamFuel:    strings.TrimSpace(os.Getenv("AI_TOOLS_UPSTREAM_FUEL")),
		MeshWorkerURL:   strings.TrimSpace(os.Getenv("AI_TOOLS_MESHWORKER_URL")),
		MeshWorkerToken: os.Getenv("AI_TOOLS_MESHWORKER_TOKEN"),
	}
}

// NewRegistry returns a registry with every AI tool registered.
func NewRegistry(cfg Config) *mcp.Registry {
	reg := mcp.NewRegistry()
	t := &tools{cfg: cfg, catalogs: catalogsFor(cfg)}
	t.registerFuel(reg)
	t.registerSDF(reg)
	t.registerProject(reg)
	t.registerScene(reg)
	t.registerModels(reg)
	if cfg.MeshWorkerURL != "" {
		t.registerMesh(reg)
	}
	return reg
}

type tools struct {
	cfg Config
	// catalogs are searched in order: this server's, then the upstream.
	catalogs []Catalog
	// mesh is the mesh worker client, set when MeshWorkerURL is.
	mesh *meshTools
}

func catalogsFor(cfg Config) []Catalog {
	local := localCatalog{host: "fuel.gazebosim.org"}
	if len(cfg.FuelHosts) > 0 {
		local.host = cfg.FuelHosts[0]
	}
	cats := []Catalog{local}
	if cfg.UpstreamFuel == "" {
		return cats
	}
	up, err := newUpstreamCatalog(cfg.UpstreamFuel)
	if err != nil {
		log.Printf("aitools: ignoring AI_TOOLS_UPSTREAM_FUEL: %v", err)
		return cats
	}
	// This server is the upstream (production): searching it twice
	// would only duplicate results.
	if (&tools{cfg: cfg}).isFuelHost(up.Host()) {
		return cats
	}
	return append(cats, up)
}

// -- shared helpers ----------------------------------------------------------

// schema builds a JSON Schema object for a tool's input.
func schema(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

// jsonResult marshals v to indented JSON in a text content block. LLMs
// read JSON well, so structured results travel that way. HTML escaping is
// off: results carry SDF, and "\u003cmodel\u003e" costs tokens and
// readability.
func jsonResult(v any) (mcp.CallToolResult, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("marshal: %w", err)
	}
	return mcp.TextResult(strings.TrimSuffix(b.String(), "\n")), nil
}

// gzError turns a gz.ErrMsg into a tool-level error result, keeping the
// wrapped cause (e.g. "file changed since base_rev") so the model can act
// on it.
func gzError(em *gz.ErrMsg) mcp.CallToolResult {
	if em == nil {
		return mcp.ErrorResult("unknown error")
	}
	msg := em.Msg
	if em.BaseError != nil && em.BaseError.Error() != "" {
		msg += ": " + em.BaseError.Error()
	}
	return mcp.ErrorResult(msg)
}

func argString(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return s
}

// requireStrings returns the named string args, or an error result naming
// every missing one.
func requireStrings(args map[string]any, keys ...string) ([]string, *mcp.CallToolResult) {
	out := make([]string, len(keys))
	var missing []string
	for i, k := range keys {
		out[i] = argString(args, k)
		if out[i] == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		r := mcp.ErrorResult(strings.Join(missing, ", ") + " required")
		return nil, &r
	}
	return out, nil
}

// perPage normalizes a per_page arg (JSON numbers decode as float64).
func perPage(v any, def, max int) int {
	n := def
	switch x := v.(type) {
	case float64:
		n = int(x)
	case int:
		n = x
	}
	if n <= 0 {
		n = def
	}
	if n > max {
		n = max
	}
	return n
}
