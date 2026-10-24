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

const geminiDefaultBase = "https://generativelanguage.googleapis.com"

// Gemini is the Provider implementation for Google's Gemini API.
//
// Uses the v1beta streamGenerateContent endpoint with SSE. Tool use is
// mapped to Gemini's function-calling format.
type Gemini struct {
	apiKey string
	base   string
	hc     *http.Client
}

// NewGemini builds a Gemini provider.
func NewGemini(apiKey, base string) (*Gemini, error) {
	if apiKey == "" {
		return nil, errors.New("gemini: api key required")
	}
	if base == "" {
		base = geminiDefaultBase
	}
	return &Gemini{apiKey: apiKey, base: base, hc: http.DefaultClient}, nil
}

func (g *Gemini) Name() string          { return "gemini" }
func (g *Gemini) SupportsToolUse() bool { return true }

// ListModels queries GET /v1beta/models, paginating through Google's
// response, and returns the chat-capable models with their upstream-supplied
// display names. The list is narrowed to the Gemini family (Gemma and other
// product lines are excluded) and to the clean family alias for each model
// so the UI roughly matches what users see in ai.studio.
func (g *Gemini) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	type apiModel struct {
		Name                       string   `json:"name"`
		DisplayName                string   `json:"displayName"`
		Description                string   `json:"description"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	}

	all := []apiModel{}
	pageToken := ""
	for {
		u := fmt.Sprintf("%s/v1beta/models?pageSize=1000", g.base)
		if pageToken != "" {
			u += "&pageToken=" + pageToken
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-goog-api-key", g.apiKey)
		resp, err := g.hc.Do(req)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("gemini: list models status %d: %s", resp.StatusCode, string(body))
		}
		var page struct {
			Models        []apiModel `json:"models"`
			NextPageToken string     `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Models...)
		if page.NextPageToken == "" {
			break
		}
		pageToken = page.NextPageToken
	}

	// Deduplicate model families: when Google returns both a pinned snapshot
	// (gemini-2.0-flash-001) and its clean alias (gemini-2.0-flash), prefer
	// the alias. Group by the base form and keep one entry per family.
	type candidate struct {
		id          string
		displayName string
	}
	bestByBase := map[string]candidate{}
	order := []string{}
	for _, m := range all {
		if !slicesContains(m.SupportedGenerationMethods, "generateContent") {
			continue
		}
		id := strings.TrimPrefix(m.Name, "models/")
		if !isGeminiUserFacingModel(id) {
			continue
		}
		base := geminiBaseFamily(id)
		cand := candidate{id: id, displayName: m.DisplayName}
		cur, ok := bestByBase[base]
		if !ok {
			bestByBase[base] = cand
			order = append(order, base)
			continue
		}
		// Prefer the shortest id (i.e. the alias) within a family.
		if len(id) < len(cur.id) {
			bestByBase[base] = cand
		}
	}

	out := make([]llm.ModelInfo, 0, len(order))
	for _, base := range order {
		c := bestByBase[base]
		dn := c.displayName
		if dn == "" {
			dn = humanizeID(c.id)
		}
		out = append(out, llm.ModelInfo{ID: c.id, DisplayName: dn})
	}
	return out, nil
}

func slicesContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// isGeminiUserFacingModel is a curation filter over what /v1beta/models
// returns. We drop non-chat families (embedding, imagen, tts, aqa, bison,
// gecko), other product lines that share the API (Gemma, LearnLM), retired
// Gemini families, and preview/experimental snapshots so the list roughly
// matches what ai.studio offers in its model picker.
func isGeminiUserFacingModel(id string) bool {
	// Only Gemini — Gemma, LearnLM, etc. show up in the same list endpoint
	// but are distinct products the chat UI shouldn't surface.
	if !strings.HasPrefix(id, "gemini-") {
		return false
	}
	// Non-chat modalities / legacy families that can share the gemini- prefix.
	skipSubstr := []string{
		"embedding", "embed",
		"aqa",
		"bison", "gecko", "text-",
		"imagen", "veo",
		"tts", "audio", "native-audio",
		"image-generation",
	}
	for _, s := range skipSubstr {
		if strings.Contains(id, s) {
			return false
		}
	}
	// Retired families.
	if strings.HasPrefix(id, "gemini-1.0") ||
		id == "gemini-pro" || id == "gemini-pro-vision" {
		return false
	}
	// Preview / experimental snapshots.
	if strings.Contains(id, "-exp") ||
		strings.Contains(id, "preview") ||
		strings.Contains(id, "thinking") {
		return false
	}
	return true
}

// geminiBaseFamily collapses versioned/aliased variants of the same model to
// a single key so the caller can dedupe. Examples:
//
//	gemini-2.0-flash        -> gemini-2.0-flash
//	gemini-2.0-flash-001    -> gemini-2.0-flash
//	gemini-2.0-flash-latest -> gemini-2.0-flash
func geminiBaseFamily(id string) string {
	s := strings.TrimSuffix(id, "-latest")
	s = geminiNumericSuffixRe.ReplaceAllString(s, "")
	return s
}

var geminiNumericSuffixRe = regexp.MustCompile(`-\d{3,}$`)

// humanizeID falls back to a best-effort title-cased label when the upstream
// API does not ship a displayName for a model id.
func humanizeID(id string) string {
	parts := strings.Split(id, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(p)
		// Leave things that are already mixed-case or look like a version
		// (e.g. "4o", "2.5") alone; otherwise upper-case the first rune.
		hasDigit := false
		for _, c := range r {
			if c >= '0' && c <= '9' {
				hasDigit = true
				break
			}
		}
		if !hasDigit && r[0] >= 'a' && r[0] <= 'z' {
			r[0] = r[0] - ('a' - 'A')
		}
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}

type geminiPart struct {
	Text             string              `json:"text,omitempty"`
	InlineData       *geminiBlob         `json:"inlineData,omitempty"`
	FunctionCall     *geminiFuncCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFuncResponse `json:"functionResponse,omitempty"`
	// ThoughtSignature is emitted by Gemini 2.5+ "thinking" models on any
	// part that captures latent reasoning state (thoughts, functionCalls).
	// The API rejects the next turn if the signature isn't echoed back
	// verbatim on the same part it arrived on.
	ThoughtSignature string `json:"thoughtSignature,omitempty"`
}

type geminiBlob struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiFuncCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type geminiFuncResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type geminiRequest struct {
	Contents          []geminiContent `json:"contents"`
	Tools             []geminiTool    `json:"tools,omitempty"`
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
}

func (g *Gemini) ChatStream(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
	body, err := buildGeminiRequest(req)
	if err != nil {
		return nil, err
	}
	model := req.Model
	if model == "" {
		// An alias for Google's newest Flash model, so the default isn't
		// retired from under us (gemini-2.0-flash was).
		model = "gemini-flash-latest"
	}
	// Send the key via the x-goog-api-key header rather than ?key= in the
	// query string. Both authenticate, but the header is Google's recommended
	// form and keeps the key out of access logs / proxy URL dumps.
	url := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", g.base, model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", g.apiKey)
	resp, err := g.hc.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini: status %d: %s", resp.StatusCode, string(b))
	}
	out := make(chan llm.Chunk, 16)
	go func() {
		defer resp.Body.Close()
		defer close(out)
		parseGeminiStream(resp.Body, out, model)
	}()
	return out, nil
}

func buildGeminiRequest(req llm.Request) ([]byte, error) {
	body := geminiRequest{}
	// Gemini's implicit caching keys on any repeated prefix ≥ a model-
	// dependent minimum (typically 1k–4k tokens). Dropping the cached
	// prefix into systemInstruction as the first part keeps it stable
	// across turns so the cache hits.
	sys := req.SystemPrompt
	if req.CachedPrefix != "" {
		if sys != "" {
			sys = req.CachedPrefix + "\n\n" + sys
		} else {
			sys = req.CachedPrefix
		}
	}
	if sys != "" {
		body.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: sys}}}
	}
	for _, m := range req.Messages {
		role := m.Role
		if role == "user" {
			role = "user"
		} else if role == "assistant" {
			role = "model"
		}
		c := geminiContent{Role: role}
		for _, img := range m.Images {
			c.Parts = append(c.Parts, geminiPart{InlineData: &geminiBlob{MimeType: img.MimeType, Data: img.Data}})
		}
		if m.Content != "" {
			c.Parts = append(c.Parts, geminiPart{Text: m.Content})
		}
		for _, tc := range m.ToolCalls {
			c.Parts = append(c.Parts, geminiPart{
				FunctionCall: &geminiFuncCall{
					Name: tc.Name,
					Args: tc.Input,
				},
				ThoughtSignature: tc.ProviderMetadata["gemini.thoughtSignature"],
			})
		}
		for _, tr := range m.ToolResults {
			resp, _ := anyToMap(tr.Content)
			c.Parts = append(c.Parts, geminiPart{FunctionResponse: &geminiFuncResponse{
				Name:     "tool",
				Response: resp,
			}})
		}
		// Function responses hold only JSON: images follow in the same turn.
		if imgs := llm.ToolResultImages(m.ToolResults); len(imgs) > 0 {
			c.Parts = append(c.Parts, geminiPart{Text: llm.ToolImagesNote})
			for _, img := range imgs {
				c.Parts = append(c.Parts, geminiPart{InlineData: &geminiBlob{MimeType: img.MimeType, Data: img.Data}})
			}
		}
		if len(c.Parts) > 0 {
			body.Contents = append(body.Contents, c)
		}
	}
	if len(req.Tools) > 0 {
		decls := make([]geminiFunctionDecl, 0, len(req.Tools))
		for _, t := range req.Tools {
			decls = append(decls, geminiFunctionDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			})
		}
		body.Tools = []geminiTool{{FunctionDeclarations: decls}}
	}
	return llm.JSON(body)
}

func anyToMap(v interface{}) (map[string]interface{}, error) {
	b, err := llm.JSON(v)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		// Wrap primitives and arrays.
		return map[string]interface{}{"value": v}, nil
	}
	return m, nil
}

func parseGeminiStream(r io.Reader, out chan<- llm.Chunk, model string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Gemini streams each chunk with a cumulative `usageMetadata` block.
	// The final chunk's values are the ones we want; overwrite on each
	// frame so the final read wins. Journal Q-MCP-13.
	var (
		promptTokens int
		cachedTokens int
		outputTokens int
		thoughts     int
		sawUsage     bool
	)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var chunk struct {
			Candidates []struct {
				Content struct {
					Parts []geminiPart `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			} `json:"candidates"`
			UsageMetadata *struct {
				PromptTokenCount        int `json:"promptTokenCount"`
				CachedContentTokenCount int `json:"cachedContentTokenCount"`
				CandidatesTokenCount    int `json:"candidatesTokenCount"`
				ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
			} `json:"usageMetadata"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.UsageMetadata != nil {
			promptTokens = chunk.UsageMetadata.PromptTokenCount
			cachedTokens = chunk.UsageMetadata.CachedContentTokenCount
			outputTokens = chunk.UsageMetadata.CandidatesTokenCount
			thoughts = chunk.UsageMetadata.ThoughtsTokenCount
			sawUsage = true
		}
		for _, c := range chunk.Candidates {
			for _, p := range c.Content.Parts {
				if p.Text != "" {
					out <- llm.Chunk{Type: llm.ChunkText, Text: p.Text}
				}
				if p.FunctionCall != nil {
					tc := &llm.ToolCall{
						ID:    p.FunctionCall.Name,
						Name:  p.FunctionCall.Name,
						Input: p.FunctionCall.Args,
					}
					if p.ThoughtSignature != "" {
						tc.ProviderMetadata = map[string]string{
							"gemini.thoughtSignature": p.ThoughtSignature,
						}
					}
					out <- llm.Chunk{Type: llm.ChunkToolCall, ToolCall: tc}
				}
			}
			if c.FinishReason != "" {
				out <- llm.Chunk{Type: llm.ChunkStop, StopReason: c.FinishReason}
			}
		}
	}
	if sawUsage {
		log.Printf(
			"llm: provider=gemini model=%s prompt_tokens=%d cached=%d completion_tokens=%d thoughts=%d",
			model, promptTokens, cachedTokens, outputTokens, thoughts,
		)
		// In Anthropic's terms, which the chat's usage events use: input
		// is the uncached part of the prompt, and output includes thinking.
		out <- llm.Chunk{Type: llm.ChunkUsage, Usage: &llm.Usage{Input: promptTokens - cachedTokens,
			CacheRead: cachedTokens, Output: outputTokens + thoughts, Thinking: thoughts}}
	}
}
