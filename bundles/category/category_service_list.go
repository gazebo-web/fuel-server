package category

import (
	"gitlab.com/ignitionrobotics/web/ign-go"
	"github.com/jinzhu/gorm"
)

// List returns a list of categories.
func (cs *Service) List(tx *gorm.DB) (*Categories, *ign.ErrMsg) {
	// Get the categories
	var categories Categories

	q := tx.Model(&Category{})

	if err := q.Find(&categories).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}
	return &categories, nil
}
