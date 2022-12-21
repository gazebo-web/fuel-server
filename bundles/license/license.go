package license

import (
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
)

// License is a license name and ID
//
// swagger:model
type License struct {
	gorm.Model
	Name       *string `gorm:"not null;unique" json:"name,omitempty"`
	ContentURL *string `json:"url,omitempty"`
	ImageURL   *string `json:"image_url,omitempty"`
}

// Licenses is an slice of License
// swagger:model
type Licenses []License

// ByID finds and returns a License record from DB.
// It returns error if not found.
func ByID(tx *gorm.DB, id int) (*License, error) {
	var license License
	if err := tx.First(&license, id).Error; err != nil {
		return nil, err
	}
	return &license, nil
}

// List returns a paginated list of licenses.
func List(p *gz.PaginationRequest, tx *gorm.DB) (*Licenses, *gz.PaginationResult, *gz.ErrMsg) {
	// Get the licenses
	var licenses Licenses

	// Create the DB query
	q := tx.Model(&License{})

	pagination, err := gz.PaginateQuery(q, &licenses, *p)
	if err != nil {
		return nil, nil, gz.NewErrorMessageWithBase(gz.ErrorInvalidPaginationRequest, err)
	}
	if !pagination.PageFound {
		return nil, nil, gz.NewErrorMessage(gz.ErrorPaginationPageNotFound)
	}
	return &licenses, pagination, nil
}
