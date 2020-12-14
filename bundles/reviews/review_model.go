package reviews

import (
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


