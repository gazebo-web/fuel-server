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

func TestAIToolsValidateDocument(t *testing.T) {
	jwt := createValidJWTForIdentity("aitools-validate-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)

	r := mcpJSON(t, jwt, "sdf_validate", map[string]any{"sdf": `<sdf version="1.9"><model name="m"><static>maybe</static></model></sdf>`})
	assert.Equal(t, false, r["valid"])
	assert.Contains(t, r["summary"], "expected true or false")
}
