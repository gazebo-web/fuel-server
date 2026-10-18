package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Measure jobs: the bounding box of an existing Fuel model, as Gazebo
// shows it (plan §5.6). fuel-server sends the model's zip; the worker
// unpacks it and runs gzmesh.measure in the same jail as model code,
// since mesh files are untrusted input to their parsers.
//
//	POST /v1/measure?wait=90   model zip -> 200 report JSON | 202 | 413 | 429 | 400

func (w *worker) handleMeasure(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(rw, http.StatusMethodNotAllowed, "POST a model zip")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, w.cfg.MaxModelBytes+1))
	if err != nil {
		httpError(rw, http.StatusBadRequest, err.Error())
		return
	}
	if int64(len(body)) > w.cfg.MaxModelBytes {
		httpError(rw, http.StatusRequestEntityTooLarge, fmt.Sprintf("the model zip is larger than %d MB", w.cfg.MaxModelBytes>>20))
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		httpError(rw, http.StatusBadRequest, "body must be a model zip: "+err.Error())
		return
	}
	wait, ok := w.waitParam(rw, r)
	if !ok {
		return
	}
	sum := sha256.Sum256(append([]byte("measure\n"+measureVersion+"\n"), body...))
	w.serve(rw, r, hex.EncodeToString(sum[:]), "application/json", wait, func(slot int) result {
		return w.measure(zr, slot)
	})
}

func (w *worker) measure(zr *zip.Reader, slot int) result {
	dir, err := w.jobDir()
	if err != nil {
		return result{err: err}
	}
	defer os.RemoveAll(dir)
	if err := w.unzip(zr, filepath.Join(dir, "model")); err != nil {
		// A bad archive is the model's problem, not the worker's: answer
		// with a report so the caller records it.
		return result{body: []byte(fmt.Sprintf(`{"ok":false,"version":%q,"error":%q}`, measureVersion, err.Error())),
			ctype: "application/json"}
	}
	code, err := w.run(dir, slot, "gzmesh.measure", "model")
	if err != nil {
		return result{err: err}
	}
	report, err := os.ReadFile(filepath.Join(dir, "out", "report.json"))
	if err != nil {
		return result{err: err}
	}
	return result{body: report, ctype: "application/json", internal: code == 1}
}

// unzip writes the archive's regular files under dest, refusing paths that
// leave it and archives over the file-count or unpacked-size limits (the
// sizes are counted as written, not taken from the headers).
func (w *worker) unzip(zr *zip.Reader, dest string) error {
	if len(zr.File) > w.cfg.MaxModelFiles {
		return fmt.Errorf("the model has more than %d files", w.cfg.MaxModelFiles)
	}
	left := w.cfg.MaxUnpackedBytes
	for _, f := range zr.File {
		if !f.Mode().IsRegular() {
			continue // directories are made as needed; links are skipped
		}
		name := path.Clean(strings.ReplaceAll(f.Name, "\\", "/"))
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("the model zip has an unsafe path %q", f.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			rc.Close()
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		n, err := io.Copy(out, io.LimitReader(rc, left+1))
		rc.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		if left -= n; left < 0 {
			return fmt.Errorf("the model unpacks to more than %d MB", w.cfg.MaxUnpackedBytes>>20)
		}
	}
	return nil
}
