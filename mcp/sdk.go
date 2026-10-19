package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The public MCP endpoint (design §3.2.3) is the official Go SDK's
// streamable HTTP server, stateless. Each request gets a server listing
// only the tools the caller's scopes allow; calls run through
// Registry.Call, as the in-app agent's do.

// ServerName and ServerVersion identify this server to clients.
const (
	ServerName    = "gazebo-fuel"
	ServerVersion = "1.0"
)

// Instructions is what clients are told about the server on connect.
const Instructions = "Gazebo Fuel: search and fetch simulation models and worlds, and build worlds and " +
	"models in your private AI Builder projects (SDF for Gazebo). Start a project with " +
	"project_create, read scene_summary before editing, and validate SDF with the write tools' reports."

// SDKServer returns an MCP server exposing the tools the caller may use.
func (r *Registry) SDKServer(c Caller) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: ServerName, Title: "Gazebo Fuel", Version: ServerVersion},
		&sdk.ServerOptions{Instructions: Instructions})
	for _, spec := range r.Specs() {
		t, _ := r.Get(spec.Name)
		if !c.Allowed(t) {
			continue
		}
		s.AddTool(sdkTool(t), r.sdkHandler(c, spec.Name))
	}
	return s
}

func sdkTool(t Tool) *sdk.Tool {
	a := t.Annotations
	ann := &sdk.ToolAnnotations{ReadOnlyHint: a.ReadOnly, IdempotentHint: a.Idempotent || a.ReadOnly,
		OpenWorldHint: &a.OpenWorld}
	if !a.ReadOnly {
		ann.DestructiveHint = &a.Destructive
	}
	schema := t.Spec.InputSchema
	if schema == nil {
		schema = map[string]any{"type": "object"}
	}
	return &sdk.Tool{Name: t.Spec.Name, Description: t.Spec.Description, InputSchema: schema, Annotations: ann}
}

func (r *Registry) sdkHandler(c Caller, name string) sdk.ToolHandler {
	return func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		args := map[string]any{}
		if raw := req.Params.Arguments; len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &args); err != nil {
				return sdkResult(ErrorResult("arguments must be a JSON object: " + err.Error())), nil
			}
		}
		res, err := r.Call(ctx, c, name, args)
		if err != nil {
			return sdkResult(ErrorResult(fmt.Sprintf("%s failed: %v", name, err))), nil
		}
		return sdkResult(res), nil
	}
}

func sdkResult(r CallToolResult) *sdk.CallToolResult {
	out := &sdk.CallToolResult{IsError: r.IsError, Content: []sdk.Content{}}
	for _, b := range r.Content {
		switch b.Type {
		case "image":
			data, err := base64.StdEncoding.DecodeString(b.Data)
			if err != nil {
				continue
			}
			out.Content = append(out.Content, &sdk.ImageContent{Data: data, MIMEType: b.MimeType})
		default:
			out.Content = append(out.Content, &sdk.TextContent{Text: b.Text})
		}
	}
	return out
}
