package reviews

import (
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/jinzhu/gorm"
	fuel "gitlab.com/ignitionrobotics/web/fuelserver/proto"
)

// ModelReview contains information to create a review for a model
type ModelReview struct {
	// Review for a model
	Review

	// ModelID that is under review
	ModelID *uint
}

// CreateModelReview contains information for creating a review for a model
type CreateModelReview struct {
	// relay all fields from CreateReview struct
	CreateReview

	// Model ID under review
	ModelID *uint
}

// ModelReviews is an array of ModelReview
type ModelReviews []ModelReview

// ToReviewStatus converts ReviewStatus type to fuel ReviewStatus enum
func ToReviewStatus(status ReviewStatus) fuel.Review_ReviewStatus {
	switch status {
	case ReviewOpen:
		return fuel.Review_Open
	case ReviewMerged:
		return fuel.Review_Merged
	case ReviewClosed:
		return fuel.Review_Closed
	}
	return fuel.Review_Open
}

// ToProto creates a new 'fuel.Review' from the given review.
func (mr *ModelReview) ToProto() interface{} {
	fuelReview := fuel.Review{
		// Note: time.RFC3339 is the format expected by Go's JSON unmarshal
		CreatedAt:   proto.String(mr.Review.CreatedAt.UTC().Format(time.RFC3339)),
		UpdatedAt:   proto.String(mr.Review.UpdatedAt.UTC().Format(time.RFC3339)),
		Creator:     proto.String(*mr.Review.Creator),
		Owner:       proto.String(*mr.Review.Owner),
		Title:       proto.String(*mr.Review.Title),
		Description: proto.String(*mr.Review.Description),
		Branch:      proto.String(*mr.Review.Branch),
		Reviewers:   mr.Review.Reviewers,
		Approvals:   mr.Review.Approvals,
		Private:     mr.Review.Private,
	}

	status := ToReviewStatus(mr.Review.Status)
	fuelReview.Status = &status

	modelID := uint64(*mr.ModelID)

	fuelModelReview := fuel.ModelReview{
		Review:  &fuelReview,
		ModelId: &modelID,
	}

	return &fuelModelReview
}

// NewModelReview creates a new Review struct
func NewModelReview(title, description, owner, branch *string, status ReviewStatus, reviewers, approvals []string, modelID *uint) (ModelReview, error) {
	createTime := time.Now()
	updateTime := time.Now()

	review := Review{CreatedAt: createTime, UpdatedAt: updateTime, Title: title,
		Description: description, Owner: owner, Branch: branch,
		Status: status, Reviewers: reviewers, Approvals: approvals}

	modelReview := ModelReview{Review: review, ModelID: modelID}
	return modelReview, nil
}

// QueryForModelReviews returns a list of reviews for a selected model using modelID
// requested by the user
func QueryForModelReviews(q *gorm.DB, modelID uint) *gorm.DB {
	// get all matched reviews
	q = q.Where("model_id = ?", modelID)
	return q
}
