package reviews

import (
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/comments"
)

// swagger:model
type ReviewComment struct {
	comments.Comment

	// Review that this comment is bound to
	ReviewID uint `json:"reviewId"`
}
