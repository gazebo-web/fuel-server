package commonres

// Metadatum contains a key-value pair for a resources.
//
// swagger:model dbModel
type Metadatum struct {
	// Key is the string containing the metadata key value.
	Key *string `json:"key,omitempty"`

	// Value is the string containing the metadata value associated with the key.
	Value *string `json:"value,omitempty"`
}

// IsEmpty returns true if the Metadatum has no key and value.
func (mm Metadatum) IsEmpty() bool {
	return (mm.Key == nil || len(*mm.Key) == 0) && (mm.Value == nil || len(*mm.Value) == 0)
}

// Metadata is an array of Metadatum
//
// swagger:model
type Metadata []Metadatum
