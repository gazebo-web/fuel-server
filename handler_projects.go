package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gazebo-web/fuel-server/bundles/projects"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
)

// maxThumbnailBytes caps the body size of a thumbnail upload. Project cards
// only need a small preview, so a few hundred KB is the realistic ceiling
// — 5 MB is generous-but-bounded to keep the git repo sane.
const maxThumbnailBytes = 5 << 20

// createProjectBody is the payload for POST /1.0/projects.
type createProjectBody struct {
	Name          string `json:"name" validate:"required"`
	Description   string `json:"description,omitempty"`
	GazeboVersion string `json:"gazebo_version,omitempty"`
	SDFVersion    string `json:"sdf_version,omitempty"`
	ROS2Distro    string `json:"ros2_distro,omitempty"`
}

// updateProjectBody is the payload for PATCH /1.0/projects/{name}.
// Any omitted (zero-value) field is treated as "no change". Callers that
// need to clear a field can send an explicit empty string; we preserve
// pointer semantics here so the service can tell the two apart.
type updateProjectBody struct {
	Description   *string `json:"description,omitempty"`
	GazeboVersion *string `json:"gazebo_version,omitempty"`
	SDFVersion    *string `json:"sdf_version,omitempty"`
	ROS2Distro    *string `json:"ros2_distro,omitempty"`
	GitHubRepo    *string `json:"github_repo,omitempty"`
	GazeboWSURL   *string `json:"gazebo_ws_url,omitempty"`
	GazeboWSKey   *string `json:"gazebo_ws_key,omitempty"`
}

// ProjectList returns the caller's projects, most-recently-updated first.
// Follows the pagHandler signature so it slots into PaginationHandler.
func ProjectList(p *gz.PaginationRequest, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.PaginationResult, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}

	limit := int(p.PerPage)
	offset := int((p.Page - 1) * p.PerPage)
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	s := &projects.Service{}
	list, em := s.List(tx, jwtUser, limit, offset)
	if em != nil {
		return nil, nil, em
	}

	// Total for pagination — cheap enough over a private per-user set.
	var total int
	_ = tx.Model(&projects.Project{}).Where("owner = ?", *jwtUser.Username).Count(&total).Error
	result := gz.PaginationResult{
		Page:       p.Page,
		PerPage:    p.PerPage,
		PageFound:  true,
		QueryCount: int64(total),
		URL:        r.URL.String(),
	}
	return list, &result, nil
}

// ProjectCreate handles POST /1.0/projects.
func ProjectCreate(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *gz.ErrMsg) {

	jwtUser, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	var body createProjectBody
	if em := ParseStruct(&body, r, false); em != nil {
		return nil, em
	}
	if strings.TrimSpace(body.Name) == "" {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNameWrongFormat,
			errors.New("name is required"))
	}

	s := &projects.Service{}
	p, em := s.Create(r.Context(), tx, jwtUser,
		&body.Name, &body.Description,
		&body.GazeboVersion, &body.SDFVersion, &body.ROS2Distro)
	if em != nil {
		return nil, em
	}
	return p, nil
}

// ProjectGet handles GET /1.0/projects/{name}. The owner is always the
// calling user, so the URL only carries the project name.
func ProjectGet(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	s := &projects.Service{}
	return s.GetForCaller(tx, name, jwtUser)
}

// ProjectUpdate handles PATCH /1.0/projects/{name}.
func ProjectUpdate(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	var body updateProjectBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnmarshalJSON, err)
	}

	s := &projects.Service{}
	p, em := s.Update(tx, *jwtUser.Username, name, jwtUser, projects.ProjectUpdate{
		Description:   body.Description,
		GazeboVersion: body.GazeboVersion,
		SDFVersion:    body.SDFVersion,
		ROS2Distro:    body.ROS2Distro,
		GitHubRepo:    body.GitHubRepo,
		GazeboWSURL:   body.GazeboWSURL,
		GazeboWSKey:   body.GazeboWSKey,
	})
	if em != nil {
		return nil, em
	}
	return p, nil
}

// ProjectDelete handles DELETE /1.0/projects/{name}.
func ProjectDelete(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	s := &projects.Service{}
	if em := s.Delete(tx, *jwtUser.Username, name, jwtUser); em != nil {
		return nil, em
	}
	return map[string]string{"status": "ok"}, nil
}

// ProjectVersions handles GET /1.0/projects/{name}/versions.
func ProjectVersions(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	s := &projects.Service{}
	list, em := s.ListVersions(r.Context(), tx, *jwtUser.Username, name, jwtUser)
	if em != nil {
		return nil, em
	}
	return list, nil
}

// ProjectThumbnailUpload handles POST /1.0/projects/{name}/thumbnail. The
// request body is the raw image bytes (PNG); the client renders the
// project's gzweb canvas to a small PNG and posts it whole. It is stored
// as `thumbnail.png` in the project repo, amended into HEAD so thumbnails
// don't add versions, and served back via the existing
// /projects/{name}/{version}/files/thumbnail.png read path.
func ProjectThumbnailUpload(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxThumbnailBytes)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorFormInvalidValue, err)
	}
	if len(body) == 0 {
		return nil, gz.NewErrorMessage(gz.ErrorMissingField)
	}

	s := &projects.Service{}
	res, em := s.WriteFileWithOptions(r.Context(), tx, *jwtUser.Username, name,
		"thumbnail.png", body, jwtUser, projects.WriteFileOptions{Amend: true})
	if em != nil {
		return nil, em
	}
	return res, nil
}

// writeProjectFileBody is the payload for PUT /1.0/projects/{name}/files/{path}.
type writeProjectFileBody struct {
	Content string `json:"content"`
	// BaseRev rejects the write with 409 if the file changed since then.
	BaseRev string `json:"base_rev,omitempty"`
	// Message labels the save; empty records an automatic save.
	Message string `json:"message,omitempty"`
}

// ProjectFileWrite handles PUT /1.0/projects/{name}/files/{path:.*}: the
// SDF editor's save. Same operation as the project_write_file tool.
func ProjectFileWrite(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	var body writeProjectFileBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnmarshalJSON, err)
	}
	msg := body.Message
	if msg == "" {
		msg = projects.AutoSaveMessage + ": editor"
	}
	s := &projects.Service{}
	return s.WriteFileWithOptions(r.Context(), tx, *jwtUser.Username, name, mux.Vars(r)["path"],
		[]byte(body.Content), jwtUser, projects.WriteFileOptions{CommitMessage: msg, BaseRev: body.BaseRev})
}

// checkpointBody is the payload for POST /1.0/projects/{name}/checkpoint.
type checkpointBody struct {
	Message string `json:"message"`
}

// ProjectCheckpoint handles POST /1.0/projects/{name}/checkpoint: a
// labeled save that folds the automatic saves since the last one. Same
// operation as the project_checkpoint tool.
func ProjectCheckpoint(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	var body checkpointBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnmarshalJSON, err)
	}
	s := &projects.Service{}
	return s.Checkpoint(r.Context(), tx, *jwtUser.Username, name, body.Message, jwtUser)
}

// ProjectFileAtVersion handles GET /1.0/projects/{name}/{version}/files/{path:.*}.
// Returns the raw file content with a generic content-type header; the caller
// is expected to know the file's format.
func ProjectFileAtVersion(name string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	vars := mux.Vars(r)
	version := vars["version"]
	path := vars["path"]
	if path == "" {
		return nil, gz.NewErrorMessage(gz.ErrorNameWrongFormat)
	}

	s := &projects.Service{}
	bytes, resolved, em := s.GetFile(r.Context(), tx, *jwtUser.Username, name, path, version, jwtUser)
	if em != nil {
		return nil, em
	}

	writeIgnResourceVersionHeader(w, resolved)
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(*bytes)
	return nil, nil
}
