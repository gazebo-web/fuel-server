package category

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCategory_HasParent(t *testing.T) {
	name := "Example"
	slug := "example"

	parentCategory := Category{
		Name:     &name,
		Slug:     &slug,
		ParentID: nil,
	}

	childCategory := Category{
		Name:     &name,
		Slug:     &slug,
		ParentID: &parentCategory.ID,
	}

	assert.False(t, parentCategory.HasParent(), "A Parent category should not have a ParentID")
	assert.True(t, childCategory.HasParent(), "A Child category should have a ParentID")
}

func TestCategoriesToStrSlice(t *testing.T) {
	name := "Example"
	slug := "example"

	sl := []string{name, name, name}

	c := Category{
		Name:     &name,
		Slug:     &slug,
		ParentID: nil,
	}

	cts := Categories{c, c, c}

	result := CategoriesToStrSlice(cts)
	assert.Equal(t, sl, result)
}
