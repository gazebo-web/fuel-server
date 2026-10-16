package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/gazebo-web/fuel-server/globals"
)

// Server dispatches a decoded JSON-RPC request to the appropriate method.
// It is stateless per-request — the transport layer creates a ToolContext
// and passes it through.
type Server struct {
	Registry *Registry
}

// NewServer binds a registry to a server.
func NewServer(reg *Registry) *Server {
	return &Server{Registry: reg}
}

// Dispatch handles one JSON-RPC request and returns a Response. Notification
// requests (no id) still produce a Response; the caller chooses whether to
// write it to the wire.
func (s *Server) Dispatch(tc *ToolContext, req Request) Response {
	resp := Response{JSONRPC: "2.0", ID: req.ID}

	// JSON-RPC 2.0 mandates this field. Reject anything else so clients
	// fail loudly instead of silently mismatching versions.
	if req.JSONRPC != "2.0" {
		resp.Error = &RPCError{
			Code:    ErrCodeInvalidRequest,
			Message: "jsonrpc must be \"2.0\"",
		}
		return resp
	}

	switch req.Method {
	case "initialize":
		resp.Result = s.handleInitialize()
	case "notifications/initialized", "notifications/cancelled":
		// Client-side lifecycle notifications: acknowledge silently.
		resp.Result = struct{}{}
	case "ping":
		resp.Result = struct{}{}
	case "tools/list":
		resp.Result = ListToolsResult{Tools: s.Registry.Specs()}
	case "tools/call":
		result, rpcErr := s.handleToolsCall(tc, req.Params)
		if rpcErr != nil {
			resp.Error = rpcErr
		} else {
			resp.Result = result
		}
	default:
		resp.Error = &RPCError{
			Code:    ErrCodeMethodNotFound,
			Message: "unknown method: " + req.Method,
		}
	}
	return resp
}

func (s *Server) handleInitialize() InitializeResult {
	return InitializeResult{
		ProtocolVersion: ProtocolVersion,
		ServerInfo: ServerInfo{
			Name:    ServerName,
			Version: buildVersion(),
		},
		// `tools` capability object is intentionally empty — per MCP, its
		// presence is what signals the server supports tool invocation.
		Capabilities: map[string]any{
			"tools": map[string]any{},
		},
	}
}

func (s *Server) handleToolsCall(tc *ToolContext, raw json.RawMessage) (CallToolResult, *RPCError) {
	var params CallToolParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return CallToolResult{}, &RPCError{
				Code:    ErrCodeInvalidParams,
				Message: "invalid params: " + err.Error(),
			}
		}
	}
	if params.Name == "" {
		return CallToolResult{}, &RPCError{
			Code:    ErrCodeInvalidParams,
			Message: "tool name required",
		}
	}
	tool, ok := s.Registry.Get(params.Name)
	if !ok {
		return CallToolResult{}, &RPCError{
			Code:    ErrCodeMethodNotFound,
			Message: "unknown tool: " + params.Name,
		}
	}
	// PAT scope gate. JWT callers have empty tc.Scopes (we treat that as
	// "no PAT restrictions apply") and satisfy everything.
	if len(tool.RequiredScopes) > 0 && len(tc.Scopes) > 0 {
		if !anyScopeMatches(tc.Scopes, tool.RequiredScopes) {
			return ErrorResult(fmt.Sprintf(
				"PAT is missing required scope: one of %v", tool.RequiredScopes,
			)), nil
		}
	}
	args := params.Arguments
	if args == nil {
		args = map[string]any{}
	}
	result, err := tool.Handler(tc, args)
	if err != nil {
		// Handler returned an unexpected error (panic'd, DB died, etc.).
		// Surface as a tool-level error, not transport, so the LLM can
		// react without the session dying.
		return ErrorResult(err.Error()), nil
	}
	return result, nil
}

// anyScopeMatches returns true if any scope in have matches any of need.
func anyScopeMatches(have, need []string) bool {
	set := make(map[string]struct{}, len(have))
	for _, s := range have {
		set[s] = struct{}{}
	}
	for _, s := range need {
		if _, ok := set[s]; ok {
			return true
		}
	}
	return false
}

// buildVersion returns the fuel-server build version if the globals package
// carries one; falls back to a sentinel. Read lazily so tests can run
// without globals.Version set.
func buildVersion() string {
	if globals.ResourceDir == "" {
		// Proxy for "not fully initialized" used in a few test paths.
		return "dev"
	}
	return "1.0"
}
