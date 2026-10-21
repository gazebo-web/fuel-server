package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gazebo-web/fuel-server/bundles/projects"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
)

// Project files: uploads the AI builds from (bundles/projects/assets.go).

// maxAssetRequestBytes caps one upload request: a few files at the
// per-file limit.
const maxAssetRequestBytes = 4*projects.MaxAssetSize + 1<<20

// ProjectAssetsUpload handles POST /1.0/projects/{name}/assets: a
// multipart form with one or more "file" parts and an optional "kind"
// ("texture" marks images as a material). Returns {assets, skipped}: the
// added files (a zip of several files adds each, and its Gazebo models)
// and the names of files in zips that were skipped.
func ProjectAssetsUpload(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAssetRequestBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return nil, userError(gz.ErrorFormInvalidValue, fmt.Sprintf("upload at most %d MB per request", maxAssetRequestBytes>>20))
		}
		return nil, gz.NewErrorMessageWithBase(gz.ErrorForm, err)
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	kind := r.FormValue("kind")
	var ups []projects.AssetUpload
	for _, fh := range r.MultipartForm.File["file"] {
		if fh.Size > projects.MaxAssetSize {
			return nil, userError(gz.ErrorFormInvalidValue, fmt.Sprintf("%s: larger than %d MB", fh.Filename, projects.MaxAssetSize>>20))
		}
		f, err := fh.Open()
		if err != nil {
			return nil, gz.NewErrorMessageWithBase(gz.ErrorForm, err)
		}
		data, err := io.ReadAll(io.LimitReader(f, projects.MaxAssetSize+1))
		f.Close()
		if err != nil {
			return nil, gz.NewErrorMessageWithBase(gz.ErrorForm, err)
		}
		ups = append(ups, projects.AssetUpload{Name: fh.Filename, Data: data, Kind: kind})
	}
	s := &projects.Service{}
	return s.AddAssets(r.Context(), tx, name, jwtUser, ups)
}

// userError is an error whose message the user reads as is.
func userError(code int64, msg string) *gz.ErrMsg {
	em := gz.NewErrorMessage(code)
	em.Msg = msg
	return em
}

// ProjectAssetsList handles GET /1.0/projects/{name}/assets.
func ProjectAssetsList(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	s := &projects.Service{}
	return s.ListAssets(tx, name, jwtUser)
}

// renameAssetBody is the payload for PATCH /1.0/projects/{name}/assets/{asset}.
type renameAssetBody struct {
	Name string `json:"name"`
}

// ProjectAsset handles GET, PATCH (rename) and DELETE on
// /1.0/projects/{name}/assets/{asset}, where asset is an id or a name.
func ProjectAsset(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	ref := mux.Vars(r)["asset"]
	s := &projects.Service{}
	switch r.Method {
	case http.MethodPatch:
		var body renameAssetBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, gz.NewErrorMessageWithBase(gz.ErrorUnmarshalJSON, err)
		}
		return s.RenameAsset(tx, name, ref, body.Name, jwtUser)
	case http.MethodDelete:
		if em := s.DeleteAsset(tx, name, ref, jwtUser); em != nil {
			return nil, em
		}
		return map[string]string{"status": "ok"}, nil
	}
	_, a, em := s.GetAsset(tx, name, ref, jwtUser)
	return a, em
}

// ProjectAssetRetry handles POST /1.0/projects/{name}/assets/{asset}/retry:
// make a failed file's pictures again (after the mesh worker was busy).
func ProjectAssetRetry(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	s := &projects.Service{}
	return s.RetryAsset(r.Context(), tx, name, mux.Vars(r)["asset"], jwtUser)
}

// ProjectAssetFile handles GET /1.0/projects/{name}/assets/{asset}/{file}:
// the original ("original") or a view (e.g. thumb.jpg).
func ProjectAssetFile(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	vars := mux.Vars(r)
	s := &projects.Service{}
	a, fp, em := s.AssetFile(tx, name, vars["asset"], vars["file"], jwtUser)
	if em != nil {
		return nil, em
	}
	return nil, serveAssetFile(w, r, a, vars["file"], fp)
}

// ProjectAssetsURL handles GET /1.0/projects/{name}/assets-url: a signed
// base path for the project's files, for <img> tags.
func ProjectAssetsURL(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	s := &projects.Service{}
	return s.SignAssetsURL(tx, name, jwtUser)
}

// ProjectAssetFileByToken handles GET /1.0/project-assets/{token}/{asset}/{file}.
func ProjectAssetFileByToken(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {
	vars := mux.Vars(r)
	s := &projects.Service{}
	a, fp, em := s.AssetFileByToken(tx, vars["token"], vars["asset"], vars["file"])
	if em != nil {
		return nil, em
	}
	return nil, serveAssetFile(w, r, a, vars["file"], fp)
}

// serveAssetFile writes a project file. Views are images this server
// made. Originals are whatever was uploaded: raster images show inline,
// anything else (an SVG could carry script) only downloads, and none may
// run script even if opened directly.
func serveAssetFile(w http.ResponseWriter, r *http.Request, a *projects.Asset, file, fp string) *gz.ErrMsg {
	f, err := os.Open(fp)
	if err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorNameNotFound, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("Cache-Control", "private, max-age=3600")
	if file == "original" {
		h.Set("Content-Type", a.Mime)
		inline := a.Kind == projects.AssetImage || (a.Kind == projects.AssetTexture && strings.HasPrefix(a.Mime, "image/"))
		disp := "attachment"
		if inline && r.URL.Query().Get("download") == "" {
			disp = "inline"
		}
		h.Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": a.Name}))
	} else {
		h.Set("Content-Type", projectFileType(filepath.Base(fp)))
	}
	http.ServeContent(w, r, "", st.ModTime(), f)
	return nil
}
