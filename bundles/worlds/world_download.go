package worlds

import (
	"github.com/jinzhu/gorm"
)

// WorldDownload represents a single download of a world.
//
type WorldDownload struct {
	gorm.Model

	// The ID of the user that made the download
	UserID *uint
	// The ID of the world that was downloaded
	WorldID *uint
	// User-Agent sent in the http request (optional)
	UserAgent string
}
