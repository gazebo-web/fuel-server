package providers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gazebo-web/fuel-server/aibuilder/llm"
)

func TestOllamaStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat", r.URL.Path)
		// Ollama emits newline-delimited JSON, not SSE.
		io.WriteString(w, `{"message":{"content":"Hi "}}`+"\n")
		io.WriteString(w, `{"message":{"content":"there"}}`+"\n")
		io.WriteString(w, `{"message":{"content":""},"done":true}`+"\n")
	}))
	defer server.Close()

	p, err := NewOllama("", server.URL)
	require.NoError(t, err)
	ch, err := p.ChatStream(context.Background(), llm.Request{
		Model:    "llama3.1:8b",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)

	var out strings.Builder
	var stopped bool
	for c := range ch {
		if c.Type == llm.ChunkText {
			out.WriteString(c.Text)
		}
		if c.Type == llm.ChunkStop {
			stopped = true
		}
	}
	assert.Equal(t, "Hi there", out.String())
	assert.True(t, stopped)
}

func TestOllamaRequiresModel(t *testing.T) {
	p, _ := NewOllama("", "http://localhost:11434")
	_, err := p.ChatStream(context.Background(), llm.Request{})
	assert.Error(t, err)
}
