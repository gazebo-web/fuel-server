package category

import (
	"context"
	"fmt"
	"github.com/gosimple/slug"
	"github.com/jinzhu/gorm"
	dtos "gitlab.com/ignitionrobotics/web/fuelserver/bundles/category/dtos"
	"gitlab.com/ignitionrobotics/web/ign-go"
)

// Create creates a new Category in DB using the data from
// the given CreateCategory dto.
func (cs *CategoryService) Create(ctx context.Context, tx *gorm.DB,
	newCategory dtos.CreateCategory) (*Category, *ign.ErrMsg) {

	var count int64
	// Sanity check: Make sure that the category name is not already present.
	if err := tx.Model(&Category{}).Where("name = ?", newCategory.Name).Count(&count).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorResourceExists, err)
	}

	if count > 0 {
		return nil, ign.NewErrorMessage(ign.ErrorResourceExists)
	}

	// Undelete the category, if it was deleted. Otherwise, create the category.
	var oldCategory Category
	tx.Unscoped().Where("name = ?", newCategory.Name).Find(&oldCategory)

	if oldCategory.DeletedAt != nil {
		var restoredCategory *Category
		var err *ign.ErrMsg
		if restoredCategory, err = restoreCategory(tx, oldCategory, newCategory); err != nil {
			return nil, err
		}
		ign.LoggerFromContext(ctx).Info(fmt.Sprintf("Category [%s] %s has been restored.", *restoredCategory.Slug, *restoredCategory.Name))
		return restoredCategory, nil
	}

	createdCategory, err := createCategory(tx, newCategory)
	if err != nil {
		return nil, err
	}
	ign.LoggerFromContext(ctx).Info(fmt.Sprintf("Category [%s] %s has been created.", *createdCategory.Slug, *createdCategory.Name))

	return createdCategory, nil
}

// createCategory creates a new entry in the categories table
func createCategory(tx *gorm.DB, newCategory dtos.CreateCategory) (*Category, *ign.ErrMsg) {
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
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}
	return &cat, nil
}

// restoreCategory restores a soft-deleted category.
func restoreCategory(tx *gorm.DB, oldCategory Category, newCategory dtos.CreateCategory) (*Category, *ign.ErrMsg) {
	oldCategory.DeletedAt = nil
	oldCategory.ParentID = newCategory.ParentID
	if err := tx.Unscoped().Save(oldCategory).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}
	return &oldCategory, nil
}
