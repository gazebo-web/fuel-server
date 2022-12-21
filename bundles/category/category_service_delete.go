package category

import (
	"context"
	"fmt"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
)

// Delete deletes a category by the given slug.
func (cs *Service) Delete(ctx context.Context, tx *gorm.DB, categorySlug string) (*Category, *gz.ErrMsg) {
	var cat *Category
	var err error
	// Sanity check: Make sure that the category exists.
	if cat, err = BySlug(tx, categorySlug); err != nil {
		return nil, gz.NewErrorMessage(gz.ErrorNonExistentResource)
	}

	if err := tx.Delete(&Category{}, "slug = ?", cat.Slug).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}

	// Update all child categories to remove parent ID.
	var isParent *bool
	if isParent, err = cat.IsParent(tx); err == nil {
		if *isParent {
			tx.Unscoped().Model(Category{}).Where("parent_id = ?", cat.ID).UpdateColumn("parent_id", nil)
		}
	} else {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}

	gz.LoggerFromContext(ctx).Info(fmt.Sprintf("Category [%s] %s has been removed.", *cat.Slug, *cat.Name))
	return cat, nil
}
