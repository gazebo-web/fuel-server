package reviews

import (
  "time"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/models"
)

// contains information to create a model review
type ModelReview struct {
  // Override default GORM Model fields
  ID        uint      `gorm:"primary_key"`
  CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`
  UpdatedAt time.Time

	// Review for a model
	Review *Review

  // Model that is under review
	Model *models.Model
}

// ModelReviews is an array of ModelReview
//
type ModelReviews []ModelReview


