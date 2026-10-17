// Command aismoke runs the AI Builder smoke corpus (design §5.5): a fixed
// set of world-building prompts sent through a running agent, each into a
// fresh project. For every prompt it records whether the agent refused,
// how many edits and tool calls it made, and whether the final world.sdf
// validates and has the expected content. It exits non-zero if any prompt
// fails, so it can gate CI.
//
// Usage:
//
//	AISMOKE_TOKEN=<fuel JWT or access token> go run ./cmd/aismoke \
//	  -fuel http://localhost:8000 -agent http://localhost:8010 \
//	  -provider anthropic -report smoke.json
//
// The agent uses the caller's stored LLM key, so each run costs tokens.
package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gazebo-web/fuel-server/sdf"
)

//go:embed corpus.json
var defaultCorpus []byte

// Case is one corpus prompt and what a good result must contain.
type Case struct {
	ID          string `json:"id"`
	Prompt      string `json:"prompt"`
	MinEntities int    `json:"min_entities"`
	WantInclude bool   `json:"want_include,omitempty"`
	WantJoints  int    `json:"want_joints,omitempty"`
}

// Result is the outcome of one case.
type Result struct {
	ID        string        `json:"id"`
	Passed    bool          `json:"passed"`
	Failures  []string      `json:"failures,omitempty"`
	Writes    int           `json:"writes"`
	ToolCalls int           `json:"tool_calls"`
	Errors    int           `json:"validation_errors"`
	Warnings  int           `json:"validation_warnings"`
	Entities  int           `json:"entities"`
	Includes  int           `json:"includes"`
	Models    int           `json:"project_models"`
	Joints    int           `json:"joints"`
	Duration  time.Duration `json:"duration_ns"`
	Usage     Usage         `json:"usage"`
	Reply     string        `json:"reply,omitempty"`
}

type runner struct {
	fuel, agent, token string
	provider, model    string
	keep               bool
	timeout            time.Duration
	http               *http.Client
}

func main() {
	r := &runner{http: &http.Client{}}
	flag.StringVar(&r.fuel, "fuel", "http://localhost:8000", "fuel-server base URL")
	flag.StringVar(&r.agent, "agent", "http://localhost:8010", "AI Builder agent base URL")
	flag.StringVar(&r.provider, "provider", "", "LLM provider (default: the user's first key)")
	flag.StringVar(&r.model, "model", "", "model id (default: provider default)")
	flag.BoolVar(&r.keep, "keep", false, "keep the smoke projects instead of deleting them")
	flag.DurationVar(&r.timeout, "timeout", 10*time.Minute, "per-prompt timeout")
	corpusPath := flag.String("corpus", "", "corpus JSON (default: the embedded corpus)")
	only := flag.String("only", "", "comma-separated case ids to run")
	reportPath := flag.String("report", "", "write a JSON report here")
	flag.Parse()

	r.token = os.Getenv("AISMOKE_TOKEN")
	if r.token == "" {
		log.Fatal("set AISMOKE_TOKEN to a fuel-server JWT or access token")
	}
	cases, err := loadCorpus(*corpusPath, *only)
	if err != nil {
		log.Fatal(err)
	}

	var results []Result
	failed := 0
	for _, c := range cases {
		res := r.run(c)
		results = append(results, res)
		status := "PASS"
		if !res.Passed {
			status, failed = "FAIL", failed+1
		}
		fmt.Printf("%-4s %-18s writes=%-3d tools=%-3d errors=%-2d entities=%-3d fuel=%-3d models=%-2d in=%-7d cached=%-7d out=%-6d %5.0fs %s\n",
			status, c.ID, res.Writes, res.ToolCalls, res.Errors, res.Entities, res.Includes, res.Models,
			res.Usage.Input+res.Usage.CacheWrite, res.Usage.CacheRead, res.Usage.Output,
			res.Duration.Seconds(), strings.Join(res.Failures, "; "))
	}
	fmt.Printf("%d/%d passed\n", len(cases)-failed, len(cases))

	if *reportPath != "" {
		b, _ := json.MarshalIndent(results, "", "  ")
		if err := os.WriteFile(*reportPath, b, 0644); err != nil {
			log.Fatal(err)
		}
	}
	if failed > 0 {
		os.Exit(1)
	}
}

func loadCorpus(path, only string) ([]Case, error) {
	data := defaultCorpus
	if path != "" {
		var err error
		if data, err = os.ReadFile(path); err != nil {
			return nil, err
		}
	}
	var cases []Case
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, fmt.Errorf("corpus: %w", err)
	}
	if only == "" {
		return cases, nil
	}
	want := map[string]bool{}
	for _, id := range strings.Split(only, ",") {
		want[strings.TrimSpace(id)] = true
	}
	var out []Case
	for _, c := range cases {
		if want[c.ID] {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *runner) run(c Case) Result {
	start := time.Now()
	res := Result{ID: c.ID}
	name := fmt.Sprintf("smoke-%s-%d", c.ID, start.Unix())
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	if err := r.call(ctx, "POST", r.fuel+"/1.0/projects", map[string]string{
		"name": name, "gazebo_version": "Harmonic", "sdf_version": "1.9", "ros2_distro": "Jazzy",
	}, nil); err != nil {
		res.Failures = append(res.Failures, "create project: "+err.Error())
		return res
	}
	if !r.keep {
		defer func() {
			_ = r.call(context.Background(), "DELETE", r.fuel+"/1.0/projects/"+url.PathEscape(name), nil, nil)
		}()
	}

	stream, err := r.open(ctx, r.agent+"/1.0/ai-builder/projects/"+url.PathEscape(name)+"/chat", map[string]string{
		"message": c.Prompt, "provider": r.provider, "model": r.model,
	})
	if err != nil {
		res.Failures = append(res.Failures, "chat: "+err.Error())
		return res
	}
	t := tally{}
	err = readSSE(stream, t.add)
	stream.Close()
	res.Duration = time.Since(start)
	res.Writes, res.ToolCalls, res.Reply = t.writes, t.toolCalls, truncate(t.reply.String(), 400)
	res.Usage = t.usage
	if err != nil {
		res.Failures = append(res.Failures, "stream: "+err.Error())
	}
	if t.err != "" {
		res.Failures = append(res.Failures, "agent error: "+t.err)
	}

	var world []byte
	err = r.call(ctx, "GET", r.fuel+"/1.0/projects/"+url.PathEscape(name)+"/tip/files/world.sdf", nil, &world)
	if err != nil {
		res.Failures = append(res.Failures, "no world.sdf (refused or did nothing)")
		return res
	}
	load := func(model string) (string, bool) {
		var b []byte
		u := r.fuel + "/1.0/projects/" + url.PathEscape(name) + "/tip/files/models/" + url.PathEscape(model) + "/model.sdf"
		if r.call(ctx, "GET", u, nil, &b) != nil {
			return "", false
		}
		return string(b), true
	}
	score(c, string(world), load, &res)
	return res
}

// modelLoader returns the model.sdf of a project model (models/<name>/),
// so includes of model://<name> are scored with their contents.
type modelLoader func(name string) (string, bool)

// score fills in validation and content checks for the final world.
func score(c Case, world string, load modelLoader, res *Result) {
	v := sdf.Validate(world)
	res.Errors, res.Warnings = len(v.Errors), len(v.Warnings)
	if res.Errors > 0 {
		res.Failures = append(res.Failures, fmt.Sprintf("%d validation errors (first: %s)", res.Errors, v.Errors[0].Message))
	}
	if s, err := sdf.Summarize(world); err == nil {
		countEntities(s.Entities, load, map[string]bool{}, res)
	}
	if res.Writes == 0 {
		res.Failures = append(res.Failures, "agent made no edits")
	}
	if res.Entities < c.MinEntities {
		res.Failures = append(res.Failures, fmt.Sprintf("%d entities, want >= %d", res.Entities, c.MinEntities))
	}
	if c.WantInclude && res.Includes == 0 {
		res.Failures = append(res.Failures, "no Fuel includes")
	}
	if res.Joints < c.WantJoints {
		res.Failures = append(res.Failures, fmt.Sprintf("%d joints, want >= %d", res.Joints, c.WantJoints))
	}
	res.Passed = len(res.Failures) == 0
}

func countEntities(es []sdf.Entity, load modelLoader, inside map[string]bool, res *Result) {
	for _, e := range es {
		switch e.Kind {
		case "model", "include", "actor":
			res.Entities++
		}
		if e.Kind == "include" {
			if model, ok := strings.CutPrefix(strings.TrimSpace(e.URI), "model://"); ok {
				res.Models++
				countProjectModel(strings.TrimSuffix(model, "/"), load, inside, res)
			} else {
				res.Includes++
			}
		}
		res.Joints += e.Joints
		countEntities(e.Children, load, inside, res)
	}
}

// countProjectModel adds the joints and nested includes of one included
// project model; the include itself is already counted as an entity.
// inside holds the models being expanded, to stop include cycles.
func countProjectModel(name string, load modelLoader, inside map[string]bool, res *Result) {
	if inside[name] || load == nil {
		return
	}
	inside[name] = true
	defer delete(inside, name)
	src, ok := load(name)
	if !ok {
		return
	}
	s, err := sdf.Summarize(src)
	if err != nil {
		return
	}
	for _, m := range s.Entities {
		res.Joints += m.Joints
		var nested Result
		countEntities(m.Children, load, inside, &nested)
		res.Joints += nested.Joints
		res.Includes += nested.Includes
		res.Models += nested.Models
	}
}

// tally accumulates chat stream events.
// Usage is the agent's token usage for a prompt, summed over its LLM calls
// (the agent's "usage" events).
type Usage struct {
	Calls      int `json:"llm_calls"`
	Input      int `json:"input_tokens"`
	CacheWrite int `json:"cache_write_tokens"`
	CacheRead  int `json:"cache_read_tokens"`
	Output     int `json:"output_tokens"`
	Thinking   int `json:"thinking_tokens"`
}

type tally struct {
	writes, toolCalls int
	reply             strings.Builder
	err               string
	usage             Usage
}

func (t *tally) add(event string, data []byte) {
	var ev struct {
		Text  string `json:"text"`
		Error string `json:"error"`
		Usage *struct {
			Input      int `json:"input_tokens"`
			CacheWrite int `json:"cache_write_tokens"`
			CacheRead  int `json:"cache_read_tokens"`
			Output     int `json:"output_tokens"`
			Thinking   int `json:"thinking_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(data, &ev)
	switch event {
	case "usage":
		if u := ev.Usage; u != nil {
			t.usage.Calls++
			t.usage.Input += u.Input
			t.usage.CacheWrite += u.CacheWrite
			t.usage.CacheRead += u.CacheRead
			t.usage.Output += u.Output
			t.usage.Thinking += u.Thinking
		}
	case "text":
		t.reply.WriteString(ev.Text)
	case "tool_call":
		t.toolCalls++
	case "project_changed":
		t.writes++
	case "error":
		t.err = ev.Error
	}
}

// readSSE calls fn for each server-sent event until the stream ends or a
// "done" or "error" event arrives.
func readSSE(r io.Reader, fn func(event string, data []byte)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	event, data := "", bytes.Buffer{}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if event != "" || data.Len() > 0 {
				fn(event, data.Bytes())
				if event == "done" || event == "error" {
					return nil
				}
			}
			event = ""
			data.Reset()
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	return sc.Err()
}

func (r *runner) request(ctx context.Context, method, u string, body any) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		resp.Body.Close()
		return nil, fmt.Errorf("%s %s: %d %s", method, u, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return resp, nil
}

// call does a request and, if out is a *[]byte, stores the body.
func (r *runner) call(ctx context.Context, method, u string, body any, out *[]byte) error {
	resp, err := r.request(ctx, method, u, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if out != nil {
		*out = b
	}
	return err
}

func (r *runner) open(ctx context.Context, u string, body any) (io.ReadCloser, error) {
	resp, err := r.request(ctx, "POST", u, body)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
