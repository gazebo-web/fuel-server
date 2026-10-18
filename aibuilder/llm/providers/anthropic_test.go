package providers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gazebo-web/fuel-server/aibuilder/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sseScript builds an SSE response emulating Anthropic's stream protocol.
func sseScript(events ...string) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString("data: ")
		b.WriteString(e)
		b.WriteString("\n\n")
	}
	return b.String()
}

func TestAnthropicChatStreamText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.Header.Get("x-api-key"))
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, sseScript(
			`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", world!"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			`{"type":"message_stop"}`,
		))
	}))
	defer server.Close()

	p, err := NewAnthropic("test-key", server.URL)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := p.ChatStream(ctx, llm.Request{
		Model:    "claude-sonnet-4-5",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)

	var text strings.Builder
	sawStop := false
	for chunk := range ch {
		switch chunk.Type {
		case llm.ChunkText:
			text.WriteString(chunk.Text)
		case llm.ChunkStop:
			sawStop = true
		}
	}
	assert.Equal(t, "Hello, world!", text.String())
	assert.True(t, sawStop)
}

func TestAnthropicChatStreamToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, sseScript(
			`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t1","name":"search_models"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"prius\"}"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
			`{"type":"message_stop"}`,
		))
	}))
	defer server.Close()

	p, _ := NewAnthropic("test-key", server.URL)
	ch, err := p.ChatStream(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: "user", Content: "add a prius"}},
		Tools:    []llm.Tool{{Name: "search_models", Description: "x", InputSchema: map[string]interface{}{"type": "object"}}},
	})
	require.NoError(t, err)

	var gotCall *llm.ToolCall
	for chunk := range ch {
		if chunk.Type == llm.ChunkToolCall {
			gotCall = chunk.ToolCall
		}
	}
	require.NotNil(t, gotCall)
	assert.Equal(t, "search_models", gotCall.Name)
	assert.Equal(t, "prius", gotCall.Input["query"])
}

func TestAnthropicErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"invalid api key"}`)
	}))
	defer server.Close()

	p, _ := NewAnthropic("bad-key", server.URL)
	_, err := p.ChatStream(context.Background(), llm.Request{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestAnthropicMissingKey(t *testing.T) {
	_, err := NewAnthropic("", "")
	assert.Error(t, err)
}

func TestAnthropicMaxTokensPerModel(t *testing.T) {
	body, err := buildAnthropicRequest(llm.Request{Model: "claude-sonnet-5-5"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"max_tokens":64000`) {
		t.Errorf("current model: %s", body)
	}
	if got := anthropicMaxTokens("claude-3-5-haiku-latest"); got != 8192 {
		t.Errorf("claude-3 model got %d", got)
	}
}

// A tool call with no arguments must be replayed with "input": {};
// Anthropic rejects tool_use blocks without input.
func TestAnthropicReplaysEmptyToolInput(t *testing.T) {
	body, err := buildAnthropicRequest(llm.Request{Messages: []llm.Message{
		{Role: "user", Content: "list my projects"},
		{Role: "assistant", Content: "Checking.", ToolCalls: []llm.ToolCall{{ID: "t1", Name: "project_list"}}},
		{Role: "user", ToolResults: []llm.ToolResult{{ToolCallID: "t1", Content: "[]"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, `"type":"tool_use","id":"t1","name":"project_list","input":{}`) {
		t.Errorf("tool_use without input: %s", s)
	}
	if strings.Contains(s, `"type":"text","text":"Checking.","input"`) {
		t.Errorf("text block carries input: %s", s)
	}
}

// Structured tool results go as JSON text: Anthropic would read a JSON
// array as content blocks and reject them for lacking "type".
func TestAnthropicToolResultIsText(t *testing.T) {
	body, err := buildAnthropicRequest(llm.Request{Messages: []llm.Message{
		{Role: "user", Content: "list"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "t1", Name: "project_list"}}},
		{Role: "user", ToolResults: []llm.ToolResult{{ToolCallID: "t1", Content: []interface{}{map[string]interface{}{"name": "p"}}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"tool_use_id":"t1","content":"[{\"name\":\"p\"}]"`) {
		t.Errorf("tool_result content not JSON text: %s", body)
	}
}

// Thinking blocks must be passed back unchanged in a tool loop, so the
// stream yields the whole message, in order, with signatures.
func TestAnthropicStreamNativeAndUsage(t *testing.T) {
	r := strings.NewReader(sseScript(
		`{"type":"message_start","message":{"usage":{"input_tokens":100,"cache_creation_input_tokens":5,"cache_read_input_tokens":900,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"SIG=="}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"redacted_thinking","data":"ENC"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"Writing <world>."}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"t1","name":"project_write_file","input":{}}}`,
		`{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{\"content\":\"<sdf/>\"}"}}`,
		`{"type":"content_block_stop","index":3}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":700,"output_tokens_details":{"thinking_tokens":600}}}`,
		`{"type":"message_stop"}`,
	))
	out := make(chan llm.Chunk, 32)
	parseAnthropicStream(r, out, "claude-sonnet-5-5")
	close(out)

	var native string
	var usage *llm.Usage
	var call *llm.ToolCall
	for c := range out {
		switch c.Type {
		case llm.ChunkNative:
			native = string(c.Native)
		case llm.ChunkUsage:
			usage = c.Usage
		case llm.ChunkToolCall:
			call = c.ToolCall
		}
	}
	assert.Equal(t, `[{"signature":"SIG==","thinking":"","type":"thinking"},`+
		`{"type":"redacted_thinking","data":"ENC"},`+
		`{"text":"Writing <world>.","type":"text"},`+
		`{"id":"t1","input":{"content":"<sdf/>"},"name":"project_write_file","type":"tool_use"}]`, native)
	require.NotNil(t, usage)
	assert.Equal(t, llm.Usage{Input: 100, CacheWrite: 5, CacheRead: 900, Output: 700, Thinking: 600}, *usage)
	require.NotNil(t, call)
	assert.Equal(t, "<sdf/>", call.Input["content"])
}

func TestAnthropicReplaysNativeAndCachesHistory(t *testing.T) {
	native := []byte(`[{"type":"thinking","thinking":"","signature":"SIG=="},{"type":"tool_use","id":"t1","name":"scene_summary","input":{}}]`)
	msgs := []llm.Message{
		{Role: "user", Content: "build"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "t1", Name: "scene_summary"}}, Native: native, NativeProvider: "anthropic"},
		{Role: "user", ToolResults: []llm.ToolResult{{ToolCallID: "t1", Content: map[string]interface{}{"sdf": "<model/>"}}}},
	}
	body, err := buildAnthropicRequest(llm.Request{Model: "claude-sonnet-5-5", Effort: "medium", Messages: msgs})
	require.NoError(t, err)
	s := string(body)
	assert.Contains(t, s, `{"role":"assistant","content":`+string(native)+`}`)
	assert.Contains(t, s, `"content":"{\"sdf\":\"<model/>\"}","cache_control":{"type":"ephemeral"}}]}]`)
	assert.Equal(t, 1, strings.Count(s, `"cache_control"`), "only the newest block is a breakpoint")
	assert.Contains(t, s, `"output_config":{"effort":"medium"}`)
	assert.NotContains(t, s, `\u003c`)

	// Another provider's native content is ignored, and older models get
	// no effort.
	msgs[1].NativeProvider = "openai"
	body, err = buildAnthropicRequest(llm.Request{Model: "claude-3-5-haiku-latest", Effort: "medium", Messages: msgs})
	require.NoError(t, err)
	s = string(body)
	assert.NotContains(t, s, "SIG==")
	assert.Contains(t, s, `"type":"tool_use","id":"t1","name":"scene_summary","input":{}`)
	assert.NotContains(t, s, "output_config")
}

// A tool result with images goes to Anthropic as text plus image blocks.
func TestAnthropicToolResultImages(t *testing.T) {
	body, err := buildAnthropicRequest(llm.Request{Messages: []llm.Message{
		{Role: "user", Content: "make a cone"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "t1", Name: "model_generate"}}},
		{Role: "user", ToolResults: []llm.ToolResult{{ToolCallID: "t1", Content: llm.RichResult{
			Value:  map[string]interface{}{"uri": "model://cone"},
			Images: []llm.Image{{MimeType: "image/png", Data: "iVBOR"}},
		}}}},
	}})
	require.NoError(t, err)
	assert.Contains(t, string(body), `"content":[{"text":"{\"uri\":\"model://cone\"}","type":"text"},{"source":{"data":"iVBOR","media_type":"image/png","type":"base64"},"type":"image"}]`)
}
