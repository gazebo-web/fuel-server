// Package mcp is the AI tool registry (the aitools package registers the
// tools) and its two front ends: the public Model Context Protocol server,
// built on the official Go SDK (sdk.go), and Registry.Call, which the
// in-app agent uses. Callers are authenticated before a tool runs, so
// handlers can trust the user they are given.
package mcp

import (
	"encoding/base64"
)

// ToolSpec describes one tool to the client.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// CallToolResult wraps a tool's output in MCP's content-block format.
// IsError is true for tool-level failures (e.g. RBAC rejections, upstream
// errors) that the model can react to.
type CallToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a single entry in a CallToolResult.Content slice: text,
// or an image (base64 Data with its MimeType), e.g. a generated model's
// preview for the agent to check (plan §5.5).
type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// ImageBlock is an image content block.
func ImageBlock(mimeType string, data []byte) ContentBlock {
	return ContentBlock{Type: "image", Data: base64.StdEncoding.EncodeToString(data), MimeType: mimeType}
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
