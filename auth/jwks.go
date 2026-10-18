package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// jwks caches an issuer's signing keys by key id. A token with an unknown
// key id refetches the set, at most once a minute, so rotations are picked
// up without letting bad tokens hammer the issuer.
type jwks struct {
	url    string
	client *http.Client

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

const (
	jwksMaxAge   = time.Hour
	jwksMinRetry = time.Minute
)

func newJWKS(url string) *jwks {
	return &jwks{url: url, client: &http.Client{Timeout: 10 * time.Second}, keys: map[string]*rsa.PublicKey{}}
}

func (j *jwks) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	k, ok := j.keys[kid]
	stale := time.Since(j.fetched) > jwksMaxAge
	if ok && !stale {
		return k, nil
	}
	if !stale && time.Since(j.fetched) < jwksMinRetry {
		if ok {
			return k, nil
		}
		return nil, fmt.Errorf("unknown signing key %q", kid)
	}
	keys, err := j.fetch(ctx)
	j.fetched = time.Now()
	if err != nil {
		if ok {
			return k, nil // keep using a known key through an outage
		}
		return nil, fmt.Errorf("signing keys: %w", err)
	}
	j.keys = keys
	if k, ok = keys[kid]; !ok {
		return nil, fmt.Errorf("unknown signing key %q", kid)
	}
	return k, nil
}

func (j *jwks) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: status %d", j.url, resp.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&set); err != nil {
		return nil, err
	}
	out := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || (k.Use != "" && k.Use != "sig") || k.Kid == "" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		exp := 0
		for _, b := range e {
			exp = exp<<8 | int(b)
		}
		out[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exp}
	}
	if len(out) == 0 {
		return nil, errors.New("no RSA signing keys in the set")
	}
	return out, nil
}
