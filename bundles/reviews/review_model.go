package reviews

import (
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/models"
)

// contains information to create a model review
type ModelReview struct {
	// information in a reveiw
	Review *Review
	// information in a model
	Model *models.Model
}

func NewModelReview(review *Review, model *models.Model) (ModelReview, error){
	modelReview := ModelReview{Review:review, Model: model}
	return modelReview, nil
}

type CreateModelReview struct {
	// relay all fields from CreateModel struct
	models.CreateModel

	// relay all fields from CreateReview struct
	CreateReview
}
