package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gazebo-web/fuel-server/bundles/projects"
	"github.com/gazebo-web/fuel-server/globals"
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
