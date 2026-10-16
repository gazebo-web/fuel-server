package main

import (
	"net/http"

	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/mcp"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
)

// mcpServer is the process-wide MCP server (one registry, many requests).
// Populated in application.go during startup.
var mcpServer *mcp.Server

// MCPEndpoint handles POST /1.0/mcp. It resolves the caller from the
// already-validated JWT/PAT, constructs a ToolContext, and hands the
// request off to the MCP server for dispatch.
//
// This matches the pattern gz.HandlerWithResult expects (returns an
// interface{} + *gz.ErrMsg), but because the transport writes the
// response body itself we return (nil, nil) on success and let gz.Handler
// short-circuit the JSONResult wrapping by using NoResult below in routes.
func MCPEndpoint(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {
	if mcpServer == nil {
		return nil, gz.NewErrorMessage(gz.ErrorUnexpected)
	}
	jwtUser, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}
	tc := mcp.NewToolContext(r.Context(), tx, jwtUser, patScopes(r))
	mcpServer.ServeHTTPWith(w, r, tc)
	return nil, nil
}

// patScopes returns the PAT scopes attached to the caller's request, or an
// empty slice if the caller used a JWT. Today gz.AccessToken doesn't carry
// scope metadata, so all PATs are full-access for Phase 2. When scopes land
// (design §4.6 / §13.12) this is the single point to read them.
func patScopes(r *http.Request) []string {
	// Placeholder: gz-go's AccessToken doesn't yet expose per-token scopes.
	// Leave empty so the MCP scope gate is a no-op for now.
	_ = r
	return nil
}

// initMCPServer is called from application.go after the registry has been
// populated with concrete tools. Separated so wiring order is explicit.
func initMCPServer(reg *mcp.Registry) {
	mcpServer = mcp.NewServer(reg)
}

// buildMCPRegistry wires every tool the MCP endpoint exposes. It is called
// once at startup. Tools are registered in their own registerX helpers that
// live next to the bundle they belong to, so adding a tool means adding one
// line here and one handler file under bundles/<thing>/.
func buildMCPRegistry() *mcp.Registry {
	reg := mcp.NewRegistry()
	registerFuelTools(reg)
	registerSDFTools(reg)
	registerProjectTools(reg)
	return reg
}

// Phase 2 placeholder so `users` import isn't flagged as unused once the
// tool packages land in separate files. Remove after A5.
var _ = users.User{}

// Same for globals — touched once application.go is rewired.
var _ = globals.ResourceDir
