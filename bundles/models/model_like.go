package models

import (
	"time"
)

// ModelLike represents a like of a model.
type ModelLike struct {
	// Override default GORM Model fields
	ID        uint      `gorm:"primary_key"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`
	UpdatedAt time.Time
	// DeletedAt is not included in order to disable the soft delete feature.

	// The ID of the user that made the like
	UserID *uint `gorm:"unique_index:idx_user_model_like"`

	// The ID of the model that was liked
	ModelID *uint `gorm:"unique_index:idx_user_model_like"`
}
