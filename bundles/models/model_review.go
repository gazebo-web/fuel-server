package models

import (
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/reviews"
)

// contains information to create a model review
type ModelReview struct {
	// information in a reveiw
	reviews.Review
	// information in a model
	Model
}

type CreateModelReview struct {
	// relay all fields from CreateModel struct
	CreateModel

	// relay all fields from CreateReview struct
	reviews.CreateReview
}
