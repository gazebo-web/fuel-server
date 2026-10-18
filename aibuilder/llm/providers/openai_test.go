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

func TestOpenAITextStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, sseScript(
			`{"choices":[{"delta":{"content":"Hello"}}]}`,
			`{"choices":[{"delta":{"content":", world!"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		))
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p, err := NewOpenAI("test", server.URL)
	require.NoError(t, err)
	ch, err := p.ChatStream(context.Background(), llm.Request{Messages: []llm.Message{{Role: "user", Content: "hi"}}})
	require.NoError(t, err)

	var text strings.Builder
	for c := range ch {
		if c.Type == llm.ChunkText {
			text.WriteString(c.Text)
		}
	}
	assert.Equal(t, "Hello, world!", text.String())
}

func TestOpenAIToolCallStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, sseScript(
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"t1","function":{"name":"fuel_search_models","arguments":"{\"query\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"prius\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		))
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p, _ := NewOpenAI("test", server.URL)
	ch, _ := p.ChatStream(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: "user", Content: "find"}},
		Tools:    []llm.Tool{{Name: "fuel_search_models", Description: "x", InputSchema: map[string]interface{}{"type": "object"}}},
	})
	var call *llm.ToolCall
	for c := range ch {
		if c.Type == llm.ChunkToolCall {
			call = c.ToolCall
		}
	}
	require.NotNil(t, call)
	assert.Equal(t, "fuel_search_models", call.Name, "dot in tool name should be restored")
	assert.Equal(t, "prius", call.Input["query"])
}

func TestOpenAIBuildRequestInjectsSystem(t *testing.T) {
	body, err := buildOpenAIRequest(llm.Request{
		SystemPrompt: "sys",
		Messages:     []llm.Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
	assert.Contains(t, string(body), `"role":"system"`)
	assert.Contains(t, string(body), `"content":"sys"`)
}
