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
	FuelHosts []string
}

// ConfigFromEnv reads AI_TOOLS_FUEL_HOSTS (comma-separated).
func ConfigFromEnv() Config {
	var hosts []string
	for _, h := range strings.Split(os.Getenv("AI_TOOLS_FUEL_HOSTS"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			hosts = append(hosts, h)
		}
	}
	return Config{FuelHosts: hosts}
}

// NewRegistry returns a registry with every AI tool registered.
func NewRegistry(cfg Config) *mcp.Registry {
	reg := mcp.NewRegistry()
	t := &tools{cfg: cfg}
	t.registerFuel(reg)
	t.registerSDF(reg)
	t.registerProject(reg)
	t.registerScene(reg)
	return reg
}

type tools struct {
	cfg Config
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
// read JSON well, so structured results travel that way.
func jsonResult(v any) (mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("marshal: %w", err)
	}
	return mcp.TextResult(string(b)), nil
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
