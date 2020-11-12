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

	// the branch where the model is on
	Branch *string `json:"branch,omitempty"`

	// reviewers requested for this pull request
	Reviewers []*string `gorm:"-" json:"reviewers,omitempty"`

	// owner organization for this model review
	Owner *string `json:"branch,omitempty"`

	// creator of this pull request
	Creator *string `json:"branch,omitempty"`
}
