// Package providers contains concrete LLM provider implementations.
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
	"sort"
	"strings"

	"github.com/gazebo-web/fuel-server/aibuilder/llm"
)

const (
	anthropicAPIBase      = "https://api.anthropic.com"
	anthropicAPIVersion   = "2023-06-01"
	anthropicDefaultModel = "claude-sonnet-5-5"
)

// Anthropic is the Provider implementation for Claude models.
type Anthropic struct {
	apiKey string
	base   string
	client *http.Client
}

// NewAnthropic builds an Anthropic provider. If base is empty, the public
// api.anthropic.com endpoint is used.
func NewAnthropic(apiKey, base string) (*Anthropic, error) {
	if apiKey == "" {
		return nil, errors.New("anthropic: api key required")
	}
	if base == "" {
		base = anthropicAPIBase
	}
	return &Anthropic{
		apiKey: apiKey,
		base:   base,
		client: http.DefaultClient,
	}, nil
}

// Name returns "anthropic".
func (a *Anthropic) Name() string { return "anthropic" }

// SupportsToolUse returns true.
func (a *Anthropic) SupportsToolUse() bool { return true }

// ListModels queries GET /v1/models for the caller's account. Anthropic
// returns both dated snapshots (claude-3-5-sonnet-20241022) and clean
// aliases; we keep only the aliases, and use display_name from the API for
// the friendly label.
func (a *Anthropic) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.base+"/v1/models?limit=1000", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic: list models status %d: %s", resp.StatusCode, string(b))
	}
	var body struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]llm.ModelInfo, 0, len(body.Data))
	for _, m := range body.Data {
		if !isAnthropicUserFacingModel(m.ID) {
			continue
		}
		dn := m.DisplayName
		if dn == "" {
			dn = humanizeID(m.ID)
		}
		out = append(out, llm.ModelInfo{ID: m.ID, DisplayName: dn})
	}
	return out, nil
}

// isAnthropicUserFacingModel drops the date-stamped snapshots Anthropic
// returns alongside the clean aliases (e.g. claude-3-5-sonnet-20241022 vs
// claude-3-5-sonnet-latest / claude-sonnet-4-5). The aliases always resolve
// to the best current version, so the UI only shows those.
func isAnthropicUserFacingModel(id string) bool {
	return !anthropicDatedSnapshotRe.MatchString(id)
}

// anthropicDatedSnapshotRe matches Anthropic's 8-digit date suffix
// (e.g. `-20241022`).
var anthropicDatedSnapshotRe = regexp.MustCompile(`-\d{8}$`)

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	// Input is set only on tool_use blocks, where Anthropic requires it
	// even when the call has no arguments ({}). RawMessage keeps it off
	// other block types (omitempty) without dropping an empty object.
	Input json.RawMessage `json:"input,omitempty"`
	// For tool_result blocks
	ToolUseID string      `json:"tool_use_id,omitempty"`
	Content   interface{} `json:"content,omitempty"`
	IsError   bool        `json:"is_error,omitempty"`

	CacheControl *anthropicCacheControlSpec `json:"cache_control,omitempty"`
	// Source is set only on image blocks.
	Source *anthropicImageSource `json:"source,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// anthropicMessage.Content is []anthropicContentBlock, or the
// json.RawMessage of an assistant message replayed verbatim (llm.Message
// Native).
type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type anthropicOutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type anthropicRequest struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	// System is either a plain string (when CachedPrefix is empty) or a
	// list of content blocks (when CachedPrefix is non-empty) so we can
	// mark the prefix with cache_control.
	System   interface{}        `json:"system,omitempty"`
	Messages []anthropicMessage `json:"messages"`
	Tools    []anthropicTool    `json:"tools,omitempty"`
	Stream   bool               `json:"stream"`

	OutputConfig *anthropicOutputConfig `json:"output_config,omitempty"`
}

// anthropicSystemBlock is one entry in the System array form. cache_control
// tells Claude to stash the preceding blocks in the ephemeral (5-minute)
// cache. We set it on the cached prefix only; the short system prompt is
// re-sent every turn.
type anthropicSystemBlock struct {
	Type         string                     `json:"type"`
	Text         string                     `json:"text"`
	CacheControl *anthropicCacheControlSpec `json:"cache_control,omitempty"`
}

type anthropicCacheControlSpec struct {
	Type string `json:"type"` // "ephemeral"
}

// ChatStream calls the Messages API with streaming.
func (a *Anthropic) ChatStream(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
	body, err := buildAnthropicRequest(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic: status %d: %s", resp.StatusCode, string(b))
	}

	model := req.Model
	if model == "" {
		model = anthropicDefaultModel
	}
	out := make(chan llm.Chunk, 16)
	go func() {
		defer resp.Body.Close()
		defer close(out)
		parseAnthropicStream(resp.Body, out, model)
	}()
	return out, nil
}

func buildAnthropicRequest(req llm.Request) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = anthropicDefaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = anthropicMaxTokens(model)
	}

	msgs := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		role := m.Role
		if role == "system" {
			// system prompt goes in the "system" field, not in messages
			continue
		}
		if role == "assistant" && len(m.Native) > 0 && m.NativeProvider == "anthropic" {
			// Replay as streamed: thinking blocks and their signatures
			// must come back unchanged within a tool-use loop.
			msgs = append(msgs, anthropicMessage{Role: role, Content: m.Native})
			continue
		}
		blocks := []anthropicContentBlock{}
		// Attached images come before the text that asks about them.
		for _, img := range m.Images {
			blocks = append(blocks, anthropicContentBlock{Type: "image",
				Source: &anthropicImageSource{Type: "base64", MediaType: img.MimeType, Data: img.Data}})
		}
		if m.Content != "" {
			blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
		}
		for _, tc := range m.ToolCalls {
			input := json.RawMessage("{}")
			if len(tc.Input) > 0 {
				b, err := llm.JSON(tc.Input)
				if err != nil {
					return nil, fmt.Errorf("tool %s input: %w", tc.Name, err)
				}
				input = b
			}
			blocks = append(blocks, anthropicContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Name,
				Input: input,
			})
		}
		for _, tr := range m.ToolResults {
			blocks = append(blocks, anthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: tr.ToolCallID,
				Content:   anthropicToolResultContent(tr.Content),
				IsError:   tr.IsError,
			})
		}
		if len(blocks) == 0 {
			continue
		}
		msgs = append(msgs, anthropicMessage{Role: role, Content: blocks})
	}

	// Cache the conversation so far: each request in a tool loop re-sends
	// the whole history, which would otherwise be billed in full every
	// round. The breakpoint is on the newest block (the last message is
	// always the user's text or tool results, built here); the next
	// request reads everything up to it from the cache.
	if n := len(msgs); n > 0 {
		if blocks, ok := msgs[n-1].Content.([]anthropicContentBlock); ok && len(blocks) > 0 {
			blocks[len(blocks)-1].CacheControl = &anthropicCacheControlSpec{Type: "ephemeral"}
		}
	}

	tools := make([]anthropicTool, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	body := anthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    buildAnthropicSystem(req),
		Messages:  msgs,
		Tools:     tools,
		Stream:    true,
	}
	if req.Effort != "" && anthropicSupportsEffort(model) {
		body.OutputConfig = &anthropicOutputConfig{Effort: req.Effort}
	}
	return llm.JSON(body)
}

// anthropicSupportsEffort reports whether a model accepts
// output_config.effort; older models reject it.
func anthropicSupportsEffort(model string) bool {
	for _, p := range []string{
		"claude-fable-", "claude-mythos-", "claude-opus-5", "claude-sonnet-5", "claude-haiku-5",
		"claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6", "claude-opus-4-5", "claude-sonnet-4-6",
	} {
		if strings.HasPrefix(model, p) {
			return true
		}
	}
	return false
}

// anthropicToolResultContent renders a tool result: text, or for a result
// with images (a generated model's preview) a text block followed by image
// blocks, so the model can look at what it built.
func anthropicToolResultContent(v interface{}) interface{} {
	rich, ok := v.(llm.RichResult)
	if !ok {
		return anthropicToolResultText(v)
	}
	blocks := []map[string]interface{}{{"type": "text", "text": anthropicToolResultText(rich.Value)}}
	for _, img := range rich.Images {
		blocks = append(blocks, map[string]interface{}{
			"type":   "image",
			"source": map[string]string{"type": "base64", "media_type": img.MimeType, "data": img.Data},
		})
	}
	return blocks
}

// anthropicToolResultText renders a tool result as text. Anthropic reads
// a non-string tool_result content as an array of content blocks, so a
// structured result (e.g. the [] from project_list) must go as JSON text.
func anthropicToolResultText(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := llm.JSON(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// anthropicMaxTokens is the per-model default output budget. Anthropic
// requires max_tokens, and it caps thinking plus the response: a low value
// truncates long tool inputs (a whole file in project_write_file). Current
// models think by default, so they get a large budget; it costs nothing
// unless used.
func anthropicMaxTokens(model string) int {
	switch {
	case strings.HasPrefix(model, "claude-3-"):
		return 8192
	case strings.HasPrefix(model, "claude-opus-4-0"), strings.HasPrefix(model, "claude-opus-4-1"),
		strings.HasPrefix(model, "claude-opus-4-2025"):
		return 32000
	}
	return 64000
}

// buildAnthropicSystem chooses between the string and block-list shapes of
// the Anthropic `system` field. When CachedPrefix is non-empty, the prefix
// block carries cache_control so repeat turns within ~5 minutes reuse the
// cached tokens instead of re-billing them.
func buildAnthropicSystem(req llm.Request) interface{} {
	if req.CachedPrefix == "" {
		return req.SystemPrompt
	}
	blocks := []anthropicSystemBlock{{
		Type:         "text",
		Text:         req.CachedPrefix,
		CacheControl: &anthropicCacheControlSpec{Type: "ephemeral"},
	}}
	if req.SystemPrompt != "" {
		blocks = append(blocks, anthropicSystemBlock{Type: "text", Text: req.SystemPrompt})
	}
	return blocks
}

// parseAnthropicStream reads SSE events from the response body and emits
// llm.Chunk values on out: text and tool calls as they complete, then the
// whole message as ChunkNative (for verbatim replay, thinking blocks
// included) and its ChunkUsage. It also logs one usage line per stream —
// see journal Q-MCP-13 for why we care. The model arg is only used in the
// log line.
func parseAnthropicStream(r io.Reader, out chan<- llm.Chunk, model string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	type blockState struct {
		kind      string // "text" | "tool_use" | "thinking" | other
		id        string
		name      string
		buf       strings.Builder // text, tool input JSON or thinking text
		signature string
		start     json.RawMessage // the content_block as it started
	}
	blocks := map[int]*blockState{}
	native := map[int]json.RawMessage{}

	// Anthropic ships usage in two places: message_start (input + cache
	// counts) and message_delta (output_tokens updates as the stream
	// progresses, with the thinking share at the end). We accumulate so
	// the log at message_stop has the final numbers.
	var usage llm.Usage
	done := false
	finish := func() {
		if done {
			return
		}
		done = true
		logAnthropicUsage(model, usage)
		out <- llm.Chunk{Type: llm.ChunkUsage, Usage: &usage}
	}
	defer finish() // a stream closed without message_stop still logs usage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var evt struct {
			Type         string          `json:"type"`
			Index        int             `json:"index"`
			ContentBlock json.RawMessage `json:"content_block"`
			Delta        struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				Thinking    string `json:"thinking"`
				Signature   string `json:"signature"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Message struct {
				Usage anthropicUsage `json:"usage"`
			} `json:"message"`
			Usage *anthropicUsage `json:"usage,omitempty"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}
		switch evt.Type {
		case "message_start":
			u := evt.Message.Usage
			usage.Input, usage.CacheWrite, usage.CacheRead = u.InputTokens, u.CacheCreationInputTokens, u.CacheReadInputTokens
			usage.Output = u.OutputTokens
		case "content_block_start":
			var cb struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			_ = json.Unmarshal(evt.ContentBlock, &cb)
			blocks[evt.Index] = &blockState{kind: cb.Type, id: cb.ID, name: cb.Name, start: evt.ContentBlock}
		case "content_block_delta":
			b, ok := blocks[evt.Index]
			if !ok {
				continue
			}
			switch evt.Delta.Type {
			case "text_delta":
				b.buf.WriteString(evt.Delta.Text)
				out <- llm.Chunk{Type: llm.ChunkText, Text: evt.Delta.Text}
			case "input_json_delta":
				b.buf.WriteString(evt.Delta.PartialJSON)
			case "thinking_delta":
				b.buf.WriteString(evt.Delta.Thinking)
			case "signature_delta":
				b.signature += evt.Delta.Signature
			}
		case "content_block_stop":
			b, ok := blocks[evt.Index]
			if !ok {
				continue
			}
			delete(blocks, evt.Index)
			var block interface{}
			switch b.kind {
			case "text":
				block = map[string]string{"type": "text", "text": b.buf.String()}
			case "thinking":
				block = map[string]string{"type": "thinking", "thinking": b.buf.String(), "signature": b.signature}
			case "tool_use":
				raw := json.RawMessage("{}")
				var input map[string]interface{}
				if b.buf.Len() > 0 && json.Unmarshal([]byte(b.buf.String()), &input) == nil {
					raw = json.RawMessage(b.buf.String())
				}
				block = map[string]interface{}{"type": "tool_use", "id": b.id, "name": b.name, "input": raw}
				out <- llm.Chunk{
					Type: llm.ChunkToolCall,
					ToolCall: &llm.ToolCall{
						ID:    b.id,
						Name:  b.name,
						Input: input,
					},
				}
			default:
				// redacted_thinking and anything newer arrive whole in
				// content_block_start; replay them as they came.
				block = b.start
			}
			if raw, err := llm.JSON(block); err == nil {
				native[evt.Index] = raw
			}
		case "message_delta":
			if evt.Usage != nil {
				// Only output_tokens is guaranteed to update here;
				// other fields may be zero-valued if Anthropic hasn't
				// replayed them. Overwrite what we have.
				if evt.Usage.OutputTokens > 0 {
					usage.Output = evt.Usage.OutputTokens
				}
				if t := evt.Usage.OutputTokensDetails.ThinkingTokens; t > 0 {
					usage.Thinking = t
				}
			}
			if evt.Delta.StopReason != "" {
				out <- llm.Chunk{Type: llm.ChunkStop, StopReason: evt.Delta.StopReason}
			}
		case "message_stop":
			if msg := orderedBlocks(native); msg != nil {
				out <- llm.Chunk{Type: llm.ChunkNative, Native: msg}
			}
			finish()
			return
		case "error":
			out <- llm.Chunk{Type: llm.ChunkError, Err: data}
			return
		}
	}
}

// orderedBlocks joins content blocks into a JSON array in index order.
func orderedBlocks(blocks map[int]json.RawMessage) json.RawMessage {
	if len(blocks) == 0 {
		return nil
	}
	idx := make([]int, 0, len(blocks))
	for i := range blocks {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	list := make([]json.RawMessage, len(idx))
	for i, k := range idx {
		list[i] = blocks[k]
	}
	b, err := llm.JSON(list)
	if err != nil {
		return nil
	}
	return b
}

// anthropicUsage mirrors the Anthropic SSE `usage` block. Fields beyond
// these four exist (e.g. server_tool_use, cache_creation) but aren't
// useful for cache-hit observation today.
type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokensDetails      struct {
		ThinkingTokens int `json:"thinking_tokens"`
	} `json:"output_tokens_details"`
}

func logAnthropicUsage(model string, u llm.Usage) {
	log.Printf(
		"llm: provider=anthropic model=%s prompt_tokens=%d cache_created=%d cache_read=%d completion_tokens=%d thinking_tokens=%d",
		model, u.Input, u.CacheWrite, u.CacheRead, u.Output, u.Thinking,
	)
}
