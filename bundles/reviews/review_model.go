package reviews

import (
	"time"

	"github.com/golang/protobuf/proto"
	"gitlab.com/ignitionrobotics/web/fuelserver/proto"
)

// contains information to create a review for a model
type ModelReview struct {
	// information in a review
	Review

	// Model ID under review
	ModelID *uint
}

// create a review for a model
type CreateModelReview struct {
	// relay all fields from CreateReview struct
	CreateReview

	// Model ID under review
	ModelID *uint
}

// ModelReviews is an array of ModelReview
//
type ModelReviews []ModelReview

// ReviewToProto creates a new 'fuel.Review' from the given review.
func (mr *ModelReview) ToProto() interface{} {
	fuelReview := fuel.Review{
		// Note: time.RFC3339 is the format expected by Go's JSON unmarshal
		CreatedAt:    proto.String(mr.Review.CreatedAt.UTC().Format(time.RFC3339)),
		UpdatedAt:    proto.String(mr.Review.UpdatedAt.UTC().Format(time.RFC3339)),
		Creator:      proto.String(*mr.Review.Creator),
		Owner:        proto.String(*mr.Review.Owner),
		Title:        proto.String(*mr.Review.Title),
		Description:  proto.String(*mr.Review.Description),
		Branch:       proto.String(*mr.Review.Branch),
		Status:       proto.String(*mr.Review.Status),
		Reviewers:    mr.Review.Reviewers,
		Approvals:    mr.Review.Approvals,
		Private:  	  mr.Review.Private,
	}

	fuelModelReview := fuel.ModelReview{
		Review:	&fuelReview,
	}

	return &fuelModelReview
}

// NewReview creates a new Review struct
func NewModelReview(title, description, owner, branch, status *string, reviewers, approvals []string, modelID *uint) (ModelReview, error) {
	createTime := time.Now()
	updateTime := time.Now()

	review := Review{CreatedAt: createTime, UpdatedAt: updateTime, Title: title,
		Description: description, Owner: owner, Branch: branch,
		Status: status, Reviewers: reviewers, Approvals: approvals}

	modelReview := ModelReview{Review: review, ModelID: modelID}
	return modelReview, nil
}
