package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gazebo-web/fuel-server/aibuilder"
	"github.com/gazebo-web/fuel-server/aibuilder/llm"
	"github.com/gazebo-web/fuel-server/aitools"
	"github.com/gazebo-web/fuel-server/globals"
)

// End-to-end tests of the in-app agent (cmd/aibuilder) over its HTTP API,
// with real authentication, MySQL and aitools, and a scripted LLM.

// scriptedLLM answers each request with the next round of chunks. A round
// starting with a "BLOCK" text chunk waits for release (or cancellation).
type scriptedLLM struct {
	mu      sync.Mutex
	rounds  [][]llm.Chunk
	release chan struct{}
	reqs    int
}

func (p *scriptedLLM) Name() string          { return "scripted" }
func (p *scriptedLLM) SupportsToolUse() bool { return true }
func (p *scriptedLLM) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "scripted-1", DisplayName: "Scripted 1"}}, nil
}

func (p *scriptedLLM) ChatStream(ctx context.Context, _ llm.Request) (<-chan llm.Chunk, error) {
	p.mu.Lock()
	var round []llm.Chunk
	if p.reqs < len(p.rounds) {
		round = p.rounds[p.reqs]
	}
	p.reqs++
	p.mu.Unlock()
	out := make(chan llm.Chunk)
	go func() {
		defer close(out)
		for _, c := range round {
			if c.Type == llm.ChunkText && c.Text == "BLOCK" {
				select {
				case <-p.release:
					continue
				case <-ctx.Done():
					out <- llm.Chunk{Type: llm.ChunkError, Err: ctx.Err().Error()}
					return
				}
			}
			out <- c
		}
	}()
	return out, nil
}

type sse struct {
	id    string
	event string
	data  map[string]any
}

// readSSE reads events until done (or the body ends).
func readSSE(t *testing.T, resp *http.Response) []sse {
	t.Helper()
	defer resp.Body.Close()
	var out []sse
	var cur sse
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "id: "):
			cur.id = line[4:]
		case strings.HasPrefix(line, "event: "):
			cur.event = line[7:]
		case strings.HasPrefix(line, "data: "):
			require.NoError(t, json.Unmarshal([]byte(line[6:]), &cur.data))
		case line == "" && cur.event != "":
			out = append(out, cur)
			if cur.event == "done" {
				return out
			}
			cur = sse{}
		}
	}
	return out
}

func eventTypes(evs []sse) []string {
	var out []string
	for _, e := range evs {
		out = append(out, e.event)
	}
	return out
}

func TestAIBuilderAgent(t *testing.T) {
	setup()
	jwt := createValidJWTForIdentity("aibuilder-agent-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)
	name := fmt.Sprintf("agent-%d", time.Now().UnixNano())
	mcpJSON(t, jwt, "project_create", map[string]any{"name": name})

	world := `<?xml version="1.0"?><sdf version="1.9"><world name="w"><model name="box"><static>true</static>` +
		`<link name="l"><visual name="v"><geometry><box><size>1 1 1</size></box></geometry></visual></link>` +
		`</model></world></sdf>`
	fake := &scriptedLLM{release: make(chan struct{}), rounds: [][]llm.Chunk{
		// Turn 1: write the world, then reply.
		{{Type: llm.ChunkText, Text: "Building "}, {Type: llm.ChunkText, Text: "it."},
			{Type: llm.ChunkToolCall, ToolCall: &llm.ToolCall{ID: "t1", Name: "project_write_file",
				Input: map[string]any{"name": name, "path": "world.sdf", "content": world}}}},
		{{Type: llm.ChunkText, Text: "Done: one box."}},
		// Turn 2: a tool error the model sees, then a reply.
		{{Type: llm.ChunkToolCall, ToolCall: &llm.ToolCall{ID: "t2", Name: "project_read_file",
			Input: map[string]any{"name": name, "path": "missing.sdf"}}}},
		{{Type: llm.ChunkText, Text: "That file doesn't exist."}},
		// Turn 3: blocks until cancelled.
		{{Type: llm.ChunkText, Text: "Thinking"}, {Type: llm.ChunkText, Text: "BLOCK"}},
	}}
	providers := llm.NewRegistry()
	providers.Register("scripted", func(string, string) (llm.Provider, error) { return fake, nil })
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	enc, err := aibuilder.NewEncryptor(key)
	require.NoError(t, err)
	srv, err := aibuilder.NewServer(globals.Server.Db, aibuilder.Config{
		Encryptor: enc, Providers: providers, Tools: aitools.NewRegistry(aitools.Config{}),
		Auth: aibuilder.GzAuth(globals.Server.Db, globals.Server),
	})
	require.NoError(t, err)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	do := func(method, path string, body any, header ...string) *http.Response {
		t.Helper()
		var b []byte
		if body != nil {
			b, _ = json.Marshal(body)
		}
		req, _ := http.NewRequest(method, ts.URL+"/1.0/ai-builder"+path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+jwt)
		for i := 0; i+1 < len(header); i += 2 {
			req.Header.Set(header[i], header[i+1])
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}
	jsonOf := func(resp *http.Response) map[string]any {
		t.Helper()
		defer resp.Body.Close()
		var m map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
		return m
	}

	// No key yet.
	resp := do("POST", "/projects/"+name+"/chat", map[string]any{"message": "hi"})
	assert.Equal(t, http.StatusPreconditionRequired, resp.StatusCode)

	// Keys: only your own, stored encrypted, listed masked.
	resp = do("POST", "/users/someone-else/llm-keys", map[string]any{"provider": "scripted", "api_key": "x"})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp = do("POST", "/users/"+username+"/llm-keys", map[string]any{"provider": "nope", "api_key": "x"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp = do("POST", "/users/"+username+"/llm-keys", map[string]any{"provider": "scripted", "api_key": "sk-secret-1234"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "…1234", jsonOf(resp)["api_key_masked"])
	var stored aibuilder.LLMKey
	require.NoError(t, globals.Server.Db.Where("owner = ?", username).First(&stored).Error)
	assert.NotContains(t, stored.Encrypted, "sk-secret")
	resp = do("GET", "/users/"+username+"/llm-keys", nil)
	var keys []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&keys))
	require.Len(t, keys, 1)
	assert.NotContains(t, fmt.Sprint(keys), "sk-secret")
	resp = do("GET", "/providers/scripted/models", nil)
	assert.Contains(t, fmt.Sprint(jsonOf(resp)["models"]), "scripted-1")

	// Turn 1: the tool runs in-process as the user and the world changes.
	resp = do("POST", "/projects/"+name+"/chat", map[string]any{"message": "build a box"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	turn1 := resp.Header.Get("X-Turn-ID")
	evs := readSSE(t, resp)
	assert.Equal(t, []string{"text", "tool_call", "tool_result", "project_changed", "text", "done"}, eventTypes(evs))
	assert.Equal(t, "Building it.", evs[0].data["text"], "text pieces are stored together")
	assert.Equal(t, "world.sdf", evs[3].data["path"])
	assert.Equal(t, "done", evs[5].data["status"])
	text, isErr := mcpCall(t, jwt, "project_read_file", map[string]any{"name": name, "path": "world.sdf"})
	require.False(t, isErr, text)
	assert.Contains(t, text, `<box><size>1 1 1</size></box>`)

	// The project downloads as a zip (fuel-server's route).
	zr := aiRequest(t, "GET", "/1.0/projects/"+name+"/export.zip", jwt, nil)
	require.Equal(t, http.StatusOK, zr.Code, zr.Body.String())
	zf, err := zip.NewReader(bytes.NewReader(zr.Body.Bytes()), int64(zr.Body.Len()))
	require.NoError(t, err)
	var zipped []string
	for _, f := range zf.File {
		zipped = append(zipped, f.Name)
	}
	assert.Contains(t, zipped, name+"/world.sdf")

	// The stream resumes after an event id.
	resp = do("GET", "/projects/"+name+"/chat/stream?turn="+turn1, nil, "Last-Event-ID", evs[3].id)
	again := readSSE(t, resp)
	assert.Equal(t, []string{"text", "done"}, eventTypes(again))

	// Turn 2: a tool's error goes back to the model, not to the stream.
	resp = do("POST", "/projects/"+name+"/chat", map[string]any{"message": "read missing.sdf", "provider": "scripted"})
	evs = readSSE(t, resp)
	assert.Equal(t, []string{"tool_call", "tool_result", "text", "done"}, eventTypes(evs))
	assert.Contains(t, fmt.Sprint(evs[1].data["result"]), "file not found")

	// Turn 3 runs in the background: leaving its stream doesn't stop it,
	// a second message waits, the history shows it, and cancel stops it.
	resp = do("POST", "/projects/"+name+"/chat", map[string]any{"message": "think"})
	turn3 := resp.Header.Get("X-Turn-ID")
	resp.Body.Close()
	assert.Eventually(t, func() bool {
		h := jsonOf(do("GET", "/projects/"+name+"/chat/history", nil))
		active, _ := h["active_turn"].(map[string]any)
		if active == nil || active["turn_id"] != turn3 {
			return false
		}
		// The running turn's messages are left out of the history.
		assert.Equal(t, "think", active["message"])
		assert.Len(t, h["messages"], 8)
		return true
	}, 5*time.Second, 50*time.Millisecond)
	resp = do("POST", "/projects/"+name+"/chat", map[string]any{"message": "again"})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	resp = do("DELETE", "/projects/"+name+"/chat/history", nil)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	resp = do("POST", "/projects/"+name+"/chat/"+turn3+"/cancel", nil)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp = do("GET", "/projects/"+name+"/chat/stream", nil)
	evs = readSSE(t, resp)
	require.NotEmpty(t, evs)
	last := evs[len(evs)-1]
	assert.Equal(t, "done", last.event)
	assert.Equal(t, "cancelled", last.data["status"])
	assert.Equal(t, "Thinking", evs[0].data["text"])

	// The history: user, assistant and tool messages of every turn.
	h := jsonOf(do("GET", "/projects/"+name+"/chat/history", nil))
	assert.Nil(t, h["active_turn"])
	var roles []string
	for _, m := range h["messages"].([]any) {
		roles = append(roles, m.(map[string]any)["role"].(string))
	}
	assert.Equal(t, []string{"user", "assistant", "tool_result", "assistant",
		"user", "assistant", "tool_result", "assistant", "user"}, roles)

	// Someone else's project isn't found.
	other := createValidJWTForIdentity("aibuilder-agent-other", t)
	otherName := createUserWithJWT(other, t)
	defer removeUserWithJWT(otherName, other, t)
	req, _ := http.NewRequest("GET", ts.URL+"/1.0/ai-builder/projects/"+name+"/chat/history", nil)
	req.Header.Set("Authorization", "Bearer "+other)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// No login, no access.
	resp, err = http.Get(ts.URL + "/1.0/ai-builder/projects/" + name + "/chat/history")
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	resp = do("DELETE", "/projects/"+name+"/chat/history", nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	h = jsonOf(do("GET", "/projects/"+name+"/chat/history", nil))
	assert.Empty(t, h["messages"])
	resp = do("DELETE", "/users/"+username+"/llm-keys/scripted", nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}
