package comments

import (
	"time"

	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
)

type Comment struct {
	// Override default GORM Model fields
	ID        uint      `gorm:"primary_key" json:"-"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`
	UpdatedAt time.Time `gorm:"type:timestamp(3) NULL"`

	// for casbin permission tracking
	UUID *string `json:"-"`

	// User that submitted the comment
	Owner *string `json:"owner"`

	// Main body of the comment
	Body *string `json:"body"`

	// Number of likes
	Likes *int `json:"likes"`

	// TODO: Support emote reactions?
	// Reactions *Reactions
}

func NewComment(
	owner string,
	body string,
) (Comment, error) {
	uuid, _, err := users.NewUUID(owner, "comments")
	if err != nil {
		return Comment{}, err
	}

	likes := 0
	return Comment{
		UUID:      &uuid,
		Owner:     &owner,
		Body:      &body,
		Likes:     &likes,
		UpdatedAt: time.Now(),
		CreatedAt: time.Now(),
	}, nil
}

// swagger:model
type PostComment struct {
	Body string `json:"body"`
}
