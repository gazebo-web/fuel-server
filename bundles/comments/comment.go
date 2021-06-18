package comments

import "time"

type Comment struct {
	// Override default GORM Model fields
	ID        uint      `gorm:"primary_key" json:"-"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`
	UpdatedAt time.Time `gorm:"type:timestamp(3) NULL"`

	// User that submitted the comment
	Owner *string `json:"owner"`

	// Main body of the comment
	Body *string `json:"body"`

	// Number of likes
	Likes *int `json:"likes"`

	// TODO: Support emote reactions?
	// Reactions *Reactions
}

// swagger:model
type PostComment struct {
	Body string `json:"body"`
}
