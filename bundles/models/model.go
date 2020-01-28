package models

import (
	"path"
	"time"

	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/license"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	"github.com/jinzhu/gorm"
)

// TODO: move DB related functions to a DB Accessor. Inject the db accessor to the models service.

const (
	models string = "models"
)

// Model represents information about a simulation model
//
// A model contains information about a single simulation object, such
// as a robot, table, or structure.
//
// swagger:model dbModel
type Model struct {
	// Override default GORM Model fields
	ID        uint      `gorm:"primary_key" json:"-"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`
	UpdatedAt time.Time
	// Added 2 milliseconds to DeletedAt field, and added it to the unique index to help disambiguate
	// when soft deleted rows are involved.
	DeletedAt *time.Time `gorm:"type:timestamp(2) NULL; unique_index:idx_modelname_owner" sql:"index"`

	// The name of the model
	Name *string `gorm:"unique_index:idx_modelname_owner" json:"name,omitempty"`

	// Optional user friendly name to use in URLs
	URLName *string `json:"url_name,omitempty"`

	// Unique identifier for the the model
	UUID *string `json:"-"`

	// A description of the model (max 65,535 chars)
	// Interesting post about TEXT vs VARCHAR(30000) performance:
	// https://nicj.net/mysql-text-vs-varchar-performance/
	Description *string `gorm:"type:text" json:"description,omitempty"`

	// Number of likes
	Likes int `json:"likes,omitempty"`

	// Bytes of the model, when downloaded as a zip
	Filesize int `json:"filesize,omitempty"`

	// Number of downloads
	Downloads int `json:"downloads,omitempty"`

	// Date and time the model was first uploaded
	UploadDate *time.Time `json:"upload_date,omitempty"`

	// Date and time the model was modified
	ModifyDate *time.Time `json:"modify_date,omitempty"`

	// Tags associated to this model
	Tags Tags `gorm:"many2many:model_tags;" json:"tags,omitempty"`

	// Metadata associated to this model
	Metadata ModelMetadata `json:"metadata,omitempty"`

	// Location of the model on disk
	Location *string `json:"-"`

	// The owner of this model (must exist in UniqueOwners). Can be user or org.
	Owner *string `gorm:"unique_index:idx_modelname_owner" json:"owner,omitempty"`

	// The username of the User that created this model (usually got from the JWT)
	Creator *string `json:"creator,omitempty"`

	// Permission - 0: public, 1: owner, (future: team, others)
	Permission int `json:"permission,omitempty"`

	// The license associated to this model
	License   license.License `json:"license,omitempty"`
	LicenseID int             `json:"lic_id,omitempty"`

	// Private - True to make this a private resource
	Private *bool `json:"private,omitempty"`
}

// GetID returns the ID
func (m *Model) GetID() uint {
	return m.ID
}

// GetName returns the model's name
func (m *Model) GetName() *string {
	return m.Name
}

// GetOwner returns the model's owner
func (m *Model) GetOwner() *string {
	return m.Owner
}

// GetLocation returns the model's location on disk
func (m *Model) GetLocation() *string {
	return m.Location
}

// GetUUID returns the model's UUID
func (m *Model) GetUUID() *string {
	return m.UUID
}

// Models is an array of Model
//
type Models []Model

// QueryForModels returns a gorm query configured to query Models with
// preloaded License and Tags.
func QueryForModels(q *gorm.DB) *gorm.DB {
	return q.Model(&Model{}).Order("id").Preload("Tags").Preload("License")
}

// GetModelByName queries a Model by model name and owner.
func GetModelByName(tx *gorm.DB, modelName string, owner string) (*Model, error) {
	var model Model
	if err := QueryForModels(tx).Where("owner = ? AND name = ?", owner, modelName).First(&model).Error; err != nil {
		return nil, err
	}
	return &model, nil
}

// NewModelAndUUID creates a Model struct with a new UUID.
func NewModelAndUUID(name, urlName, desc, location, owner, creator *string, lic license.License, permission int, tags Tags, private bool) (Model, error) {
	uuidStr, _, err := users.NewUUID(*owner, models)
	if err != nil {
		return Model{}, err
	}
	return NewModel(&uuidStr, name, urlName, desc, location, owner, creator, lic, permission, tags, private)
}

// NewModel creates a new Model struct
func NewModel(uuidStr, name, urlName, desc, location, owner, creator *string, lic license.License, permission int, tags Tags, private bool) (Model, error) {

	var modelPath string
	// Override the generated location if we got a model location as argument
	if location != nil {
		modelPath = *location
	} else {
		modelPath = path.Join(globals.ResourceDir, *owner, models, *uuidStr)
	}

	uploadDate := time.Now()
	modifyDate := time.Now()
	model := Model{Name: name, URLName: urlName, Owner: owner, Creator: creator, UUID: uuidStr,
		Description: desc, Location: &modelPath, Likes: 0, Downloads: 0,
		UploadDate: &uploadDate, ModifyDate: &modifyDate, Tags: tags,
		License: lic, Permission: permission, Private: &private,
	}
	return model, nil
}

// CreateModel encapsulates data required to create a model
type CreateModel struct {
	// The name of the Model
	// required: true
	Name string `json:"name" validate:"required,min=3,noforwardslash" form:"name"`
	// Optional Owner of the model. Must be a user or an org.
	// If not set, the current user will be used as owner
	Owner string `json:"owner" form:"owner"`
	// Url name
	URLName string `json:"urlName" validate:"omitempty,base64" form:"urlName"`
	// License ID
	// required: true
	// minimum: 1
	License int `json:"license" validate:"required,gte=1" form:"license"`
	// The associated permissions. 0 for public, 1 for private models.
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

// CloneModel encapsulates data required to clone a model
type CloneModel struct {
	// The name of the Model
	// required: false
	Name string `json:"name" validate:"omitempty,noforwardslash,min=3" form:"name"`
	// Optional Owner of the model. Must be a user or an org.
	// If not set, the current user will be used as owner
	Owner string `json:"owner" form:"owner"`
	// Private privacy/visibility setting
	Private *bool `json:"private" validate:"omitempty" form:"private"`
}

// UpdateModel encapsulates data that can be updated in a model
type UpdateModel struct {
	// Optional description
	Description *string `json:"description" form:"description"`
	// Optional list of tags (comma separated)
	Tags *string `json:"tags" form:"tags"`
	// One or more files
	File string `json:"file" validate:"omitempty,gt=0" form:"-"`
	// Private privacy/visibility setting
	Private *bool `json:"private" validate:"omitempty" form:"private"`
	// Metadata associated to this model
	Metadata *ModelMetadata `json:"metadata" form:"metadata"`
}

// CreateReport encapsulates the data required to report a model
type CreateReport struct {
	Reason string `json:"reason" form:"reason"`
}

// IsEmpty returns true is the struct is empty.
func (um UpdateModel) IsEmpty() bool {
	return um.Description == nil && um.Tags == nil
}
