package aitools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestInspectClient(t *testing.T) {
	var polls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/v1/jobs/slow":
			if atomic.AddInt32(&polls, 1) < 2 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				w.Write([]byte(`{"job_id":"slow"}`))
				return
			}
			w.Header().Set("Content-Type", "application/x-tar")
			w.Write(makeTar(t, "report.json", `{"ok":true,"info":{"pages":1}}`, "page-1.jpg", "P"))
		case q.Get("file") == "original.pdf" && q.Get("kind") == "document":
			w.Header().Set("Content-Type", "application/x-tar")
			w.Write(makeTar(t, "report.json", `{"ok":true,"info":{"pages":2}}`, "ai.jpg", "A",
				"page-1.jpg", "1", "page-2.jpg", "2", "thumb.jpg", "T"))
		case q.Get("file") == "original.zip":
			if q.Get("main") != "chair/chair.obj" {
				t.Errorf("main = %q", q.Get("main"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(`{"job_id":"slow"}`))
		case q.Get("file") == "original.glb":
			w.Header().Set("Content-Type", "application/x-tar")
			w.Write(makeTar(t, "report.json", `{"ok":false,"error":"the mesh can't be read: bad header"}`))
		case q.Get("file") == "original.svg":
			w.Header().Set("Content-Type", "application/x-tar")
			w.Write(makeTar(t, "report.json", `{"ok":true}`, "../escape.jpg", "x"))
		default:
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer srv.Close()
	m := newMeshClient(Config{MeshWorkerURL: srv.URL, MeshWorkerToken: "tok"})
	ctx := context.Background()

	info, views, err := m.inspect(ctx, "document", "original.pdf", "", []byte("%PDF"))
	if err != nil || info["pages"] != float64(2) || len(views) != 4 || string(views["page-2.jpg"]) != "2" {
		t.Fatalf("pdf: %v %v %v", info, views, err)
	}
	info, views, err = m.inspect(ctx, "mesh", "original.zip", "chair/chair.obj", []byte("PK"))
	if err != nil || info["pages"] != float64(1) || string(views["page-1.jpg"]) != "P" {
		t.Fatalf("after a 202: %v %v %v", info, views, err)
	}
	if _, _, err := m.inspect(ctx, "mesh", "original.glb", "", nil); err == nil || err.Error() != "the mesh can't be read: bad header" {
		t.Errorf("unreadable: %v", err)
	}
	if _, _, err := m.inspect(ctx, "drawing", "original.svg", "", nil); err == nil || !strings.Contains(err.Error(), "unexpected file") {
		t.Errorf("bad view name: %v", err)
	}
	if _, _, err := m.inspect(ctx, "drawing", "original.dxf", "", nil); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Errorf("busy: %v", err)
	}
	srv.Close()
	if _, _, err := m.inspect(ctx, "document", "original.pdf", "", nil); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("down: %v", err)
	}
}
