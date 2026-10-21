package projects

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/gazebo-web/fuel-server/sdf"
)

// Mixed zips. A zip that isn't one mesh or one texture set is unpacked
// into project files, each checked like an upload of its own:
//   - a folder with model.config is a Gazebo model: it becomes a project
//     model (models/<name>/ in the repo), recorded as a file of kind model;
//   - a mesh goes with the files it uses (an OBJ's material libraries and
//     their textures, a glTF's buffers and images, Collada's images);
//   - a map YAML (Nav2/ROS) and the image it names are one map;
//   - a folder of texture maps (a color map and others) is a texture set;
//   - other images, PDFs and drawings are files of their own;
//   - anything else (notes, nested zips, stray files) is skipped and named.

// AssetModel is a Gazebo model folder from a zip, added to the project as
// models/<name>/ and included as model://<name>.
const AssetModel = "model"

// assetBundle is sniffZip's kind for a zip to unpack.
const assetBundle = "bundle"

type zipFile struct {
	name string // path in the zip
	data []byte
}

// modelFolder is a Gazebo model found in a zip.
type modelFolder struct {
	dir   string            // its folder in the zip ("" at the root)
	files map[string][]byte // path relative to dir -> content
}

// readZip returns a zip's regular files, without macOS leftovers and
// dotfiles. Names were checked by sniffZip.
func readZip(data []byte) ([]zipFile, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	var out []zipFile
	var total int64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || strings.HasPrefix(f.Name, "__MACOSX/") || strings.HasPrefix(path.Base(f.Name), ".") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		// Sizes are counted as read, not taken from the headers.
		b, err := io.ReadAll(io.LimitReader(rc, MaxAssetZipUnpacked-total+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %v", f.Name, err)
		}
		if total += int64(len(b)); total > MaxAssetZipUnpacked {
			return nil, fmt.Errorf("the zip unpacks to more than %d MB", MaxAssetZipUnpacked>>20)
		}
		out = append(out, zipFile{f.Name, b})
	}
	return out, nil
}

// expandBundle splits a mixed zip into uploads and model folders, and
// names the files it skipped.
func expandBundle(zipName string, data []byte) ([]AssetUpload, []modelFolder, []string, error) {
	files, err := readZip(data)
	if err != nil {
		return nil, nil, nil, err
	}
	byName := map[string][]byte{}
	for _, f := range files {
		byName[f.name] = f.data
	}
	claimed := map[string]bool{}

	// Gazebo models: the shallowest folders holding model.config.
	var modelDirs []string
	for _, f := range files {
		if path.Base(f.name) == "model.config" {
			modelDirs = append(modelDirs, path.Dir(f.name))
		}
	}
	sort.Slice(modelDirs, func(i, j int) bool {
		return strings.Count(modelDirs[i], "/") < strings.Count(modelDirs[j], "/")
	})
	var models []modelFolder
	for _, dir := range modelDirs {
		if claimed[path.Join(dir, "model.config")] {
			continue // inside a model already taken
		}
		m := modelFolder{dir: dir, files: map[string][]byte{}}
		for _, f := range files {
			if rel, ok := under(dir, f.name); ok && !claimed[f.name] {
				m.files[rel] = f.data
				claimed[f.name] = true
			}
		}
		models = append(models, m)
	}

	var ups []AssetUpload
	from := func(name string) AssetInfo { return AssetInfo{"from_zip": zipName, "path": name} }
	// Maps: a YAML and the image it names, as SLAM writes them.
	for _, f := range files {
		if claimed[f.name] || !isYAML(f.name) {
			continue
		}
		m, ok := parseNavMap(f.data)
		if !ok {
			continue
		}
		img, ok := mapImage(f.name, m, func(n string) bool { return byName[n] != nil && !claimed[n] })
		if !ok {
			continue
		}
		b, err := mapPair(f.name, f.data, img, byName[img])
		if err != nil {
			return nil, nil, nil, err
		}
		claimed[f.name], claimed[img] = true, true
		ups = append(ups, AssetUpload{Name: path.Base(f.name), Data: b, Info: from(f.name)})
	}
	// Meshes with the files they use.
	for _, f := range files {
		if claimed[f.name] || !meshExts[strings.ToLower(path.Ext(f.name))] {
			continue
		}
		refs := meshRefs(f.name, f.data, byName)
		claimed[f.name] = true
		up := AssetUpload{Name: path.Base(f.name), Data: f.data, Info: from(f.name)}
		if len(refs) > 0 {
			group := map[string][]byte{f.name: f.data}
			for _, r := range refs {
				group[r] = byName[r]
				claimed[r] = true
			}
			b, err := zipOf(group)
			if err != nil {
				return nil, nil, nil, err
			}
			up.Data = b
		}
		ups = append(ups, up)
	}
	// Texture sets: a folder of unclaimed images with a color map and
	// another map.
	dirImages := map[string][]string{}
	for _, f := range files {
		if !claimed[f.name] && imageExts[strings.ToLower(path.Ext(f.name))] {
			dirImages[path.Dir(f.name)] = append(dirImages[path.Dir(f.name)], f.name)
		}
	}
	var dirs []string
	for d := range dirImages {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		imgs := dirImages[d]
		if len(imgs) < 2 || !hasMap(imgs, colorMapRe) || !hasMap(imgs, otherMapRe) {
			continue
		}
		group := map[string][]byte{}
		for _, n := range imgs {
			group[path.Base(n)] = byName[n]
			claimed[n] = true
		}
		b, err := zipOf(group)
		if err != nil {
			return nil, nil, nil, err
		}
		name := path.Base(d)
		if d == "." {
			name = strings.TrimSuffix(path.Base(zipName), path.Ext(zipName))
		}
		ups = append(ups, AssetUpload{Name: name + " textures", Data: b, Info: from(d)})
	}
	// Everything else that can be a file of its own.
	var skipped []string
	for _, f := range files {
		if claimed[f.name] {
			continue
		}
		switch strings.ToLower(path.Ext(f.name)) {
		case ".png", ".jpg", ".jpeg", ".webp", ".pgm", ".pdf", ".svg", ".dxf":
			ups = append(ups, AssetUpload{Name: path.Base(f.name), Data: f.data, Info: from(f.name)})
		default:
			skipped = append(skipped, f.name)
		}
	}
	return ups, models, skipped, nil
}

// under returns name relative to dir, if it is inside it.
func under(dir, name string) (string, bool) {
	if dir == "." {
		return name, true
	}
	rel, ok := strings.CutPrefix(name, dir+"/")
	return rel, ok
}

var (
	colorMapRe = regexp.MustCompile(`(?i)(color|colour|albedo|basecolor|diffuse|diff|_col)`)
	otherMapRe = regexp.MustCompile(`(?i)(normal|nrm|_nor|rough|metal|_ao|occlusion|height|disp)`)
	mtllibRe   = regexp.MustCompile(`(?m)^\s*mtllib\s+(.+?)\s*$`)
	initFromRe = regexp.MustCompile(`<init_from>\s*([^<\s]+)\s*</init_from>`)
)

func hasMap(names []string, re *regexp.Regexp) bool {
	for _, n := range names {
		if re.MatchString(path.Base(n)) {
			return true
		}
	}
	return false
}

// meshRefs are the files in the zip that a mesh refers to: an OBJ's
// material libraries and their textures, a glTF's buffers and images, a
// Collada file's images. Paths resolve from the mesh's folder.
func meshRefs(name string, data []byte, files map[string][]byte) []string {
	dir := path.Dir(name)
	seen := map[string]bool{}
	var out []string
	add := func(from, ref string) string {
		ref = strings.ReplaceAll(strings.TrimSpace(ref), "\\", "/")
		ref = strings.TrimPrefix(ref, "file://")
		if ref == "" || strings.Contains(ref, "://") || strings.HasPrefix(ref, "data:") {
			return ""
		}
		p := path.Clean(path.Join(from, ref))
		if _, ok := files[p]; !ok {
			// Exporters often keep absolute paths: try the file name.
			p = path.Join(from, path.Base(ref))
			if _, ok := files[p]; !ok {
				return ""
			}
		}
		if p != name && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
		return p
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".obj":
		for _, m := range mtllibRe.FindAllSubmatch(data, -1) {
			for _, lib := range strings.Fields(string(m[1])) {
				if p := add(dir, lib); p != "" {
					for _, tex := range mtlTextures(files[p]) {
						add(path.Dir(p), tex)
					}
				}
			}
		}
	case ".gltf":
		var g struct {
			Buffers []struct{ URI string } `json:"buffers"`
			Images  []struct{ URI string } `json:"images"`
		}
		if json.Unmarshal(data, &g) == nil {
			for _, b := range g.Buffers {
				add(dir, b.URI)
			}
			for _, i := range g.Images {
				add(dir, i.URI)
			}
		}
	case ".dae":
		for _, m := range initFromRe.FindAllSubmatch(data, -1) {
			add(dir, string(m[1]))
		}
	}
	sort.Strings(out)
	return out
}

// mtlTextures are the texture files a material library names: the last
// word of its map_*, bump, disp, decal and refl lines (options come first).
func mtlTextures(mtl []byte) []string {
	var out []string
	for _, line := range strings.Split(string(mtl), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		k := strings.ToLower(f[0])
		if strings.HasPrefix(k, "map_") || k == "bump" || k == "disp" || k == "decal" || k == "refl" || k == "norm" {
			out = append(out, f[len(f)-1])
		}
	}
	return out
}

func zipOf(files map[string][]byte) ([]byte, error) {
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(files[n]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// modelNameRe is a project model's folder name (aitools' rule).
var modelNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// modelName makes a folder's name a valid model name that isn't taken.
func modelName(dir string, taken map[string]bool) string {
	base := path.Base(dir)
	if dir == "." || base == "" {
		base = "model"
	}
	name := strings.Map(func(r rune) rune {
		if r < 128 && (r == '_' || r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return r
		}
		return '_'
	}, base)
	if len(name) > 60 {
		name = name[:60]
	}
	if !modelNameRe.MatchString(name) {
		name = "model"
	}
	out := name
	for n := 2; taken[strings.ToLower(out)]; n++ {
		out = fmt.Sprintf("%s_%d", name, n)
	}
	taken[strings.ToLower(out)] = true
	return out
}

// rewriteModelURIs makes an uploaded model's links to its own files work
// in the project, where it lives at models/<newName>/:
//   - model://<oldName>/... of a renamed model becomes model://<newName>/...;
//   - file:// paths written for another layout (AWS's models use
//     file://models/<name>/meshes/x.DAE, relative to their repo) become
//     model://<newName>/<path>, when the end of the path is a file of the
//     model.
func rewriteModelURIs(m modelFolder, oldName, newName string) {
	for rel, b := range m.files {
		if !strings.HasSuffix(strings.ToLower(rel), ".sdf") {
			continue
		}
		b = uriRe.ReplaceAllFunc(b, func(tag []byte) []byte {
			sub := uriRe.FindSubmatch(tag)
			uri := strings.TrimSpace(string(sub[2]))
			var p string
			switch {
			case oldName != newName && oldName != "." && strings.HasPrefix(uri, "model://"+oldName+"/"):
				p = strings.TrimPrefix(uri, "model://"+oldName+"/")
			case strings.HasPrefix(uri, "file://"):
				p = ownFile(m, strings.TrimPrefix(uri, "file://"))
			default:
				return tag
			}
			if p == "" {
				return tag
			}
			return []byte(string(sub[1]) + "model://" + newName + "/" + p + string(sub[3]))
		})
		m.files[rel] = b
	}
}

var uriRe = regexp.MustCompile(`(<uri>)([^<]*)(</uri>)`)

// ownFile finds the model file a foreign path ends with: the longest
// trailing part of it that names one of the model's files.
func ownFile(m modelFolder, p string) string {
	parts := strings.Split(strings.Trim(path.Clean("/"+p), "/"), "/")
	for i := range parts {
		if cand := strings.Join(parts[i:], "/"); m.files[cand] != nil {
			return cand
		}
	}
	return ""
}

var configSDFRe = regexp.MustCompile(`<sdf[^>]*>\s*([^<\s]+)\s*</sdf>`)

// modelInfo describes an uploaded model: its name and URI, its files, and
// how its SDF validates (it is kept either way; the AI sees the result).
func modelInfo(name string, m modelFolder) AssetInfo {
	info := AssetInfo{"model": name, "uri": "model://" + name, "files": len(m.files)}
	sdfFile := "model.sdf"
	if all := configSDFRe.FindAllSubmatch(m.files["model.config"], -1); len(all) > 0 {
		sdfFile = string(all[len(all)-1][1]) // the newest version is listed last
	}
	src, ok := m.files[sdfFile]
	if !ok {
		info["validation"] = map[string]any{"errors": 1, "first": "model.config names " + sdfFile + ", which is missing"}
		return info
	}
	r := sdf.Validate(string(src))
	v := map[string]any{"sdf": sdfFile, "errors": len(r.Errors), "warnings": len(r.Warnings)}
	if len(r.Errors) > 0 {
		v["first"] = r.Errors[0].Message
	}
	info["validation"] = v
	return info
}

// modelThumbnail makes the views of an uploaded model from its first
// thumbnail image, if it has one (Fuel models keep them in thumbnails/).
func modelThumbnail(m modelFolder) map[string][]byte {
	var names []string
	for rel := range m.files {
		if strings.HasPrefix(rel, "thumbnails/") && imageExts[strings.ToLower(path.Ext(rel))] {
			names = append(names, rel)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		_, mime, _, err := sniffAsset(m.files[n])
		if err != nil || !rasterMimes[mime] {
			continue
		}
		if _, views, _, err := processImage(m.files[n], mime); err == nil {
			return views
		}
	}
	return nil
}
