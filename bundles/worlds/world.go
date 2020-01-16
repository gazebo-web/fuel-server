package worlds

import (
	"bitbucket.org/ignitionrobotics/ign-fuelserver/bundles/license"
	"bitbucket.org/ignitionrobotics/ign-fuelserver/bundles/models"
	"bitbucket.org/ignitionrobotics/ign-fuelserver/bundles/users"
	"github.com/jinzhu/gorm"
	"time"
)

const (
	worlds string = "worlds"
)

// World represents information about a simulation world.
//
// swagger:model dbWorld
type World struct {
	// Override default GORM Model fields
	ID        uint      `gorm:"primary_key" json:"-"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`
	UpdatedAt time.Time
	// Added 2 milliseconds to DeletedAt field, and added it to the unique index
	// to help disambiguate when soft deleted rows are involved.
	DeletedAt *time.Time `gorm:"type:timestamp(2) NULL; unique_index:idx_world_owner" sql:"index"`

	// The name of the world
	Name *string `gorm:"unique_index:idx_world_owner" json:"name,omitempty"`

	// Unique identifier for the world
	UUID *string `json:"-"`

	// A description of the world (max 65,535 chars)
	// Interesting post about TEXT vs VARCHAR(30000) performance:
	// https://nicj.net/mysql-text-vs-varchar-performance/
	Description *string `gorm:"type:text" json:"description,omitempty"`

	// Number of likes
	Likes int `json:"likes,omitempty"`

	// Bytes of the world, when downloaded as a zip
	Filesize int `json:"filesize,omitempty"`

	// Number of downloads
	Downloads int `json:"downloads,omitempty"`

	// Date and time the world was first uploaded
	UploadDate *time.Time `json:"upload_date,omitempty"`

	// Modification Date and time
	ModifyDate *time.Time `json:"modify_date,omitempty"`

	// Tags associated to this world
	Tags models.Tags `gorm:"many2many:world_tags;" json:"tags,omitempty"`

	// Location of the world on disk
	Location *string `json:"-"`

	// The user who created this world
	Owner *string `gorm:"unique_index:idx_world_owner" json:"owner,omitempty"`

	// The username of the User that created this world (usually got from the JWT)
	Creator *string `json:"creator,omitempty"`

	// Permission - 0: public, 1: owner, (future: team, others)
	Permission int `json:"permission,omitempty"`

	// The license associated to this world
	License   license.License `json:"license,omitempty"`
	LicenseID int             `json:"lic_id,omitempty"`

	// Private - True to make this a private resource
	Private *bool `json:"private,omitempty"`
}

// ModelInclude represents an external model "included" in a world
// Includes are usually in the form of "full urls" or prefixed with "model://"
type ModelInclude struct {
	// Override default GORM Model fields
	ID uint `gorm:"primary_key" json:"-"`
	// Owning world ID
	WorldID      uint `json:"-"`
	WorldVersion *int `json:"world_version"`
	// The owner name of the model
	ModelOwner *string `json:"model_owner,omitempty"`
	// The name of the model
	ModelName *string `json:"model_name,omitempty"`
	// The version of the model
	ModelVersion *int `json:"model_version,omitempty"`
	// The Include type, eg. full_url, model://, etc
	IncludeType *string `json:"type,omitempty"`
}

// ModelIncludes is a slice of ModelInclude
// swagger:model
type ModelIncludes []ModelInclude

// GetID returns the ID
func (w *World) GetID() uint {
	return w.ID
}

// GetName returns the world's name
func (w *World) GetName() *string {
	return w.Name
}

// GetOwner returns the world's owner
func (w *World) GetOwner() *string {
	return w.Owner
}

// GetLocation returns the world's location on disk
func (w *World) GetLocation() *string {
	return w.Location
}

// GetUUID returns the world's UUID
func (w *World) GetUUID() *string {
	return w.UUID
}

// Worlds is an array of World
//
type Worlds []World

// QueryForWorlds returns a gorm query configured to query Worlds with
// preloaded License and Tags.
func QueryForWorlds(q *gorm.DB) *gorm.DB {
	return q.Model(&World{}).Order("id").Preload("Tags").Preload("License")
}

// GetModelByName queries a World by name and owner.
func GetWorldByName(tx *gorm.DB, name string, owner string) (*World, error) {
	var w World
	if err := QueryForWorlds(tx).Where("owner = ? AND name = ?", owner, name).First(&w).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

// NewWorldAndUUID creates a World struct with a new UUID.
func NewWorldAndUUID(name, desc, location, owner, creator *string,
	lic license.License, permission int, tags models.Tags,
	private bool) (World, error) {

	uuidStr, _, err := users.NewUUID(*owner, worlds)
	if err != nil {
		return World{}, err
	}
	return NewWorld(&uuidStr, name, desc, location, owner, creator, lic,
		permission, tags, private)
}

// NewWorld creates a new World struct
func NewWorld(uuidStr, name, desc, location, owner, creator *string,
	lic license.License, permission int, tags models.Tags,
	private bool) (World, error) {

	// Override the generated location if we got a location as argument
	var wPath string
	if location != nil {
		wPath = *location
	} else {
		wPath = users.GetResourcePath(*owner, *uuidStr, worlds)
	}
	uploadDate := time.Now()
	modifyDate := time.Now()
	w := World{Name: name, Owner: owner, Creator: creator, UUID: uuidStr,
		Description: desc, Location: &wPath, Likes: 0, Downloads: 0,
		UploadDate: &uploadDate, ModifyDate: &modifyDate, Tags: tags,
		License: lic, Permission: permission, Private: &private,
	}
	return w, nil
}

// CreateWorld encapsulates data required to create a world
type CreateWorld struct {
	// The name of the World
	// required: true
	Name string `json:"name" validate:"required,noforwardslash,min=3" form:"name"`
	// Optional Owner of the world. Must be a user or an org.
	// If not set, the current user will be used as owner
	Owner string `json:"owner" form:"owner"`
	// License ID
	// required: true
	// minimum: 1
	License int `json:"license" validate:"required,gte=1" form:"license"`
	// The associated permissions. 0 for public, 1 for private.
	// enum: 0, 1
	Permission int `json:"permission" validate:"gte=0,lte=1" form:"permission"`
	// Optional description
	Description string `json:"description" form:"description"`
	// A comma separated list of tags
	Tags string `json:"tags" validate:"printascii" form:"tags"`
	// One or more files
	// required: true
	File string `json:"file" validate:"omitempty,gt=0" form:"-"`
	// Optional privacy/visibility setting.
	Private *bool `json:"private" validate:"omitempty" form:"private"`
}

// CloneWorld encapsulates data required to clone a world
type CloneWorld struct {
	// The name of the World
	// required: false
	Name string `json:"name" validate:"omitempty,noforwardslash,min=3" form:"name"`
	// Optional Owner of the world. Must be a user or an org.
	// If not set, the current user will be used as owner
	Owner string `json:"owner" form:"owner"`
	// Optional privacy/visibility setting.
	Private *bool `json:"private" validate:"omitempty" form:"private"`
}

// UpdateWorld encapsulates data that can be updated in a world
type UpdateWorld struct {
	// Optional description
	Description *string `json:"description" form:"description"`
	// Optional list of tags (comma separated)
	Tags *string `json:"tags" form:"tags"`
	// One or more files
	File string `json:"file" validate:"omitempty,gt=0" form:"-"`
	// Optional privacy/visibility setting.
	Private *bool `json:"private" validate:"omitempty" form:"private"`
}

// CreateReport encapsulates the data required to report a world
type CreateReport struct {
	Reason string `json:"reason" form:"reason"`
}

// IsEmpty returns true is the struct is empty.
func (uw UpdateWorld) IsEmpty() bool {
	return uw.Description == nil && uw.Tags == nil
}
