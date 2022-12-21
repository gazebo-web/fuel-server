package category

import (
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
)

// List returns a list of categories.
func (cs *Service) List(tx *gorm.DB) (*Categories, *gz.ErrMsg) {
	// Get the categories
	var categories Categories

	q := tx.Model(&Category{})

	if err := q.Find(&categories).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}
	return &categories, nil
}
