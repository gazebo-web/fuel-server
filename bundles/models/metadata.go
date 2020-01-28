package models

import (
	"time"
)

// ModelMetadatum contains a key-value pair for a model.
//
// swagger:model dbModel
type ModelMetadatum struct {
	// Override default GORM Model fields
	ID        uint      `gorm:"primary_key" json:"-"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`
	UpdatedAt time.Time

	ModelID uint
	// The name of the model
	Key *string `json:"key,omitempty"`

	Value *string `json:"value,omitempty"`
}

// IsEmpty returns true is the ModelMetadatum has no key and value.
func (mm ModelMetadatum) IsEmpty() bool {
	return (mm.Key == nil || len(*mm.Key) == 0) && (mm.Value == nil || len(*mm.Value) == 0)
}

// ModelMetadata is an array of Metadatum
//
// swagger:model
type ModelMetadata []ModelMetadatum
