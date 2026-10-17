package aitools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/bundles/worlds"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/mcp"
	"github.com/gazebo-web/gz-go/v7"
)

// Catalog kinds, as they appear in Fuel URLs.
const (
	kindModels = "models"
	kindWorlds = "worlds"
)

// errNotInCatalog means the catalog has no such asset (or the caller can't
// see it), as opposed to the catalog being unreachable.
var errNotInCatalog = errors.New("not found")

// Catalog is a source of Fuel models and worlds: this server's database, or
// a public Fuel server's REST API (design §5.4).
type Catalog interface {
	// Host is the host that include URIs for this catalog's assets use.
	Host() string
	Search(tc *mcp.ToolContext, kind, query, owner string, n int) ([]record, error)
	Details(tc *mcp.ToolContext, kind, owner, name string) (json.RawMessage, error)
	File(tc *mcp.ToolContext, kind, owner, name, path, version string) ([]byte, error)
}

// record is the subset of a Fuel model or world listing the tools use. The
// local and upstream catalogs share the JSON shape, so both decode into it.
type record struct {
	Name         string   `json:"name"`
	Owner        string   `json:"owner"`
	Description  string   `json:"description"`
	LicenseName  string   `json:"license_name"`
	ThumbnailURL string   `json:"thumbnail_url"`
	Filesize     int64    `json:"filesize"`
	Private      bool     `json:"private"`
	Tags         []string `json:"tags"`
}

// catalogItem is one search result as returned to the model.
type catalogItem struct {
	URI          string   `json:"uri"`
	Owner        string   `json:"owner"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	License      string   `json:"license,omitempty"`
	Public       bool     `json:"public"`
	ThumbnailURL string   `json:"thumbnail_url,omitempty"`
	FileSize     int64    `json:"file_size,omitempty"`
	Source       string   `json:"source"`
}

func fuelURI(host, kind, owner, name string) string {
	return "https://" + host + "/1.0/" + url.PathEscape(owner) + "/" + kind + "/" + url.PathEscape(name)
}

func (r record) item(host, kind, source string) catalogItem {
	thumb := r.ThumbnailURL
	if strings.HasPrefix(thumb, "/") {
		thumb = "https://" + host + "/1.0" + thumb
	}
	return catalogItem{
		URI: fuelURI(host, kind, r.Owner, r.Name), Owner: r.Owner, Name: r.Name,
		Description: r.Description, Tags: r.Tags, License: r.LicenseName,
		Public: !r.Private, ThumbnailURL: thumb, FileSize: r.Filesize, Source: source,
	}
}

// searchCatalogs merges results, local first, de-duplicated by owner/name.
// Private assets are dropped unless includePrivate. An unreachable upstream
// is only an error when nothing else answered.
func searchCatalogs(tc *mcp.ToolContext, cats []Catalog, kind, query, owner string, n int, includePrivate bool) ([]catalogItem, error) {
	items := []catalogItem{}
	seen := map[string]bool{}
	var firstErr error
	for i, c := range cats {
		recs, err := c.Search(tc, kind, query, owner, n)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", c.Host(), err)
			}
			continue
		}
		source := "local"
		if i > 0 {
			source = c.Host()
		}
		for _, r := range recs {
			key := strings.ToLower(r.Owner + "/" + r.Name)
			if seen[key] || (r.Private && !includePrivate) || len(items) >= n {
				continue
			}
			seen[key] = true
			items = append(items, r.item(c.Host(), kind, source))
		}
	}
	if len(items) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return items, nil
}

// -- local: this server's database -------------------------------------------

type localCatalog struct{ host string }

func (c localCatalog) Host() string { return c.host }

func (c localCatalog) Search(tc *mcp.ToolContext, kind, query, owner string, n int) ([]record, error) {
	pr := &gz.PaginationRequest{Page: 1, PerPage: int64(n), PageRequested: true}
	var o *string
	if owner != "" {
		o = &owner
	}
	var list any
	var em *gz.ErrMsg
	if kind == kindWorlds {
		ws := &worlds.Service{Storage: globals.Storage}
		list, _, em = ws.WorldList(pr, tc.DB, o, "recent", query, nil, tc.User)
	} else {
		ms := &models.Service{Storage: globals.Storage}
		list, _, em = ms.ModelList(pr, tc.DB, o, "recent", query, nil, tc.User, nil, true)
	}
	if em != nil {
		return nil, gzErr(em)
	}
	// fuel.Models / fuel.Worlds marshal to {"models": [...]} / {"worlds": [...]}.
	b, err := json.Marshal(list)
	if err != nil {
		return nil, err
	}
	var wrapped map[string][]record
	if err := json.Unmarshal(b, &wrapped); err != nil {
		return nil, err
	}
	return wrapped[kind], nil
}

func (c localCatalog) Details(tc *mcp.ToolContext, kind, owner, name string) (json.RawMessage, error) {
	var v any
	var em *gz.ErrMsg
	if kind == kindWorlds {
		ws := &worlds.Service{Storage: globals.Storage}
		v, em = ws.GetWorldProto(tc.Ctx, tc.DB, owner, name, tc.User)
	} else {
		ms := &models.Service{Storage: globals.Storage}
		v, em = ms.GetModelProto(tc.Ctx, tc.DB, owner, name, tc.User)
	}
	if em != nil {
		return nil, gzErr(em)
	}
	return json.Marshal(v)
}

func (c localCatalog) File(tc *mcp.ToolContext, kind, owner, name, path, version string) ([]byte, error) {
	var bs *[]byte
	var em *gz.ErrMsg
	if kind == kindWorlds {
		ws := &worlds.Service{Storage: globals.Storage}
		bs, _, em = ws.GetFile(tc.Ctx, tc.DB, owner, name, path, version, tc.User)
	} else {
		ms := &models.Service{Storage: globals.Storage}
		bs, _, em = ms.GetFile(tc.Ctx, tc.DB, owner, name, path, version, tc.User)
	}
	if em != nil {
		return nil, gzErr(em)
	}
	return *bs, nil
}

// gzErr maps "not found" style errors to errNotInCatalog so callers can fall
// back to the next catalog.
func gzErr(em *gz.ErrMsg) error {
	if em.StatusCode == http.StatusNotFound || em.StatusCode == http.StatusForbidden ||
		em.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w: %s", errNotInCatalog, em.Msg)
	}
	msg := em.Msg
	if em.BaseError != nil {
		msg += ": " + em.BaseError.Error()
	}
	return errors.New(msg)
}

// -- upstream: a public Fuel server's REST API --------------------------------

// upstreamCatalog reads a public Fuel server anonymously, so it only ever
// sees public assets. Responses are cached briefly: agents repeat searches
// within a turn, and the public catalog changes slowly.
type upstreamCatalog struct {
	base   *url.URL // e.g. https://fuel.gazebosim.org
	client *http.Client
	ttl    time.Duration

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	body    []byte
	err     error
	expires time.Time
}

const upstreamCacheMax = 1024

func newUpstreamCatalog(base string) (*upstreamCatalog, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("upstream Fuel URL %q: want e.g. https://fuel.gazebosim.org", base)
	}
	return &upstreamCatalog{
		base: u, client: &http.Client{Timeout: 15 * time.Second},
		ttl: 5 * time.Minute, cache: map[string]cached{},
	}, nil
}

func (c *upstreamCatalog) Host() string { return c.base.Host }

func (c *upstreamCatalog) Search(tc *mcp.ToolContext, kind, query, owner string, n int) ([]record, error) {
	p := "/1.0/" + kind
	if owner != "" {
		p = "/1.0/" + url.PathEscape(owner) + "/" + kind
	}
	q := url.Values{"page": {"1"}, "per_page": {strconv.Itoa(n)}}
	if query != "" {
		q.Set("q", query)
	}
	b, err := c.get(tc.Ctx, p+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var recs []record
	if err := json.Unmarshal(b, &recs); err != nil {
		return nil, fmt.Errorf("decode %s search: %w", kind, err)
	}
	return recs, nil
}

func (c *upstreamCatalog) Details(tc *mcp.ToolContext, kind, owner, name string) (json.RawMessage, error) {
	return c.get(tc.Ctx, "/1.0/"+url.PathEscape(owner)+"/"+kind+"/"+url.PathEscape(name))
}

func (c *upstreamCatalog) File(tc *mcp.ToolContext, kind, owner, name, path, version string) ([]byte, error) {
	return c.get(tc.Ctx, "/1.0/"+url.PathEscape(owner)+"/"+kind+"/"+url.PathEscape(name)+"/"+
		url.PathEscape(version)+"/files/"+path)
}

// get fetches base+rel, caching successes and not-founds.
func (c *upstreamCatalog) get(ctx context.Context, rel string) ([]byte, error) {
	key := rel
	c.mu.Lock()
	if e, ok := c.cache[key]; ok && time.Now().Before(e.expires) {
		c.mu.Unlock()
		return e.body, e.err
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base.String()+rel, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden ||
		resp.StatusCode == http.StatusUnauthorized:
		err = fmt.Errorf("%w on %s", errNotInCatalog, c.base.Host)
		body = nil
	default:
		// Transient: don't cache.
		return nil, fmt.Errorf("%s: status %d", c.base.Host, resp.StatusCode)
	}
	c.mu.Lock()
	if len(c.cache) >= upstreamCacheMax {
		c.cache = map[string]cached{}
	}
	c.cache[key] = cached{body: body, err: err, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
	return body, err
}
