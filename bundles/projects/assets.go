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
	"strings"
	"time"
	"unicode"

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

// Value stores AssetInfo as JSON.
func (i AssetInfo) Value() (driver.Value, error) {
	if i == nil {
		return "{}", nil
	}
	b, err := json.Marshal(i)
	return string(b), err
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
	Name   string    `gorm:"type:varchar(191);unique_index:idx_asset_project_name" json:"name"`
	Kind   string    `gorm:"type:varchar(16)" json:"kind"`
	Mime   string    `gorm:"type:varchar(64)" json:"mime"`
	Size   int64     `json:"size"`
	SHA256 string    `gorm:"column:sha256;type:char(64)" json:"sha256"`
	Status string    `gorm:"type:varchar(16)" json:"status"`
	Error  string    `gorm:"type:text" json:"error,omitempty"`
	Info   AssetInfo `gorm:"type:mediumtext" json:"info"`
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
type AssetInspector func(ctx context.Context, kind, file, main string, data []byte) (AssetInfo, map[string][]byte, error)

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

// AssetUpload is one file to add.
type AssetUpload struct {
	Name string
	Data []byte
	// Kind optionally overrides the detected kind; only "texture" is
	// accepted, for an image meant as a material.
	Kind string
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
		if unicode.IsControl(r) || r == '/' {
			return -1
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

// AddAssets checks and stores uploads in a project the caller can write.
// Either every file is added or none is.
func (s *Service) AddAssets(ctx context.Context, tx *gorm.DB, name string, jwtUser *users.User,
	ups []AssetUpload) ([]Asset, *gz.ErrMsg) {
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
	var existing []Asset
	if err := tx.Where("project_id = ?", p.ID).Find(&existing).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNoDatabase, err)
	}
	if len(existing)+len(ups) > MaxProjectAssets {
		return nil, assetErr(gz.ErrorFormInvalidValue, "a project holds at most %d files; it has %d",
			MaxProjectAssets, len(existing))
	}
	taken := map[string]bool{}
	var total int64
	for _, a := range existing {
		taken[a.Name] = true
		total += a.Size
	}

	type prepared struct {
		asset Asset
		data  []byte
		views map[string][]byte
	}
	var out []prepared
	for _, up := range ups {
		fname := cleanAssetName(up.Name)
		if len(up.Data) > MaxAssetSize {
			return nil, assetErr(gz.ErrorFormInvalidValue, "%s: larger than %d MB", fname, MaxAssetSize>>20)
		}
		if total += int64(len(up.Data)); total > MaxProjectAssetsSize {
			return nil, assetErr(gz.ErrorFormInvalidValue, "a project's files may total at most %d MB",
				MaxProjectAssetsSize>>20)
		}
		kind, mime, info, err := sniffAsset(up.Data)
		if err != nil {
			return nil, assetErr(gz.ErrorFormInvalidValue, "%s: %v", fname, err)
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
		if strings.HasPrefix(mime, "image/") && mime != "image/svg+xml" {
			var more AssetInfo
			data, views, more, err = processImage(up.Data, mime)
			if err != nil {
				return nil, assetErr(gz.ErrorFormInvalidValue, "%s: %v", fname, err)
			}
			info = merge(info, more)
		}
		status, errMsg := AssetReady, ""
		if views == nil && InspectAsset != nil {
			// Documents, drawings, meshes and texture sets get their views
			// from the mesh worker. If that fails the file is still kept:
			// the AI then sees its details only.
			main, _ := info["main"].(string)
			ictx, cancel := context.WithTimeout(ctx, inspectTimeout)
			more, v, err := InspectAsset(ictx, kind, "original"+mimeExts[mime], main, data)
			cancel()
			if err != nil {
				status, errMsg = AssetFailed, err.Error()
			} else {
				views, info = v, merge(info, more)
			}
		}
		fname = uniqueName(fname, taken)
		taken[fname] = true
		sum := sha256.Sum256(data)
		a := Asset{
			ID: newAssetID(), ProjectID: p.ID, Name: fname, Kind: kind, Mime: mime,
			Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]),
			Status: status, Error: errMsg, Info: merge(info, nil), CreatedBy: *jwtUser.Username,
		}
		var names []string
		for v := range views {
			names = append(names, v)
		}
		sort.Strings(names)
		a.Views = strings.Join(names, ",")
		out = append(out, prepared{a, data, views})
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
		if err := tx.Create(&a).Error; err != nil {
			_ = os.RemoveAll(dir)
			cleanup()
			return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
		}
		a.ViewList = splitViews(a.Views)
		added = append(added, a)
	}
	return added, nil
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
	tx.Model(&Asset{}).Where("project_id = ? AND name = ?", p.ID, clean).Count(&n)
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
