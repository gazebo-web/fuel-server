// Package auth checks the JWTs that fuel-server and the AI Builder agent
// accept (design §8.4), replacing gz-go's middleware on the unmaintained
// dgrijalva/jwt-go and auth0/go-jwt-middleware with golang-jwt/jwt/v5.
//
// A token must be RS256, signed by a configured key, unexpired, and, when
// configured, issued by the expected issuer for one of the expected
// audiences. Keys come from the Auth0 certificate fuel-server always used
// (AUTH0_RSA256_PUBLIC_KEY) and, optionally, the tenant's JWKS, so a key
// rotation needs no redeploy.
//
// Configuration (environment):
//
//	AUTH0_RSA256_PUBLIC_KEY  the signing certificate's base64 body (no PEM header)
//	AUTH0_JWKS_URL           e.g. https://<tenant>/.well-known/jwks.json (optional)
//	AUTH0_AUDIENCE           accepted audiences, comma-separated (optional; checked when set)
//	AUTH0_ISSUER             the expected issuer, e.g. https://<tenant>/ (optional; checked when set)
package auth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Config says which tokens a Verifier accepts.
type Config struct {
	// PublicKeyBase64 is a certificate's (or PKIX public key's) base64
	// body, as AUTH0_RSA256_PUBLIC_KEY holds it.
	PublicKeyBase64 string
	// PublicKeySource, when set, supplies PublicKeyBase64 at each check
	// instead, so a key changed at runtime (gz.Server.SetAuth0RsaPublicKey)
	// takes effect.
	PublicKeySource func() string
	JWKSURL         string
	Audiences       []string
	Issuer          string
}

// ConfigFromEnv reads the AUTH0_* variables above.
func ConfigFromEnv() Config {
	var aud []string
	for _, a := range strings.Split(os.Getenv("AUTH0_AUDIENCE"), ",") {
		if a = strings.TrimSpace(a); a != "" {
			aud = append(aud, a)
		}
	}
	return Config{
		PublicKeyBase64: strings.TrimSpace(os.Getenv("AUTH0_RSA256_PUBLIC_KEY")),
		JWKSURL:         strings.TrimSpace(os.Getenv("AUTH0_JWKS_URL")),
		Audiences:       aud,
		Issuer:          strings.TrimSpace(os.Getenv("AUTH0_ISSUER")),
	}
}

// Verifier checks tokens against a Config.
type Verifier struct {
	cfg  Config
	jwks *jwks

	mu        sync.Mutex
	staticSrc string // the key text static was parsed from
	static    *rsa.PublicKey
}

// NewVerifier parses the configured key. A Verifier with no key at all
// rejects every token.
func NewVerifier(cfg Config) (*Verifier, error) {
	v := &Verifier{cfg: cfg}
	if cfg.PublicKeySource == nil && cfg.PublicKeyBase64 != "" {
		if _, err := v.staticKey(); err != nil {
			return nil, err
		}
	}
	if cfg.JWKSURL != "" {
		v.jwks = newJWKS(cfg.JWKSURL)
	}
	return v, nil
}

// staticKey is the configured key, parsed again when its text changes.
func (v *Verifier) staticKey() (*rsa.PublicKey, error) {
	src := v.cfg.PublicKeyBase64
	if v.cfg.PublicKeySource != nil {
		src = strings.TrimSpace(v.cfg.PublicKeySource())
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if src == v.staticSrc && (v.static != nil || src == "") {
		return v.static, nil
	}
	v.staticSrc, v.static = src, nil
	if src == "" {
		return nil, nil
	}
	k, err := parsePublicKey(src)
	if err != nil {
		return nil, fmt.Errorf("AUTH0_RSA256_PUBLIC_KEY: %w", err)
	}
	v.static = k
	return k, nil
}

// parsePublicKey reads an RSA key from a certificate or a PKIX public key,
// given as a PEM block or just its base64 body.
func parsePublicKey(s string) (*rsa.PublicKey, error) {
	if !strings.Contains(s, "-----BEGIN") {
		s = "-----BEGIN CERTIFICATE-----\n" + s + "\n-----END CERTIFICATE-----"
	}
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, errors.New("not PEM")
	}
	var pub any
	if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
		pub = cert.PublicKey
	} else if pub, err = x509.ParsePKIXPublicKey(block.Bytes); err != nil {
		return nil, errors.New("neither a certificate nor a public key")
	}
	k, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("not an RSA key")
	}
	return k, nil
}

// Claims are a verified token's claims.
type Claims = jwt.MapClaims

// Scopes are the scopes a token grants: its space-separated "scope" claim
// plus Auth0's "permissions" list (RBAC).
func Scopes(c Claims) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if s, ok := c["scope"].(string); ok {
		for _, f := range strings.Fields(s) {
			add(f)
		}
	}
	if ps, ok := c["permissions"].([]any); ok {
		for _, p := range ps {
			if s, ok := p.(string); ok {
				add(s)
			}
		}
	}
	return out
}

// Verify checks a token and returns its claims.
func (v *Verifier) Verify(ctx context.Context, token string) (Claims, error) {
	return v.VerifyFor(ctx, token, v.cfg.Audiences)
}

// VerifyFor is Verify with its own accepted audiences (none: not
// checked), e.g. the MCP endpoint's resource URL.
func (v *Verifier) VerifyFor(ctx context.Context, token string, audiences []string) (Claims, error) {
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(time.Minute),
	}
	if v.cfg.Issuer != "" {
		opts = append(opts, jwt.WithIssuer(v.cfg.Issuer))
	}
	claims := Claims{}
	static, staticErr := v.staticKey()
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if v.jwks != nil && kid != "" {
			if k, err := v.jwks.key(ctx, kid); err == nil {
				return k, nil
			} else if static == nil {
				return nil, err
			}
		}
		if staticErr != nil {
			return nil, staticErr
		}
		if static == nil {
			return nil, errors.New("no signing key configured")
		}
		return static, nil
	}, opts...)
	if err != nil {
		return nil, err
	}
	// golang-jwt checks one audience; any of ours will do.
	if len(audiences) > 0 {
		aud, err := claims.GetAudience()
		if err != nil || !overlaps(aud, audiences) {
			return nil, errors.New("token is not for this server (audience)")
		}
	}
	return claims, nil
}

func overlaps(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// -- request context ---------------------------------------------------------

type ctxKey struct{}

// WithClaims returns ctx carrying a verified token's claims.
func WithClaims(ctx context.Context, c Claims) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

// Identity is the subject of the request's verified token, if any.
func Identity(r *http.Request) (string, bool) {
	c, ok := r.Context().Value(ctxKey{}).(Claims)
	if !ok {
		return "", false
	}
	sub, err := c.GetSubject()
	return sub, err == nil && sub != ""
}

// bearer is the token in "Authorization: Bearer <token>": "" when there is
// no header, an error when the header is malformed.
func bearer(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", nil
	}
	parts := strings.Fields(h)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", errors.New("Authorization header format must be Bearer {token}")
	}
	return parts[1], nil
}
