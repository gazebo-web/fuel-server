package users

import (
	"bitbucket.org/ignitionrobotics/ign-go"
	"github.com/jinzhu/gorm"
	"time"
)

// UniqueOwner is a separate table to help ensure cross table username (and org)
// uniqueness. Each record here will be 'a user' or 'an org' (orgs and users cannot
// repeat names). In the future we can add more common data to this table.
type UniqueOwner struct {
	Name *string `gorm:"primary_key:true"`

	CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`

	UpdatedAt time.Time

	DeletedAt *time.Time `sql:"index"`

	OwnerType string
}

// OwnerTypeOrg represents the 'organizations' OwnerType value.
const OwnerTypeOrg string = "organizations"

// OwnerTypeUser represents the 'users' OwnerType value.
const OwnerTypeUser string = "users"

// User information
//
// swagger:model
type User struct {
	gorm.Model

	Identity *string `json:"identity,omitempty"`

	// Person name
	Name *string `json:"name,omitempty"`

	// Username is unique in the Fuel community (including organizations)
	Username *string `gorm:"not null;unique" json:"username,omitempty" validate:"required,min=3,alphanum,notinblacklist"`
	// Note: foreign keys must be added manually (through Model().AddForeignKey())

	// DEPRECATED: Organization is an ignored field.
	Organization *string `json:"org,omitempty"`

	Email *string `json:"email,omitempty" validate:"required,email"`

	// A comma separated list of features enabled for the user.
	ExpFeatures *string `json:"exp_features,omitempty" validate:"omitempty,expfeatures,max=255"`

	ModelCount       *uint `json:"model_count,omitempty"`
	LikedModels      *uint `json:"liked_models,omitempty"`
	DownloadedModels *uint `json:"downloaded_models,omitempty"`

	WorldCount       *uint `json:"world_count,omitempty"`
	LikedWorlds      *uint `json:"liked_worlds,omitempty"`
	DownloadedWorlds *uint `json:"downloaded_worlds,omitempty"`

	// AccessTokens are personal access tokens granted to a user by a user.
	AccessTokens ign.AccessTokens
}

// Users is an slice of User
type Users []User

// UserResponse stores user information used in REST responses.
//
// swagger:model
type UserResponse struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	// private
	Email         string   `json:"email"`
	Organizations []string `json:"orgs"`
	// private
	OrgRoles map[string]string `json:"orgRoles"`
	// private
	ID uint `json:"id"`
	// private
	ExpFeatures string `json:"exp_features,omitempty"`
	// True if the user is a system administrator
	SysAdmin bool `json:"sysAdmin"`
}

// UserResponses is a slice of UserResponse
// swagger:model
type UserResponses []UserResponse

// UpdateUserInput encapsulates data that can be updated in an user
type UpdateUserInput struct {
	// Optional name
	Name *string `json:"name,omitempty"`
	// Optional email
	Email       *string `json:"email" validate:"omitempty,email"`
	ExpFeatures *string `json:"exp_features,omitempty" validate:"omitempty,expfeatures,max=255"`
}

// IsEmpty returns true is the struct is empty.
func (uu UpdateUserInput) IsEmpty() bool {
	return uu.Name == nil && uu.Email == nil && uu.ExpFeatures == nil
}

// ByUsername queries a user by username.
func ByUsername(tx *gorm.DB, username string, deleted bool) (*User, *ign.ErrMsg) {
	q := tx
	if deleted {
		// Allow to search in already deleted users
		q = q.Unscoped()
	}
	var user User
	if q.Where("username = ?", username).First(&user); q.Error != nil && !q.RecordNotFound() {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorNoDatabase, q.Error)
	}
	if user.Username == nil {
		return nil, ign.NewErrorMessage(ign.ErrorUserUnknown)
	}
	return &user, nil
}

// ByIdentity queries a user by identity.
func ByIdentity(tx *gorm.DB, identity string, deleted bool) (*User, *ign.ErrMsg) {
	q := tx
	if deleted {
		// Allow to search in already deleted users
		q = q.Unscoped()
	}
	var aUser User
	if q.Where("identity = ?", identity).First(&aUser); q.Error != nil && !q.RecordNotFound() {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorNoDatabase, q.Error)
	}
	if aUser.Identity == nil || *aUser.Identity != identity {
		return nil, ign.NewErrorMessage(ign.ErrorAuthNoUser)
	}
	return &aUser, nil
}

// OwnerByName queries a the unique owner names.
func OwnerByName(tx *gorm.DB, name string, deleted bool) (*UniqueOwner, *ign.ErrMsg) {
	q := tx
	if deleted {
		// Allow to search in already deleted users
		q = q.Unscoped()
	}
	var owner UniqueOwner
	if q.Where("name = ?", name).First(&owner); q.Error != nil && !q.RecordNotFound() {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorNoDatabase, q.Error)
	}
	if owner.Name == nil {
		return nil, ign.NewErrorMessage(ign.ErrorUserUnknown)
	}
	return &owner, nil
}
