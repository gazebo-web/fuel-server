package models

import (
	"testing"

	"github.com/gazebo-web/fuel-server/bundles/category"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/stretchr/testify/assert"
)

// TestIsBasicModelListQuery checks which model list queries can use the
// shared memcache result.
func TestIsBasicModelListQuery(t *testing.T) {
	name := "user"
	owner := "owner"
	p := &gz.PaginationRequest{}
	user := &users.User{Username: &name}
	categories := &category.Categories{category.Category{}}
	empty := &category.Categories{}

	assert.True(t, isbasicModelListQuery(p, nil, "", "", nil, nil, nil, false))
	assert.True(t, isbasicModelListQuery(p, nil, "", "", nil, nil, empty, false))
	assert.True(t, isbasicModelListQuery(&gz.PaginationRequest{PageRequested: true, PerPage: 20}, nil, "", "", nil, nil, nil, false))

	// Results for a logged in user can include private models.
	assert.False(t, isbasicModelListQuery(p, nil, "", "", nil, user, nil, false))
	// Category filtered results are a subset of the full list.
	assert.False(t, isbasicModelListQuery(p, nil, "", "", nil, nil, categories, false))
	assert.False(t, isbasicModelListQuery(p, &owner, "", "", nil, nil, nil, false))
	assert.False(t, isbasicModelListQuery(p, nil, "asc", "", nil, nil, nil, false))
	assert.False(t, isbasicModelListQuery(p, nil, "", "search", nil, nil, nil, false))
	assert.False(t, isbasicModelListQuery(p, nil, "", "", user, nil, nil, false))
	assert.False(t, isbasicModelListQuery(p, nil, "", "", nil, nil, nil, true))
	assert.False(t, isbasicModelListQuery(nil, nil, "", "", nil, nil, nil, false))
	assert.False(t, isbasicModelListQuery(&gz.PaginationRequest{PageRequested: true, PerPage: 50}, nil, "", "", nil, nil, nil, false))
}
