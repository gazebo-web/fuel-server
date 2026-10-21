package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gazebo-web/fuel-server/aitools"
	"github.com/gazebo-web/fuel-server/bundles/projects"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/mcp"
)

func testJPEG(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for x := 0; x < 32; x++ {
		for y := 0; y < 32; y++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	return buf.Bytes()
}

func zipFiles(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, b := range files {
		w, err := zw.Create(n)
		require.NoError(t, err)
		w.Write(b)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func tarFiles(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for n, c := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: int64(len(c))}))
		tw.Write([]byte(c))
	}
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

// importedModel is a worker import result for model name.
func importedModel(t *testing.T, name string) []byte {
	t.Helper()
	return tarFiles(t, map[string]string{
		"report.json": `{"ok": true, "triangles": 1200, "bounding_box_m": [0.5, 0.5, 0.9], "warnings": [],
			"import": {"up_axis": "y", "scale": 0.001, "how": "height 0.9 m"}}`,
		"preview.png": "\x89PNG fake",
		"models/" + name + "/model.sdf": fmt.Sprintf(`<?xml version="1.0"?>
<sdf version="1.9"><model name="%s"><static>false</static><link name="link">
<inertial><mass>5</mass><inertia><ixx>0.4</ixx><ixy>0</ixy><ixz>0</ixz><iyy>0.4</iyy><iyz>0</iyz><izz>0.2</izz></inertia></inertial>
<visual name="visual"><geometry><mesh><uri>meshes/%s.glb</uri></mesh></geometry></visual>
<collision name="c"><geometry><box><size>0.5 0.5 0.9</size></box></geometry></collision>
</link></model></sdf>
`, name, name),
		"models/" + name + "/model.config": fmt.Sprintf(`<?xml version="1.0"?>
<model><name>%s</name><version>1.0</version><sdf version="1.9">model.sdf</sdf><author><name>AI Builder</name></author><description>x</description></model>
`, name),
		"models/" + name + "/meshes/" + name + ".glb": "glTF visual",
		"models/" + name + "/source/import.json":      `{"imported_from": "chair.zip"}`,
	})
}

// model_import sends an uploaded mesh to the worker and commits the
// model; model_generate sends the uploaded textures its code names.
func TestAIToolsModelImport(t *testing.T) {
	jwt := createValidJWTForIdentity("aitools-import-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)
	user, em := getUserFromDb(username, t)
	require.Nil(t, em)
	name := fmt.Sprintf("import-%d", time.Now().UnixNano())
	mcpJSON(t, jwt, "project_create", map[string]any{"name": name})

	var imports []map[string][]byte
	var jobs []map[string]any
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/x-tar")
		switch r.URL.Path {
		case "/v1/inspect":
			info := `{}`
			if r.URL.Query().Get("kind") == "texture" {
				info = `{"maps": {"color": "brick_color.jpg", "roughness": "brick_roughness.jpg"}}`
			}
			w.Write(tarFiles(t, map[string]string{"report.json": `{"ok": true, "info": ` + info + `}`, "thumb.jpg": "T"}))
		case "/v1/import":
			zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
			require.NoError(t, err)
			files := map[string][]byte{}
			for _, f := range zr.File {
				rc, _ := f.Open()
				files[f.Name], _ = io.ReadAll(rc)
				rc.Close()
			}
			imports = append(imports, files)
			var job map[string]any
			require.NoError(t, json.Unmarshal(files["job.json"], &job))
			w.Write(importedModel(t, job["name"].(string)))
		case "/v1/jobs":
			var job map[string]any
			require.NoError(t, json.Unmarshal(body, &job))
			jobs = append(jobs, job)
			w.Write(generatedModel(t, "wall"))
		}
	}))
	defer worker.Close()
	defer func(f projects.AssetInspector) { projects.InspectAsset = f }(projects.InspectAsset)
	reg := aitools.NewRegistry(aitools.Config{MeshWorkerURL: worker.URL})
	tc := &mcp.ToolContext{Ctx: context.Background(), DB: globals.Server.Db, User: user}
	call := func(tool string, args map[string]any) mcp.CallToolResult {
		t.Helper()
		h, ok := reg.Get(tool)
		require.True(t, ok, tool)
		r, err := h.Handler(tc, args)
		require.NoError(t, err)
		return r
	}

	obj := []byte("v 0 0 0\nv 1 0 0\nv 0 1 0\nf 1 2 3\n")
	rec := uploadAssets(t, jwt, name, map[string][]byte{
		"chair.zip": zipFiles(t, map[string][]byte{"chair/chair.obj": obj, "chair/wood.png": assetTestPNG(t, 4, 4)}),
		"brick.zip": zipFiles(t, map[string][]byte{"brick_color.jpg": testJPEG(t, color.RGBA{160, 50, 30, 255}),
			"brick_roughness.jpg": testJPEG(t, color.Gray{200})}),
		"photo.png": assetTestPNG(t, 8, 8),
		"box.stl":   []byte("solid b\n facet normal 0 0 1\n"),
	}, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	r := call("model_import", map[string]any{"name": name, "asset": "chair.zip", "model": "chair", "height": 0.9, "static": true})
	require.False(t, r.IsError, r.Content[0].Text)
	require.Len(t, imports, 1)
	var job map[string]any
	require.NoError(t, json.Unmarshal(imports[0]["job.json"], &job))
	assert.Equal(t, "input/chair/chair.obj", job["path"])
	assert.EqualValues(t, 0.9, job["height"])
	assert.Equal(t, true, job["static"])
	assert.Equal(t, "chair.zip", job["source_name"])
	assert.Equal(t, obj, imports[0]["input/chair/chair.obj"])
	assert.Contains(t, imports[0], "input/chair/wood.png")
	assert.Contains(t, r.Content[0].Text, `"how": "height 0.9 m"`)
	require.Len(t, r.Content, 2, "with the preview")
	listed := mcpJSON(t, jwt, "project_list_files", map[string]any{"name": name})
	assert.Contains(t, fmt.Sprint(listed["files"]), "models/chair/source/import.json")

	// A single-file mesh goes in as input/original.<ext>; importing again
	// replaces the model.
	r = call("model_import", map[string]any{"name": name, "asset": "box.stl", "model": "chair", "units": "mm"})
	require.False(t, r.IsError, r.Content[0].Text)
	require.NoError(t, json.Unmarshal(imports[1]["job.json"], &job))
	assert.Equal(t, "input/original.stl", job["path"])
	assert.Equal(t, "mm", job["units"])

	r = call("model_import", map[string]any{"name": name, "asset": "photo.png", "model": "photo"})
	assert.True(t, r.IsError)
	assert.Contains(t, r.Content[0].Text, "not a mesh")
	mcpJSON(t, jwt, "model_create", map[string]any{"name": name, "model": "shelf"})
	r = call("model_import", map[string]any{"name": name, "asset": "box.stl", "model": "shelf"})
	assert.True(t, r.IsError)
	assert.Contains(t, r.Content[0].Text, "wasn't imported")

	// Generated models get the uploaded textures their code names.
	code := `def build(): return gz.part(Box(1, 1, 1), "w", gz.texture("project:brick.zip", scale=2))`
	r = call("model_generate", map[string]any{"name": name, "model": "wall", "code": code})
	require.False(t, r.IsError, r.Content[0].Text)
	require.Len(t, jobs, 1)
	tex := jobs[0]["project_textures"].(map[string]any)["brick.zip"].(map[string]any)
	for _, k := range []string{"color", "roughness"} {
		b, err := base64.StdEncoding.DecodeString(tex[k].(string))
		require.NoError(t, err)
		_, err = jpeg.Decode(bytes.NewReader(b))
		assert.NoError(t, err, k)
	}
	r = call("model_generate", map[string]any{"name": name, "model": "wall",
		"code": `gz.texture('project:missing.zip')`})
	assert.True(t, r.IsError)
	assert.Contains(t, r.Content[0].Text, "no file named")
	assert.Len(t, jobs, 1, "nothing sent")

	r = call("mesh_list_materials", map[string]any{"name": name})
	assert.Contains(t, r.Content[0].Text, `gz.texture("project:brick.zip")`)
	assert.Contains(t, r.Content[0].Text, `gz.texture("project:photo.png")`)
	assert.False(t, strings.Contains(r.Content[0].Text, "project:chair.zip"))
}
