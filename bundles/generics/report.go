package generics

import (
	"github.com/jinzhu/gorm"
)

// Report represents a generic resource report
type Report struct {
	gorm.Model
	// Reason is the justification why the resource was reported
	Reason *string `gorm:"type:text" json:"reason,omitempty"`
}
