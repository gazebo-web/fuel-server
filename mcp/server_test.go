package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestServer wires a registry with one echo tool and returns a Server
// ready for dispatch.
func newTestServer() *Server {
	reg := NewRegistry()
	reg.Register(Tool{
		Spec: ToolSpec{
			Name:        "echo",
			Description: "Return the msg argument as a text block.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"msg": map[string]any{"type": "string"},
				},
				"required": []string{"msg"},
			},
		},
		Handler: func(_ *ToolContext, args map[string]any) (CallToolResult, error) {
			msg, _ := args["msg"].(string)
			return TextResult(msg), nil
		},
	})
	return NewServer(reg)
}

// doRPC marshals a request, runs it through the server's transport,
// and returns the decoded response for assertions.
func doRPC(t *testing.T, srv *Server, id, method string, params any) Response {
	t.Helper()
	payload := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		payload["params"] = params
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/1.0/mcp", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTPWith(w, r, &ToolContext{Ctx: context.Background()})
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", w.Body.String(), err)
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("content-type = %q, want application/json", got)
	}
	return resp
}

func TestInitialize(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "1", "initialize", map[string]any{"protocolVersion": "2025-11-25"})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	// Result is an untyped interface here — round-trip to InitializeResult.
	raw, _ := json.Marshal(resp.Result)
	var init InitializeResult
	if err := json.Unmarshal(raw, &init); err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if init.ProtocolVersion != ProtocolVersion {
		t.Errorf("protocol version = %q, want %q", init.ProtocolVersion, ProtocolVersion)
	}
	if init.ServerInfo.Name != ServerName {
		t.Errorf("server name = %q, want %q", init.ServerInfo.Name, ServerName)
	}
	if _, ok := init.Capabilities["tools"]; !ok {
		t.Errorf("capabilities missing tools key; got %+v", init.Capabilities)
	}
}

func TestToolsList(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "2", "tools/list", nil)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	raw, _ := json.Marshal(resp.Result)
	var list ListToolsResult
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if len(list.Tools) != 1 || list.Tools[0].Name != "echo" {
		t.Errorf("tools = %+v, want [echo]", list.Tools)
	}
}

func TestToolsCall_Echo(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "3", "tools/call", map[string]any{
		"name":      "echo",
		"arguments": map[string]any{"msg": "hello"},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	raw, _ := json.Marshal(resp.Result)
	var result CallToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if result.IsError {
		t.Errorf("result marked isError; content = %+v", result.Content)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" || result.Content[0].Text != "hello" {
		t.Errorf("content = %+v, want one text block with 'hello'", result.Content)
	}
}

func TestToolsCall_UnknownTool(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "4", "tools/call", map[string]any{"name": "nope"})
	if resp.Error == nil {
		t.Fatalf("expected RPCError")
	}
	if resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeMethodNotFound)
	}
}

func TestUnknownMethod(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "5", "pigs/fly", nil)
	if resp.Error == nil || resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("got %+v, want method-not-found", resp.Error)
	}
}

func TestBadJSONRPCVersion(t *testing.T) {
	srv := newTestServer()
	body := []byte(`{"jsonrpc":"1.0","id":"6","method":"initialize"}`)
	r := httptest.NewRequest(http.MethodPost, "/1.0/mcp", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTPWith(w, r, &ToolContext{Ctx: context.Background()})
	var resp Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeInvalidRequest {
		t.Errorf("got %+v, want invalid-request", resp.Error)
	}
}

func TestPATScopeGate(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Tool{
		Spec:           ToolSpec{Name: "write_thing", Description: "x", InputSchema: map[string]any{"type": "object"}},
		Handler:        func(_ *ToolContext, _ map[string]any) (CallToolResult, error) { return TextResult("ok"), nil },
		RequiredScopes: []string{"fuel:write"},
	})
	srv := NewServer(reg)

	// PAT with an unrelated scope: expect a tool-level error, not success.
	r := httptest.NewRequest(http.MethodPost, "/1.0/mcp",
		bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":"7","method":"tools/call","params":{"name":"write_thing"}}`)))
	w := httptest.NewRecorder()
	srv.ServeHTTPWith(w, r, &ToolContext{Ctx: context.Background(), Scopes: []string{"fuel:read"}})
	var resp Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error != nil {
		t.Fatalf("expected tool-level error, got rpc error: %+v", resp.Error)
	}
	raw, _ := json.Marshal(resp.Result)
	var result CallToolResult
	_ = json.Unmarshal(raw, &result)
	if !result.IsError {
		t.Errorf("expected result.IsError=true; got %+v", result)
	}

	// JWT caller (no scopes) bypasses the gate.
	r2 := httptest.NewRequest(http.MethodPost, "/1.0/mcp",
		bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":"8","method":"tools/call","params":{"name":"write_thing"}}`)))
	w2 := httptest.NewRecorder()
	srv.ServeHTTPWith(w2, r2, &ToolContext{Ctx: context.Background()})
	var resp2 Response
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2.Error != nil {
		t.Fatalf("JWT caller failed: %+v", resp2.Error)
	}
	raw2, _ := json.Marshal(resp2.Result)
	var result2 CallToolResult
	_ = json.Unmarshal(raw2, &result2)
	if result2.IsError {
		t.Errorf("JWT caller should not hit scope gate; got %+v", result2)
	}
}
