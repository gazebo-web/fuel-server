package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gazebo-web/fuel-server/aitools"
	"github.com/gazebo-web/fuel-server/bootstrap"
	"github.com/gazebo-web/fuel-server/bundles/projects"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/mcp"
	"github.com/gazebo-web/fuel-server/sdf"
	jwt5 "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End-to-end tests of the AI tools over the real router: MCP JSON-RPC at
// /1.0/mcp plus the project REST endpoints the editor uses.

const aiTestWorld = `<?xml version="1.0"?>
<sdf version="1.9">
  <world name="demo">
    <model name="wall">
      <static>true</static>
      <pose>0 2 1.5 0 0 0</pose>
      <link name="l">
        <collision name="c"><geometry><box><size>4 0.2 3</size></box></geometry></collision>
        <visual name="v"><geometry><box><size>4 0.2 3</size></box></geometry></visual>
      </link>
    </model>
  </world>
</sdf>`

func aiRequest(t *testing.T, method, url, jwt string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b := new(bytes.Buffer)
	if body != nil {
		require.NoError(t, json.NewEncoder(b).Encode(body))
	}
	req, _ := http.NewRequest(method, url, b)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	if strings.HasSuffix(url, "/mcp") {
		// Streamable HTTP clients must accept both.
		req.Header.Set("Accept", "application/json, text/event-stream")
	}
	rec := httptest.NewRecorder()
	globals.Server.Router.ServeHTTP(rec, req)
	return rec
}

// mcpCall invokes a tool and returns its text result and isError flag.
func mcpCall(t *testing.T, jwt, tool string, args map[string]any) (string, bool) {
	t.Helper()
	rec := aiRequest(t, "POST", "/1.0/mcp", jwt, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
			IsError bool                    `json:"isError"`
		} `json:"result"`
		Error *struct{ Message string } `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
	require.Nil(t, resp.Error, rec.Body.String())
	require.NotEmpty(t, resp.Result.Content)
	return resp.Result.Content[0].Text, resp.Result.IsError
}

func mcpJSON(t *testing.T, jwt, tool string, args map[string]any) map[string]any {
	t.Helper()
	text, isErr := mcpCall(t, jwt, tool, args)
	require.False(t, isErr, "%s failed: %s", tool, text)
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &out), text)
	return out
}

func TestAIToolsProjectWorkflow(t *testing.T) {
	jwt := createValidJWTForIdentity("aitools-test-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)
	name := fmt.Sprintf("aitools-%d", time.Now().UnixNano())

	// tools/list exposes the new catalog only.
	rec := aiRequest(t, "POST", "/1.0/mcp", jwt, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"project_edit_file"`)
	assert.NotContains(t, rec.Body.String(), `"project_save_world_sdf"`)

	created := mcpJSON(t, jwt, "project_create", map[string]any{"name": name, "gazebo_release": "Harmonic"})
	assert.Equal(t, "1.9", created["sdf_version"])
	assert.NotEmpty(t, created["uuid"])

	// Write a world: validated, returns a rev.
	w := mcpJSON(t, jwt, "project_write_file", map[string]any{"name": name, "path": "world.sdf", "content": aiTestWorld})
	rev1 := w["rev"].(string)
	assert.Equal(t, true, w["validation"].(map[string]any)["valid"], w)

	// Summary sees the wall.
	s := mcpJSON(t, jwt, "scene_summary", map[string]any{"name": name})
	assert.Contains(t, fmt.Sprint(s["summary"]), "box 4 0.2 3")

	// Edit with base_rev: moves the wall; validation reports a bad value.
	e := mcpJSON(t, jwt, "project_edit_file", map[string]any{
		"name": name, "path": "world.sdf", "base_rev": rev1,
		"old_str": "<pose>0 2 1.5 0 0 0</pose>", "new_str": "<pose>0 2 1.5 0 0</pose>",
	})
	rev2 := e["rev"].(string)
	assert.NotEqual(t, rev1, rev2)
	v := e["validation"].(map[string]any)
	assert.Equal(t, false, v["valid"])
	assert.Contains(t, v["summary"], "world.sdf:6")

	// A second edit from the old rev is stale.
	text, isErr := mcpCall(t, jwt, "project_edit_file", map[string]any{
		"name": name, "path": "world.sdf", "base_rev": rev1,
		"old_str": "<static>true</static>", "new_str": "<static>false</static>",
	})
	assert.True(t, isErr)
	assert.Contains(t, text, "changed since base_rev")

	// The thumbnail amends HEAD: no new version, edits from rev2 still apply.
	before := projectVersionCount(t, jwt, name)
	thumb := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/1.0/projects/"+name+"/thumbnail", strings.NewReader("png-bytes"))
	req.Header.Set("Authorization", "Bearer "+jwt)
	globals.Server.Router.ServeHTTP(thumb, req)
	require.Equal(t, http.StatusOK, thumb.Code, thumb.Body.String())
	assert.Equal(t, before, projectVersionCount(t, jwt, name))

	// Editor save over REST: stale base_rev is a 409, current one succeeds.
	rec = aiRequest(t, "PUT", "/1.0/projects/"+name+"/files/world.sdf", jwt,
		map[string]any{"content": aiTestWorld, "base_rev": rev1})
	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	rec = aiRequest(t, "PUT", "/1.0/projects/"+name+"/files/world.sdf", jwt,
		map[string]any{"content": aiTestWorld, "base_rev": rev2})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Checkpoint folds the auto-saves into one labeled version.
	rec = aiRequest(t, "POST", "/1.0/projects/"+name+"/checkpoint", jwt, map[string]any{"message": "Wall placed"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var cp struct {
		Rev      string `json:"rev"`
		Squashed int    `json:"squashed"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cp))
	assert.Equal(t, 3, cp.Squashed)

	// The project reports the new head and its uuid.
	rec = aiRequest(t, "GET", "/1.0/projects/"+name, jwt, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var p map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	assert.Equal(t, cp.Rev, p["head_rev"])
	assert.Equal(t, created["uuid"], p["uuid"])

	files := mcpJSON(t, jwt, "project_list_files", map[string]any{"name": name})
	assert.ElementsMatch(t, []any{".gitignore", "thumbnail.png", "world.sdf"}, files["files"])

	read := mcpJSON(t, jwt, "project_read_file", map[string]any{"name": name, "path": "world.sdf"})
	assert.Equal(t, aiTestWorld, read["content"])

	// REST read at a commit hash returns that revision's content.
	rec = aiRequest(t, "GET", "/1.0/projects/"+name+"/"+rev2+"/files/world.sdf", jwt, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "<pose>0 2 1.5 0 0</pose>")

	// Paths can't escape the repo.
	text, isErr = mcpCall(t, jwt, "project_write_file", map[string]any{"name": name, "path": "../escape.sdf", "content": "x"})
	assert.True(t, isErr)
	assert.Contains(t, text, "invalid path")
}

// projectVersionCount returns the number of versions in a project.
func projectVersionCount(t *testing.T, jwt, name string) int {
	t.Helper()
	text, isErr := mcpCall(t, jwt, "project_list_versions", map[string]any{"name": name})
	require.False(t, isErr, text)
	var list []any
	require.NoError(t, json.Unmarshal([]byte(text), &list), text)
	return len(list)
}

func TestSDFValidateEndpoint(t *testing.T) {
	b := strings.NewReader(`{"sdf": "<sdf version=\"1.9\"><model name=\"m\"><static>maybe</static></model></sdf>"}`)
	req, _ := http.NewRequest("POST", "/1.0/sdf/validate", b)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	globals.Server.Router.ServeHTTP(rec, req) // no Authorization header
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var r struct {
		Valid  bool `json:"valid"`
		Errors []struct {
			Line    int    `json:"line"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &r))
	assert.False(t, r.Valid)
	require.Len(t, r.Errors, 1)
	assert.Contains(t, r.Errors[0].Message, "expected true or false")
}

func TestAIToolsValidateDocument(t *testing.T) {
	jwt := createValidJWTForIdentity("aitools-validate-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)

	r := mcpJSON(t, jwt, "sdf_validate", map[string]any{"sdf": `<sdf version="1.9"><model name="m"><static>maybe</static></model></sdf>`})
	assert.Equal(t, false, r["valid"])
	assert.Contains(t, r["summary"], "expected true or false")
}

// The local catalog returns compact, public-by-default results with the
// include URI to use.
func TestAIToolsLocalCatalog(t *testing.T) {
	setup()
	jwt := createValidJWTForIdentity("aitools-catalog-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)
	createTestModelWithOwner(t, &jwt, "public_table", username, false)
	createTestModelWithOwner(t, &jwt, "private_shelf", username, true)

	search := func(args map[string]any) []map[string]any {
		text, isErr := mcpCall(t, jwt, "fuel_search_models", args)
		require.False(t, isErr, text)
		var items []map[string]any
		require.NoError(t, json.Unmarshal([]byte(text), &items), text)
		return items
	}
	names := func(items []map[string]any) (out []string) {
		for _, it := range items {
			out = append(out, it["name"].(string))
		}
		return out
	}

	items := search(map[string]any{"owner": username})
	require.Equal(t, []string{"public_table"}, names(items))
	assert.Equal(t, "https://fuel.gazebosim.org/1.0/"+username+"/models/public_table", items[0]["uri"])
	assert.Equal(t, true, items[0]["public"])
	assert.Equal(t, "local", items[0]["source"])
	assert.NotEmpty(t, items[0]["license"])

	assert.ElementsMatch(t, []string{"public_table", "private_shelf"},
		names(search(map[string]any{"owner": username, "include_private": true})))

	text, isErr := mcpCall(t, jwt, "fuel_get_model_details", map[string]any{"owner": username, "name": "public_table"})
	require.False(t, isErr, text)
	assert.Contains(t, text, `"uri": "https://fuel.gazebosim.org/1.0/`+username+`/models/public_table"`)

	text, isErr = mcpCall(t, jwt, "fuel_get_model_details", map[string]any{"owner": username, "name": "missing"})
	assert.True(t, isErr, text)
}

// Harmonic is the oldest release a project can be created for or switched
// to; the release sets the SDF version and ROS 2 distro.
func TestProjectReleases(t *testing.T) {
	jwt := createValidJWTForIdentity("aitools-release-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)
	name := fmt.Sprintf("release-%d", time.Now().UnixNano())

	rec := aiRequest(t, "POST", "/1.0/projects", jwt, map[string]any{
		"name": name, "gazebo_version": "Fortress", "sdf_version": "1.8", "ros2_distro": "Humble"})
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	text, isErr := mcpCall(t, jwt, "project_create", map[string]any{"name": name, "gazebo_release": "Fortress"})
	assert.True(t, isErr)
	assert.Contains(t, text, "supported: Harmonic")

	// The client's sdf_version is replaced by the release's.
	rec = aiRequest(t, "POST", "/1.0/projects", jwt, map[string]any{"name": name, "sdf_version": "1.6"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"gazebo_version":"Harmonic"`)
	assert.Contains(t, rec.Body.String(), `"sdf_version":"1.9"`)

	rec = aiRequest(t, "PATCH", "/1.0/projects/"+name, jwt, map[string]any{"gazebo_version": "Fortress"})
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// Binary files go in through the raw PUT, come back with their content
// type, and the text tools only ever see their metadata.
func TestProjectBinaryFiles(t *testing.T) {
	jwt := createValidJWTForIdentity("aitools-binary-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)
	name := fmt.Sprintf("binary-%d", time.Now().UnixNano())
	mcpJSON(t, jwt, "project_create", map[string]any{"name": name})

	put := func(path string, body []byte) *httptest.ResponseRecorder {
		req, _ := http.NewRequest("PUT", "/1.0/projects/"+name+"/files/"+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Authorization", "Bearer "+jwt)
		rec := httptest.NewRecorder()
		globals.Server.Router.ServeHTTP(rec, req)
		return rec
	}
	glb := append([]byte("glTF\x02\x00\x00\x00"), make([]byte, 64)...)
	rec := put("models/crate/meshes/visual.glb", glb)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = aiRequest(t, "GET", "/1.0/projects/"+name+"/tip/files/models/crate/meshes/visual.glb", jwt, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "model/gltf-binary", rec.Header().Get("Content-Type"))
	assert.Equal(t, glb, rec.Body.Bytes())

	read := mcpJSON(t, jwt, "project_read_file", map[string]any{"name": name, "path": "models/crate/meshes/visual.glb"})
	assert.Equal(t, true, read["binary"])
	assert.EqualValues(t, len(glb), read["size"])
	assert.Nil(t, read["content"])

	text, isErr := mcpCall(t, jwt, "project_write_file", map[string]any{"name": name, "path": "models/crate/meshes/x.glb", "content": "abc"})
	assert.True(t, isErr, text)
	text, isErr = mcpCall(t, jwt, "project_edit_file", map[string]any{"name": name, "path": "models/crate/meshes/visual.glb",
		"old_str": "glTF", "new_str": "nope", "base_rev": "tip"})
	assert.True(t, isErr, text)

	defer func(n int64) { projects.MaxFileSize = n }(projects.MaxFileSize)
	projects.MaxFileSize = 32
	rec = put("models/crate/meshes/big.glb", glb)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
}

// A custom model is created in the project, included from the world by
// model://, and its files are checked against the project.
func TestAIToolsProjectModels(t *testing.T) {
	jwt := createValidJWTForIdentity("aitools-models-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)
	name := fmt.Sprintf("models-%d", time.Now().UnixNano())
	mcpJSON(t, jwt, "project_create", map[string]any{"name": name})
	text, isErr := mcpCall(t, jwt, "scene_summary", map[string]any{"name": name})
	assert.True(t, isErr)
	assert.Contains(t, text, "world.sdf doesn't exist yet; create it with project_write_file")

	created := mcpJSON(t, jwt, "model_create", map[string]any{
		"name": name, "model": "crate", "description": "A wooden crate",
		"size": []any{0.6, 0.4, 0.4}, "density": 300.0})
	assert.Equal(t, "model://crate", created["uri"])
	assert.Equal(t, true, created["validation"].(map[string]any)["valid"], created)
	_, isErr = mcpCall(t, jwt, "model_create", map[string]any{"name": name, "model": "crate"})
	assert.True(t, isErr, "duplicate model")

	world := `<?xml version="1.0"?>
<sdf version="1.9"><world name="w">
  <include><uri>model://crate</uri><name>crate_1</name><pose>0 0 0.2 0 0 0</pose></include>
  <include><uri>model://sun</uri></include>
</world></sdf>`
	w := mcpJSON(t, jwt, "project_write_file", map[string]any{"name": name, "path": "world.sdf", "content": world})
	v := w["validation"].(map[string]any)
	assert.EqualValues(t, 1, v["errors"], v)
	assert.Contains(t, v["summary"], `no model "sun" in this project (project models: crate)`)

	// A mesh reference must point at a file in the project.
	sdfDoc := mcpJSON(t, jwt, "project_read_file", map[string]any{"name": name, "path": "models/crate/model.sdf"})
	withMesh := strings.Replace(sdfDoc["content"].(string),
		"<visual name=\"visual\">\n        <geometry><box><size>0.6 0.4 0.4</size></box></geometry>",
		"<visual name=\"visual\">\n        <geometry><mesh><uri>meshes/visual.glb</uri></mesh></geometry>", 1)
	require.NotEqual(t, sdfDoc["content"], withMesh)
	w = mcpJSON(t, jwt, "project_write_file", map[string]any{"name": name, "path": "models/crate/model.sdf", "content": withMesh})
	assert.Contains(t, w["validation"].(map[string]any)["summary"], "no file models/crate/meshes/visual.glb")

	req, _ := http.NewRequest("PUT", "/1.0/projects/"+name+"/files/models/crate/meshes/visual.glb", strings.NewReader("glTF\x00"))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	globals.Server.Router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	val := mcpJSON(t, jwt, "sdf_validate", map[string]any{"name": name, "path": "models/crate/model.sdf"})
	assert.Equal(t, true, val["valid"], val)

	list := mcpJSON(t, jwt, "project_list_models", map[string]any{"name": name})
	models := list["models"].([]any)
	require.Len(t, models, 1)
	m := models[0].(map[string]any)
	assert.Equal(t, "crate", m["name"])
	assert.Equal(t, "A wooden crate", m["description"])
	assert.EqualValues(t, 1, m["links"])
	assert.EqualValues(t, 3, m["files"])

	rec = aiRequest(t, "GET", "/1.0/projects/"+name+"/models", jwt, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"uri":"model://crate"`)

	// Sensors need world systems: a model write warns about the world that
	// includes it, and the world check sees sensors inside included models.
	withCamera := strings.Replace(withMesh, "</link>", `<sensor name="cam" type="camera"/></link>`, 1)
	w = mcpJSON(t, jwt, "project_write_file", map[string]any{"name": name, "path": "models/crate/model.sdf", "content": withCamera})
	assert.Contains(t, w["validation"].(map[string]any)["summary"],
		"world.sdf includes this model, but its sensors publish nothing until world.sdf gets <plugin filename=\"gz-sim-sensors-system\"")
	val = mcpJSON(t, jwt, "sdf_validate", map[string]any{"name": name, "path": "world.sdf"})
	assert.Contains(t, val["summary"], "camera sensors (model crate_1) publish nothing")
}

// A signed file URL serves one revision without an Authorization header,
// with project models laid out like Fuel models for gzweb.
func TestProjectSignedFileURLs(t *testing.T) {
	jwt := createValidJWTForIdentity("aitools-fileurl-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)
	name := fmt.Sprintf("fileurl-%d", time.Now().UnixNano())
	mcpJSON(t, jwt, "project_create", map[string]any{"name": name})
	mcpJSON(t, jwt, "model_create", map[string]any{"name": name, "model": "crate"})
	w := mcpJSON(t, jwt, "project_write_file", map[string]any{"name": name, "path": "world.sdf", "content": aiTestWorld})

	rec := aiRequest(t, "GET", "/1.0/projects/"+name+"/file-url", jwt, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var fu struct {
		BasePath string `json:"base_path"`
		Rev      string `json:"rev"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &fu))
	assert.Equal(t, w["rev"], fu.Rev)
	require.True(t, strings.HasPrefix(fu.BasePath, "/1.0/project-files/"), fu.BasePath)

	get := func(p string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest("GET", fu.BasePath+p, nil) // no Authorization
		rec := httptest.NewRecorder()
		globals.Server.Router.ServeHTTP(rec, req)
		return rec
	}
	rec = get("/files/world.sdf")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, aiTestWorld, rec.Body.String())
	assert.Equal(t, "application/xml", rec.Header().Get("Content-Type"))

	rec = get("/models/crate/tip/files")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"name":"crate","file_tree":[{"name":"model.config","path":"/model.config"},{"name":"model.sdf","path":"/model.sdf"}]}`, rec.Body.String())
	rec = get("/models/crate/tip/files/model.sdf")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `<model name="crate">`)

	// Later writes don't change what the URL serves.
	mcpJSON(t, jwt, "project_write_file", map[string]any{"name": name, "path": "world.sdf", "content": "<sdf version=\"1.9\"/>"})
	assert.Equal(t, aiTestWorld, get("/files/world.sdf").Body.String())

	assert.Equal(t, http.StatusNotFound, get("/models/missing/tip/files").Code)
	tampered := strings.Replace(fu.BasePath, "/project-files/", "/project-files/x", 1)
	req, _ := http.NewRequest("GET", tampered+"/files/world.sdf", nil)
	rec = httptest.NewRecorder()
	globals.Server.Router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
}

func TestBuildFileTree(t *testing.T) {
	b, _ := json.Marshal(buildFileTree([]string{"meshes/a.glb", "meshes/b/c.png", "model.sdf"}))
	assert.JSONEq(t, `[{"name":"meshes","path":"/meshes","children":[{"name":"a.glb","path":"/meshes/a.glb"},
		{"name":"b","path":"/meshes/b","children":[{"name":"c.png","path":"/meshes/b/c.png"}]}]},
		{"name":"model.sdf","path":"/model.sdf"}]`, string(b))
}

// generatedModel is a mesh worker result for model `name` (cmd/meshworker's
// tarball layout) with the given collision mesh files.
func generatedModel(t *testing.T, name string, collisions ...string) []byte {
	t.Helper()
	var coll strings.Builder
	files := map[string]string{}
	for i, c := range collisions {
		fmt.Fprintf(&coll, "      <collision name=\"collision_%d\"><geometry><mesh><uri>meshes/%s</uri></mesh></geometry></collision>\n", i, c)
		files["models/"+name+"/meshes/"+c] = "glTF collision"
	}
	files["models/"+name+"/model.sdf"] = fmt.Sprintf(`<?xml version="1.0"?>
<sdf version="1.9">
  <model name="%s">
    <static>false</static>
    <link name="link">
      <inertial><pose>0 0 0.3 0 0 0</pose><mass>2.5</mass>
        <inertia><ixx>0.06</ixx><ixy>0</ixy><ixz>0</ixz><iyy>0.06</iyy><iyz>0</iyz><izz>0.024</izz></inertia>
      </inertial>
      <visual name="visual"><geometry><mesh><uri>meshes/%s.glb</uri></mesh></geometry></visual>
%s    </link>
  </model>
</sdf>
`, name, name, coll.String())
	files["models/"+name+"/model.config"] = fmt.Sprintf(`<?xml version="1.0"?>
<model><name>%s</name><version>1.0</version><sdf version="1.9">model.sdf</sdf><author><name>AI Builder</name></author><description>cone</description></model>
`, name)
	files["models/"+name+"/meshes/"+name+".glb"] = "glTF visual"
	files["models/"+name+"/source/model.py"] = "def build(): ...\n"
	files["preview.png"] = "\x89PNG fake"
	files["report.json"] = `{"ok": true, "triangles": 4034, "bounding_box_m": [0.36, 0.36, 0.6], "warnings": []}`
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, n := range []string{"report.json"} {
		tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: int64(len(files[n]))})
		tw.Write([]byte(files[n]))
	}
	for n, c := range files {
		if n == "report.json" {
			continue
		}
		tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: int64(len(c))})
		tw.Write([]byte(c))
	}
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

func TestAIToolsModelGenerate(t *testing.T) {
	jwt := createValidJWTForIdentity("aitools-meshgen-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)
	user, em := getUserFromDb(username, t)
	require.Nil(t, em)
	name := fmt.Sprintf("meshgen-%d", time.Now().UnixNano())
	mcpJSON(t, jwt, "project_create", map[string]any{"name": name})

	// A stub worker: the code text picks the result.
	var jobs []map[string]any
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet { // the async job finishing
			w.Header().Set("Content-Type", "application/x-tar")
			w.Write(generatedModel(t, "slow", "slow_collision_0.glb"))
			return
		}
		var job map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&job))
		jobs = append(jobs, job)
		code := job["code"].(string)
		switch {
		case strings.Contains(code, "SLOW"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(`{"job_id":"job-1","status":"running"}`))
			return
		case strings.Contains(code, "INTERNAL"):
			w.Header().Set("Content-Type", "application/x-tar")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(makeReportTar(t, `{"ok": false, "error": "internal error: RuntimeError: Resource temporarily unavailable"}`))
			return
		case strings.Contains(code, "BROKEN"):
			w.Header().Set("Content-Type", "application/x-tar")
			w.Write(makeReportTar(t, `{"ok": false, "error": "ZeroDivisionError: division by zero", "line": 4}`))
			return
		case strings.Contains(code, "V2"):
			w.Header().Set("Content-Type", "application/x-tar")
			w.Write(generatedModel(t, "cone", "cone_collision_0.glb"))
			return
		}
		w.Header().Set("Content-Type", "application/x-tar")
		w.Write(generatedModel(t, "cone", "cone_collision_0.glb", "cone_collision_1.glb"))
	}))
	defer worker.Close()

	reg := aitools.NewRegistry(aitools.Config{MeshWorkerURL: worker.URL})
	call := func(tool string, args map[string]any, as *mcp.ToolContext) mcp.CallToolResult {
		t.Helper()
		h, ok := reg.Get(tool)
		require.True(t, ok, tool)
		r, err := h.Handler(as, args)
		require.NoError(t, err)
		return r
	}
	tc := &mcp.ToolContext{Ctx: context.Background(), DB: globals.Server.Db, User: user}
	files := func() string {
		out := mcpJSON(t, jwt, "project_list_files", map[string]any{"name": name})
		b, _ := json.Marshal(out["files"])
		return string(b)
	}

	r := call("model_generate", map[string]any{"name": name, "model": "cone", "code": "def build(): ...", "mass": 2.5}, tc)
	require.False(t, r.IsError, r.Content[0].Text)
	require.Len(t, r.Content, 2)
	assert.Equal(t, "image", r.Content[1].Type)
	assert.Equal(t, "image/png", r.Content[1].MimeType)
	var res map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.Content[0].Text), &res))
	assert.Equal(t, "model://cone", res["uri"])
	assert.Equal(t, true, res["validation"].(map[string]any)["valid"], res["validation"])
	assert.EqualValues(t, 4034, res["triangles"])
	assert.Equal(t, "1.9", jobs[0]["sdf_version"])
	assert.EqualValues(t, 2.5, jobs[0]["mass"])
	got := files()
	for _, f := range []string{"models/cone/model.sdf", "models/cone/meshes/cone.glb", "models/cone/meshes/cone_collision_1.glb",
		"models/cone/source/model.py", "models/cone/previews/preview.png"} {
		assert.Contains(t, got, f)
	}

	// Regenerating replaces the folder: the second hull is gone.
	r = call("model_generate", map[string]any{"name": name, "model": "cone", "code": "V2"}, tc)
	require.False(t, r.IsError, r.Content[0].Text)
	assert.NotContains(t, files(), "cone_collision_1.glb")

	// The model code failed: an error result with the line, nothing committed.
	r = call("model_generate", map[string]any{"name": name, "model": "cone", "code": "BROKEN"}, tc)
	assert.True(t, r.IsError)
	assert.Contains(t, r.Content[0].Text, "ZeroDivisionError")
	assert.Contains(t, r.Content[0].Text, `"line": 4`)

	// The worker failed, not the code: the hint says so.
	r = call("model_generate", map[string]any{"name": name, "model": "cone", "code": "INTERNAL"}, tc)
	assert.True(t, r.IsError)
	assert.Contains(t, r.Content[0].Text, "not your code")

	// A hand-made model is not overwritten.
	mcpJSON(t, jwt, "model_create", map[string]any{"name": name, "model": "shelf"})
	r = call("model_generate", map[string]any{"name": name, "model": "shelf", "code": "x"}, tc)
	assert.True(t, r.IsError)
	assert.Contains(t, r.Content[0].Text, "wasn't generated")
	r = call("model_generate", map[string]any{"name": name, "model": "Bad-Name", "code": "x"}, tc)
	assert.True(t, r.IsError)

	// A slow job: model_generate returns a job id, model_generate_status
	// commits it, and only for the same user.
	r = call("model_generate", map[string]any{"name": name, "model": "slow", "code": "SLOW"}, tc)
	require.False(t, r.IsError, r.Content[0].Text)
	assert.Contains(t, r.Content[0].Text, `"job_id": "job-1"`)
	other := "someone-else"
	r = call("model_generate_status", map[string]any{"job_id": "job-1"}, &mcp.ToolContext{Ctx: context.Background(), DB: globals.Server.Db, User: &users.User{Username: &other}})
	assert.True(t, r.IsError)
	r = call("model_generate_status", map[string]any{"job_id": "job-1"}, tc)
	require.False(t, r.IsError, r.Content[0].Text)
	assert.Contains(t, files(), "models/slow/meshes/slow.glb")
}

func makeReportTar(t *testing.T, report string) []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{Name: "report.json", Mode: 0o644, Size: int64(len(report))})
	tw.Write([]byte(report))
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

// Fuel model sizes (plan §5.6): the sweep measures this server's public
// models through the worker, search and details return the stored size,
// and scene_summary places it in the world.
func TestAIToolsModelSizes(t *testing.T) {
	setup()
	jwt := createValidJWTForIdentity("aitools-sizes-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)
	user, em := getUserFromDb(username, t)
	require.Nil(t, em)
	createTestModelWithOwner(t, &jwt, "sized_table", username, false)
	createTestModelWithOwner(t, &jwt, "hidden_table", username, true)

	var measured []string
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/measure", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		require.NoError(t, err)
		var names []string
		for _, f := range zr.File {
			names = append(names, f.Name)
		}
		measured = append(measured, strings.Join(names, ","))
		w.Write([]byte(`{"ok":true,"bbox":{"min":[-0.5,-0.4,0],"max":[0.5,0.4,0.75]},"size":[1,0.8,0.75],` +
			`"visuals":1,"measured":1,"from":"visual"}`))
	}))
	defer worker.Close()
	cfg := aitools.Config{FuelHosts: []string{"fuel.gazebosim.org"}, MeshWorkerURL: worker.URL}

	// The sweep measures public models without a size, once.
	todo, done, err := aitools.MeasureLocalModels(cfg, globals.Server.Db, 1000)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, todo, 1)
	assert.Equal(t, todo, done)
	require.NotEmpty(t, measured)
	assert.Contains(t, measured[len(measured)-1], "model.config")
	todo, _, err = aitools.MeasureLocalModels(cfg, globals.Server.Db, 1000)
	require.NoError(t, err)
	assert.Zero(t, todo)
	var hidden int
	globals.Server.Db.Model(&aitools.ModelSize{}).Where("name = ?", "hidden_table").Count(&hidden)
	assert.Zero(t, hidden, "private models aren't measured")
	calls := len(measured)

	reg := aitools.NewRegistry(cfg)
	tc := &mcp.ToolContext{Ctx: context.Background(), DB: globals.Server.Db, User: user}
	call := func(tool string, args map[string]any) string {
		t.Helper()
		h, ok := reg.Get(tool)
		require.True(t, ok, tool)
		r, err := h.Handler(tc, args)
		require.NoError(t, err)
		require.False(t, r.IsError, r.Content[0].Text)
		return r.Content[0].Text
	}
	var items []map[string]any
	require.NoError(t, json.Unmarshal([]byte(call("fuel_search_models", map[string]any{"owner": username})), &items))
	require.Len(t, items, 1)
	assert.Equal(t, map[string]any{"size": []any{1.0, 0.8, 0.75}, "min": []any{-0.5, -0.4, 0.0},
		"max": []any{0.5, 0.4, 0.75}}, items[0]["bbox"])
	assert.Contains(t, call("fuel_get_model_details", map[string]any{"owner": username, "name": "sized_table"}),
		`"size": [`)
	assert.Equal(t, calls, len(measured), "stored sizes aren't measured again")

	name := fmt.Sprintf("sizes-%d", time.Now().UnixNano())
	mcpJSON(t, jwt, "project_create", map[string]any{"name": name})
	world := `<?xml version="1.0"?>
<sdf version="1.9"><world name="w">
  <include><uri>https://fuel.gazebosim.org/1.0/` + username + `/models/sized_table</uri><name>t1</name>
    <pose>2 3 0 0 0 1.5708</pose></include>
</world></sdf>`
	mcpJSON(t, jwt, "project_write_file", map[string]any{"name": name, "path": "world.sdf", "content": world})
	var summary struct {
		Summary sdf.Summary `json:"summary"`
	}
	require.NoError(t, json.Unmarshal([]byte(call("scene_summary", map[string]any{"name": name})), &summary))
	box := summary.Summary.Entities[0].BBox.(map[string]any)
	assert.Equal(t, []any{1.6, 2.5, 0.0}, box["world_min"])
	assert.Equal(t, []any{2.4, 3.5, 0.75}, box["world_max"])
}

// The public MCP endpoint's OAuth surface (design §8.4): protected
// resource metadata, 401s that point at it, personal access tokens with
// scopes, and OAuth tokens limited to this resource and their scopes.
func TestMCPAuth(t *testing.T) {
	setup()
	jwt := createValidJWTForIdentity("mcp-auth-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)

	call := func(h http.Handler, path, header, value, method, tool string) *httptest.ResponseRecorder {
		body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
		if tool != "" {
			body["params"] = map[string]any{"name": tool, "arguments": map[string]any{}}
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if header != "" {
			req.Header.Set(header, value)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	toolsOf := func(rec *httptest.ResponseRecorder) string {
		var resp struct {
			Result struct {
				Tools []struct{ Name string } `json:"tools"`
			} `json:"result"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
		var names []string
		for _, tool := range resp.Result.Tools {
			names = append(names, tool.Name)
		}
		return strings.Join(names, " ")
	}
	router := globals.Server.Router

	// Metadata, at the RFC 9728 path for /mcp and the bare one.
	for _, p := range []string{"/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		require.Equal(t, http.StatusOK, rec.Code, p)
		var meta map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &meta))
		assert.Equal(t, "http://example.com/mcp", meta["resource"])
		assert.Equal(t, []any{"fuel:read", "project:read", "project:write", "mesh:generate"}, meta["scopes_supported"])
	}

	// No token: 401 pointing at the metadata, on /mcp and /1.0/mcp.
	for _, p := range []string{"/mcp", "/1.0/mcp"} {
		rec := call(router, p, "", "", "tools/list", "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code, p)
		assert.Contains(t, rec.Header().Get("WWW-Authenticate"),
			`resource_metadata="http://example.com/.well-known/oauth-protected-resource/mcp"`)
	}

	// The web session's token sees every tool (no resource configured).
	all := toolsOf(call(router, "/mcp", "Authorization", "Bearer "+jwt, "tools/list", ""))
	assert.Contains(t, all, "project_write_file")

	// A personal access token limited to reading, as Bearer or Private-Token.
	rec := aiRequest(t, "POST", "/1.0/users/"+username+"/access-tokens", jwt,
		map[string]any{"name": "readonly", "scopes": []string{"fuel:read", "project:read"}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var tok struct{ Prefix, Key string }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tok))
	pat := tok.Prefix + "." + tok.Key
	for _, h := range [][2]string{{"Authorization", "Bearer " + pat}, {"Private-Token", pat}} {
		names := toolsOf(call(router, "/mcp", h[0], h[1], "tools/list", ""))
		assert.Contains(t, names, "fuel_search_models", h[0])
		assert.Contains(t, names, "project_read_file", h[0])
		assert.NotContains(t, names, "project_write_file", h[0])
	}
	// Tools outside the token's scopes aren't served at all.
	rec = call(router, "/mcp", "Private-Token", pat, "tools/call", "project_create")
	assert.Contains(t, rec.Body.String(), `unknown tool \"project_create\"`, rec.Body.String())
	rec = aiRequest(t, "POST", "/1.0/users/"+username+"/access-tokens", jwt,
		map[string]any{"name": "bogus", "scopes": []string{"everything"}})
	assert.NotEqual(t, http.StatusOK, rec.Code, "unknown scopes are refused")
	assert.Equal(t, http.StatusUnauthorized,
		call(router, "/mcp", "Authorization", "Bearer nope.nope", "tools/list", "").Code)

	// With a resource configured, an OAuth token must be for it, and grants
	// only the scopes it carries.
	cfg := mcpConfig{Resource: "https://fuel.test/mcp", AuthServers: []string{"https://tenant.test/"}}
	endpoint := mcpEndpoint(buildMCPRegistry(), bootstrap.Verifier(), cfg)
	forUs := generateJWT(testJWT{jwtClaims: &jwt5.MapClaims{"sub": "mcp-auth-identity",
		"aud": []string{"https://fuel.test/mcp"}, "scope": "openid fuel:read project:read"}}, t)
	names := toolsOf(call(endpoint, "/mcp", "Authorization", "Bearer "+forUs, "tools/list", ""))
	assert.Contains(t, names, "fuel_search_models")
	assert.NotContains(t, names, "project_write_file")
	rec = call(endpoint, "/mcp", "Authorization", "Bearer "+jwt, "tools/list", "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "a token for another audience")
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "https://fuel.test/.well-known/oauth-protected-resource/mcp")
}
