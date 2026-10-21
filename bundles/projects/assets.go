package projects

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
)

// Project files ("assets") are what a user gives the AI to build from:
// photos, drawings and floor plans, meshes and texture sets. They are
// kept beside the project's repo, not in it, so deleting one frees its
// space, they never leave with an export or a published world, and large
// photos don't weigh on the repo's history. What the AI makes from them
// (an imported model, walls traced from a plan) is committed as usual.
//
// On disk: <owner dir>/project-assets/<project uuid>/<asset id>/ holds
// "original" and the derived views (ai.jpg, thumb.jpg, ...).

// Limits on project files.
const (
	MaxAssetSize         = 25 << 20
	MaxAssetZipUnpacked  = 100 << 20
	MaxAssetZipEntries   = 200
	MaxProjectAssetsSize = 500 << 20
	MaxProjectAssets     = 200
	MaxUserAssetsSize    = 2 << 30
	MaxAssetPixels       = 50_000_000
	maxAssetNameLen      = 120
)

// Asset statuses.
const (
	AssetProcessing = "processing"
	AssetReady      = "ready"
	AssetFailed     = "failed"
)

// AssetInfo is what processing learned about a file: pixel size, pages,
// a mesh's bounds, a texture set's maps.
type AssetInfo map[string]any

// Value stores AssetInfo as JSON in ASCII: other characters (PDF text, ×,
// −) as \u escapes, so any column character set holds it (MySQL 5.7's
// default latin1 rejects them).
func (i AssetInfo) Value() (driver.Value, error) {
	if i == nil {
		return "{}", nil
	}
	b, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}
	return asciiJSON(b), nil
}

// asciiJSON escapes JSON's non-ASCII characters as \uXXXX (surrogate
// pairs above the BMP); the JSON means the same.
func asciiJSON(b []byte) string {
	var out strings.Builder
	for _, r := range string(b) {
		switch {
		case r < 0x80:
			out.WriteRune(r)
		case r > 0xFFFF:
			r1, r2 := utf16.EncodeRune(r)
			fmt.Fprintf(&out, "\\u%04x\\u%04x", r1, r2)
		default:
			fmt.Fprintf(&out, "\\u%04x", r)
		}
	}
	return out.String()
}

// AssetError is an error message stored in ASCII: non-ASCII text is kept
// as a JSON string, read back as it was.
type AssetError string

// Value stores the message.
func (e AssetError) Value() (driver.Value, error) {
	for _, r := range string(e) {
		if r >= 0x80 {
			b, err := json.Marshal(string(e))
			if err != nil {
				return nil, err
			}
			return asciiJSON(b), nil
		}
	}
	return string(e), nil
}

// Scan reads the message.
func (e *AssetError) Scan(v any) error {
	var s string
	switch x := v.(type) {
	case nil:
	case []byte:
		s = string(x)
	case string:
		s = x
	default:
		return fmt.Errorf("asset error: unexpected %T", v)
	}
	if strings.HasPrefix(s, `"`) {
		var u string
		if json.Unmarshal([]byte(s), &u) == nil {
			s = u
		}
	}
	*e = AssetError(s)
	return nil
}

// Scan reads AssetInfo from JSON.
func (i *AssetInfo) Scan(v any) error {
	var b []byte
	switch x := v.(type) {
	case nil:
		*i = AssetInfo{}
		return nil
	case []byte:
		b = x
	case string:
		b = []byte(x)
	default:
		return fmt.Errorf("asset info: unexpected %T", v)
	}
	return json.Unmarshal(b, i)
}

// Asset is one project file.
//
// swagger:model dbProjectAsset
type Asset struct {
	ID        string `gorm:"primary_key;type:varchar(16)" json:"id"`
	ProjectID uint   `gorm:"unique_index:idx_asset_project_name" json:"-"`
	// Name is the file's name, unique in its project; the AI refers to
	// files by it or by ID.
	Name   string     `gorm:"type:varchar(191);unique_index:idx_asset_project_name" json:"name"`
	Kind   string     `gorm:"type:varchar(16)" json:"kind"`
	Mime   string     `gorm:"type:varchar(64)" json:"mime"`
	Size   int64      `json:"size"`
	SHA256 string     `gorm:"column:sha256;type:char(64)" json:"sha256"`
	Status string     `gorm:"type:varchar(16)" json:"status"`
	Error  AssetError `gorm:"type:text" json:"error,omitempty"`
	Info   AssetInfo  `gorm:"type:mediumtext" json:"info"`
	// Views are the derived files beside "original", e.g. ai.jpg.
	Views     string    `gorm:"type:varchar(1024)" json:"-"`
	ViewList  []string  `gorm:"-" json:"views"`
	CreatedBy string    `gorm:"type:varchar(191)" json:"created_by"`
	CreatedAt time.Time `gorm:"type:datetime(3)" json:"created_at"`
	UpdatedAt time.Time `gorm:"type:datetime(3)" json:"updated_at"`
}

// TableName is project_assets.
func (Asset) TableName() string { return "project_assets" }

// AfterFind fills ViewList.
func (a *Asset) AfterFind() error {
	a.ViewList = splitViews(a.Views)
	return nil
}

func splitViews(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}

// HasView reports whether the asset has the derived file name.
func (a *Asset) HasView(name string) bool {
	for _, v := range splitViews(a.Views) {
		if v == name {
			return true
		}
	}
	return false
}

// assetsDir is where a project's files live.
func assetsDir(p *Project) string {
	// Location is <ResourceDir>/<owner>/projects/<uuid>.
	return filepath.Join(filepath.Dir(filepath.Dir(*p.Location)), "project-assets", *p.UUID)
}

func assetDir(p *Project, id string) string { return filepath.Join(assetsDir(p), id) }

// AssetInspector draws and describes an upload the server can't read
// itself (a PDF, a drawing, a mesh, a texture set): the mesh worker's
// inspect job, which runs the parsers in its sandbox. It returns what it
// learned and the views it made (ai.jpg, thumb.jpg, page-N.jpg,
// preview.png). file is a name with the upload's extension; main is the
// mesh inside a zip.
// user is who uploads, for the worker's per-user limits.
type AssetInspector func(ctx context.Context, user, kind, file, main string, data []byte) (AssetInfo, map[string][]byte, error)

// RetryableError is an inspection that failed for a passing reason (the
// worker busy, down or slow; a rate limit), not because of the file.
// The file is kept, marked failed, and can be inspected again.
type RetryableError struct{ Err error }

func (e RetryableError) Error() string { return e.Err.Error() }
func (e RetryableError) Unwrap() error { return e.Err }

// InspectAsset is set when a mesh worker is configured (aitools). Without
// one, such uploads are kept with no pictures.
var InspectAsset AssetInspector

// inspectTimeout bounds one upload's inspection.
const inspectTimeout = 2 * time.Minute

// mimeExts is the extension the inspector is given for each type.
var mimeExts = map[string]string{
	"application/pdf": ".pdf", "image/svg+xml": ".svg", "image/vnd.dxf": ".dxf",
	"model/gltf-binary": ".glb", "model/obj": ".obj", "model/stl": ".stl",
	"model/vnd.collada+xml": ".dae", "application/zip": ".zip",
}

// rasterMimes are the images the server reads itself (SVG and DXF are
// drawings, drawn by the mesh worker).
var rasterMimes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, pgmMime: true}

// AssetExt is the file extension for a project file's type, e.g. ".obj".
func AssetExt(mime string) string { return mimeExts[mime] }

// AssetUpload is one file to add.
type AssetUpload struct {
	Name string
	Data []byte
	// Kind optionally overrides the detected kind; only "texture" is
	// accepted, for an image meant as a material.
	Kind string
	// Info is kept with the file: where in a zip it came from.
	Info AssetInfo
}

// assetIDRe matches an asset id: 12 hex characters.
var assetIDRe = regexp.MustCompile(`^[0-9a-f]{12}$`)

func newAssetID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// cleanAssetName makes an upload's name safe to show and to use as a key:
// the last path element, without control characters, at most
// maxAssetNameLen characters.
func cleanAssetName(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r) || r == '/':
			return -1
		case r > 0xFFFF:
			// The database's utf8 holds three bytes per character: no emoji.
			return '_'
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > maxAssetNameLen {
		ext := path.Ext(name)
		if len([]rune(ext)) > 16 {
			ext = ""
		}
		name = string(r[:maxAssetNameLen-len([]rune(ext))]) + ext
	}
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	return name
}

// uniqueName returns name, or name-2.ext, name-3.ext, ... if taken.
func uniqueName(name string, taken map[string]bool) string {
	if !taken[name] {
		return name
	}
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 2; ; n++ {
		c := fmt.Sprintf("%s-%d%s", stem, n, ext)
		if !taken[c] {
			return c
		}
	}
}

// assetErr is an error whose message says what's wrong, since a user
// reads it in the upload panel.
func assetErr(code int64, format string, a ...any) *gz.ErrMsg {
	em := gz.NewErrorMessage(code)
	em.Msg = fmt.Sprintf(format, a...)
	return em
}

// AddResult is what an upload added: the files, and the names of files in
// zips that were skipped (not a kind the AI can use).
type AddResult struct {
	Assets  []Asset  `json:"assets"`
	Skipped []string `json:"skipped,omitempty"`
}

// AddAssets checks and stores uploads in a project the caller can write.
// Zips that aren't one mesh or one texture set are unpacked (bundles):
// their files are added one by one and their Gazebo model folders become
// project models. Either everything is added or nothing is (a failure
// after the models' commit leaves them in the repo).
func (s *Service) AddAssets(ctx context.Context, tx *gorm.DB, name string, jwtUser *users.User,
	ups []AssetUpload) (*AddResult, *gz.ErrMsg) {
	p, em := s.GetForCaller(tx, name, jwtUser)
	if em != nil {
		return nil, em
	}
	if !s.canWrite(p, jwtUser) {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}
	if len(ups) == 0 {
		return nil, assetErr(gz.ErrorMissingField, "no files")
	}
	// One upload at a time per owner, so the limits below can't be raced.
	// (Per process: with several replicas each enforces its own share.)
	unlock := lockOwner(*p.Owner)
	defer unlock()
	var userTotal struct{ N int64 }
	if err := tx.Raw(`SELECT COALESCE(SUM(a.size), 0) AS n FROM project_assets a JOIN projects p ON p.id = a.project_id
		WHERE p.owner = ? AND p.deleted_at IS NULL`, *p.Owner).Scan(&userTotal).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNoDatabase, err)
	}
	var existing []Asset
	if err := tx.Where("project_id = ?", p.ID).Find(&existing).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNoDatabase, err)
	}
	// Names are unique as the database compares them, which ignores case.
	taken := map[string]bool{}
	var total int64
	for _, a := range existing {
		taken[strings.ToLower(a.Name)] = true
		total += a.Size
	}
	userTotal.N -= total

	type prepared struct {
		asset Asset
		data  []byte
		views map[string][]byte
	}
	var out []prepared
	var models []modelFolder
	var modelFrom []string // the zip each model came from
	result := &AddResult{}
	addSize := func(fname string, n int) *gz.ErrMsg {
		if total += int64(n); total > MaxProjectAssetsSize {
			return assetErr(gz.ErrorFormInvalidValue, "a project's files may total at most %d MB",
				MaxProjectAssetsSize>>20)
		}
		if userTotal.N+total > MaxUserAssetsSize {
			return assetErr(gz.ErrorFormInvalidValue, "your project files may total at most %d GB; "+
				"delete some to make room", MaxUserAssetsSize>>30)
		}
		return nil
	}
	for i := 0; i < len(ups); i++ { // ups grows as bundles unpack
		up := ups[i]
		fname := cleanAssetName(up.Name)
		if len(up.Data) > MaxAssetSize {
			return nil, assetErr(gz.ErrorFormInvalidValue, "%s: larger than %d MB", fname, MaxAssetSize>>20)
		}
		kind, mime, info, err := sniffAsset(up.Data)
		if err != nil {
			return nil, assetErr(gz.ErrorFormInvalidValue, "%s: %v", fname, err)
		}
		if kind == assetBundle {
			if up.Info["from_zip"] != nil {
				result.Skipped = append(result.Skipped, fmt.Sprint(up.Info["path"])+" (a zip inside a zip)")
				continue
			}
			if up.Kind != "" {
				return nil, assetErr(gz.ErrorFormInvalidValue, "%s: a zip of several files can't be one texture", fname)
			}
			more, ms, skipped, err := expandBundle(fname, up.Data)
			if err != nil {
				return nil, assetErr(gz.ErrorFormInvalidValue, "%s: %v", fname, err)
			}
			ups = append(ups, more...)
			models = append(models, ms...)
			for range ms {
				modelFrom = append(modelFrom, fname)
			}
			for _, sk := range skipped {
				result.Skipped = append(result.Skipped, fname+": "+sk)
			}
			continue
		}
		if em := addSize(fname, len(up.Data)); em != nil {
			return nil, em
		}
		switch up.Kind {
		case "", kind:
		case AssetTexture:
			if kind != AssetImage {
				return nil, assetErr(gz.ErrorFormInvalidValue, "%s: only an image or a zip of images can be a texture", fname)
			}
			kind = AssetTexture
		default:
			return nil, assetErr(gz.ErrorFormInvalidValue, "%s: kind can only be set to %q", fname, AssetTexture)
		}
		data, views := up.Data, map[string][]byte(nil)
		switch {
		case rasterMimes[mime]:
			var more AssetInfo
			data, views, more, err = processImage(up.Data, mime)
			if err != nil {
				return nil, assetErr(gz.ErrorFormInvalidValue, "%s: %v", fname, err)
			}
			info = merge(info, more)
		case kind == AssetMap:
			var more AssetInfo
			if views, more, err = processMap(up.Data); err != nil {
				return nil, assetErr(gz.ErrorFormInvalidValue, "%s: %v", fname, err)
			}
			info = merge(info, more)
		}
		info = merge(info, up.Info)
		status, errMsg := AssetReady, ""
		if views == nil {
			views, info, status, errMsg = inspect(ctx, *jwtUser.Username, kind, mime, info, data)
		}
		fname = uniqueFoldedName(fname, taken)
		taken[strings.ToLower(fname)] = true
		out = append(out, prepared{newAsset(p, fname, kind, mime, data, info, views, status, errMsg, jwtUser), data, views})
	}

	// Gazebo models: committed to the repo as models/<name>/ in one
	// commit, each recorded as a file of kind model.
	repoFiles := map[string][]byte{}
	if len(models) > 0 {
		files, _, em := s.ListFiles(ctx, tx, *p.Owner, *p.Name, "", jwtUser)
		if em != nil {
			return nil, em
		}
		takenModels := map[string]bool{}
		for _, f := range files {
			if rest, ok := strings.CutPrefix(f, "models/"); ok {
				takenModels[strings.ToLower(strings.SplitN(rest, "/", 2)[0])] = true
			}
		}
		for i, m := range models {
			mname := modelName(m.dir, takenModels)
			rewriteModelURIs(m, path.Base(m.dir), mname)
			zipData, err := zipOf(prefixed(mname+"/", m.files))
			if err != nil {
				return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
			}
			if em := addSize(mname, len(zipData)); em != nil {
				return nil, em
			}
			info := merge(modelInfo(mname, m), AssetInfo{"from_zip": modelFrom[i], "path": m.dir})
			views := modelThumbnail(m)
			status, errMsg := AssetReady, ""
			// The worker measures the model; its pictures are the model's own
			// thumbnails, if it has any.
			_, measured, st, msg := inspect(ctx, *jwtUser.Username, AssetModel, "application/zip", info, zipData)
			info, status, errMsg = measured, st, msg
			fname := uniqueFoldedName(mname, taken)
			taken[strings.ToLower(fname)] = true
			out = append(out, prepared{newAsset(p, fname, AssetModel, "application/zip", zipData, info, views, status,
				errMsg, jwtUser), zipData, views})
			for rel, b := range m.files {
				repoFiles[path.Join("models", mname, rel)] = b
			}
		}
	}
	if len(existing)+len(out) > MaxProjectAssets {
		return nil, assetErr(gz.ErrorFormInvalidValue, "a project holds at most %d files; it has %d and this "+
			"upload adds %d", MaxProjectAssets, len(existing), len(out))
	}
	if len(repoFiles) > 0 {
		if _, em := s.WriteFiles(ctx, tx, *p.Owner, *p.Name, repoFiles, nil, jwtUser, WriteFileOptions{
			CommitMessage: AutoSaveMessage + ": uploaded models"}); em != nil {
			return nil, em
		}
	}

	var added []Asset
	// tx may not be a transaction (REST handlers get the plain handle),
	// so a failure undoes the files added so far by hand.
	cleanup := func() {
		for _, a := range added {
			tx.Delete(&Asset{ID: a.ID})
			_ = os.RemoveAll(assetDir(p, a.ID))
		}
	}
	for _, pr := range out {
		dir := assetDir(p, pr.asset.ID)
		if err := writeAssetFiles(dir, pr.data, pr.views); err != nil {
			_ = os.RemoveAll(dir)
			cleanup()
			return nil, gz.NewErrorMessageWithBase(gz.ErrorCreatingFile, err)
		}
		a := pr.asset
		err := tx.Create(&a).Error
		// The database may see two names as the same where Go doesn't
		// (accents): take the next free suffix.
		for i := 0; err != nil && isDuplicate(err) && i < 20; i++ {
			taken[strings.ToLower(a.Name)] = true
			a.Name = uniqueFoldedName(a.Name, taken)
			err = tx.Create(&a).Error
		}
		if err != nil {
			_ = os.RemoveAll(dir)
			cleanup()
			return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
		}
		a.ViewList = splitViews(a.Views)
		added = append(added, a)
	}
	result.Assets = added
	return result, nil
}

func newAsset(p *Project, name, kind, mime string, data []byte, info AssetInfo, views map[string][]byte,
	status, errMsg string, user *users.User) Asset {
	sum := sha256.Sum256(data)
	var names []string
	for v := range views {
		names = append(names, v)
	}
	sort.Strings(names)
	return Asset{
		ID: newAssetID(), ProjectID: p.ID, Name: name, Kind: kind, Mime: mime,
		Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Status: status, Error: AssetError(errMsg),
		Info: merge(info, nil), Views: strings.Join(names, ","), CreatedBy: *user.Username,
	}
}

func prefixed(prefix string, files map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(files))
	for n, b := range files {
		out[prefix+n] = b
	}
	return out
}

// inspect makes the views of an upload the server can't draw itself, on
// the mesh worker. It returns the views, the info (the server's own keys
// win over the worker's) and the status: ready, or failed with the reason
// (retryable when the failure was the worker's, not the file's).
func inspect(ctx context.Context, user, kind, mime string, info AssetInfo, data []byte) (map[string][]byte,
	AssetInfo, string, string) {
	if InspectAsset == nil {
		return nil, info, AssetReady, ""
	}
	main, _ := info["main"].(string)
	ictx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	more, views, err := InspectAsset(ictx, user, kind, "original"+mimeExts[mime], main, data)
	var retry RetryableError
	switch {
	case errors.As(err, &retry):
		out := merge(info, nil)
		out["retryable"] = true
		return nil, out, AssetFailed, err.Error() + "; retry it later"
	case err != nil:
		return nil, info, AssetFailed, err.Error()
	}
	out := merge(more, info)
	delete(out, "retryable")
	return views, out, AssetReady, ""
}

// RetryAsset inspects again a file whose inspection failed.
func (s *Service) RetryAsset(ctx context.Context, tx *gorm.DB, name, ref string, jwtUser *users.User) (*Asset, *gz.ErrMsg) {
	p, a, em := s.GetAsset(tx, name, ref, jwtUser)
	if em != nil {
		return nil, em
	}
	if !s.canWrite(p, jwtUser) {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}
	if a.Status != AssetFailed {
		return a, nil
	}
	data, em := ReadAssetView(p, a, "original")
	if em != nil {
		return nil, em
	}
	views, info, status, errMsg := inspect(ctx, *jwtUser.Username, a.Kind, a.Mime, a.Info, data)
	if views == nil {
		// Measuring a model makes no pictures: keep its thumbnail.
		views = map[string][]byte{}
		for _, v := range splitViews(a.Views) {
			if b, err := os.ReadFile(filepath.Join(assetDir(p, a.ID), v)); err == nil {
				views[v] = b
			}
		}
	}
	dir := assetDir(p, a.ID)
	for v, b := range views {
		if err := os.WriteFile(filepath.Join(dir, v), b, 0600); err != nil {
			return nil, gz.NewErrorMessageWithBase(gz.ErrorCreatingFile, err)
		}
	}
	var names []string
	for v := range views {
		names = append(names, v)
	}
	sort.Strings(names)
	if err := tx.Model(a).Updates(map[string]any{"views": strings.Join(names, ","), "info": info,
		"status": status, "error": errMsg}).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}
	a.Views, a.ViewList, a.Info, a.Status, a.Error = strings.Join(names, ","), splitViews(strings.Join(names, ",")),
		info, status, AssetError(errMsg)
	return a, nil
}

func isDuplicate(err error) bool { return strings.Contains(err.Error(), "Duplicate entry") }

// uniqueFoldedName is uniqueName with names compared ignoring case.
func uniqueFoldedName(name string, takenLower map[string]bool) string {
	if !takenLower[strings.ToLower(name)] {
		return name
	}
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if i := strings.LastIndex(stem, "-"); i > 0 {
		if _, err := strconv.Atoi(stem[i+1:]); err == nil {
			stem = stem[:i]
		}
	}
	for n := 2; ; n++ {
		c := fmt.Sprintf("%s-%d%s", stem, n, ext)
		if !takenLower[strings.ToLower(c)] {
			return c
		}
	}
}

var ownerLocks sync.Map // owner -> *sync.Mutex

// lockOwner serializes uploads of one owner; call the returned unlock.
func lockOwner(owner string) func() {
	m, _ := ownerLocks.LoadOrStore(owner, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func merge(a, b AssetInfo) AssetInfo {
	out := AssetInfo{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func writeAssetFiles(dir string, original []byte, views map[string][]byte) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "original"), original, 0600); err != nil {
		return err
	}
	for name, b := range views {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			return err
		}
	}
	return nil
}

// ListAssets returns a project's files, newest first.
func (s *Service) ListAssets(tx *gorm.DB, name string, jwtUser *users.User) ([]Asset, *gz.ErrMsg) {
	p, em := s.GetForCaller(tx, name, jwtUser)
	if em != nil {
		return nil, em
	}
	list := []Asset{}
	if err := tx.Where("project_id = ?", p.ID).Order("created_at desc, name").Find(&list).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNoDatabase, err)
	}
	return list, nil
}

// ErrAssetNotFound means no file has that id or name in the project.
var ErrAssetNotFound = errors.New("no such project file")

// GetAsset finds a project file by id or name.
func (s *Service) GetAsset(tx *gorm.DB, name, ref string, jwtUser *users.User) (*Project, *Asset, *gz.ErrMsg) {
	p, em := s.GetForCaller(tx, name, jwtUser)
	if em != nil {
		return nil, nil, em
	}
	a, em := findAsset(tx, p, ref)
	return p, a, em
}

func findAsset(tx *gorm.DB, p *Project, ref string) (*Asset, *gz.ErrMsg) {
	var a Asset
	q := tx.Where("project_id = ?", p.ID)
	if assetIDRe.MatchString(ref) {
		q = q.Where("id = ? OR name = ?", ref, ref)
	} else {
		q = q.Where("name = ?", ref)
	}
	if err := q.First(&a).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, gz.NewErrorMessageWithBase(gz.ErrorNameNotFound, fmt.Errorf("%w: %s", ErrAssetNotFound, ref))
		}
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNoDatabase, err)
	}
	return &a, nil
}

// AssetFile is the path of a project file's original ("original") or one
// of its views.
func (s *Service) AssetFile(tx *gorm.DB, name, ref, file string, jwtUser *users.User) (*Asset, string, *gz.ErrMsg) {
	p, a, em := s.GetAsset(tx, name, ref, jwtUser)
	if em != nil {
		return nil, "", em
	}
	return assetFilePath(p, a, file)
}

func assetFilePath(p *Project, a *Asset, file string) (*Asset, string, *gz.ErrMsg) {
	if file != "original" && !a.HasView(file) {
		return nil, "", gz.NewErrorMessageWithBase(gz.ErrorNameNotFound,
			fmt.Errorf("%s has no %q; it has original and %s", a.Name, file, strings.Join(splitViews(a.Views), ", ")))
	}
	return a, filepath.Join(assetDir(p, a.ID), file), nil
}

// RenameAsset renames a project file.
func (s *Service) RenameAsset(tx *gorm.DB, name, ref, newName string, jwtUser *users.User) (*Asset, *gz.ErrMsg) {
	p, a, em := s.GetAsset(tx, name, ref, jwtUser)
	if em != nil {
		return nil, em
	}
	if !s.canWrite(p, jwtUser) {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}
	clean := cleanAssetName(newName)
	if strings.TrimSpace(newName) == "" || clean != strings.TrimSpace(newName) {
		return nil, assetErr(gz.ErrorFormInvalidValue, "invalid name %q (no slashes or control characters, "+
			"at most %d characters)", newName, maxAssetNameLen)
	}
	if clean == a.Name {
		return a, nil
	}
	var n int
	if err := tx.Model(&Asset{}).Where("project_id = ? AND name = ? AND id <> ?", p.ID, clean, a.ID).
		Count(&n).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNoDatabase, err)
	}
	if n > 0 {
		return nil, assetErr(gz.ErrorResourceExists, "a file named %q already exists", clean)
	}
	if err := tx.Model(a).Update("name", clean).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}
	a.Name = clean
	return a, nil
}

// DeleteAsset removes a project file and its views.
func (s *Service) DeleteAsset(tx *gorm.DB, name, ref string, jwtUser *users.User) *gz.ErrMsg {
	p, a, em := s.GetAsset(tx, name, ref, jwtUser)
	if em != nil {
		return em
	}
	if !s.canWrite(p, jwtUser) {
		return gz.NewErrorMessage(gz.ErrorUnauthorized)
	}
	if err := tx.Delete(a).Error; err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}
	if err := os.RemoveAll(assetDir(p, a.ID)); err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}
	return nil
}

// deleteProjectAssets removes every file of a project being deleted. The
// repo is kept for recovery, but uploads are the user's private material
// and go with the project.
func deleteProjectAssets(tx *gorm.DB, p *Project) error {
	if err := tx.Where("project_id = ?", p.ID).Delete(&Asset{}).Error; err != nil {
		return err
	}
	return os.RemoveAll(assetsDir(p))
}

// ReadAssetView reads an asset's original or view by its file name.
func ReadAssetView(p *Project, a *Asset, file string) ([]byte, *gz.ErrMsg) {
	_, fp, em := assetFilePath(p, a, file)
	if em != nil {
		return nil, em
	}
	b, err := os.ReadFile(fp)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}
	return b, nil
}
