package aitools

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/image/draw"

	"github.com/gazebo-web/fuel-server/bundles/projects"
	"github.com/gazebo-web/fuel-server/mcp"
)

// Uploaded meshes and textures in models (plan C4): model_import turns a
// mesh the user uploaded into models/<model>/ on the mesh worker (scaled,
// Z up, origin at the bottom center, simplified, with collision and
// inertia), and model_generate code can paint parts with an uploaded
// texture set as gz.texture("project:<name>").

func (t *tools) registerImport(reg *mcp.Registry) {
	num := func(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
	reg.Register(mcp.Tool{
		Spec: mcp.ToolSpec{
			Name: "model_import",
			Description: "Turn a mesh the user uploaded (asset_list: kind mesh) into a project model: the " +
				"server converts it to GLB, scales it to meters, turns it Z up with its origin at the bottom " +
				"center, simplifies it to tri_budget, and adds collision and inertia, keeping its materials " +
				"and textures. Look at it with asset_view first: its info has the size in file units and " +
				"guesses at the units and up axis. Give height when you know the real size (e.g. a chair is " +
				"about 0.9 m tall). Returns a report and a preview image: check the size and that it stands " +
				"upright, then call again with the same model to fix it. Include it with " +
				"<include><uri>model://<model></uri></include>.",
			InputSchema: schema([]string{"name", "asset", "model"}, map[string]any{
				"name":        str("Project name"),
				"asset":       str("The uploaded mesh's id or name"),
				"model":       str("Model name: lowercase letters, digits and '_', starting with a letter"),
				"height":      num("Real height in meters after turning Z up; scales the mesh to it"),
				"scale":       num("Or: a scale factor from file units to meters"),
				"units":       map[string]any{"type": "string", "enum": []string{"auto", "mm", "cm", "m", "in", "ft"}, "description": "Or: the file's units (default auto: guessed from its size)"},
				"up_axis":     map[string]any{"type": "string", "enum": []string{"auto", "x", "y", "z"}, "description": "The file's up axis (default auto: Y for glTF and OBJ, Collada's own, Z for STL)"},
				"static":      map[string]any{"type": "boolean", "description": "Immovable (shelves, buildings). Default false"},
				"mass":        num("Mass in kg (or density)"),
				"density":     num("Density in kg/m³ (default 1000 when no mass is given)"),
				"collision":   map[string]any{"type": "string", "enum": []string{"auto", "primitive", "convex"}, "description": "auto (default): a primitive when it fits, else one hull, else a convex decomposition"},
				"tri_budget":  num("Max triangles (default 50000, at most 200000)"),
				"description": str("What the model is (model.config)"),
				"base_rev":    str("Optional: fail if the project changed since this revision"),
			}),
		},
		RequiredScopes: []string{"project:write"},
		Handler:        t.modelImport,
	})
}

func (t *tools) modelImport(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	v, errRes := requireStrings(args, "name", "asset", "model")
	if errRes != nil {
		return *errRes, nil
	}
	project, ref, model := v[0], v[1], v[2]
	if !generatedModelRe.MatchString(model) {
		return mcp.ErrorResult("model name must be lowercase letters, digits and '_', starting with a letter"), nil
	}
	user := *tc.User.Username
	p, a, em := projectService.GetAsset(tc.DB, project, ref, tc.User)
	if em != nil {
		return gzError(em), nil
	}
	if a.Kind != projects.AssetMesh {
		return mcp.ErrorResult(fmt.Sprintf("%s is a %s, not a mesh; model_import takes GLB, OBJ, STL or DAE uploads",
			a.Name, a.Kind)), nil
	}
	if wait, ok := t.mesh.limit.allow(user, time.Now()); !ok {
		return mcp.ErrorResult(fmt.Sprintf("mesh job rate limit (%d per minute, %d per day); try again in %s",
			meshPerMinute, meshPerDay, wait.Round(time.Second))), nil
	}
	dir := path.Join(modelsDir, model)
	files, _, em := projectService.ListFiles(tc.Ctx, tc.DB, user, project, "", tc.User)
	if em != nil {
		return gzError(em), nil
	}
	if newProjectFiles("", files).hasModel(dir) && !contains(files, dir+"/source/import.json") {
		return mcp.ErrorResult(fmt.Sprintf("%s exists and wasn't imported; choose another model name", dir)), nil
	}
	original, em := projects.ReadAssetView(p, a, "original")
	if em != nil {
		return gzError(em), nil
	}

	sdfVersion := defaultSDFVersion
	if p.SDFVersion != nil && *p.SDFVersion != "" {
		sdfVersion = *p.SDFVersion
	}
	job := map[string]any{"name": model, "sdf_version": sdfVersion, "source_name": a.Name}
	for _, k := range []string{"units", "up_axis", "collision", "description"} {
		if s := argString(args, k); s != "" {
			job[k] = s
		}
	}
	for _, k := range []string{"height", "scale", "mass", "density"} {
		if x, ok := args[k].(float64); ok {
			job[k] = x
		}
	}
	if x, ok := args["tri_budget"].(float64); ok {
		job["tri_budget"] = int(x)
	}
	if s, ok := args["static"].(bool); ok {
		job["static"] = s
	}
	body, err := importZip(job, a, original)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}
	tarball, jobID, err := t.mesh.post(tc.Ctx, "/v1/import", "application/zip", body)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}
	return t.finishJob(tc, pendingMesh{user: user, project: project, model: model, baseRev: argString(args, "base_rev"),
		retry: retryImport}, tarball, jobID)
}

// importZip is the worker's import job: job.json and the mesh (with the
// other files of a zip upload) under input/.
func importZip(job map[string]any, a *projects.Asset, original []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err == nil {
			_, err = w.Write(data)
		}
		return err
	}
	if a.Mime == "application/zip" {
		main, _ := a.Info["main"].(string)
		job["path"] = "input/" + main
		zr, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
		if err != nil {
			return nil, fmt.Errorf("the uploaded zip can't be read: %w", err)
		}
		for _, f := range zr.File {
			// Names were checked when the zip was uploaded.
			if f.FileInfo().IsDir() || strings.HasPrefix(f.Name, "__MACOSX/") || strings.HasPrefix(path.Base(f.Name), ".") {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(io.LimitReader(rc, projects.MaxAssetZipUnpacked+1))
			rc.Close()
			if err != nil {
				return nil, err
			}
			if err := add("input/"+f.Name, data); err != nil {
				return nil, err
			}
		}
	} else {
		name := "input/original" + projects.AssetExt(a.Mime)
		job["path"] = name
		if err := add(name, original); err != nil {
			return nil, err
		}
	}
	j, _ := json.Marshal(job)
	if err := add("job.json", j); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// -- project textures --------------------------------------------------------

var projectTextureRe = regexp.MustCompile(`texture\(\s*["']project:([^"']+)["']`)

// textureMapSize is the largest side of a project texture's maps sent to
// the worker: as large as the vendored sets.
const textureMapSize = 1024

// projectTextures loads the uploaded textures the code names, as the
// worker's project_textures: {name: {"color", "roughness"?, "metalness"?}}
// with each map a base64 JPEG.
func (t *tools) projectTextures(tc *mcp.ToolContext, project, code string) (map[string]any, *mcp.CallToolResult) {
	out := map[string]any{}
	for _, m := range projectTextureRe.FindAllStringSubmatch(code, -1) {
		name := m[1]
		if _, done := out[name]; done {
			continue
		}
		p, a, em := projectService.GetAsset(tc.DB, project, name, tc.User)
		if em != nil {
			r := mcp.ErrorResult(fmt.Sprintf("texture %q: the project has no file named %q; asset_list shows the "+
				"uploaded textures", "project:"+name, name))
			return nil, &r
		}
		maps, err := textureMaps(p, a)
		if err != nil {
			r := mcp.ErrorResult(fmt.Sprintf("texture %q: %v", "project:"+name, err))
			return nil, &r
		}
		out[name] = maps
	}
	return out, nil
}

// textureMaps reads an uploaded texture: an image is its color map; a
// zip's maps are the ones its inspection named.
func textureMaps(p *projects.Project, a *projects.Asset) (map[string]any, error) {
	original, em := projects.ReadAssetView(p, a, "original")
	if em != nil {
		return nil, fmt.Errorf("%s", em.Msg)
	}
	raw := map[string][]byte{}
	switch {
	case a.Kind == projects.AssetImage || (a.Kind == projects.AssetTexture && strings.HasPrefix(a.Mime, "image/")):
		raw["color"] = original
	case a.Kind == projects.AssetTexture:
		names, _ := a.Info["maps"].(map[string]any)
		if names == nil {
			return nil, fmt.Errorf("%s has no maps found (%s)", a.Name, a.Error)
		}
		zr, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			for kind, file := range names {
				if f.Name == file && (kind == "color" || kind == "roughness" || kind == "metalness") {
					rc, err := f.Open()
					if err != nil {
						return nil, err
					}
					b, err := io.ReadAll(io.LimitReader(rc, projects.MaxAssetSize))
					rc.Close()
					if err != nil {
						return nil, err
					}
					raw[kind] = b
				}
			}
		}
	default:
		return nil, fmt.Errorf("%s is a %s, not a texture or an image", a.Name, a.Kind)
	}
	if raw["color"] == nil {
		return nil, fmt.Errorf("%s has no color map", a.Name)
	}
	out := map[string]any{}
	for kind, b := range raw {
		jpg, err := mapJPEG(b)
		if err != nil {
			return nil, fmt.Errorf("%s %s map: %v", a.Name, kind, err)
		}
		out[kind] = base64.StdEncoding.EncodeToString(jpg)
	}
	return out, nil
}

// mapJPEG decodes a map, scales it to at most textureMapSize and encodes
// it as JPEG.
func mapJPEG(b []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if int64(cfg.Width)*int64(cfg.Height) > projects.MaxAssetPixels {
		return nil, fmt.Errorf("too large (%d×%d)", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if r := img.Bounds(); r.Dx() > textureMapSize || r.Dy() > textureMapSize {
		w, h := textureMapSize, r.Dy()*textureMapSize/r.Dx()
		if r.Dy() > r.Dx() {
			w, h = r.Dx()*textureMapSize/r.Dy(), textureMapSize
		}
		dst := image.NewRGBA(image.Rect(0, 0, max(1, w), max(1, h)))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, r, draw.Src, nil)
		img = dst
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// meshListMaterials is the materials reference, plus the project's
// uploaded textures when a project is named.
func (t *tools) meshListMaterials(tc *mcp.ToolContext, args map[string]any) (mcp.CallToolResult, error) {
	ref, err := t.meshAPIReference(tc, map[string]any{"topic": "materials"})
	project := argString(args, "name")
	if err != nil || project == "" {
		return ref, err
	}
	list, em := projectService.ListAssets(tc.DB, project, tc.User)
	if em != nil {
		return gzError(em), nil
	}
	var names []string
	for _, a := range list {
		if a.Kind == projects.AssetTexture || a.Kind == projects.AssetImage {
			names = append(names, fmt.Sprintf("- gz.texture(\"project:%s\") — %s, uploaded", a.Name, a.Kind))
		}
	}
	sort.Strings(names)
	extra := "\n\n## This project's uploaded textures\n\n"
	if len(names) == 0 {
		extra += "None yet. Images and texture sets the user uploads appear here."
	} else {
		extra += strings.Join(names, "\n") + "\n\nThey tile every 1 m; pass scale to change that."
	}
	ref.Content[0].Text += extra
	return ref, nil
}
