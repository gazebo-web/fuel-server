package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gazebo-web/fuel-server/aibuilder"
	"github.com/gazebo-web/fuel-server/aibuilder/llm"
	"github.com/gazebo-web/fuel-server/aitools"
	"github.com/gazebo-web/fuel-server/bootstrap"
	"github.com/gazebo-web/fuel-server/globals"
)

// recordingLLM replies "ok" and keeps every request.
type recordingLLM struct {
	mu   sync.Mutex
	reqs []llm.Request
}

func (p *recordingLLM) Name() string          { return "recording" }
func (p *recordingLLM) SupportsToolUse() bool { return true }
func (p *recordingLLM) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}
func (p *recordingLLM) ChatStream(_ context.Context, req llm.Request) (<-chan llm.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()
	out := make(chan llm.Chunk, 1)
	out <- llm.Chunk{Type: llm.ChunkText, Text: "ok"}
	close(out)
	return out, nil
}

// Attached project files reach the model: pictures with the message, and
// a note naming them in later turns.
func TestAIBuilderAttachments(t *testing.T) {
	setup()
	jwt := createValidJWTForIdentity("aibuilder-attach-identity", t)
	username := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(username, jwt, t)
	name := fmt.Sprintf("attach-%d", time.Now().UnixNano())
	mcpJSON(t, jwt, "project_create", map[string]any{"name": name})
	rec := uploadAssets(t, jwt, name, map[string][]byte{"mug.png": assetTestPNG(t, 300, 200),
		"mug.glb": append([]byte("glTF\x02\x00\x00\x00"), make([]byte, 16)...)}, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	fake := &recordingLLM{}
	providers := llm.NewRegistry()
	providers.Register("recording", func(string, string) (llm.Provider, error) { return fake, nil })
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	enc, err := aibuilder.NewEncryptor(key)
	require.NoError(t, err)
	srv, err := aibuilder.NewServer(globals.Server.Db, aibuilder.Config{
		Encryptor: enc, Providers: providers, Tools: aitools.NewRegistry(aitools.Config{}),
		Auth: aibuilder.Authenticate(globals.Server.Db, bootstrap.Verifier()),
	})
	require.NoError(t, err)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	do := func(method, path string, body any) *http.Response {
		t.Helper()
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, ts.URL+"/1.0/ai-builder"+path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+jwt)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}
	resp := do("POST", "/users/"+username+"/llm-keys", map[string]any{"provider": "recording", "api_key": "k"})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = do("POST", "/projects/"+name+"/chat", map[string]any{"message": "make this", "attachments": []string{"nope.png"}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	resp = do("POST", "/projects/"+name+"/chat", map[string]any{"message": "make this mug",
		"attachments": []string{"mug.png", "mug.glb"}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	readSSE(t, resp)
	require.Len(t, fake.reqs, 1)
	last := fake.reqs[0].Messages[len(fake.reqs[0].Messages)-1]
	require.Len(t, last.Images, 1, "only the image has a picture")
	assert.Equal(t, "image/jpeg", last.Images[0].MimeType)
	assert.Contains(t, last.Content, "make this mug")
	assert.Contains(t, last.Content, "mug.png (id ")
	assert.Contains(t, last.Content, "image, 300×200 px")
	assert.Contains(t, last.Content, "mug.glb (id ")

	// A message may be only attachments; the earlier one is named, not resent.
	resp = do("POST", "/projects/"+name+"/chat", map[string]any{"attachments": []string{"mug.glb"}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	readSSE(t, resp)
	require.Len(t, fake.reqs, 2)
	first := fake.reqs[1].Messages[0]
	assert.Empty(t, first.Images)
	assert.Contains(t, first.Content, "mug.png (id ")

	resp = do("GET", "/projects/"+name+"/chat/history", nil)
	var h struct {
		Messages []aibuilder.ChatMessage `json:"messages"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&h))
	resp.Body.Close()
	require.NotEmpty(t, h.Messages)
	assert.Equal(t, "mug.png", h.Messages[0].Attachments[0].Name)
	assert.Equal(t, "make this mug", h.Messages[0].Content)
}
