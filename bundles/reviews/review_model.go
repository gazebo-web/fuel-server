package reviews

import (
	"github.com/golang/protobuf/proto"
	"gitlab.com/ignitionrobotics/web/fuelserver/proto"
	"time"
)

// contains information to create a model review
type ModelReview struct {
	// Review for a model
	Review

	// Model that is under review
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

    modelID := uint64(*mr.ModelID)

	fuelModelReview := fuel.ModelReview{
		Review:	&fuelReview,
		ModelId: &modelID,
	}

	return &fuelModelReview
}
