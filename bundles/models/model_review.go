package models

import (
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/reviews"
)

// contains information to create a model review
type ModelReview struct {
	reviews.Review

	Model
}
