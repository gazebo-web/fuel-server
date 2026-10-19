package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gazebo-web/gz-go/v7"
	"github.com/gorilla/mux"
	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/gazebo-web/fuel-server/aitools"
	"github.com/gazebo-web/fuel-server/auth"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/mcp"
)

// The public MCP endpoint (design §3.2.3, §8.4), at /mcp with /1.0/mcp as
// an alias: the official Go SDK's stateless streamable HTTP server over the
// aitools registry. fuel-server is an OAuth protected resource and Auth0
// the authorization server:
//
//   - /.well-known/oauth-protected-resource[/mcp] is the RFC 9728 metadata
//     that tells clients where to sign in and which scopes exist;
//   - a request without a valid token gets 401 with a WWW-Authenticate
//     header pointing at it;
//   - an OAuth token must be for this endpoint (audience = its resource URL,
//     AI_TOOLS_MCP_RESOURCE) and grants the scopes it carries; a personal
//     access token ("prefix.key", as Bearer or Private-Token) grants its
//     scopes, or all of them if it predates scopes;
//   - each caller is rate limited (mcp.DefaultLimits).
//
// Without AI_TOOLS_MCP_RESOURCE (local development, tests) the audience
// isn't checked here and a web-session token grants every scope.

type mcpConfig struct {
	// Resource is the endpoint's public URL, e.g. https://fuel.gazebosim.org/mcp.
	Resource string
	// AuthServers are the OAuth issuers clients sign in with (AUTH0_ISSUER).
	AuthServers []string
}

func mcpConfigFromEnv() mcpConfig {
	c := mcpConfig{Resource: strings.TrimSpace(os.Getenv("AI_TOOLS_MCP_RESOURCE"))}
	if iss := strings.TrimSpace(os.Getenv("AUTH0_ISSUER")); iss != "" {
		c.AuthServers = []string{iss}
	}
	return c
}

// resource is the configured resource URL, or this request's /mcp.
func (c mcpConfig) resource(r *http.Request) string {
	if c.Resource != "" {
		return c.Resource
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/mcp"
}

// metadataURL is where the resource's metadata lives (RFC 9728 §3.1: the
// well-known path inserted before the resource's path).
func metadataURL(resource string) string {
	i := strings.Index(resource, "://")
	if i < 0 {
		return resource
	}
	rest := resource[i+3:]
	host, path := rest, ""
	if j := strings.Index(rest, "/"); j >= 0 {
		host, path = rest[:j], rest[j:]
	}
	return resource[:i+3] + host + "/.well-known/oauth-protected-resource" + path
}

// mcpEndpoint is the whole endpoint: CORS, credentials, limits, the SDK.
func mcpEndpoint(reg *mcp.Registry, v *auth.Verifier, cfg mcpConfig) http.Handler {
	limiter := mcp.NewLimiter(mcp.DefaultLimits, reg)
	server := sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server {
		return reg.SDKServer(callerFrom(r))
	}, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: mcp.MaxBodyBytes})
	limited := limiter.Middleware(func(r *http.Request) string {
		if ti := sdkauth.TokenInfoFromContext(r.Context()); ti != nil {
			return fmt.Sprint(ti.Extra["credential"])
		}
		return r.RemoteAddr
	})(server)
	verify := mcpTokenVerifier(v, cfg)
	return mcpCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A Private-Token header is accepted as a bearer token.
		if pt := r.Header.Get("Private-Token"); pt != "" && r.Header.Get("Authorization") == "" {
			r.Header.Set("Authorization", "Bearer "+pt)
		}
		sdkauth.RequireBearerToken(verify, &sdkauth.RequireBearerTokenOptions{
			ResourceMetadataURL: metadataURL(cfg.resource(r)),
			ClockSkew:           time.Minute,
		})(limited).ServeHTTP(w, r)
	}))
}

// callerFrom is the caller the token verifier resolved.
func callerFrom(r *http.Request) mcp.Caller {
	ti := sdkauth.TokenInfoFromContext(r.Context())
	if ti == nil {
		return mcp.Caller{DB: globals.Server.Db, Scopes: []string{}}
	}
	user, _ := ti.Extra["user"].(*users.User)
	scopes, _ := ti.Extra["scopes"].([]string) // nil: every scope
	return mcp.Caller{User: user, Scopes: scopes, DB: globals.Server.Db}
}

// mcpTokenVerifier resolves a bearer token to a user and their scopes.
func mcpTokenVerifier(v *auth.Verifier, cfg mcpConfig) sdkauth.TokenVerifier {
	known := map[string]bool{}
	for _, s := range aitools.Scopes {
		known[s] = true
	}
	invalid := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", sdkauth.ErrInvalidToken, fmt.Sprintf(format, a...))
	}
	return func(ctx context.Context, token string, r *http.Request) (*sdkauth.TokenInfo, error) {
		db := globals.Server.Db
		if strings.Count(token, ".") == 1 { // a personal access token
			at, em := gz.ValidateAccessToken(token, db)
			if em != nil {
				return nil, invalid("unknown access token")
			}
			var u users.User
			if err := db.Where("id = ?", at.UserID).First(&u).Error; err != nil {
				return nil, invalid("the access token's user is gone")
			}
			now := time.Now()
			at.LastUsed = &now
			db.Save(at)
			scopes, err := users.TokenScopes(db, at.ID)
			if err != nil {
				return nil, err
			}
			return &sdkauth.TokenInfo{Scopes: scopes, UserID: *u.Username, Expiration: now.Add(time.Hour),
				Extra: map[string]any{"user": &u, "scopes": scopes, "credential": fmt.Sprint("pat:", at.ID)}}, nil
		}
		var claims auth.Claims
		var err error
		if cfg.Resource != "" {
			claims, err = v.VerifyFor(ctx, token, []string{cfg.Resource})
		} else {
			claims, err = v.Verify(ctx, token)
		}
		if err != nil {
			return nil, invalid("%v", err)
		}
		sub, _ := claims.GetSubject()
		u, em := users.ByIdentity(db, sub, false)
		if em != nil {
			return nil, invalid("no Gazebo Fuel account for this login; sign in at the website once to create one")
		}
		// An OAuth token for this endpoint grants the scopes it carries.
		var scopes []string
		if cfg.Resource != "" {
			scopes = []string{}
			for _, s := range auth.Scopes(claims) {
				if known[s] {
					scopes = append(scopes, s)
				}
			}
		}
		exp, _ := claims.GetExpirationTime()
		info := &sdkauth.TokenInfo{Scopes: scopes, UserID: *u.Username,
			Extra: map[string]any{"user": u, "scopes": scopes, "credential": "user:" + *u.Username}}
		if exp != nil {
			info.Expiration = exp.Time
		}
		return info, nil
	}
}

// protectedResourceMetadata serves RFC 9728 metadata for the endpoint.
func protectedResourceMetadata(cfg mcpConfig) http.Handler {
	return mcpCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		meta := oauthex.ProtectedResourceMetadata{
			Resource:               cfg.resource(r),
			AuthorizationServers:   cfg.AuthServers,
			ScopesSupported:        aitools.Scopes,
			BearerMethodsSupported: []string{"header"},
			ResourceName:           "Gazebo Fuel",
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_ = json.NewEncoder(w).Encode(meta)
	}))
}

// mcpCORS lets browser-based MCP clients call the endpoint and read its
// metadata. Credentials are bearer tokens, never cookies, so any origin
// may.
func mcpCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers",
			"Authorization, Content-Type, Accept, Mcp-Protocol-Version, Mcp-Session-Id, Last-Event-ID, Private-Token")
		h.Set("Access-Control-Expose-Headers", "WWW-Authenticate, Mcp-Session-Id, Retry-After")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// buildMCPRegistry returns the AI tool registry (aitools) the MCP
// endpoint serves. The in-app agent uses the same registry.
func buildMCPRegistry() *mcp.Registry {
	return aitools.NewRegistry(aitools.ConfigFromEnv())
}

// mountMCP adds the endpoint and its metadata to the main router. They go
// on the root router ahead of the /1.0 API, so /1.0/mcp is an alias here
// rather than a gz route.
func mountMCP(router *mux.Router, reg *mcp.Registry, v *auth.Verifier) {
	cfg := mcpConfigFromEnv()
	endpoint := mcpEndpoint(reg, v, cfg)
	router.Handle("/mcp", endpoint)
	router.Handle("/"+globals.APIVersion+"/mcp", endpoint)
	meta := protectedResourceMetadata(cfg)
	for _, p := range []string{"", "/mcp", "/" + globals.APIVersion + "/mcp"} {
		router.Handle("/.well-known/oauth-protected-resource"+p, meta)
	}
}
