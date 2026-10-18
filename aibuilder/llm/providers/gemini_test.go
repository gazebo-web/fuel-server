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

func TestGeminiStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "streamGenerateContent")
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, sseScript(
			`{"candidates":[{"content":{"parts":[{"text":"Hello"}]}}]}`,
			`{"candidates":[{"content":{"parts":[{"text":" world"}]},"finishReason":"STOP"}]}`,
		))
	}))
	defer server.Close()

	p, err := NewGemini("key", server.URL)
	require.NoError(t, err)
	ch, err := p.ChatStream(context.Background(), llm.Request{Model: "gemini-2.0-flash", Messages: []llm.Message{{Role: "user", Content: "hi"}}})
	require.NoError(t, err)

	var out strings.Builder
	for c := range ch {
		if c.Type == llm.ChunkText {
			out.WriteString(c.Text)
		}
	}
	assert.Equal(t, "Hello world", out.String())
}
