package category

import (
	"context"
	"fmt"
	dtos "github.com/gazebo-web/fuel-server/bundles/category/dtos"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/gosimple/slug"
	"github.com/jinzhu/gorm"
)

// Update updates a category in DB using the data from
// the given Service struct.
// Returns a Service.
func (cs *Service) Update(ctx context.Context, tx *gorm.DB,
	categorySlug string, cat dtos.UpdateCategory) (*Category, *gz.ErrMsg) {

	var savedCategory *Category
	var err error
	// Sanity check: Make sure that the category exists.
	if savedCategory, err = BySlug(tx, categorySlug); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNonExistentResource, err)
	}

	updatedCategory := updateCategoryFields(*savedCategory, cat)

	if err := tx.Save(updatedCategory).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	gz.LoggerFromContext(ctx).Info(fmt.Sprintf("Category [%s] %s has been updated.", *updatedCategory.Slug, *updatedCategory.Name))

	return &updatedCategory, nil
}

// updateCategoryFields instantiate a Category entity by the given UpdateCategory dto.
func updateCategoryFields(categoryToUpdate Category, cat dtos.UpdateCategory) Category {
	namedChanged := false
	if cat.Name != nil && cat.Name != categoryToUpdate.Name {
		categoryToUpdate.Name = cat.Name
		namedChanged = true
	}

	if cat.Slug == nil && namedChanged {
		newSlug := slug.Make(*categoryToUpdate.Name)
		categoryToUpdate.Slug = &newSlug
	}

	if cat.Slug != nil && cat.Slug != categoryToUpdate.Slug && slug.IsSlug(*cat.Slug) {
		categoryToUpdate.Slug = cat.Slug
	}

	if cat.ParentID != nil && *cat.ParentID != 0 {
		categoryToUpdate.ParentID = cat.ParentID
	} else {
		categoryToUpdate.ParentID = nil
	}
	return categoryToUpdate
}
