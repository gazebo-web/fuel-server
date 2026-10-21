package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Inspect jobs: pictures and details of a file a user uploaded to a
// project (fuel-server's project files), so the AI can see it: PDF pages,
// drawings, mesh previews, texture swatches. The worker runs
// gzmesh.inspect in the same jail as model code, since uploads are
// untrusted input to the parsers.
//
//	POST /v1/inspect?kind=document|drawing|mesh|texture&file=NAME[&main=PATH]&wait=90
//	  the file, or a zip (a mesh with its files, a texture set)
//	  -> 200 tarball (report.json and the views it lists) | 202 | 400 | 413 | 429
//
// file names the upload (its extension picks the parser); main is the
// mesh inside a zip.

// inspectVersion is part of the inspect cache key; bump it with
// gzmesh/inspect.py VERSION.
const inspectVersion = "1"

var inspectKinds = map[string]bool{"document": true, "drawing": true, "mesh": true, "texture": true}

var uploadNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func (w *worker) handleInspect(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(rw, http.StatusMethodNotAllowed, "POST a file")
		return
	}
	q := r.URL.Query()
	kind, file, main := q.Get("kind"), q.Get("file"), q.Get("main")
	if !inspectKinds[kind] {
		httpError(rw, http.StatusBadRequest, "kind must be document, drawing, mesh or texture")
		return
	}
	if !uploadNameRE.MatchString(file) {
		httpError(rw, http.StatusBadRequest, "file must be a plain file name")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, w.cfg.MaxModelBytes+1))
	if err != nil {
		httpError(rw, http.StatusBadRequest, err.Error())
		return
	}
	if int64(len(body)) > w.cfg.MaxModelBytes {
		httpError(rw, http.StatusRequestEntityTooLarge, fmt.Sprintf("the file is larger than %d MB", w.cfg.MaxModelBytes>>20))
		return
	}
	var zr *zip.Reader
	if bytes.HasPrefix(body, []byte("PK\x03\x04")) {
		if zr, err = zip.NewReader(bytes.NewReader(body), int64(len(body))); err != nil {
			httpError(rw, http.StatusBadRequest, "unreadable zip: "+err.Error())
			return
		}
		if main != "" {
			main = path.Clean(main)
			if path.IsAbs(main) || main == ".." || strings.HasPrefix(main, "../") {
				httpError(rw, http.StatusBadRequest, "main must be a path inside the zip")
				return
			}
		}
	}
	wait, ok := w.waitParam(rw, r)
	if !ok {
		return
	}
	h := sha256.New()
	fmt.Fprintf(h, "inspect\n%s\n%s\n%s\n%s\n", inspectVersion, kind, file, main)
	h.Write(body)
	w.serve(rw, r, hex.EncodeToString(h.Sum(nil)), "application/x-tar", wait, func(slot int) result {
		return w.inspect(kind, file, main, body, zr, slot)
	})
}

func (w *worker) inspect(kind, file, main string, body []byte, zr *zip.Reader, slot int) result {
	dir, err := w.jobDir()
	if err != nil {
		return result{err: err}
	}
	defer os.RemoveAll(dir)
	input := filepath.Join(dir, "input")
	target := "input/" + file
	if zr != nil {
		if err := w.unzip(zr, input); err != nil {
			// A bad archive is the upload's problem: answer with a report.
			return w.reportOnly(dir, map[string]any{"ok": false, "version": inspectVersion, "error": err.Error()})
		}
		target = "input"
		if main != "" {
			target = "input/" + main
		}
	} else {
		if err := os.MkdirAll(input, 0o755); err != nil {
			return result{err: err}
		}
		if err := os.WriteFile(filepath.Join(input, file), body, 0o644); err != nil {
			return result{err: err}
		}
	}
	job, _ := json.Marshal(map[string]string{"kind": kind, "path": target})
	if err := os.WriteFile(filepath.Join(dir, "inspect.json"), job, 0o644); err != nil {
		return result{err: err}
	}
	code, err := w.run(dir, slot, "gzmesh.inspect", "inspect.json")
	if err != nil {
		return result{err: err}
	}
	tarball, err := tarDir(filepath.Join(dir, "out"), w.cfg.MaxOutput)
	if err != nil {
		return result{err: err}
	}
	return result{body: tarball, ctype: "application/x-tar", internal: code == 1}
}

// reportOnly answers with a tarball holding just report.
func (w *worker) reportOnly(dir string, report map[string]any) result {
	b, _ := json.Marshal(report)
	if err := os.WriteFile(filepath.Join(dir, "out", "report.json"), b, 0o644); err != nil {
		return result{err: err}
	}
	tarball, err := tarDir(filepath.Join(dir, "out"), w.cfg.MaxOutput)
	if err != nil {
		return result{err: err}
	}
	return result{body: tarball, ctype: "application/x-tar"}
}

// Import jobs: an uploaded mesh to a model folder (gzmesh.import_mesh),
// laid out like a model job's result.
//
//	POST /v1/import?wait=90   zip of job.json and input/... (the mesh and its files)
//	  -> 200 tarball | 202 | 400 | 413 | 429

// importVersion is part of the import cache key; bump it with
// gzmesh/import_mesh.py VERSION.
const importVersion = "1"

func (w *worker) handleImport(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(rw, http.StatusMethodNotAllowed, "POST a zip")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, w.cfg.MaxModelBytes+1))
	if err != nil {
		httpError(rw, http.StatusBadRequest, err.Error())
		return
	}
	if int64(len(body)) > w.cfg.MaxModelBytes {
		httpError(rw, http.StatusRequestEntityTooLarge, fmt.Sprintf("the upload is larger than %d MB", w.cfg.MaxModelBytes>>20))
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		httpError(rw, http.StatusBadRequest, "body must be a zip: "+err.Error())
		return
	}
	var job map[string]any
	for _, f := range zr.File {
		switch name := path.Clean(f.Name); {
		case name == "job.json":
			rc, err := f.Open()
			if err == nil {
				err = json.NewDecoder(io.LimitReader(rc, 64<<10)).Decode(&job)
				rc.Close()
			}
			if err != nil {
				httpError(rw, http.StatusBadRequest, "job.json: "+err.Error())
				return
			}
		case strings.HasPrefix(name, "input/") || f.FileInfo().IsDir():
		default:
			httpError(rw, http.StatusBadRequest, "the zip may hold only job.json and input/...")
			return
		}
	}
	if job == nil {
		httpError(rw, http.StatusBadRequest, "the zip has no job.json")
		return
	}
	if name, _ := job["name"].(string); !nameRE.MatchString(name) {
		httpError(rw, http.StatusBadRequest, "name must be lowercase letters, digits and underscores, starting with a letter")
		return
	}
	wait, ok := w.waitParam(rw, r)
	if !ok {
		return
	}
	sum := sha256.Sum256(append([]byte("import\n"+importVersion+"\n"), body...))
	w.serve(rw, r, hex.EncodeToString(sum[:]), "application/x-tar", wait, func(slot int) result {
		return w.importJob(zr, slot)
	})
}

func (w *worker) importJob(zr *zip.Reader, slot int) result {
	dir, err := w.jobDir()
	if err != nil {
		return result{err: err}
	}
	defer os.RemoveAll(dir)
	if err := w.unzip(zr, dir); err != nil {
		return w.reportOnly(dir, map[string]any{"ok": false, "error": err.Error()})
	}
	code, err := w.run(dir, slot, "gzmesh.import_mesh", "job.json")
	if err != nil {
		return result{err: err}
	}
	tarball, err := tarDir(filepath.Join(dir, "out"), w.cfg.MaxOutput)
	if err != nil {
		return result{err: err}
	}
	return result{body: tarball, ctype: "application/x-tar", internal: code == 1}
}
