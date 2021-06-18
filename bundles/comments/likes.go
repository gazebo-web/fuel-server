package comments

import "time"

// CommentLike represents a like of a comment.
type CommentLike struct {
	// Override default GORM Model fields
	ID        uint      `gorm:"primary_key"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`
	UpdatedAt time.Time `gorm:"type:timestamp(3) NULL"`

	// The ID of the user that made the like
	UserID *uint `gorm:"unique_index:idx_user_comment_like"`

	// The ID of the comment that was liked
	CommentID *uint `gorm:"unique_index:idx_user_comment_like"`
}
