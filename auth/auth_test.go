package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// certBody is the base64 body of a self-signed certificate for k, the
// form AUTH0_RSA256_PUBLIC_KEY takes.
func certBody(t *testing.T, k *rsa.PrivateKey) string {
	t.Helper()
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(der)
}

func sign(t *testing.T, k *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if kid != "" {
		tok.Header["kid"] = kid
	}
	s, err := tok.SignedString(k)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func future() int64 { return time.Now().Add(time.Hour).Unix() }

func TestVerify(t *testing.T) {
	key, other := newKey(t), newKey(t)
	v, err := NewVerifier(Config{PublicKeyBase64: certBody(t, key)})
	if err != nil {
		t.Fatal(err)
	}
	ok := sign(t, key, "", jwt.MapClaims{"sub": "auth0|me", "exp": future()})
	c, err := v.Verify(context.Background(), ok)
	if err != nil {
		t.Fatalf("valid token: %v", err)
	}
	if sub, _ := c.GetSubject(); sub != "auth0|me" {
		t.Fatalf("subject %q", sub)
	}

	hs, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "x", "exp": future()}).
		SignedString([]byte(certBody(t, key)))
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "x", "exp": future()}).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	for name, tok := range map[string]string{
		"expired":         sign(t, key, "", jwt.MapClaims{"sub": "x", "exp": time.Now().Add(-time.Hour).Unix()}),
		"no expiry":       sign(t, key, "", jwt.MapClaims{"sub": "x"}),
		"issued later":    sign(t, key, "", jwt.MapClaims{"sub": "x", "exp": future(), "iat": future()}),
		"other key":       sign(t, other, "", jwt.MapClaims{"sub": "x", "exp": future()}),
		"HS256 with cert": hs,
		"alg none":        none,
		"garbage":         "not.a.token",
	} {
		if _, err := v.Verify(context.Background(), tok); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestAudienceAndIssuer(t *testing.T) {
	key := newKey(t)
	v, _ := NewVerifier(Config{PublicKeyBase64: certBody(t, key),
		Audiences: []string{"https://fuel.example.org", "https://staging.example.org"},
		Issuer:    "https://tenant.example.org/"})
	good := jwt.MapClaims{"sub": "x", "exp": future(), "iss": "https://tenant.example.org/",
		"aud": []string{"https://staging.example.org", "https://tenant.example.org/userinfo"}}
	if _, err := v.Verify(context.Background(), sign(t, key, "", good)); err != nil {
		t.Fatalf("good: %v", err)
	}
	for name, claims := range map[string]jwt.MapClaims{
		"other audience": {"sub": "x", "exp": future(), "iss": "https://tenant.example.org/", "aud": "https://elsewhere"},
		"no audience":    {"sub": "x", "exp": future(), "iss": "https://tenant.example.org/"},
		"other issuer":   {"sub": "x", "exp": future(), "iss": "https://evil.example.org/", "aud": "https://fuel.example.org"},
	} {
		if _, err := v.Verify(context.Background(), sign(t, key, "", claims)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestJWKS(t *testing.T) {
	k1, k2 := newKey(t), newKey(t)
	served := map[string]*rsa.PrivateKey{"one": k1}
	fetches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches++
		var keys []map[string]string
		for kid, k := range served {
			keys = append(keys, map[string]string{"kid": kid, "kty": "RSA", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes())})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	defer srv.Close()
	v, _ := NewVerifier(Config{JWKSURL: srv.URL})
	ctx := context.Background()
	if _, err := v.Verify(ctx, sign(t, k1, "one", jwt.MapClaims{"sub": "x", "exp": future()})); err != nil {
		t.Fatalf("key one: %v", err)
	}
	// A key the set doesn't have, wrongly claiming a known id, fails.
	if _, err := v.Verify(ctx, sign(t, k2, "one", jwt.MapClaims{"sub": "x", "exp": future()})); err == nil {
		t.Fatal("wrong key under a known id accepted")
	}
	// A rotated-in key is found once the minimum retry interval passes.
	served["two"] = k2
	tok := sign(t, k2, "two", jwt.MapClaims{"sub": "x", "exp": future()})
	if _, err := v.Verify(ctx, tok); err == nil {
		t.Fatal("refetched sooner than the retry interval")
	}
	v.jwks.fetched = time.Now().Add(-2 * jwksMinRetry)
	if _, err := v.Verify(ctx, tok); err != nil {
		t.Fatalf("rotated key: %v", err)
	}
	if fetches != 2 {
		t.Fatalf("%d fetches", fetches)
	}
}

func TestMiddleware(t *testing.T) {
	key := newKey(t)
	v, _ := NewVerifier(Config{PublicKeyBase64: certBody(t, key)})
	var seen string
	next := func(w http.ResponseWriter, r *http.Request) {
		seen, _ = Identity(r)
		w.WriteHeader(http.StatusTeapot)
	}
	good := "Bearer " + sign(t, key, "", jwt.MapClaims{"sub": "auth0|me", "exp": future()})
	run := func(mw func(http.ResponseWriter, *http.Request, http.HandlerFunc), method, header string) int {
		seen = ""
		r := httptest.NewRequest(method, "/x", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		w := httptest.NewRecorder()
		mw(w, r, next)
		return w.Code
	}
	opt, req := v.Optional(), v.Required(nil)
	for _, c := range []struct {
		name   string
		mw     func(http.ResponseWriter, *http.Request, http.HandlerFunc)
		method string
		header string
		code   int
		who    string
	}{
		{"optional, no token", opt, "GET", "", http.StatusTeapot, ""},
		{"optional, good token", opt, "GET", good, http.StatusTeapot, "auth0|me"},
		{"optional, bad token", opt, "GET", "Bearer junk", http.StatusUnauthorized, ""},
		{"optional, malformed header", opt, "GET", "Basic abc", http.StatusUnauthorized, ""},
		{"required, no token", req, "GET", "", http.StatusUnauthorized, ""},
		{"required, good token", req, "GET", good, http.StatusTeapot, "auth0|me"},
		{"required, OPTIONS", req, "OPTIONS", "", http.StatusTeapot, ""},
	} {
		if code := run(c.mw, c.method, c.header); code != c.code || seen != c.who {
			t.Errorf("%s: %d %q, want %d %q", c.name, code, seen, c.code, c.who)
		}
	}

	// A Private-Token without a database is refused, not let through.
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Private-Token", "abc")
	w := httptest.NewRecorder()
	req(w, r, next)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "no database") {
		t.Errorf("private token without db: %d %s", w.Code, w.Body)
	}
}
