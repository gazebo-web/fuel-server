package mcp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func testRegistry() *Registry {
	reg := NewRegistry()
	obj := map[string]any{"type": "object"}
	reg.Register(Tool{Spec: ToolSpec{Name: "fuel_search_models", InputSchema: obj},
		RequiredScopes: []string{"fuel:read"}, Annotations: Annotations{ReadOnly: true, OpenWorld: true},
		Handler: func(tc *ToolContext, args map[string]any) (CallToolResult, error) {
			return TextResult("found " + argText(args, "query")), nil
		}})
	reg.Register(Tool{Spec: ToolSpec{Name: "project_write_file", InputSchema: obj},
		RequiredScopes: []string{"project:write"}, Annotations: Annotations{Destructive: true},
		Handler: func(tc *ToolContext, args map[string]any) (CallToolResult, error) {
			return CallToolResult{Content: []ContentBlock{{Type: "text", Text: "written"},
				ImageBlock("image/png", []byte("png"))}}, nil
		}})
	reg.Register(Tool{Spec: ToolSpec{Name: "sdf_validate", InputSchema: obj},
		Annotations: Annotations{ReadOnly: true},
		Handler: func(tc *ToolContext, args map[string]any) (CallToolResult, error) {
			panic("boom")
		}})
	return reg
}

func argText(args map[string]any, k string) string {
	s, _ := args[k].(string)
	return s
}

// connect runs the registry's SDK server for c over in-memory transports.
func connect(t *testing.T, reg *Registry, c Caller) *sdk.ClientSession {
	t.Helper()
	ct, st := sdk.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := reg.SDKServer(c).Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func toolNames(t *testing.T, cs *sdk.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, tool := range res.Tools {
		out = append(out, tool.Name)
	}
	sort.Strings(out)
	return out
}

func TestSDKServerScopes(t *testing.T) {
	reg := testRegistry()
	all := toolNames(t, connect(t, reg, Caller{}))
	if strings.Join(all, " ") != "fuel_search_models project_write_file sdf_validate" {
		t.Fatalf("full access sees %v", all)
	}
	read := toolNames(t, connect(t, reg, Caller{Scopes: []string{"fuel:read"}}))
	if strings.Join(read, " ") != "fuel_search_models sdf_validate" {
		t.Fatalf("fuel:read sees %v", read)
	}
	if none := toolNames(t, connect(t, reg, Caller{Scopes: []string{}})); strings.Join(none, " ") != "sdf_validate" {
		t.Fatalf("no scopes sees %v", none)
	}

	// Calling a tool the scopes don't allow is refused even if named.
	res, err := reg.Call(context.Background(), Caller{Scopes: []string{"fuel:read"}}, "project_write_file", nil)
	if err != nil || !res.IsError || !strings.Contains(res.Content[0].Text, "project:write") {
		t.Fatalf("out-of-scope call: %+v %v", res, err)
	}
}

func TestSDKServerCalls(t *testing.T) {
	cs := connect(t, testRegistry(), Caller{})
	ctx := context.Background()
	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "fuel_search_models", Arguments: map[string]any{"query": "pallet"}})
	if err != nil || res.IsError || res.Content[0].(*sdk.TextContent).Text != "found pallet" {
		t.Fatalf("search: %+v %v", res, err)
	}
	res, err = cs.CallTool(ctx, &sdk.CallToolParams{Name: "project_write_file"})
	if err != nil || len(res.Content) != 2 || string(res.Content[1].(*sdk.ImageContent).Data) != "png" {
		t.Fatalf("image content: %+v %v", res, err)
	}
	// A panicking tool is a tool error, and the server keeps going.
	res, err = cs.CallTool(ctx, &sdk.CallToolParams{Name: "sdf_validate"})
	if err != nil || !res.IsError || !strings.Contains(res.Content[0].(*sdk.TextContent).Text, "boom") {
		t.Fatalf("panic: %+v %v", res, err)
	}

	tools, _ := cs.ListTools(ctx, nil)
	for _, tool := range tools.Tools {
		a := tool.Annotations
		switch tool.Name {
		case "fuel_search_models":
			if !a.ReadOnlyHint || a.OpenWorldHint == nil || !*a.OpenWorldHint {
				t.Errorf("search annotations %+v", a)
			}
		case "project_write_file":
			if a.ReadOnlyHint || a.DestructiveHint == nil || !*a.DestructiveHint {
				t.Errorf("write annotations %+v", a)
			}
		}
	}
}

func TestLimiter(t *testing.T) {
	reg := testRegistry()
	l := NewLimiter(Limits{CallsPerMinute: 6, SearchesPerMinute: 2, WritesPerMinute: 3}, reg)
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }

	// Searches: a burst of twice the rate, then a wait.
	for i := 0; i < 4; i++ {
		if w := l.Allow("a", "fuel_search_models"); w != 0 {
			t.Fatalf("search %d waited %v", i, w)
		}
	}
	if w := l.Allow("a", "fuel_search_models"); w <= 0 || w > 31*time.Second {
		t.Fatalf("5th search should wait about 30s, got %v", w)
	}
	// The refused search wasn't counted against the overall limit, and
	// another caller is unaffected.
	if w := l.Allow("b", "fuel_search_models"); w != 0 {
		t.Fatalf("other caller waited %v", w)
	}
	for i := 0; i < 6; i++ {
		if w := l.Allow("a", "project_write_file"); (w != 0) != (i >= 6) {
			t.Fatalf("write %d waited %v", i, w)
		}
	}
	// 4 searches + 6 writes = 10 of 12 calls; writes are at their burst.
	if w := l.Allow("a", "project_write_file"); w <= 0 {
		t.Fatal("7th write allowed")
	}
	if w := l.Allow("a", "sdf_validate"); w != 0 {
		t.Fatalf("read call waited %v", w)
	}
	now = now.Add(time.Minute)
	if w := l.Allow("a", "fuel_search_models"); w != 0 {
		t.Fatalf("after a minute, search waited %v", w)
	}
}

func TestLimiterMiddleware(t *testing.T) {
	l := NewLimiter(Limits{CallsPerMinute: 1, SearchesPerMinute: 1, WritesPerMinute: 1}, testRegistry())
	var got []string
	h := l.Middleware(func(*http.Request) string { return "me" })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		got = append(got, b.String())
	}))
	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sdf_validate"}}`
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/mcp", strings.NewReader(call)))
		if w.Code != 200 {
			t.Fatalf("call %d: %d", i, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/mcp", strings.NewReader(call)))
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("3rd call: %d %q", w.Code, w.Header().Get("Retry-After"))
	}
	// Other methods aren't counted, and the handler sees the whole body.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)))
	if w.Code != 200 || got[len(got)-1] != `{"jsonrpc":"2.0","id":2,"method":"tools/list"}` {
		t.Fatalf("tools/list: %d %q", w.Code, got)
	}
	if n := calledTools([]byte(`[` + call + `,{"method":"tools/call","params":{"name":"x"}}]`)); len(n) != 2 {
		t.Fatalf("batch: %v", n)
	}
}
