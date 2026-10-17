package aitools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gazebo-web/fuel-server/mcp"
)

// fakeCatalog serves fixed records; err, when set, fails every call.
type fakeCatalog struct {
	host string
	recs []record
	err  error
}

func (f fakeCatalog) Host() string { return f.host }

func (f fakeCatalog) Search(_ *mcp.ToolContext, _, _, _ string, _ int) ([]record, error) {
	return f.recs, f.err
}

func (f fakeCatalog) find(owner, name string) error {
	if f.err != nil {
		return f.err
	}
	for _, r := range f.recs {
		if r.Owner == owner && r.Name == name {
			return nil
		}
	}
	return errNotInCatalog
}

func (f fakeCatalog) Details(_ *mcp.ToolContext, _, owner, name string) (json.RawMessage, error) {
	if err := f.find(owner, name); err != nil {
		return nil, err
	}
	return json.RawMessage(fmt.Sprintf(`{"name":%q}`, name)), nil
}

func (f fakeCatalog) File(_ *mcp.ToolContext, _, owner, name, path, _ string) ([]byte, error) {
	if err := f.find(owner, name); err != nil {
		return nil, err
	}
	return []byte(f.host + ":" + path), nil
}

func tc() *mcp.ToolContext { return &mcp.ToolContext{Ctx: context.Background()} }

func TestSearchCatalogsMerges(t *testing.T) {
	local := fakeCatalog{host: "staging-fuel.gazebosim.org", recs: []record{
		{Owner: "nate", Name: "Shelf", Private: true},
		{Owner: "OpenRobotics", Name: "Table", ThumbnailURL: "/OpenRobotics/models/Table/tip/files/thumbnails/1.png"},
	}}
	up := fakeCatalog{host: "fuel.gazebosim.org", recs: []record{
		{Owner: "openrobotics", Name: "table"}, // duplicate of the local one
		{Owner: "MovAi", Name: "pallet", LicenseName: "CC-BY-4.0", Tags: []string{"warehouse"}},
	}}

	items, err := searchCatalogs(tc(), []Catalog{local, up}, kindModels, "x", "", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "Table" || items[1].Name != "pallet" {
		t.Fatalf("want local Table then upstream pallet, got %+v", items)
	}
	if items[0].URI != "https://staging-fuel.gazebosim.org/1.0/OpenRobotics/models/Table" || items[0].Source != "local" ||
		items[0].ThumbnailURL != "https://staging-fuel.gazebosim.org/1.0/OpenRobotics/models/Table/tip/files/thumbnails/1.png" {
		t.Errorf("local item %+v", items[0])
	}
	if items[1].URI != "https://fuel.gazebosim.org/1.0/MovAi/models/pallet" || items[1].Source != "fuel.gazebosim.org" ||
		!items[1].Public || items[1].License != "CC-BY-4.0" {
		t.Errorf("upstream item %+v", items[1])
	}

	items, _ = searchCatalogs(tc(), []Catalog{local, up}, kindModels, "x", "", 2, true)
	if len(items) != 2 || items[0].Name != "Shelf" || items[0].Public {
		t.Errorf("include_private with cap 2: %+v", items)
	}
}

func TestSearchCatalogsErrors(t *testing.T) {
	down := fakeCatalog{host: "fuel.gazebosim.org", err: errors.New("timeout")}
	empty := fakeCatalog{host: "localhost"}

	items, err := searchCatalogs(tc(), []Catalog{empty, down}, kindModels, "x", "", 10, false)
	if err == nil {
		t.Fatalf("nothing answered: want an error, got %v", items)
	}
	items, err = searchCatalogs(tc(), []Catalog{fakeCatalog{host: "localhost", recs: []record{{Owner: "a", Name: "b"}}}, down},
		kindModels, "x", "", 10, false)
	if err != nil || len(items) != 1 {
		t.Fatalf("upstream down, local answered: %v %v", items, err)
	}
	items, err = searchCatalogs(tc(), []Catalog{empty}, kindModels, "x", "", 10, false)
	b, _ := json.Marshal(items)
	if err != nil || string(b) != "[]" {
		t.Fatalf("empty result must be [] not %s (%v)", b, err)
	}
}

func TestFirstFoundFallsBack(t *testing.T) {
	tl := &tools{catalogs: []Catalog{
		fakeCatalog{host: "localhost"},
		fakeCatalog{host: "fuel.gazebosim.org", recs: []record{{Owner: "o", Name: "m"}}},
	}}
	b, c, err := tl.firstFound(func(c Catalog) ([]byte, error) { return c.File(tc(), kindModels, "o", "m", "model.sdf", "tip") })
	if err != nil || c.Host() != "fuel.gazebosim.org" || string(b) != "fuel.gazebosim.org:model.sdf" {
		t.Fatalf("got %q from %v, %v", b, c, err)
	}
	_, _, err = tl.firstFound(func(c Catalog) ([]byte, error) { return c.File(tc(), kindModels, "o", "nope", "model.sdf", "tip") })
	if !errors.Is(err, errNotInCatalog) {
		t.Fatalf("missing everywhere: %v", err)
	}
}

func TestURIResolverUpstream(t *testing.T) {
	tl := &tools{catalogs: []Catalog{
		localCatalog{host: "fuel.gazebosim.org"}, // not in FuelHosts: never queried
		fakeCatalog{host: "fuel.gazebosim.org", recs: []record{{Owner: "OpenRobotics", Name: "Euro pallet"}}},
	}}
	resolve := tl.uriResolver(tc())
	if err := resolve("https://fuel.gazebosim.org/1.0/OpenRobotics/models/Euro%20pallet"); err != nil {
		t.Errorf("existing upstream model: %v", err)
	}
	if err := resolve("https://fuel.gazebosim.org/1.0/OpenRobotics/models/Invented"); err == nil ||
		!strings.Contains(err.Error(), "fuel_search_models") {
		t.Errorf("invented model: %v", err)
	}
	tl.catalogs[1] = fakeCatalog{host: "fuel.gazebosim.org", err: errors.New("unreachable")}
	if err := tl.uriResolver(tc())("https://fuel.gazebosim.org/1.0/OpenRobotics/models/Invented"); err != nil {
		t.Errorf("unreachable upstream must not fail validation: %v", err)
	}
}

func TestUpstreamCatalog(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/1.0/models":
			if r.URL.Query().Get("q") != "euro pallet" || r.URL.Query().Get("per_page") != "5" {
				t.Errorf("query %v", r.URL.Query())
			}
			fmt.Fprint(w, `[{"name":"Euro pallet","owner":"OpenRobotics","license_name":"CC0","private":false,"downloads":3}]`)
		case "/1.0/OpenRobotics/models":
			fmt.Fprint(w, `[]`)
		case "/1.0/OpenRobotics/models/Euro pallet/tip/files/model.sdf":
			fmt.Fprint(w, `<sdf/>`)
		case "/1.0/flaky/models":
			w.WriteHeader(http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	up, err := newUpstreamCatalog(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	recs, err := up.Search(tc(), kindModels, "euro pallet", "", 5)
	if err != nil || len(recs) != 1 || recs[0].Name != "Euro pallet" || recs[0].LicenseName != "CC0" {
		t.Fatalf("search: %+v %v", recs, err)
	}
	if _, err := up.Search(tc(), kindModels, "euro pallet", "", 5); err != nil || hits.Load() != 1 {
		t.Errorf("repeat search should be cached: hits=%d err=%v", hits.Load(), err)
	}
	if recs, err := up.Search(tc(), kindModels, "", "OpenRobotics", 5); err != nil || len(recs) != 0 {
		t.Errorf("owner search: %v %v", recs, err)
	}
	if b, err := up.File(tc(), kindModels, "OpenRobotics", "Euro pallet", "model.sdf", "tip"); err != nil || string(b) != "<sdf/>" {
		t.Errorf("file: %q %v", b, err)
	}
	if _, err := up.Details(tc(), kindModels, "OpenRobotics", "Invented"); !errors.Is(err, errNotInCatalog) {
		t.Errorf("404 must be errNotInCatalog: %v", err)
	}
	before := hits.Load()
	for i := 0; i < 2; i++ {
		if _, err := up.Search(tc(), kindModels, "", "flaky", 5); err == nil || errors.Is(err, errNotInCatalog) {
			t.Errorf("502 must be a plain error: %v", err)
		}
	}
	if hits.Load()-before != 2 {
		t.Errorf("transient errors must not be cached")
	}

	for _, bad := range []string{"", "fuel.gazebosim.org", "ftp://fuel.gazebosim.org"} {
		if _, err := newUpstreamCatalog(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

func TestCatalogsFor(t *testing.T) {
	if n := len(catalogsFor(Config{})); n != 1 {
		t.Errorf("no upstream: %d catalogs", n)
	}
	cats := catalogsFor(Config{FuelHosts: []string{"staging-fuel.gazebosim.org"}, UpstreamFuel: "https://fuel.gazebosim.org"})
	if len(cats) != 2 || cats[0].Host() != "staging-fuel.gazebosim.org" || cats[1].Host() != "fuel.gazebosim.org" {
		t.Errorf("staging: %v", cats)
	}
	if n := len(catalogsFor(Config{FuelHosts: []string{"fuel.gazebosim.org"}, UpstreamFuel: "https://fuel.gazebosim.org"})); n != 1 {
		t.Errorf("upstream equal to this server must be skipped: %d catalogs", n)
	}
}

// TestUpstreamCatalogLive checks the upstream client against the real
// public Fuel. Run with AITOOLS_LIVE=1.
func TestUpstreamCatalogLive(t *testing.T) {
	if os.Getenv("AITOOLS_LIVE") == "" {
		t.Skip("set AITOOLS_LIVE=1 to query fuel.gazebosim.org")
	}
	up := catalogsFor(Config{FuelHosts: []string{"localhost"}, UpstreamFuel: "https://fuel.gazebosim.org"})[1]
	tl := &tools{catalogs: []Catalog{fakeCatalog{host: "localhost"}, up}}
	items, err := searchCatalogs(tc(), tl.catalogs, kindModels, "pallet", "", 5, false)
	if err != nil || len(items) == 0 {
		t.Fatalf("search: %v %v", items, err)
	}
	t.Logf("first result: %+v", items[0])
	b, _, err := tl.firstFound(func(c Catalog) ([]byte, error) {
		return c.File(tc(), kindModels, items[0].Owner, items[0].Name, "model.sdf", "tip")
	})
	if err != nil || !strings.Contains(string(b), "<sdf") {
		t.Fatalf("model.sdf: %v", err)
	}
	if err := tl.uriResolver(tc())(items[0].URI); err != nil {
		t.Errorf("resolver rejects a real URI: %v", err)
	}
	if err := tl.uriResolver(tc())("https://fuel.gazebosim.org/1.0/OpenRobotics/models/NoSuchModel12345"); err == nil {
		t.Errorf("resolver accepts an invented URI")
	}
}
