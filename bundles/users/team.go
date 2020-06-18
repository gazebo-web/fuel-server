package users

import (
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/ign-go"
)

// Team is a group of users within an Organization
type Team struct {
	gorm.Model

	// Name of the team. Team names within an Org cannot be duplicated (even when soft-deleted)
	Name *string `gorm:"not null;unique_index:idx_org_name" json:"name" validate:"required,alphanumspace"`

	// Whether this team is visible to non-members
	Visible bool `gorm:"not null" json:"visible"`

	// (optional) Description of the team
	Description *string `json:"description"`

	// The Organization to which this team belongs
	Organization   Organization `json:"-"`
	OrganizationID uint         `gorm:"not null;unique_index:idx_org_name" json:"-"`

	// The username of the User that created this team (usually got from the JWT)
	Creator *string `json:"-"`
}

// Teams is an array of Team
type Teams []Team

// TeamResponse represents a team for API responses.
// swagger:model
type TeamResponse struct {
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	Visible     bool     `json:"visible"`
	Usernames   []string `json:"usernames"`
}

// TeamResponses is a slice of TeamResponse
// swagger:model
type TeamResponses []TeamResponse

// CreateTeamForm encapsulates data required to create a team
type CreateTeamForm struct {
	// The name of the team
	// required: true
	Name    string `json:"name" validate:"required" form:"name"`
	Visible *bool  `validate:"required" form:"visible"`
	// Optional description
	Description *string `json:"description" form:"description"`
}

// UpdateTeamForm encapsulates data required to update a team
type UpdateTeamForm struct {
	Visible  *bool    `form:"visible"`
	NewUsers []string `form:"new_users"`
	RmUsers  []string `form:"rm_users"`
	// Optional description
	Description *string `json:"description" form:"description"`
}

// QueryForTeams returns a gorm query configured to query Teams with
// preloaded Users and owning Organization.
func QueryForTeams(q *gorm.DB) *gorm.DB {
	return q.Model(&Team{}).Preload("Organization")
}

// ByTeamName finds a team by name.
func ByTeamName(tx *gorm.DB, name string, deleted bool) (*Team, *ign.ErrMsg) {
	q := tx
	if deleted {
		// Allow to search in already deleted teams
		q = q.Unscoped()
	}
	var team Team
	if QueryForTeams(q).Where("name = ?", name).First(&team); q.Error != nil && !q.RecordNotFound() {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorNoDatabase, q.Error)
	}
	if team.Name == nil {
		return nil, ign.NewErrorMessage(ign.ErrorNonExistentResource)
	}
	return &team, nil
}
