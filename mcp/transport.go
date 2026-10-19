package mcp

import (
	"context"

	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/jinzhu/gorm"
)

// NewToolContext builds a ToolContext for calling a handler directly.
func NewToolContext(ctx context.Context, tx *gorm.DB, user *users.User, scopes []string) *ToolContext {
	return &ToolContext{Ctx: ctx, DB: tx, User: user, Scopes: scopes}
}
