package category

import (
	"context"
	"fmt"
	dtos "github.com/gazebo-web/fuel-server/bundles/category/dtos"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/gosimple/slug"
	"github.com/jinzhu/gorm"
)

// Create creates a new Category in DB using the data from
// the given CreateCategory dto.
func (cs *Service) Create(ctx context.Context, tx *gorm.DB,
	newCategory dtos.CreateCategory) (*Category, *gz.ErrMsg) {

	var count int64
	// Sanity check: Make sure that the category name is not already present.
	if err := tx.Model(&Category{}).Where("name = ?", newCategory.Name).Count(&count).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorResourceExists, err)
	}

	if count > 0 {
		return nil, gz.NewErrorMessage(gz.ErrorResourceExists)
	}

	// Undelete the category, if it was deleted. Otherwise, create the category.
	var oldCategory Category
	tx.Unscoped().Where("name = ?", newCategory.Name).Find(&oldCategory)

	if oldCategory.DeletedAt != nil {
		var restoredCategory *Category
		var err *gz.ErrMsg
		if restoredCategory, err = restoreCategory(tx, oldCategory, newCategory); err != nil {
			return nil, err
		}
		gz.LoggerFromContext(ctx).Info(fmt.Sprintf("Category [%s] %s has been restored.", *restoredCategory.Slug, *restoredCategory.Name))
		return restoredCategory, nil
	}

	createdCategory, err := createCategory(tx, newCategory)
	if err != nil {
		return nil, err
	}
	gz.LoggerFromContext(ctx).Info(fmt.Sprintf("Category [%s] %s has been created.", *createdCategory.Slug, *createdCategory.Name))

	return createdCategory, nil
}

// createCategory creates a new entry in the categories table
func createCategory(tx *gorm.DB, newCategory dtos.CreateCategory) (*Category, *gz.ErrMsg) {
	var cat Category
	var slugFromName string
	if newCategory.Slug == nil || len(*newCategory.Slug) == 0 {
		slugFromName = slug.Make(newCategory.Name)
		cat.Slug = &slugFromName
	} else {
		cat.Slug = newCategory.Slug
	}
	cat.Name = &newCategory.Name
	cat.ParentID = newCategory.ParentID

	if err := tx.Create(&cat).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}
	return &cat, nil
}

// restoreCategory restores a soft-deleted category.
func restoreCategory(tx *gorm.DB, oldCategory Category, newCategory dtos.CreateCategory) (*Category, *gz.ErrMsg) {
	oldCategory.DeletedAt = nil
	oldCategory.ParentID = newCategory.ParentID
	if err := tx.Unscoped().Save(oldCategory).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}
	return &oldCategory, nil
}
