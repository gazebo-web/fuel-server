package aitools

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const palletReport = `{"ok":true,"bbox":{"min":[-0.6,-0.4,-0.072],"max":[0.6,0.4,0.072]},"size":[1.2,0.8,0.144],` +
	`"visuals":2,"measured":1,"from":"visual"}`

// fakeWorker answers /v1/measure by the model in the (fake) zip body.
func fakeWorker(t *testing.T, calls *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/jobs/job-1" {
			w.Write([]byte(palletReport))
			return
		}
		atomic.AddInt32(calls, 1)
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.Contains(string(body), "pallet"):
			w.Write([]byte(palletReport))
		case strings.Contains(string(body), "async"):
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(`{"job_id":"job-1","status":"running"}`))
		case strings.Contains(string(body), "slow"):
			time.Sleep(300 * time.Millisecond)
			w.Write([]byte(palletReport))
		case strings.Contains(string(body), "huge"):
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		case strings.Contains(string(body), "broken"):
			w.Write([]byte(`{"ok":false,"error":"no visual or collision geometry could be measured"}`))
		default:
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSizerFill(t *testing.T) {
	var calls int32
	srv := fakeWorker(t, &calls)
	cat := fakeCatalog{host: "fuel.example.org"}
	for _, n := range []string{"pallet", "async", "slow", "huge", "broken", "down", "private"} {
		cat.recs = append(cat.recs, record{Owner: "o", Name: n})
	}
	s := newSizer(&meshClient{base: srv.URL, http: srv.Client()})
	item := func(name string, public bool) catalogItem {
		return catalogItem{Owner: "o", Name: name, Public: public, cat: cat}
	}
	items := []catalogItem{item("pallet", true), item("async", true), item("slow", true), item("huge", true),
		item("broken", true), item("down", true), item("private", false)}
	s.fill(items, 100*time.Millisecond)

	p := items[0].BBox
	if p == nil || p.Size != [3]float64{1.2, 0.8, 0.144} || p.Min[2] != -0.072 || p.Note != "1 of 2 visuals measured" {
		t.Fatalf("pallet: %+v %q", p, items[0].SizeNote)
	}
	for i, want := range map[int]string{
		1: "still measuring", // the worker's 202 is followed, past the wait
		2: "still measuring",
		3: "too large",
		4: "no visual or collision geometry",
		5: "size unknown: mesh worker: status 502",
	} {
		if items[i].BBox != nil || !strings.Contains(items[i].SizeNote, want) {
			t.Errorf("%s: %+v %q, want %q", items[i].Name, items[i].BBox, items[i].SizeNote, want)
		}
	}
	if items[6].BBox != nil || items[6].SizeNote != "" || atomic.LoadInt32(&calls) != 6 {
		t.Errorf("private models aren't measured: %+v, %d worker calls", items[6], calls)
	}

	// The slow and async ones finish in the background; a failure isn't
	// retried right away.
	time.Sleep(2500 * time.Millisecond)
	again := []catalogItem{item("slow", true), item("async", true), item("down", true)}
	s.fill(again, 2*time.Second)
	if again[0].BBox == nil || again[1].BBox == nil {
		t.Errorf("finished measurements: %q %q", again[0].SizeNote, again[1].SizeNote)
	}
	if !strings.Contains(again[2].SizeNote, "failed recently") {
		t.Errorf("down: %q", again[2].SizeNote)
	}
}

func TestSizerSearchCap(t *testing.T) {
	var calls int32
	srv := fakeWorker(t, &calls)
	cat := fakeCatalog{host: "fuel.example.org"}
	var items []catalogItem
	for i := 0; i < sizeSearchMax+3; i++ {
		cat.recs = append(cat.recs, record{Owner: "o", Name: "pallet" + string(rune('a'+i))})
		items = append(items, catalogItem{Owner: "o", Name: cat.recs[i].Name, Public: true, cat: cat})
	}
	newSizer(&meshClient{base: srv.URL, http: srv.Client()}).fill(items, time.Second)
	if calls != sizeSearchMax || items[sizeSearchMax-1].BBox == nil ||
		!strings.Contains(items[sizeSearchMax].SizeNote, "fuel_get_model_details") {
		t.Errorf("%d calls; last measured %+v; first skipped %q", calls, items[sizeSearchMax-1].BBox,
			items[sizeSearchMax].SizeNote)
	}
}

func TestWorldBox(t *testing.T) {
	b := &BBox{Min: [3]float64{-0.6, -0.4, -0.072}, Max: [3]float64{0.6, 0.4, 0.072}}
	lo, hi, ok := worldBox(b, "2 3 0.072 0 0 1.5708")
	if !ok || lo != [3]float64{1.6, 2.4, 0} || hi != [3]float64{2.4, 3.6, 0.144} {
		t.Errorf("turned pallet: %v %v %v", lo, hi, ok)
	}
	if lo, hi, ok := worldBox(b, ""); !ok || lo != b.Min || hi != b.Max {
		t.Errorf("no pose: %v %v", lo, hi)
	}
	if _, _, ok := worldBox(b, "1 2 3"); ok {
		t.Error("a short pose isn't used")
	}
}

func TestNormStamp(t *testing.T) {
	for in, want := range map[string]string{
		"2018-01-26T23:51:39Z":          "2018-01-26T23:51:39Z",
		"2018-01-26T23:51:39.123+02:00": "2018-01-26T21:51:39Z",
		"2018-01-26 23:51:39":           "2018-01-26T23:51:39Z",
		"":                              "",
	} {
		if got := normStamp(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
