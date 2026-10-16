// Package mcp implements a Model Context Protocol server that exposes
// Fuel, SDF, and Project tools to LLM agents. It speaks the "Streamable
// HTTP" transport (JSON-RPC 2.0 over a single POST endpoint). The auth
// middleware resolves a user before the handler runs, so tool handlers
// can trust the caller identity the server passes them.
package mcp

import "encoding/json"

// ProtocolVersion is the MCP protocol revision this server advertises in
// its initialize response. Clients that request a newer version will
// downgrade; older clients are accepted as long as they only use methods
// this revision supports.
const ProtocolVersion = "2025-11-25"

// ServerName is the human-readable identifier returned from initialize.
const ServerName = "fuel-server"

// Request is a JSON-RPC 2.0 request envelope.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // opaque: string, number, or null
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response envelope. Exactly one of Result or
// Error should be populated.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes (-32700..-32600) plus our extensions.
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603

	// Application-level codes. Kept in the reserved -32000..-32099 range.
	ErrCodeUnauthorized = -32001
	ErrCodeForbidden    = -32002
	ErrCodeToolExec     = -32010
)

// InitializeResult is the payload of a successful `initialize` response.
type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	ServerInfo      ServerInfo     `json:"serverInfo"`
	Capabilities    map[string]any `json:"capabilities"`
}

// ServerInfo identifies the fuel-server MCP implementation.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolSpec describes one tool to the client.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ListToolsResult is the payload of a successful `tools/list` response.
type ListToolsResult struct {
	Tools []ToolSpec `json:"tools"`
}

// CallToolParams is the params shape for `tools/call` requests.
type CallToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// CallToolResult wraps a tool's output in MCP's content-block format.
// isError is true for tool-level failures (e.g. RBAC rejections, upstream
// errors) that the LLM can react to, reserving RPCError for transport /
// protocol failures that terminate the session.
type CallToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a single entry in a CallToolResult.Content slice. Only
// text blocks are used today; MCP also supports image and resource
// blocks, which we can add when a tool needs them.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// TextResult is a convenience constructor for a single-text-block result.
func TextResult(text string) CallToolResult {
	return CallToolResult{Content: []ContentBlock{{Type: "text", Text: text}}}
}

// ErrorResult is a convenience constructor for a tool-level failure.
// The LLM sees this as a tool result with isError=true, not as a JSON-RPC
// transport error.
func ErrorResult(text string) CallToolResult {
	return CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
		IsError: true,
	}
}

