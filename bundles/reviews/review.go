package reviews

import (
//	"path"
	"time"

	"github.com/jinzhu/gorm"
//	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
//	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
)

// TODO: move DB related functions to a DB Accessor. Inject the db accessor to the reviews service.

const (
	reviews string = "reviews"
)

// Review represents information about a simulation review
//
// A review contains information about a single simulation object, such
// as a robot, table, or structure.
//
// swagger:review dbReview
type Review struct {
	// Override default GORM Review fields
	ID        uint      `gorm:"primary_key" json:"-"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`
	UpdatedAt time.Time

	// The username of the User that created this model (usually got from the JWT)
	Creator *string `json:"creator,omitempty"`

	// Title the review (max 65,535 chars)
	Title *string `gorm:"type:text" json:"title,omitempty"`

	// A description of the review (max 65,535 chars)
	// Interesting post about TEXT vs VARCHAR(30000) performance:
	// https://nicj.net/mysql-text-vs-varchar-performance/
	Description *string `gorm:"type:text" json:"description,omitempty"`

	// The owner of this review
	Owner *string `gorm:"unique_index:idx_reviewname_owner" json:"owner,omitempty"`

	// The branch associated with this review
	Branch *string `json:"branch,omitempty"`

	// The status of the review
	Status *string `json:"status,omitempty"`

  // A list of reviewers for the review
  Reviewers []string `gorm:"-" json:"reviewers,omitempty"`

  // A list of approvals for the review
  Approvals []string `gorm:"-" json:"approvals,omitempty"`
}

// NewReview creates a new Review struct
func NewReview(title, description, owner, branch, status *string, reviewers, approvals []string) (Review, error) {
	createTime := time.Now()
	updateTime := time.Now()

	review := Review{CreatedAt: createTime, UpdatedAt: updateTime, Title: title,
		Description: description, Owner: owner, Branch: branch,
		Status: status, Reviewers: reviewers, Approvals: approvals}
	return review, nil
}

// Reviews is an array of Review
//
type Reviews []Review

// QueryForReviews returns a gorm query configured to query Reviews
func QueryForReviews(q *gorm.DB) *gorm.DB {
	return q.Model(&Review{}).Order("id")
}

// CreateReview encapulates data required to create a review
type CreateReview struct {
	// Optional Owner of the model. Must be a user or an org
	// If not set, the current user will be used as the owner
	Owner string `json:"owner" form:"owner"`
	// A list of reviewers for the review
	Reviewers []string `json:"reviewers" validate:"omitempty" form:"reviewers"`
	// a list of approved reviewers
	Approvals []string `json:"approvals" validate:"omitempty" form:"approvals"`
	// Description of the review
	Description string `json:"description" form:"description"`
	// The branch associated with the review
	// required: true
	Branch string `json:"branch" validate:"required" form:"branch"`
	// The status of the review
	Status string `json:"status" form:"status"`
	// The title of the review
	// required: true
	Title string `json:"title" validate:"required, noforwardslash,nopercent" form:"title"`
}
