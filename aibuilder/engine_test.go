package aibuilder

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gazebo-web/fuel-server/aibuilder/llm"
)

// scriptedProvider is a fake Provider that replays pre-canned streams.
type scriptedProvider struct {
	mu     sync.Mutex
	rounds [][]llm.Chunk
	idx    int
	reqs   []llm.Request
}

func (p *scriptedProvider) Name() string          { return "scripted" }
func (p *scriptedProvider) SupportsToolUse() bool { return true }
func (p *scriptedProvider) ListModels(_ context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "scripted-1", DisplayName: "Scripted 1"}}, nil
}
func (p *scriptedProvider) ChatStream(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reqs = append(p.reqs, req)
	if p.idx >= len(p.rounds) {
		c := make(chan llm.Chunk)
		close(c)
		return c, nil
	}
	chunks := p.rounds[p.idx]
	p.idx++
	c := make(chan llm.Chunk, len(chunks))
	for _, ch := range chunks {
		c <- ch
	}
	close(c)
	return c, nil
}

// fakeMCP is a test MCP server.
type fakeMCP struct{}

func (fakeMCP) Name() string { return "fake" }
func (fakeMCP) Tools() []llm.Tool {
	return []llm.Tool{{Name: "echo", Description: "echo", InputSchema: map[string]interface{}{"type": "object"}}}
}
func (fakeMCP) Call(ctx context.Context, name string, input map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"you said": input["text"]}, nil
}

func drainEvents(t *testing.T, ch <-chan Event) []Event {
	t.Helper()
	out := []Event{}
	timeout := time.After(2 * time.Second)
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, e)
		case <-timeout:
			t.Fatal("timeout draining events")
		}
	}
}

func TestEngineTextOnly(t *testing.T) {
	p := &scriptedProvider{rounds: [][]llm.Chunk{
		{{Type: llm.ChunkText, Text: "Hello there."}, {Type: llm.ChunkStop}},
	}}
	reg := &testTools{}
	reg.Add(fakeMCP{})
	e := NewEngine(p, reg, DefaultEngineConfig())

	out := make(chan Event, 16)
	var stored []ChatMessage
	appender := func(m *ChatMessage) error { stored = append(stored, *m); return nil }
	go e.Run(context.Background(), nil, UserInput{Text: "hi"}, "sess", out, appender)

	events := drainEvents(t, out)
	require.NotEmpty(t, events)
	assert.Equal(t, EventDone, events[len(events)-1].Type)

	var text string
	for _, ev := range events {
		if ev.Type == EventText {
			text += ev.Text
		}
	}
	assert.Equal(t, "Hello there.", text)

	// stored should have user + assistant
	roles := []string{}
	for _, m := range stored {
		roles = append(roles, m.Role)
	}
	assert.Contains(t, roles, "user")
	assert.Contains(t, roles, "assistant")
}

func TestEngineWithToolCall(t *testing.T) {
	p := &scriptedProvider{rounds: [][]llm.Chunk{
		{{Type: llm.ChunkToolCall, ToolCall: &llm.ToolCall{ID: "t1", Name: "fake.echo", Input: map[string]interface{}{"text": "hi"}}}},
		{{Type: llm.ChunkText, Text: "Done."}, {Type: llm.ChunkStop}},
	}}
	reg := &testTools{}
	reg.Add(fakeMCP{})
	e := NewEngine(p, reg, DefaultEngineConfig())

	out := make(chan Event, 32)
	appender := func(m *ChatMessage) error { return nil }
	go e.Run(context.Background(), nil, UserInput{Text: "echo hi"}, "sess", out, appender)
	events := drainEvents(t, out)

	types_ := []EventType{}
	for _, ev := range events {
		types_ = append(types_, ev.Type)
	}
	assert.Contains(t, types_, EventToolCall)
	assert.Contains(t, types_, EventToolResult)
	assert.Equal(t, EventDone, types_[len(types_)-1])
}

// fakeProject mimics fuel-server's project_* tools as seen through the
// registry ("project" + "_" + tool name).
type fakeProject struct{ fail bool }

func (fakeProject) Name() string { return "project" }
func (fakeProject) Tools() []llm.Tool {
	return []llm.Tool{{Name: "write_file", Description: "write", InputSchema: map[string]interface{}{"type": "object"}}}
}
func (f fakeProject) Call(ctx context.Context, name string, input map[string]interface{}) (interface{}, error) {
	if f.fail {
		return nil, errors.New("file changed since base_rev")
	}
	return map[string]interface{}{
		"path": input["path"], "rev": "abc123", "version": 3,
		"validation": map[string]interface{}{"valid": true, "errors": 0},
	}, nil
}

func runWrite(t *testing.T, server testServer) []Event {
	t.Helper()
	p := &scriptedProvider{rounds: [][]llm.Chunk{
		{{Type: llm.ChunkToolCall, ToolCall: &llm.ToolCall{ID: "t1", Name: "project_write_file",
			Input: map[string]interface{}{"name": "p", "path": "world.sdf", "content": "<sdf/>"}}}, {Type: llm.ChunkStop}},
		{{Type: llm.ChunkText, Text: "done"}, {Type: llm.ChunkStop}},
	}}
	reg := &testTools{}
	reg.Add(server)
	out := make(chan Event, 16)
	go NewEngine(p, reg, DefaultEngineConfig()).Run(context.Background(), nil, UserInput{Text: "build"}, "sess", out, func(*ChatMessage) error { return nil })
	return drainEvents(t, out)
}

func TestEngineEmitsProjectChanged(t *testing.T) {
	var changed []Event
	for _, ev := range runWrite(t, fakeProject{}) {
		if ev.Type == EventProjectChanged {
			changed = append(changed, ev)
		}
	}
	require.Len(t, changed, 1)
	assert.Equal(t, "world.sdf", changed[0].Path)
	assert.Equal(t, "abc123", changed[0].Rev)
	assert.NotNil(t, changed[0].Validation)
}

func TestEngineNoProjectChangedOnFailedWrite(t *testing.T) {
	for _, ev := range runWrite(t, fakeProject{fail: true}) {
		assert.NotEqual(t, EventProjectChanged, ev.Type)
	}
}

func TestEngineSendsSystemPromptAsCachedPrefix(t *testing.T) {
	p := &scriptedProvider{rounds: [][]llm.Chunk{{{Type: llm.ChunkText, Text: "hi"}, {Type: llm.ChunkStop}}}}
	cfg := DefaultEngineConfig()
	cfg.SystemPrompt = "stable prompt"
	out := make(chan Event, 8)
	go NewEngine(p, &testTools{}, cfg).Run(context.Background(), nil, UserInput{Text: "hello"}, "sess", out, func(*ChatMessage) error { return nil })
	drainEvents(t, out)
	require.Len(t, p.reqs, 1)
	assert.Equal(t, "stable prompt", p.reqs[0].CachedPrefix)
	assert.Empty(t, p.reqs[0].SystemPrompt)
}

func TestSystemPromptUsesWorkspaceTools(t *testing.T) {
	p := BuildSystemPrompt("warehouse", "Harmonic", "1.9", "Jazzy")
	for _, want := range []string{`name="warehouse"`, "Decompose, don't refuse", "project_edit_file", "base_rev", "scene_summary", "project_checkpoint"} {
		assert.Contains(t, p, want)
	}
	for _, gone := range []string{"project_save_world_sdf", "project_replace_in_world_sdf", "sdf_validate_sdf"} {
		assert.NotContains(t, p, gone)
	}
}

func TestBuildSystemPromptSubstitutes(t *testing.T) {
	p := BuildSystemPrompt("warehouse", "Harmonic", "1.9", "Jazzy")
	assert.Contains(t, p, "Harmonic")
	assert.Contains(t, p, "1.9")
	assert.Contains(t, p, "Jazzy")
	assert.NotContains(t, p, "{GAZEBO}")
}

// The provider's native message (Anthropic thinking blocks) from one tool
// round is replayed in the next, and usage and effort pass through.
func TestEngineReplaysNativeMessage(t *testing.T) {
	native := []byte(`[{"type":"thinking","signature":"S"}]`)
	usage := &llm.Usage{Input: 10, Output: 20, Thinking: 15}
	p := &scriptedProvider{rounds: [][]llm.Chunk{
		{
			{Type: llm.ChunkToolCall, ToolCall: &llm.ToolCall{ID: "t1", Name: "fake.echo", Input: map[string]interface{}{"text": "hi"}}},
			{Type: llm.ChunkNative, Native: native},
			{Type: llm.ChunkUsage, Usage: usage},
		},
		{{Type: llm.ChunkText, Text: "Done."}, {Type: llm.ChunkStop}},
	}}
	reg := &testTools{}
	reg.Add(fakeMCP{})
	cfg := DefaultEngineConfig()
	cfg.Effort = "medium"
	out := make(chan Event, 32)
	go NewEngine(p, reg, cfg).Run(context.Background(), nil, UserInput{Text: "echo hi"}, "sess", out, func(*ChatMessage) error { return nil })
	events := drainEvents(t, out)

	require.Len(t, p.reqs, 2)
	assert.Equal(t, "medium", p.reqs[0].Effort)
	second := p.reqs[1].Messages
	require.Len(t, second, 3) // user, assistant, tool results
	assert.Equal(t, string(native), string(second[1].Native))
	assert.Equal(t, "scripted", second[1].NativeProvider)

	var got *llm.Usage
	for _, ev := range events {
		if ev.Type == EventUsage {
			got = ev.Usage
		}
	}
	assert.Equal(t, usage, got)
}

func TestProjectChangedForGeneratedModels(t *testing.T) {
	ev, ok := projectChanged("model_generate", llm.ToolResult{Content: llm.RichResult{
		Value: map[string]interface{}{"model": "cone", "rev": "abc", "validation": map[string]interface{}{"valid": true}},
	}})
	assert.True(t, ok)
	assert.Equal(t, "models/cone/model.sdf", ev.Path)
	assert.Equal(t, "abc", ev.Rev)
	_, ok = projectChanged("model_generate", llm.ToolResult{IsError: true, Content: "ZeroDivisionError"})
	assert.False(t, ok)
	ev, ok = projectChanged("model_create", llm.ToolResult{Content: map[string]interface{}{"path": "models/crate/model.sdf", "rev": "r1"}})
	assert.True(t, ok)
	assert.Equal(t, "models/crate/model.sdf", ev.Path)
}

// testServer is a named group of tools, as the old MCP servers were.
type testServer interface {
	Name() string
	Tools() []llm.Tool
	Call(ctx context.Context, name string, input map[string]interface{}) (interface{}, error)
}

// testTools dispatches "<server>_<tool>" to its server, like the old
// registry did.
type testTools struct{ servers []testServer }

func (r *testTools) Add(s testServer) { r.servers = append(r.servers, s) }

func (r *testTools) Tools() []llm.Tool {
	out := []llm.Tool{}
	for _, s := range r.servers {
		for _, t := range s.Tools() {
			t.Name = s.Name() + "_" + t.Name
			out = append(out, t)
		}
	}
	return out
}

func (r *testTools) Call(ctx context.Context, name string, input map[string]interface{}) (interface{}, error) {
	for _, s := range r.servers {
		if p := s.Name() + "_"; strings.HasPrefix(name, p) {
			return s.Call(ctx, strings.TrimPrefix(name, p), input)
		}
	}
	return nil, errors.New("unknown tool")
}

// textOnlyProvider refuses requests with images, like o3-mini does.
type textOnlyProvider struct{ scriptedProvider }

func (p *textOnlyProvider) ChatStream(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
	for _, m := range req.Messages {
		if len(m.Images) > 0 || len(llm.ToolResultImages(m.ToolResults)) > 0 {
			p.mu.Lock()
			p.reqs = append(p.reqs, req)
			p.mu.Unlock()
			return nil, errors.New("openai: status 400: Invalid content type. image_url is only supported by certain models")
		}
	}
	return p.scriptedProvider.ChatStream(ctx, req)
}

// imageTools returns a tool result with a picture (model_generate's preview).
type imageTools struct{ testTools }

func (*imageTools) Call(context.Context, string, map[string]interface{}) (interface{}, error) {
	return llm.RichResult{Value: map[string]interface{}{"ok": true}, Images: []llm.Image{{MimeType: "image/png", Data: "UA=="}}}, nil
}

func TestEngineDropsImagesForTextOnlyModels(t *testing.T) {
	p := &textOnlyProvider{scriptedProvider{rounds: [][]llm.Chunk{
		{{Type: llm.ChunkToolCall, ToolCall: &llm.ToolCall{ID: "t1", Name: "echo"}}},
		{{Type: llm.ChunkText, Text: "done"}},
	}}}
	out := make(chan Event, 16)
	in := UserInput{Text: "make this", Images: []llm.Image{{MimeType: "image/jpeg", Data: "SQ=="}}}
	go NewEngine(p, &imageTools{}, DefaultEngineConfig()).Run(context.Background(), nil, in, "sess", out,
		func(*ChatMessage) error { return nil })
	evs := drainEvents(t, out)
	assert.Equal(t, EventDone, evs[len(evs)-1].Type, "%+v", evs)
	for _, ev := range evs {
		assert.NotEqual(t, EventError, ev.Type, ev.Error)
	}
	// Refused once, then the rest of the turn goes without images.
	require.Len(t, p.reqs, 3)
	last := p.reqs[2].Messages
	assert.Contains(t, last[0].Content, "can't take images")
	assert.False(t, hasImages(last))
}

// toolRound is a provider round that only calls the echo tool.
func toolRound(id string) []llm.Chunk {
	return []llm.Chunk{{Type: llm.ChunkToolCall, ToolCall: &llm.ToolCall{ID: id, Name: "fake.echo",
		Input: map[string]interface{}{"text": id}}}}
}

func TestEngineWrapsUpAtTheRoundLimit(t *testing.T) {
	// Two rounds allowed, both spent on tools; the wrap-up call answers
	// with text (and a tool call, which isn't run).
	p := &scriptedProvider{rounds: [][]llm.Chunk{
		toolRound("t1"), toolRound("t2"),
		append(toolRound("t3"), llm.Chunk{Type: llm.ChunkText, Text: "Walls are up; racks are left. Say continue."}),
	}}
	reg := &testTools{}
	reg.Add(fakeMCP{})
	cfg := DefaultEngineConfig()
	cfg.MaxToolIters = 2
	e := NewEngine(p, reg, cfg)

	out := make(chan Event, 64)
	var stored []ChatMessage
	appender := func(m *ChatMessage) error { stored = append(stored, *m); return nil }
	go e.Run(context.Background(), nil, UserInput{Text: "build"}, "sess", out, appender)
	events := drainEvents(t, out)

	results, text := 0, ""
	for _, ev := range events {
		switch ev.Type {
		case EventToolResult:
			results++
		case EventText:
			text += ev.Text
		}
	}
	assert.Equal(t, 2, results, "the wrap-up's tool call isn't run")
	assert.Equal(t, "Walls are up; racks are left. Say continue.", text)
	assert.Equal(t, EventDone, events[len(events)-1].Type)

	// The wrap-up request tells the model it's out of rounds, after the
	// tool results; the note isn't stored in the chat.
	require.Len(t, p.reqs, 3)
	last := p.reqs[2].Messages
	assert.Contains(t, last[len(last)-1].Content, "used all 2 tool-call rounds")
	assert.NotEmpty(t, last[len(last)-2].ToolResults)
	assert.NotEmpty(t, p.reqs[2].Tools, "tools stay declared")
	require.NotEmpty(t, stored)
	assert.Equal(t, "assistant", stored[len(stored)-1].Role)
	assert.Equal(t, text, stored[len(stored)-1].Content)
	for _, m := range stored {
		assert.NotContains(t, m.Content, "tool-call rounds")
	}
}

func TestEngineWrapUpFallback(t *testing.T) {
	// The model says nothing after the limit: a fixed reply says so.
	p := &scriptedProvider{rounds: [][]llm.Chunk{toolRound("t1")}}
	reg := &testTools{}
	reg.Add(fakeMCP{})
	cfg := DefaultEngineConfig()
	cfg.MaxToolIters = 1
	e := NewEngine(p, reg, cfg)

	out := make(chan Event, 32)
	go e.Run(context.Background(), nil, UserInput{Text: "build"}, "sess", out, func(*ChatMessage) error { return nil })
	text := ""
	for _, ev := range drainEvents(t, out) {
		if ev.Type == EventText {
			text += ev.Text
		}
	}
	assert.Contains(t, text, "stopped after 1 rounds")
}
