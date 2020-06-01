package category

import (
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/ign-go"
)

// List returns a list of categories.
func (cs *CategoryService) List(tx *gorm.DB) (*Categories, *ign.ErrMsg) {
	// Get the categories
	var categories Categories

	q := tx.Model(&Category{})

	if err := q.Find(&categories).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}
	return &categories, nil
}
