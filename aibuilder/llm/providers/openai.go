package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/gazebo-web/fuel-server/aibuilder/llm"
)

const openAIDefaultBase = "https://api.openai.com"

// OpenAI is the Provider implementation for OpenAI chat completions.
type OpenAI struct {
	apiKey string
	base   string
	hc     *http.Client
}

// NewOpenAI returns an OpenAI provider.
func NewOpenAI(apiKey, base string) (*OpenAI, error) {
	if apiKey == "" {
		return nil, errors.New("openai: api key required")
	}
	if base == "" {
		base = openAIDefaultBase
	}
	return &OpenAI{apiKey: apiKey, base: base, hc: http.DefaultClient}, nil
}

// Name returns "openai".
func (o *OpenAI) Name() string { return "openai" }

// SupportsToolUse returns true.
func (o *OpenAI) SupportsToolUse() bool { return true }

// ListModels queries GET /v1/models and returns the chat-capable models.
// OpenAI's list endpoint does not ship human-readable display names, so we
// beautify the id (e.g. "gpt-4o" -> "Gpt 4o") as a fallback.
func (o *OpenAI) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.base+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	resp, err := o.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai: list models status %d: %s", resp.StatusCode, string(b))
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]llm.ModelInfo, 0, len(body.Data))
	for _, m := range body.Data {
		if isOpenAIChatModel(m.ID) {
			out = append(out, llm.ModelInfo{ID: m.ID, DisplayName: humanizeID(m.ID)})
		}
	}
	return out, nil
}

func isOpenAIChatModel(id string) bool {
	// Narrow to chat/reasoning families first.
	prefixOK := strings.HasPrefix(id, "gpt-") ||
		strings.HasPrefix(id, "o1") ||
		strings.HasPrefix(id, "o3") ||
		strings.HasPrefix(id, "o4") ||
		strings.HasPrefix(id, "chatgpt-")
	if !prefixOK {
		return false
	}
	// Non-chat specialisations that share the gpt-/o-* prefix.
	for _, s := range []string{
		"audio", "realtime", "tts", "transcribe",
		"search", "vision",
		"image", "moderation",
		"instruct",
		"embedding",
	} {
		if strings.Contains(id, s) {
			return false
		}
	}
	// Preview / experimental snapshots and date-stamped checkpoints.
	if strings.Contains(id, "preview") {
		return false
	}
	if openaiDatedSnapshotRe.MatchString(id) {
		return false
	}
	return true
}

// openaiDatedSnapshotRe matches `-MMDD`, `-YYYYMMDD`, or `-YYYY-MM-DD` tails
// that OpenAI uses for pinned model snapshots (e.g. gpt-4o-2024-08-06,
// gpt-4-0613). We prefer the unpinned alias.
var openaiDatedSnapshotRe = regexp.MustCompile(`-(\d{4}(-\d{2}-\d{2})?|\d{8})$`)

type openAIToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters"`
	} `json:"function"`
}

type openAIMessage struct {
	Role string `json:"role"`
	// Content is a string, or a list of parts for a message with images.
	Content    interface{}      `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Tools    []openAIToolDef `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
	// StreamOptions.IncludeUsage makes OpenAI emit a final chunk that
	// carries the `usage` block (including prompt_tokens_details.
	// cached_tokens). Without this opt-in, streaming calls never report
	// usage. Journal Q-MCP-13.
	StreamOptions *openAIStreamOptions `json:"stream_options,omitempty"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// ChatStream calls /v1/chat/completions with stream=true.
func (o *OpenAI) ChatStream(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
	body, err := buildOpenAIRequest(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.base+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.hc.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai: status %d: %s", resp.StatusCode, string(b))
	}
	model := req.Model
	if model == "" {
		model = "gpt-4o"
	}
	out := make(chan llm.Chunk, 16)
	go func() {
		defer resp.Body.Close()
		defer close(out)
		parseOpenAIStream(resp.Body, out, model)
	}()
	return out, nil
}

func buildOpenAIRequest(req llm.Request) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = "gpt-4o"
	}
	msgs := []openAIMessage{}
	// Prepend CachedPrefix (if any) to the system message so OpenAI's
	// automatic prefix cache picks it up on repeat turns. OpenAI caches
	// identical prefixes ≥1024 tokens with no explicit directive needed.
	sys := req.SystemPrompt
	if req.CachedPrefix != "" {
		if sys != "" {
			sys = req.CachedPrefix + "\n\n" + sys
		} else {
			sys = req.CachedPrefix
		}
	}
	if sys != "" {
		msgs = append(msgs, openAIMessage{Role: "system", Content: sys})
	}
	for _, m := range req.Messages {
		switch {
		case len(m.ToolResults) > 0:
			for _, tr := range m.ToolResults {
				content, _ := llm.JSON(tr.Content)
				msgs = append(msgs, openAIMessage{Role: "tool", ToolCallID: tr.ToolCallID, Content: string(content)})
			}
			// Tool messages hold only text: images follow as a user message.
			if imgs := llm.ToolResultImages(m.ToolResults); len(imgs) > 0 {
				msgs = append(msgs, openAIMessage{Role: "user", Content: openAIParts(llm.ToolImagesNote, imgs)})
			}
		case len(m.ToolCalls) > 0:
			calls := make([]openAIToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				args := []byte("{}") // a call without arguments, not "null"
				if len(tc.Input) > 0 {
					args, _ = llm.JSON(tc.Input)
				}
				oaTC := openAIToolCall{ID: tc.ID, Type: "function"}
				oaTC.Function.Name = tc.Name
				oaTC.Function.Arguments = string(args)
				calls = append(calls, oaTC)
			}
			msg := openAIMessage{Role: "assistant", ToolCalls: calls}
			if m.Content != "" {
				msg.Content = m.Content
			}
			msgs = append(msgs, msg)
		case len(m.Images) > 0:
			msgs = append(msgs, openAIMessage{Role: m.Role, Content: openAIParts(m.Content, m.Images)})
		default:
			if m.Content == "" {
				continue
			}
			msgs = append(msgs, openAIMessage{Role: m.Role, Content: m.Content})
		}
	}
	tools := []openAIToolDef{}
	for _, t := range req.Tools {
		var td openAIToolDef
		td.Type = "function"
		td.Function.Name = t.Name
		td.Function.Description = t.Description
		td.Function.Parameters = t.InputSchema
		tools = append(tools, td)
	}
	body := openAIRequest{
		Model:         model,
		Messages:      msgs,
		Tools:         tools,
		Stream:        true,
		StreamOptions: &openAIStreamOptions{IncludeUsage: true},
	}
	return llm.JSON(body)
}

// openAIParts is a content list: the text, then the images.
func openAIParts(text string, images []llm.Image) []map[string]interface{} {
	parts := []map[string]interface{}{}
	if text != "" {
		parts = append(parts, map[string]interface{}{"type": "text", "text": text})
	}
	for _, img := range images {
		parts = append(parts, map[string]interface{}{"type": "image_url",
			"image_url": map[string]string{"url": img.DataURL()}})
	}
	return parts
}

func parseOpenAIStream(r io.Reader, out chan<- llm.Chunk, model string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Accumulate tool-call arguments across deltas.
	type tcState struct {
		id      string
		name    string
		argsBuf strings.Builder
	}
	pending := map[int]*tcState{}

	// Final usage frame lands in a chunk with an empty `choices` array
	// and a populated `usage` block — only emitted because we opted into
	// stream_options.include_usage in the request. If we miss it (old
	// server, connection drop) we just skip the log.
	var (
		promptTokens     int
		cachedTokens     int
		completionTokens int
		sawUsage         bool
	)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens        int `json:"prompt_tokens"`
				CompletionTokens    int `json:"completion_tokens"`
				PromptTokensDetails struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			promptTokens = chunk.Usage.PromptTokens
			cachedTokens = chunk.Usage.PromptTokensDetails.CachedTokens
			completionTokens = chunk.Usage.CompletionTokens
			sawUsage = true
		}
		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				out <- llm.Chunk{Type: llm.ChunkText, Text: c.Delta.Content}
			}
			for _, tc := range c.Delta.ToolCalls {
				st, ok := pending[tc.Index]
				if !ok {
					st = &tcState{}
					pending[tc.Index] = st
				}
				if tc.ID != "" {
					st.id = tc.ID
				}
				if tc.Function.Name != "" {
					st.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					st.argsBuf.WriteString(tc.Function.Arguments)
				}
			}
			if c.FinishReason != "" {
				// Flush any accumulated tool calls.
				for _, st := range pending {
					var input map[string]interface{}
					if st.argsBuf.Len() > 0 {
						_ = json.Unmarshal([]byte(st.argsBuf.String()), &input)
					}
					out <- llm.Chunk{
						Type:     llm.ChunkToolCall,
						ToolCall: &llm.ToolCall{ID: st.id, Name: st.name, Input: input},
					}
				}
				pending = map[int]*tcState{}
				out <- llm.Chunk{Type: llm.ChunkStop, StopReason: c.FinishReason}
			}
		}
	}
	if sawUsage {
		log.Printf(
			"llm: provider=openai model=%s prompt_tokens=%d cached=%d completion_tokens=%d",
			model, promptTokens, cachedTokens, completionTokens,
		)
	}
}
