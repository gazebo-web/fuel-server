package reviews

import (
	"github.com/jinzhu/gorm"
	"time"
)

type ReviewStatus int

const (
	// Open means review is still active
	ReviewOpen ReviewStatus = iota
	// Merged means reivew has been merged
	ReviewMerged
	// Merged means reivew is closed without being merged
	ReviewClosed
)

// TODO: move DB related functions to a DB Accessor. Inject the db accessor to the reviews service.

// Review contains changes proposed for a resource
//
// A review contains changes for a resource such as a model or a world. It is
// also known as a pull request.
//
// swagger:review dbReview
type Review struct {
	// ID of the review
	// Overrides the default GORM Review fields
	ID        uint      `gorm:"primary_key" json:"-"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`
	UpdatedAt time.Time

	// Creator contains the username of the User that created this model (usually
	// got from the JWT)
	Creator *string `json:"creator,omitempty"`

	// Title of the review (max 65,535 chars)
	Title *string `gorm:"type:text" json:"title,omitempty"`

	// Description of the review (max 65,535 chars)
	// Interesting post about TEXT vs VARCHAR(30000) performance:
	// https://nicj.net/mysql-text-vs-varchar-performance/
	Description *string `gorm:"type:text" json:"description,omitempty"`

	// Owner of this review
	Owner *string `json:"owner,omitempty"`

	// Branch associated with this review
	Branch *string `json:"branch,omitempty"`

	// Status of the review
	Status ReviewStatus `json:"status,omitempty"`

	// Reviewers for the review
	Reviewers []string `gorm:"-" json:"reviewers,omitempty"`

	// Approvals for the review
	Approvals []string `gorm:"-" json:"approvals,omitempty"`

	// Private - True to make this a private resource
	Private *bool `gorm:"default:true" json:"private,omitempty"`
}

// Reviews is an array of Review
type Reviews []Review

// QueryForReviews returns a gorm query configured to query Reviews
func QueryForReviews(q *gorm.DB) *gorm.DB {
	return q.Model(&Review{}).Order("id")
}

// CreateReview encapulates data required to create a review
type CreateReview struct {
	// Owner of the model. Must be a user or an org
	// If not set, the current user will be used as the owner
	Owner string `json:"owner" form:"owner"`
	// Reviewers for the review
	Reviewers []string `json:"reviewers" validate:"omitempty" form:"reviewers"`
	// Approvals for the review
	Approvals []string `json:"approvals" validate:"omitempty" form:"approvals"`
	// Description of the review
	Description string `json:"description" form:"description"`
	// Branch associated with the review
	Branch *string `json:"branch" validate:"omitempty" form:"branch"`
	// Status of the review
	Status ReviewStatus `json:"status" form:"status"`
	// Title of the review
	// required: true
	Title string `json:"title" validate:"required,noforwardslash,nopercent" form:"title"`
}
