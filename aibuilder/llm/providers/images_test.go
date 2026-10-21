package providers

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gazebo-web/fuel-server/aibuilder/llm"
)

// A conversation with an attached picture and a tool result that returns
// one (a generated model's preview).
func imageRequest() llm.Request {
	photo := llm.Image{MimeType: "image/jpeg", Data: "UEhPVE8="}
	preview := llm.Image{MimeType: "image/png", Data: "UFJFVklFVw=="}
	return llm.Request{Model: "m", Messages: []llm.Message{
		{Role: "user", Content: "make this mug", Images: []llm.Image{photo}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "t1", Name: "model_generate", Input: map[string]any{"name": "mug"}}}},
		{Role: "user", ToolResults: []llm.ToolResult{{ToolCallID: "t1",
			Content: llm.RichResult{Value: map[string]any{"model": "mug"}, Images: []llm.Image{preview}}}}},
	}}
}

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m), string(b))
	return m
}

func TestAnthropicImages(t *testing.T) {
	b, err := buildAnthropicRequest(imageRequest())
	require.NoError(t, err)
	msgs := decode(t, b)["messages"].([]any)
	first := msgs[0].(map[string]any)["content"].([]any)
	assert.Equal(t, "image", first[0].(map[string]any)["type"])
	assert.Equal(t, map[string]any{"type": "base64", "media_type": "image/jpeg", "data": "UEhPVE8="},
		first[0].(map[string]any)["source"])
	assert.Equal(t, "make this mug", first[1].(map[string]any)["text"])
	// The preview stays inside the tool result.
	assert.Contains(t, string(b), `"media_type":"image/png"`)
	assert.Len(t, msgs, 3)
}

func TestOpenAIImages(t *testing.T) {
	b, err := buildOpenAIRequest(imageRequest())
	require.NoError(t, err)
	msgs := decode(t, b)["messages"].([]any)
	require.Len(t, msgs, 4) // user, assistant, tool, then the preview as a user message
	first := msgs[0].(map[string]any)["content"].([]any)
	assert.Equal(t, "make this mug", first[0].(map[string]any)["text"])
	assert.Equal(t, "data:image/jpeg;base64,UEhPVE8=", first[1].(map[string]any)["image_url"].(map[string]any)["url"])
	assert.Nil(t, msgs[1].(map[string]any)["content"], "no empty content on a tool-call message")
	assert.Equal(t, "tool", msgs[2].(map[string]any)["role"])
	last := msgs[3].(map[string]any)
	assert.Equal(t, "user", last["role"])
	parts := last["content"].([]any)
	assert.Equal(t, llm.ToolImagesNote, parts[0].(map[string]any)["text"])
	assert.Equal(t, "data:image/png;base64,UFJFVklFVw==", parts[1].(map[string]any)["image_url"].(map[string]any)["url"])
}

func TestGeminiImages(t *testing.T) {
	b, err := buildGeminiRequest(imageRequest())
	require.NoError(t, err)
	contents := decode(t, b)["contents"].([]any)
	first := contents[0].(map[string]any)["parts"].([]any)
	assert.Equal(t, map[string]any{"mimeType": "image/jpeg", "data": "UEhPVE8="}, first[0].(map[string]any)["inlineData"])
	results := contents[2].(map[string]any)["parts"].([]any)
	require.Len(t, results, 3) // the function response, the note, the preview
	assert.NotNil(t, results[0].(map[string]any)["functionResponse"])
	assert.Equal(t, "image/png", results[2].(map[string]any)["inlineData"].(map[string]any)["mimeType"])
}

func TestOllamaImages(t *testing.T) {
	b, err := buildOllamaRequest(imageRequest())
	require.NoError(t, err)
	msgs := decode(t, b)["messages"].([]any)
	require.Len(t, msgs, 4)
	assert.Equal(t, []any{"UEhPVE8="}, msgs[0].(map[string]any)["images"])
	assert.Equal(t, []any{"UFJFVklFVw=="}, msgs[3].(map[string]any)["images"])
}
