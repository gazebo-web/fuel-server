package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// The vision judge (plan §6): a vision model scores each generated model's
// preview sheet 1-5 against the prompt. Structural checks can't tell a mug
// from a cylinder with a stick on it; this can, roughly. Spot-check its
// scores by hand before gating on them.

const judgePrompt = `You are reviewing a 3D model generated for a robot simulator.
The user asked: %q
The image shows the generated model %q from four angles (labels in each view).

Score how well this model matches what the user asked for, as an object:
5 = clearly the requested object, good proportions and detail for a simulator
4 = clearly the object, minor problems
3 = recognizable but with obvious problems (proportions, missing parts)
2 = barely recognizable
1 = not the requested object

Reply with only JSON: {"score": <1-5>, "reason": "<one sentence>"}`

// Judgement is the judge's verdict on one model.
type Judgement struct {
	Model  string `json:"model"`
	Score  int    `json:"score"`
	Reason string `json:"reason"`
}

type judge struct {
	key, model, base string
	http             *http.Client
}

var jsonObject = regexp.MustCompile(`(?s)\{.*\}`)

// score asks the judge about one preview image (PNG).
func (j *judge) score(ctx context.Context, prompt, model string, png []byte) (Judgement, error) {
	body := map[string]any{
		"model":      j.model,
		"max_tokens": 1000,
		"messages": []any{map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "image", "source": map[string]string{
				"type": "base64", "media_type": "image/png", "data": base64.StdEncoding.EncodeToString(png)}},
			map[string]any{"type": "text", "text": fmt.Sprintf(judgePrompt, prompt, model)},
		}}},
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.base+"/v1/messages", bytes.NewReader(b))
	if err != nil {
		return Judgement{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", j.key)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := j.http.Do(req)
	if err != nil {
		return Judgement{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return Judgement{}, fmt.Errorf("judge: status %d: %.300s", resp.StatusCode, raw)
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return Judgement{}, fmt.Errorf("judge: %w", err)
	}
	var text strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	v := Judgement{Model: model}
	if err := json.Unmarshal([]byte(jsonObject.FindString(text.String())), &v); err != nil || v.Score < 1 || v.Score > 5 {
		return Judgement{}, fmt.Errorf("judge: unexpected reply %.200q", text.String())
	}
	v.Model = model
	return v, nil
}
