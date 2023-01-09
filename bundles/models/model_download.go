package models

import (
	"github.com/jinzhu/gorm"
)

// ModelDownload represents a single download of a model.
type ModelDownload struct {
	gorm.Model

	// The ID of the user that made the download
	UserID *uint
	// The ID of the model that was downloaded
	ModelID *uint
	// User-Agent sent in the http request (optional)
	UserAgent string
}
