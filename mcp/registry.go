package mcp

import (
	"context"
	"sort"
	"sync"

	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/jinzhu/gorm"
)

// ToolContext bundles everything a tool handler needs to do its job. It
// is created per-request by the transport layer after auth has resolved
// the caller's identity.
type ToolContext struct {
	// Ctx is the request context. Cancel when the HTTP connection drops.
	Ctx context.Context
	// DB is a gorm transaction already scoped to this request.
	DB *gorm.DB
	// User is the authenticated user. Never nil inside a tool handler.
	User *users.User
	// Scopes is the set of PAT scopes attached to this request. Empty
	// for JWT-authenticated calls (i.e. full access).
	Scopes []string
}

// ToolHandler is the function signature every registered tool exposes.
// Args is the raw JSON-decoded arguments object from the client; each
// handler is responsible for its own schema validation.
type ToolHandler func(tc *ToolContext, args map[string]any) (CallToolResult, error)

// Tool is a registered unit: the spec shown to clients plus the handler.
type Tool struct {
	Spec    ToolSpec
	Handler ToolHandler
	// RequiredScopes lists PAT scopes needed to invoke this tool.
	// Empty means "no scope restriction" — JWTs satisfy everything,
	// PATs satisfy anything as long as they have at least one matching
	// scope. Phase 2 issues only *:read PATs; write tools should list
	// the corresponding *:write scope so PATs are rejected.
	RequiredScopes []string
}

// Registry is a concurrency-safe mapping from tool name to Tool.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

// Register adds or replaces a tool. Panics on empty name (programmer
// error; tools are registered once at server startup).
func (r *Registry) Register(t Tool) {
	if t.Spec.Name == "" {
		panic("mcp: tool registered with empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Spec.Name] = t
}

// Get returns the tool by name, or (zero, false) if unknown.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Specs returns all registered specs, sorted by name so client listings
// are stable across restarts and across the test suite.
func (r *Registry) Specs() []ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]ToolSpec, 0, len(names))
	for _, n := range names {
		out = append(out, r.tools[n].Spec)
	}
	return out
}
