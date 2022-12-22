package users

import (
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
)

// Organization consists of a group of users/teams
// swagger:model
type Organization struct {
	gorm.Model

	// Name of the organization
	// Name is unique in the Fuel community (including users)
	Name *string `gorm:"not null;unique" json:"name"`

	// Description of the organization
	Description *string `json:"description"`
	// Email
	Email *string `json:"email,omitempty"`

	// The username of the User that created this organization (usually got from the JWT)
	Creator *string `json:"-"`
}

// Organizations is an array of Organization
type Organizations []Organization

// OrganizationResponse stores organization information used in REST responses.
//
// swagger:model
type OrganizationResponse struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Email       string `json:"email,omitempty"`
	Private     bool   `json:"private,omitempty"`
}

// OrganizationResponses is a slice of OrganizationResponse
// swagger:model
type OrganizationResponses []OrganizationResponse

// CreateOrganization encapsulates data required to create an organization
type CreateOrganization struct {
	// The name of the Organization
	// required: true
	Name string `json:"name" validate:"required,min=3,alphanumspace,notinblacklist" form:"name"`
	// The email of the Organization
	Email string `json:"email" validate:"omitempty,email" form:"email"`
	// Optional description
	Description string `json:"description" form:"description"`
}

// UpdateOrganization encapsulates data that can be updated in an organization
type UpdateOrganization struct {
	// Optional email
	Email *string `json:"email" validate:"omitempty,email" form:"email"`
	// Optional description
	Description *string `json:"description" form:"description"`
}

// IsEmpty returns true is the struct is empty.
func (uo UpdateOrganization) IsEmpty() bool {
	return uo.Description == nil && uo.Email == nil
}

// AddUserToOrgInput is the input data to add a user to an org.
type AddUserToOrgInput struct {
	Username string `json:"username" validate:"required,alphanum" form:"username"`
	Role     string `json:"role" validate:"required,oneof=owner admin member" form:"role"`
}

// ByOrganizationName queries an organization by name.
func ByOrganizationName(tx *gorm.DB, name string, deleted bool) (*Organization, *gz.ErrMsg) {
	q := tx
	if deleted {
		// Allow to search in already deleted organizations
		q = q.Unscoped()
	}
	var organization Organization
	if q.Where("name = ?", name).First(&organization); q.Error != nil && !q.RecordNotFound() {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNoDatabase, q.Error)
	}
	if organization.Name == nil {
		return nil, gz.NewErrorMessage(gz.ErrorNonExistentResource)
	}
	return &organization, nil
}
