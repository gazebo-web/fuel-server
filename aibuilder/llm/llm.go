// Package llm provides a provider-agnostic interface for LLM chat completions
// with tool-use and streaming.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
)

// Message is one turn in a conversation.
type Message struct {
	Role        string       `json:"role"` // "user" | "assistant" | "system"
	Content     string       `json:"content,omitempty"`
	ToolCalls   []ToolCall   `json:"tool_calls,omitempty"`
	ToolResults []ToolResult `json:"tool_results,omitempty"`
	// Images are pictures sent with a user message: the files a user
	// attached. Providers send them beside Content.
	Images []Image `json:"-"`
	// Native is an assistant message's content in the provider's own
	// format, as streamed (ChunkNative). The provider named by
	// NativeProvider replays it verbatim instead of rebuilding the
	// message from Content and ToolCalls: Anthropic requires thinking
	// blocks, with their signatures, to be passed back unchanged in a
	// tool-use loop. It lives only for the turn and is never persisted.
	Native         json.RawMessage `json:"-"`
	NativeProvider string          `json:"-"`
}

// Request is a chat completion request.
type Request struct {
	Model        string
	SystemPrompt string
	// CachedPrefix is a large, mostly-static prefix that should be
	// delivered before SystemPrompt and marked cacheable where the
	// provider supports it (Anthropic ephemeral cache_control,
	// Gemini / OpenAI automatic prefix caching). Used by the AI Builder
	// to ship the SDF schema on every turn without re-paying its token
	// cost. Empty string disables the mechanism.
	CachedPrefix string
	Messages     []Message
	Tools        []Tool
	MaxTokens    int
	Temperature  float64
	// Effort is the reasoning effort ("low", "medium", "high", ...) for
	// providers and models that support it; "" uses the provider default.
	Effort string
}

// ChunkType classifies the kind of data in a streaming chunk.
type ChunkType string

const (
	ChunkText     ChunkType = "text"
	ChunkToolCall ChunkType = "tool_call"
	ChunkStop     ChunkType = "stop"
	ChunkError    ChunkType = "error"
	// ChunkNative carries the whole assistant message in the provider's
	// format (Native), sent once at the end of a response.
	ChunkNative ChunkType = "native"
	// ChunkUsage carries the response's token usage.
	ChunkUsage ChunkType = "usage"
)

// Chunk is one event in a streaming response.
type Chunk struct {
	Type       ChunkType       `json:"type"`
	Text       string          `json:"text,omitempty"`
	ToolCall   *ToolCall       `json:"tool_call,omitempty"`
	Err        string          `json:"error,omitempty"`
	StopReason string          `json:"stop_reason,omitempty"`
	Native     json.RawMessage `json:"native,omitempty"`
	Usage      *Usage          `json:"usage,omitempty"`
}

// Usage is the token usage of one LLM response. Input counts uncached
// prompt tokens; Output includes Thinking.
type Usage struct {
	Input      int `json:"input_tokens"`
	CacheWrite int `json:"cache_write_tokens,omitempty"`
	CacheRead  int `json:"cache_read_tokens,omitempty"`
	Output     int `json:"output_tokens"`
	Thinking   int `json:"thinking_tokens,omitempty"`
}

// Add accumulates u into the receiver.
func (t *Usage) Add(u Usage) {
	t.Input += u.Input
	t.CacheWrite += u.CacheWrite
	t.CacheRead += u.CacheRead
	t.Output += u.Output
	t.Thinking += u.Thinking
}

// JSON marshals v without HTML escaping, so SDF in tool inputs and
// results reaches the model as <model>, not \u003cmodel\u003e.
func JSON(v interface{}) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// ModelInfo describes one model returned by Provider.ListModels. ID is what
// callers pass back in Request.Model; DisplayName is the provider-supplied
// human-readable label (or a best-effort beautification of ID if the
// upstream API does not offer one).
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// Provider is an LLM backend (Anthropic, OpenAI, Gemini, Ollama).
type Provider interface {
	// Name returns the canonical provider identifier.
	Name() string
	// ChatStream streams chunks for a request. The channel is closed when the
	// stream ends. The caller must drain it to avoid leaks.
	ChatStream(ctx context.Context, req Request) (<-chan Chunk, error)
	// SupportsToolUse reports whether the provider can invoke tools.
	SupportsToolUse() bool
	// ListModels queries the provider's API for its available chat/completion
	// models.
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

// ErrUnknownProvider is returned when a provider name is not registered.
var ErrUnknownProvider = errors.New("unknown LLM provider")

// Registry maps provider names to factories.
type Registry struct {
	factories map[string]func(apiKey, endpoint string) (Provider, error)
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]func(apiKey, endpoint string) (Provider, error))}
}

// Register associates a provider name with a factory.
func (r *Registry) Register(name string, factory func(apiKey, endpoint string) (Provider, error)) {
	r.factories[name] = factory
}

// Build instantiates a provider by name.
func (r *Registry) Build(name, apiKey, endpoint string) (Provider, error) {
	f, ok := r.factories[name]
	if !ok {
		return nil, ErrUnknownProvider
	}
	return f(apiKey, endpoint)
}

// Providers returns the registered provider names.
func (r *Registry) Providers() []string {
	out := make([]string, 0, len(r.factories))
	for n := range r.factories {
		out = append(out, n)
	}
	return out
}

// Tool describes a function the LLM can call.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ToolCall is a request from the LLM to invoke a tool.
type ToolCall struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`

	// ProviderMetadata carries provider-specific round-trip state that must
	// be echoed back verbatim on the follow-up request. Gemini's 2.5+
	// "thinking" models, for example, attach a thoughtSignature to each
	// functionCall part that their API refuses to accept if missing on the
	// next turn. Other providers may leave this empty.
	ProviderMetadata map[string]string `json:"provider_metadata,omitempty"`
}

// Image is an image a tool returned, e.g. a generated model's preview.
// Data is base64.
type Image struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

// RichResult is a tool result that carries images beside its value.
// Anthropic sends both in the tool result; other providers send the value
// and then the images in a user message (ToolResultImages). As JSON, e.g.
// in the stored chat history, it is the value with a count of its images.
type RichResult struct {
	Value  interface{}
	Images []Image
}

// ToolResultImages are the images in a message's tool results, for
// providers whose tool results hold only text: they send them in a user
// message after the results instead.
func ToolResultImages(results []ToolResult) []Image {
	var out []Image
	for _, tr := range results {
		if rich, ok := tr.Content.(RichResult); ok {
			out = append(out, rich.Images...)
		}
	}
	return out
}

// ToolImagesNote introduces images moved out of tool results.
const ToolImagesNote = "Images returned by the tool calls above:"

// DataURL is an image as a data: URL.
func (i Image) DataURL() string { return "data:" + i.MimeType + ";base64," + i.Data }

// MarshalJSON is the text-only form.
func (r RichResult) MarshalJSON() ([]byte, error) {
	if m, ok := r.Value.(map[string]interface{}); ok && len(r.Images) > 0 {
		cp := make(map[string]interface{}, len(m)+1)
		for k, v := range m {
			cp[k] = v
		}
		cp["images"] = len(r.Images)
		return json.Marshal(cp)
	}
	return json.Marshal(r.Value)
}

// ToolResult is the outcome of a tool invocation returned to the LLM.
type ToolResult struct {
	ToolCallID string      `json:"tool_call_id"`
	Content    interface{} `json:"content"`
	IsError    bool        `json:"is_error,omitempty"`
}
