package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gazebo-web/fuel-server/bundles/projects"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Project files over the REST API and the AI tools.

func assetTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, h/2, color.NRGBA{0, 0, 0, 255})
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func uploadAssets(t *testing.T, jwt, project string, files map[string][]byte, kind string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for name, data := range files {
		w, err := mw.CreateFormFile("file", name)
		require.NoError(t, err)
		_, _ = w.Write(data)
	}
	if kind != "" {
		_ = mw.WriteField("kind", kind)
	}
	require.NoError(t, mw.Close())
	req, _ := http.NewRequest("POST", "/1.0/projects/"+project+"/assets", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	globals.Server.Router.ServeHTTP(rec, req)
	return rec
}

func getNoAuth(path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest("GET", path, nil)
	rec := httptest.NewRecorder()
	globals.Server.Router.ServeHTTP(rec, req)
	return rec
}

func TestProjectAssets(t *testing.T) {
	jwt := createValidJWTForIdentity("assets-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)
	name := fmt.Sprintf("assets-%d", time.Now().UnixNano())
	mcpJSON(t, jwt, "project_create", map[string]any{"name": name})

	// A stand-in for the mesh worker's inspect job.
	defer func(f projects.AssetInspector) { projects.InspectAsset = f }(projects.InspectAsset)
	busy := true
	projects.InspectAsset = func(_ context.Context, _, kind, file, _ string, _ []byte) (projects.AssetInfo, map[string][]byte, error) {
		switch {
		case kind == projects.AssetDrawing && file == "original.dxf" && busy:
			return nil, nil, projects.RetryableError{Err: errors.New("the mesh worker is busy")}
		case kind == projects.AssetDrawing && file == "original.dxf":
			return projects.AssetInfo{"units": "meters"}, map[string][]byte{"ai.jpg": []byte("D"), "thumb.jpg": []byte("t")}, nil
		case kind == projects.AssetDocument && file == "original.pdf":
			return projects.AssetInfo{"pages": 2, "pages_shown": 2, "text": "Room 1"},
				map[string][]byte{"ai.jpg": []byte("A"), "thumb.jpg": []byte("T"), "page-1.jpg": []byte("1"), "page-2.jpg": []byte("2")}, nil
		case kind == projects.AssetMesh && file == "original.glb":
			return nil, nil, errors.New("the mesh can't be read: bad header")
		}
		return nil, nil, nil
	}

	plan := assetTestPNG(t, 2000, 1000)
	glb := append([]byte("glTF\x02\x00\x00\x00"), make([]byte, 64)...)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	rec := uploadAssets(t, jwt, name, map[string][]byte{"plans/floorplan.png": plan, "crate.glb": glb, "room.svg": svg}, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res projects.AddResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	added := res.Assets
	require.Len(t, added, 3)
	byName := map[string]projects.Asset{}
	for _, a := range added {
		byName[a.Name] = a
	}
	img := byName["floorplan.png"]
	assert.Equal(t, projects.AssetImage, img.Kind)
	assert.Equal(t, []string{"ai.jpg", "thumb.jpg"}, img.ViewList)
	assert.EqualValues(t, 2000, img.Info["width"])
	assert.Equal(t, projects.AssetMesh, byName["crate.glb"].Kind)
	assert.Equal(t, projects.AssetFailed, byName["crate.glb"].Status, "kept, with the reason")
	assert.Equal(t, "the mesh can't be read: bad header", string(byName["crate.glb"].Error))
	assert.Equal(t, projects.AssetDrawing, byName["room.svg"].Kind)
	assert.Equal(t, projects.AssetReady, byName["room.svg"].Status)

	// A PDF gets its pages from the worker.
	rec = uploadAssets(t, jwt, name, map[string][]byte{"spec.pdf": []byte("%PDF-1.7\n")}, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"views":["ai.jpg","page-1.jpg","page-2.jpg","thumb.jpg"]`)
	content := mcpContent(t, jwt, "asset_view", map[string]any{"name": name, "asset": "spec.pdf", "page": 2})
	_ = content
	require.Len(t, content, 2)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("2")), content[1]["data"])
	assert.Contains(t, content[0]["text"], "Room 1")
	text, isErr := mcpCall(t, jwt, "asset_view", map[string]any{"name": name, "asset": "spec.pdf", "page": 3})
	assert.True(t, isErr, text)

	// A busy worker leaves a file to retry; viewing it retries.
	rec = uploadAssets(t, jwt, name, map[string][]byte{"site.dxf": []byte("  0\nSECTION\n  2\nHEADER\n")}, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"status":"failed"`)
	assert.Contains(t, rec.Body.String(), `"retryable":true`)
	rec = aiRequest(t, "POST", "/1.0/projects/"+name+"/assets/site.dxf/retry", jwt, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"status":"failed"`, "still busy")
	busy = false
	content = mcpContent(t, jwt, "asset_view", map[string]any{"name": name, "asset": "site.dxf"})
	require.Len(t, content, 2, "retried, with its picture")
	assert.Contains(t, content[0]["text"], `"status": "ready"`)
	require.Equal(t, http.StatusOK, aiRequest(t, "DELETE", "/1.0/projects/"+name+"/assets/site.dxf", jwt, nil).Code)

	// Names differing in case are the same name to the database.
	rec = uploadAssets(t, jwt, name, map[string][]byte{"FloorPlan.PNG": plan}, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"name":"FloorPlan-2.PNG"`)
	rec = aiRequest(t, "PATCH", "/1.0/projects/"+name+"/assets/FloorPlan-2.PNG", jwt, map[string]string{"name": "floorplan-2.png"})
	require.Equal(t, http.StatusOK, rec.Code, "a change of case only: %s", rec.Body.String())
	require.Equal(t, http.StatusOK, aiRequest(t, "DELETE", "/1.0/projects/"+name+"/assets/floorplan-2.png", jwt, nil).Code)
	require.Equal(t, http.StatusOK, aiRequest(t, "DELETE", "/1.0/projects/"+name+"/assets/spec.pdf", jwt, nil).Code)

	// A bad file refuses the whole request; a repeated name gets a suffix.
	rec = uploadAssets(t, jwt, name, map[string][]byte{"a.png": plan, "virus.exe": []byte("MZ\x90\x00\x03")}, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "virus.exe")
	rec = uploadAssets(t, jwt, name, map[string][]byte{"floorplan.png": plan}, "texture")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"name":"floorplan-2.png"`)
	assert.Contains(t, rec.Body.String(), `"kind":"texture"`)

	rec = aiRequest(t, "GET", "/1.0/projects/"+name+"/assets", jwt, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list []projects.Asset
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Len(t, list, 4)

	// Files: an image shows inline; an SVG only downloads; nothing runs script.
	rec = aiRequest(t, "GET", "/1.0/projects/"+name+"/assets/"+img.ID+"/original", jwt, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	assert.True(t, strings.HasPrefix(rec.Header().Get("Content-Disposition"), "inline"))
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "sandbox")
	rec = aiRequest(t, "GET", "/1.0/projects/"+name+"/assets/room.svg/original", jwt, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	rec = aiRequest(t, "GET", "/1.0/projects/"+name+"/assets/"+img.ID+"/thumb.jpg", jwt, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "image/jpeg", rec.Header().Get("Content-Type"))
	assert.NotEqual(t, http.StatusOK, aiRequest(t, "GET", "/1.0/projects/"+name+"/assets/"+img.ID+"/../../files/x", jwt, nil).Code)
	assert.Equal(t, http.StatusNotFound, aiRequest(t, "GET", "/1.0/projects/"+name+"/assets/crate.glb/ai.jpg", jwt, nil).Code)

	// Signed URLs serve files without a login, and only files.
	rec = aiRequest(t, "GET", "/1.0/projects/"+name+"/assets-url", jwt, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var u struct {
		BasePath string `json:"base_path"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &u))
	assert.Equal(t, http.StatusOK, getNoAuth(u.BasePath+"/"+img.ID+"/thumb.jpg").Code)
	token := strings.TrimPrefix(u.BasePath, "/1.0/project-assets/")
	assert.Equal(t, http.StatusUnauthorized, getNoAuth("/1.0/project-files/"+token+"/files/.gitignore").Code)
	rec = aiRequest(t, "GET", "/1.0/projects/"+name+"/file-url", jwt, nil)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &u))
	repoToken := strings.TrimPrefix(u.BasePath, "/1.0/project-files/")
	assert.Equal(t, http.StatusUnauthorized, getNoAuth("/1.0/project-assets/"+repoToken+"/"+img.ID+"/thumb.jpg").Code)

	// Another user sees nothing.
	otherJWT := createValidJWTForIdentity("assets-other-identity", t)
	other := createUserWithJWT(otherJWT, t)
	defer removeUserWithJWT(other, otherJWT, t)
	assert.NotEqual(t, http.StatusOK, aiRequest(t, "GET", "/1.0/projects/"+name+"/assets", otherJWT, nil).Code)
	assert.NotEqual(t, http.StatusOK, uploadAssets(t, otherJWT, name, map[string][]byte{"x.png": plan}, "").Code)

	// Rename and delete.
	rec = aiRequest(t, "PATCH", "/1.0/projects/"+name+"/assets/crate.glb", jwt, map[string]string{"name": "room.svg"})
	assert.NotEqual(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = aiRequest(t, "PATCH", "/1.0/projects/"+name+"/assets/crate.glb", jwt, map[string]string{"name": "box.glb"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"name":"box.glb"`)
	p, err := projects.GetProjectByName(globals.Server.Db, name, username)
	require.NoError(t, err)
	dir := filepath.Join(filepath.Dir(filepath.Dir(*p.Location)), "project-assets", *p.UUID, byName["crate.glb"].ID)
	require.DirExists(t, dir)
	require.Equal(t, http.StatusOK, aiRequest(t, "DELETE", "/1.0/projects/"+name+"/assets/box.glb", jwt, nil).Code)
	assert.NoDirExists(t, dir)
	assert.Equal(t, http.StatusNotFound, aiRequest(t, "GET", "/1.0/projects/"+name+"/assets/box.glb", jwt, nil).Code)

	// The AI tools.
	files := mcpJSON(t, jwt, "asset_list", map[string]any{"name": name})["files"].([]any)
	assert.Len(t, files, 3)
	assert.Contains(t, fmt.Sprint(files), "2000×1000 px")
	content = mcpContent(t, jwt, "asset_view", map[string]any{"name": name, "asset": "floorplan.png", "grid": true})
	require.Len(t, content, 2)
	assert.Contains(t, content[0]["text"], "10%")
	assert.Equal(t, "image/jpeg", content[1]["mimeType"])
	content = mcpContent(t, jwt, "asset_view", map[string]any{"name": name, "asset": "room.svg"})
	require.Len(t, content, 1)
	assert.Contains(t, content[0]["text"], "no picture")

	up := mcpJSON(t, jwt, "asset_upload", map[string]any{"name": name, "filename": "sketch.png",
		"content_base64": "data:image/png;base64," + base64.StdEncoding.EncodeToString(assetTestPNG(t, 10, 10))})
	assert.Equal(t, "image", up["kind"])
	text, isErr = mcpCall(t, jwt, "asset_upload", map[string]any{"name": name, "filename": "x.bin", "content_base64": "!!"})
	assert.True(t, isErr, text)

	// Deleting the project removes its files.
	require.Equal(t, http.StatusOK, aiRequest(t, "DELETE", "/1.0/projects/"+name, jwt, nil).Code)
	assert.NoDirExists(t, filepath.Dir(dir))
	var n int
	globals.Server.Db.Model(&projects.Asset{}).Where("project_id = ?", p.ID).Count(&n)
	assert.Zero(t, n)
	_, err = os.Stat(*p.Location)
	assert.NoError(t, err, "the repo itself is kept")
}

// mcpContent invokes a tool and returns its content blocks.
func mcpContent(t *testing.T, jwt, tool string, args map[string]any) []map[string]string {
	t.Helper()
	rec := aiRequest(t, "POST", "/1.0/mcp", jwt, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp struct {
		Result struct {
			Content []map[string]string `json:"content"`
			IsError bool                `json:"isError"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
	require.False(t, resp.Result.IsError, rec.Body.String())
	return resp.Result.Content
}

// A zip of several files is unpacked; its Gazebo model folders become
// project models.
func TestProjectAssetsBundle(t *testing.T) {
	jwt := createValidJWTForIdentity("assets-bundle-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)
	name := fmt.Sprintf("bundle-%d", time.Now().UnixNano())
	mcpJSON(t, jwt, "project_create", map[string]any{"name": name})

	defer func(f projects.AssetInspector) { projects.InspectAsset = f }(projects.InspectAsset)
	projects.InspectAsset = func(_ context.Context, _, kind, _, _ string, _ []byte) (projects.AssetInfo, map[string][]byte, error) {
		if kind == projects.AssetModel {
			// The worker's drawing of the model.
			return projects.AssetInfo{"size": []float64{1, 0.5, 2}},
				map[string][]byte{"ai.jpg": []byte("drawn"), "thumb.jpg": []byte("drawn")}, nil
		}
		return nil, nil, nil
	}

	shelf := `<?xml version="1.0"?><sdf version="1.9"><model name="shelf"><static>true</static><link name="l">` +
		`<visual name="v"><geometry><box><size>1 0.5 2</size></box></geometry></visual></link></model></sdf>`
	zip := zipFiles(t, map[string][]byte{
		"site/models/Shelf_01/model.config":     []byte(`<model><name>Shelf</name><sdf version="1.9">model.sdf</sdf></model>`),
		"site/models/Shelf_01/model.sdf":        []byte(shelf),
		"site/models/Shelf_01/thumbnails/1.png": assetTestPNG(t, 16, 16),
		"site/props/chair.obj":                  []byte("mtllib chair.mtl\nv 0 0 0\nv 1 0 0\nv 0 1 0\nf 1 2 3\n"),
		"site/props/chair.mtl":                  []byte("newmtl wood\nmap_Kd wood.png\n"),
		"site/props/wood.png":                   assetTestPNG(t, 8, 8),
		"site/plan.png":                         assetTestPNG(t, 100, 50),
		"site/README.md":                        []byte("hello"),
	})
	rec := uploadAssets(t, jwt, name, map[string][]byte{"site.zip": zip}, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res projects.AddResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	kinds := map[string]string{}
	for _, a := range res.Assets {
		kinds[a.Name] = a.Kind
		assert.Equal(t, "site.zip", a.Info["from_zip"], a.Name)
	}
	assert.Equal(t, map[string]string{"chair.obj": "mesh", "plan.png": "image", "Shelf_01": "model"}, kinds)
	assert.Equal(t, []string{"site.zip: site/README.md"}, res.Skipped)

	// The model is in the project, with the measured size and its thumbnail.
	files := mcpJSON(t, jwt, "project_list_files", map[string]any{"name": name})
	assert.Contains(t, fmt.Sprint(files["files"]), "models/Shelf_01/model.sdf")
	assert.Contains(t, fmt.Sprint(files["files"]), "models/Shelf_01/thumbnails/1.png")
	list := mcpJSON(t, jwt, "asset_list", map[string]any{"name": name})
	assert.Contains(t, fmt.Sprint(list["files"]), "uri:model://Shelf_01")
	content := mcpContent(t, jwt, "asset_view", map[string]any{"name": name, "asset": "Shelf_01"})
	require.Len(t, content, 2, "with its thumbnail")
	assert.Contains(t, content[0]["text"], `"errors": 0`)
	assert.NotEqual(t, base64.StdEncoding.EncodeToString([]byte("drawn")), content[1]["data"],
		"its own thumbnail, not the drawing")

	// A world can include it; validation finds it in the project.
	world := `<?xml version="1.0"?><sdf version="1.9"><world name="w"><include><uri>model://Shelf_01</uri>` +
		`<name>s1</name></include></world></sdf>`
	w := mcpJSON(t, jwt, "project_write_file", map[string]any{"name": name, "path": "world.sdf", "content": world})
	assert.Equal(t, true, w["validation"].(map[string]any)["valid"], w["validation"])

	// A SLAM map: its YAML and image are one file with the scale and origin.
	pix := bytes.Repeat([]byte{254}, 40*20)
	copy(pix[2*40:3*40], bytes.Repeat([]byte{0}, 40))
	pgm := append([]byte("P5\n40 20\n255\n"), pix...)
	rec = uploadAssets(t, jwt, name, map[string][]byte{"map.zip": zipFiles(t, map[string][]byte{
		"map.yaml": []byte("image: map.pgm\nresolution: 0.05\norigin: [-1.0, -0.5, 0.0]\nnegate: 0\n" +
			"occupied_thresh: 0.65\nfree_thresh: 0.196\n"),
		"map.pgm": pgm})}, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"kind":"map"`)
	list = mcpJSON(t, jwt, "asset_list", map[string]any{"name": name})
	assert.Contains(t, fmt.Sprint(list["files"]), "occupancy map 2 × 1 m at 0.05 m/px")
	content = mcpContent(t, jwt, "asset_view", map[string]any{"name": name, "asset": "map.zip", "grid": true})
	require.Len(t, content, 2)
	assert.Contains(t, content[0]["text"], "labelled in meters")
	assert.Contains(t, content[0]["text"], `"obstacle_extent"`)

	// Uploading it again adds a second model rather than replacing the first.
	rec = uploadAssets(t, jwt, name, map[string][]byte{"again.zip": zipFiles(t, map[string][]byte{
		"Shelf_01/model.config": []byte(`<model><sdf version="1.9">model.sdf</sdf></model>`),
		"Shelf_01/model.sdf":    []byte(shelf), "Shelf_01/notes/plan.pdf": []byte("%PDF-1.4")})}, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"model":"Shelf_01_2"`)
	// Without a thumbnail, its picture is the worker's drawing.
	content = mcpContent(t, jwt, "asset_view", map[string]any{"name": name, "asset": "Shelf_01_2"})
	require.Len(t, content, 2)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("drawn")), content[1]["data"])

}
