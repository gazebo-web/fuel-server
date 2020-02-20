package worlds

import (
	"time"
)

// WorldLike represents a like of a world.
type WorldLike struct {
	// Override default GORM Model fields
	ID        uint      `gorm:"primary_key"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`
	UpdatedAt time.Time
	// DeletedAt is not included in order to disable the soft delete feature.

	// The ID of the user that made the like
	UserID *uint `gorm:"unique_index:idx_user_world_like"`

	// The ID of the world that was liked
	WorldID *uint `gorm:"unique_index:idx_user_world_like"`
}
