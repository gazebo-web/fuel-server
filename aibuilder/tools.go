package aibuilder

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jinzhu/gorm"

	"github.com/gazebo-web/fuel-server/aibuilder/llm"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/mcp"
)

// userTools are the aitools registry's tools, called in-process as one
// user (design §3.2.2): no HTTP hop to fuel-server and no forwarded token.
// mcp.Registry.Call runs each in its own transaction, as fuel-server's MCP
// endpoint does.
type userTools struct {
	reg  *mcp.Registry
	db   *gorm.DB
	user *users.User
}

func (t *userTools) Tools() []llm.Tool {
	specs := t.reg.Specs()
	out := make([]llm.Tool, 0, len(specs))
	for _, s := range specs {
		out = append(out, llm.Tool{Name: s.Name, Description: s.Description, InputSchema: s.InputSchema})
	}
	return out
}

func (t *userTools) Call(ctx context.Context, name string, input map[string]interface{}) (interface{}, error) {
	// The web app's session has every scope (Scopes nil).
	res, err := t.reg.Call(ctx, mcp.Caller{User: t.user, DB: t.db}, name, input)
	if err != nil {
		return nil, err
	}
	if res.IsError {
		return nil, errors.New(joinText(res.Content))
	}
	return unwrapContent(res.Content), nil
}

func joinText(blocks []mcp.ContentBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// unwrapContent turns a tool's content blocks into the value the model
// sees: one JSON text block decoded, other text as is, and images kept
// beside the value in an llm.RichResult.
func unwrapContent(blocks []mcp.ContentBlock) interface{} {
	var text []mcp.ContentBlock
	var images []llm.Image
	for _, b := range blocks {
		switch b.Type {
		case "image":
			if b.Data != "" {
				images = append(images, llm.Image{MimeType: b.MimeType, Data: b.Data})
			}
		case "text":
			text = append(text, b)
		}
	}
	if len(images) > 0 {
		return llm.RichResult{Value: unwrapText(text), Images: images}
	}
	return unwrapText(text)
}

func unwrapText(blocks []mcp.ContentBlock) interface{} {
	if len(blocks) == 1 {
		trimmed := strings.TrimSpace(blocks[0].Text)
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			var v any
			if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
				return v
			}
		}
		return blocks[0].Text
	}
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, b.Text)
	}
	return out
}
