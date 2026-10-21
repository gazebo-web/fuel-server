package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/jinzhu/gorm"
)

// Caller is who calls a tool: the user, the scopes their credential grants
// (nil means every scope, as for the web app's own session) and the
// database to run the call against.
type Caller struct {
	User   *users.User
	Scopes []string
	DB     *gorm.DB
}

// Allowed reports whether the caller has every scope the tool requires.
func (c Caller) Allowed(t Tool) bool {
	return missingScopes(c.Scopes, t.RequiredScopes) == nil
}

func missingScopes(have, need []string) []string {
	if have == nil {
		return nil
	}
	set := make(map[string]bool, len(have))
	for _, s := range have {
		set[s] = true
	}
	var missing []string
	for _, s := range need {
		if !set[s] {
			missing = append(missing, s)
		}
	}
	return missing
}

// Call runs a tool as the caller, in its own transaction (committed unless
// the handler fails with a Go error, rolled back then). A missing scope or
// a panic becomes a tool error the model can read; a Go error from the
// handler is returned.
func (r *Registry) Call(ctx context.Context, c Caller, name string, args map[string]any) (CallToolResult, error) {
	t, ok := r.Get(name)
	if !ok {
		return ErrorResult(fmt.Sprintf("unknown tool %q", name)), nil
	}
	if missing := missingScopes(c.Scopes, t.RequiredScopes); missing != nil {
		return ErrorResult(fmt.Sprintf("%s needs the %s permission, which this connection wasn't granted; "+
			"reconnect and approve it", name, strings.Join(missing, " and "))), nil
	}
	if args == nil {
		args = map[string]any{}
	}
	if c.DB == nil || t.OwnTransactions { // no database (tests), or the tool manages its own
		return run(t, &ToolContext{Ctx: ctx, DB: c.DB, User: c.User, Scopes: c.Scopes}, args)
	}
	tx := c.DB.Begin()
	if tx.Error != nil {
		return CallToolResult{}, tx.Error
	}
	res, err := run(t, &ToolContext{Ctx: ctx, DB: tx, User: c.User, Scopes: c.Scopes}, args)
	if err != nil {
		tx.Rollback()
		return CallToolResult{}, err
	}
	if err := tx.Commit().Error; err != nil {
		return CallToolResult{}, err
	}
	return res, nil
}

// run calls a handler, turning a panic into a tool error so one bad call
// doesn't take the server down.
func run(t Tool, tc *ToolContext, args map[string]any) (res CallToolResult, err error) {
	defer func() {
		if p := recover(); p != nil {
			res, err = ErrorResult(fmt.Sprintf("internal error in %s: %v", t.Spec.Name, p)), nil
		}
	}()
	return t.Handler(tc, args)
}
