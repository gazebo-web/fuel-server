package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/jinzhu/gorm"
)

// ServeHTTP is the transport-layer entrypoint. It decodes one JSON-RPC
// request from the body, dispatches it, and writes the response as JSON.
// The caller is expected to have already authenticated the request and
// to pass the resolved user + transaction via ServeHTTPWith.
//
// This helper satisfies the http.Handler contract for callers that want
// anonymous access (none today — the route is on SecureMethods).
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.ServeHTTPWith(w, r, &ToolContext{Ctx: r.Context()})
}

// ServeHTTPWith decodes and dispatches one JSON-RPC message, using the
// supplied ToolContext (authenticated user + DB transaction) for tool
// invocation. Currently unary: the spec allows SSE upgrade for streaming
// tool output, but every tool we ship today fits in one response body.
// SSE upgrade can slot in later without changing tool handlers.
func (s *Server) ServeHTTPWith(w http.ResponseWriter, r *http.Request, tc *ToolContext) {
	// Read the whole body up front; MCP messages are bounded and caching
	// them simplifies error reporting when JSON parsing fails.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, Response{
			JSONRPC: "2.0",
			Error: &RPCError{
				Code:    ErrCodeParseError,
				Message: "failed to read request body: " + err.Error(),
			},
		})
		return
	}
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, Response{
			JSONRPC: "2.0",
			Error: &RPCError{
				Code:    ErrCodeParseError,
				Message: "parse error: " + err.Error(),
			},
		})
		return
	}
	if tc.Ctx == nil {
		tc.Ctx = r.Context()
	}
	resp := s.Dispatch(tc, req)
	writeJSON(w, http.StatusOK, resp)
}

// writeJSON emits one JSON document with the matching content-type.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// NewToolContext builds a ToolContext for a request. Called by the
// fuel-server-level HTTP handler after auth has resolved the user.
func NewToolContext(ctx context.Context, tx *gorm.DB, user *users.User, scopes []string) *ToolContext {
	return &ToolContext{Ctx: ctx, DB: tx, User: user, Scopes: scopes}
}
