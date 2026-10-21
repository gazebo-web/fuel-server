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
	"strings"

	"github.com/gazebo-web/fuel-server/aibuilder/llm"
)

// Ollama is the Provider implementation for local Ollama servers.
type Ollama struct {
	base string
	hc   *http.Client
}

// NewOllama builds an Ollama provider. apiKey is ignored (Ollama has no auth);
// endpoint defaults to http://localhost:11434.
func NewOllama(apiKey, endpoint string) (*Ollama, error) {
	_ = apiKey
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	return &Ollama{base: endpoint, hc: http.DefaultClient}, nil
}

func (o *Ollama) Name() string          { return "ollama" }
func (o *Ollama) SupportsToolUse() bool { return true }

// ListModels queries Ollama's /api/tags for locally installed models. The
// response does not carry a separate display label, so the tag name is used
// for both ID and DisplayName.
func (o *Ollama) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.base+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := o.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama: list models status %d: %s", resp.StatusCode, string(b))
	}
	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]llm.ModelInfo, 0, len(body.Models))
	for _, m := range body.Models {
		out = append(out, llm.ModelInfo{ID: m.Name, DisplayName: m.Name})
	}
	return out, nil
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	Images    []string         `json:"images,omitempty"`
}

type ollamaToolCall struct {
	Function struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	} `json:"function"`
}

type ollamaTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters"`
	} `json:"function"`
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
}

// ChatStream uses Ollama's /api/chat streaming endpoint.
func (o *Ollama) ChatStream(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
	if req.Model == "" {
		return nil, errors.New("ollama: model required")
	}
	body, err := buildOllamaRequest(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.base+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := o.hc.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama: status %d: %s", resp.StatusCode, string(b))
	}
	out := make(chan llm.Chunk, 16)
	go func() {
		defer resp.Body.Close()
		defer close(out)
		parseOllamaStream(resp.Body, out, req.Model)
	}()
	return out, nil
}

// ollamaImages are images as Ollama takes them: bare base64.
func ollamaImages(imgs []llm.Image) []string {
	var out []string
	for _, img := range imgs {
		out = append(out, img.Data)
	}
	return out
}

func buildOllamaRequest(req llm.Request) ([]byte, error) {
	msgs := []ollamaMessage{}
	// Ollama has no provider-side prefix cache; concatenate so the model
	// still receives the schema context on every turn.
	sys := req.SystemPrompt
	if req.CachedPrefix != "" {
		if sys != "" {
			sys = req.CachedPrefix + "\n\n" + sys
		} else {
			sys = req.CachedPrefix
		}
	}
	if sys != "" {
		msgs = append(msgs, ollamaMessage{Role: "system", Content: sys})
	}
	for _, m := range req.Messages {
		if len(m.ToolCalls) > 0 {
			calls := make([]ollamaToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				var oc ollamaToolCall
				oc.Function.Name = tc.Name
				oc.Function.Arguments = tc.Input
				if oc.Function.Arguments == nil {
					oc.Function.Arguments = map[string]interface{}{} // {}, not null
				}
				calls = append(calls, oc)
			}
			msgs = append(msgs, ollamaMessage{Role: "assistant", Content: m.Content, ToolCalls: calls})
			continue
		}
		if len(m.ToolResults) > 0 {
			for _, tr := range m.ToolResults {
				content, _ := llm.JSON(tr.Content)
				msgs = append(msgs, ollamaMessage{Role: "tool", Content: string(content)})
			}
			// Tool messages hold only text: images follow as a user message.
			if imgs := llm.ToolResultImages(m.ToolResults); len(imgs) > 0 {
				msgs = append(msgs, ollamaMessage{Role: "user", Content: llm.ToolImagesNote, Images: ollamaImages(imgs)})
			}
			continue
		}
		if m.Content != "" || len(m.Images) > 0 {
			msgs = append(msgs, ollamaMessage{Role: m.Role, Content: m.Content, Images: ollamaImages(m.Images)})
		}
	}
	tools := []ollamaTool{}
	for _, t := range req.Tools {
		var ot ollamaTool
		ot.Type = "function"
		ot.Function.Name = t.Name
		ot.Function.Description = t.Description
		ot.Function.Parameters = t.InputSchema
		tools = append(tools, ot)
	}
	body := ollamaRequest{Model: req.Model, Messages: msgs, Tools: tools, Stream: true}
	return llm.JSON(body)
}

func parseOllamaStream(r io.Reader, out chan<- llm.Chunk, model string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Ollama emits final token counts on the terminal `done: true` frame.
	// There is no provider-side prefix cache (Ollama re-evaluates every
	// prompt locally), so the log omits a `cached=` column — what's
	// worth observing is whether the model had to re-evaluate the full
	// schema on every turn. Journal Q-MCP-13.
	var (
		promptEval int
		completion int
		sawUsage   bool
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk struct {
			Message struct {
				Content   string           `json:"content"`
				ToolCalls []ollamaToolCall `json:"tool_calls"`
			} `json:"message"`
			Done            bool `json:"done"`
			PromptEvalCount int  `json:"prompt_eval_count"`
			EvalCount       int  `json:"eval_count"`
		}
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}
		if chunk.Message.Content != "" {
			out <- llm.Chunk{Type: llm.ChunkText, Text: chunk.Message.Content}
		}
		for _, tc := range chunk.Message.ToolCalls {
			out <- llm.Chunk{Type: llm.ChunkToolCall, ToolCall: &llm.ToolCall{
				Name:  tc.Function.Name,
				Input: tc.Function.Arguments,
			}}
		}
		if chunk.Done {
			promptEval = chunk.PromptEvalCount
			completion = chunk.EvalCount
			sawUsage = true
			out <- llm.Chunk{Type: llm.ChunkStop, StopReason: "stop"}
			break
		}
	}
	if sawUsage {
		log.Printf(
			"llm: provider=ollama model=%s prompt_tokens=%d completion_tokens=%d",
			model, promptEval, completion,
		)
	}
}
