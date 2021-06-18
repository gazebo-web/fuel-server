package reviews

import (
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/comments"
)

// swagger:model
type ModelReviewComment struct {
	comments.Comment

	// Review that this comment is bound to, this refers to `ModelReview.ID`, not `ModelReview.ModelReviewID`
	ModelReviewID uint `gorm:"unique_index:idx_modelreviewcomment_instance_id;not null" json:"-"`

	// locally defined comment id for each review
	InstanceID uint `gorm:"unique_index:idx_modelreviewcomment_instance_id;not null" json:"id"`
}
