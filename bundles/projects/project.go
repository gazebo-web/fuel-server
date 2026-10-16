// Package projects defines the Project resource: a user-private,
// git-versioned workspace used by the AI Builder to hold an in-progress
// world and its associated models. Projects are never public — promotion
// to the public Fuel catalog happens via an explicit "publish" action that
// materializes a Model or World resource from the project's repo.
package projects

import (
	"path"
	"time"

	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/jinzhu/gorm"
)

const projectsDir = "projects"

// Project represents one AI Builder project. It satisfies
// common_resources.Resource so the existing git-repo helpers
// (GetFile, GetLatestVersion, GetRevisionFromVersion, Remove) apply
// without modification.
//
// swagger:model dbProject
type Project struct {
	ID        uint      `gorm:"primary_key" json:"-"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Soft-delete with millisecond precision is included in the unique index
	// so deleted rows don't collide with newly-created ones sharing a name.
	DeletedAt *time.Time `gorm:"type:timestamp(2) NULL; unique_index:idx_projname_owner" sql:"index" json:"-"`

	// Project name — unique per (owner, deleted_at).
	Name *string `gorm:"unique_index:idx_projname_owner" json:"name,omitempty"`

	// Stable identifier used for the on-disk folder name and as the git "v1"
	// tag so the version-counting helpers in common_resources can work.
	UUID *string `json:"-"`

	// Owner is always a user (not an org) since projects are private.
	Owner *string `gorm:"unique_index:idx_projname_owner" json:"owner,omitempty"`

	// Creator is the username captured from the JWT at creation time.
	Creator *string `json:"creator,omitempty"`

	// Description — free-form, up to 64 KiB (TEXT).
	Description *string `gorm:"type:text" json:"description,omitempty"`

	// Target simulation stack. These drive system-prompt construction in
	// ai-builder-service.
	GazeboVersion *string `json:"gazebo_version,omitempty"`
	SDFVersion    *string `json:"sdf_version,omitempty"`
	ROS2Distro    *string `json:"ros2_distro,omitempty"`

	// Optional GitHub repo slug (owner/name) remembered across pushes.
	GitHubRepo *string `json:"github_repo,omitempty"`

	// Gazebo instance the user's frontend connects to for live-simulation
	// preview. Per-project because a user may keep one project wired to a
	// local Gazebo and another to a remote one. GazeboWSKey is the
	// optional bearer/token the frontend sends to that WebSocket.
	GazeboWSURL *string `json:"gazebo_ws_url,omitempty"`
	GazeboWSKey *string `json:"gazebo_ws_key,omitempty"`

	// Location of the on-disk git repo. Populated in NewProjectAndUUID.
	// Mirrors Model.Location.
	Location *string `json:"-"`
}

// Projects is a slice of Project.
//
// swagger:model
type Projects []Project

// GetID returns the primary-key id.
func (p *Project) GetID() uint { return p.ID }

// GetName returns the project's name.
func (p *Project) GetName() *string { return p.Name }

// GetOwner returns the project's owner.
func (p *Project) GetOwner() *string { return p.Owner }

// SetOwner sets the owner on an already-populated struct.
func (p *Project) SetOwner(owner string) { *p.Owner = owner }

// GetLocation returns the on-disk repo path.
func (p *Project) GetLocation() *string { return p.Location }

// SetLocation overrides the on-disk repo path.
func (p *Project) SetLocation(location string) { *p.Location = location }

// GetUUID returns the project's UUID.
func (p *Project) GetUUID() *string { return p.UUID }

// NewProjectAndUUID mints a new UUID for the project and resolves its
// on-disk location under globals.ResourceDir/{owner}/projects/{uuid}.
// The caller is responsible for creating the directory and initializing
// the git repo via globals.VCSRepoFactory.
func NewProjectAndUUID(name, desc, owner, creator, gazebo, sdf, ros2 *string) (Project, error) {
	uuidStr, resPath, err := users.NewUUID(*owner, projectsDir)
	if err != nil {
		return Project{}, err
	}
	return newProject(&uuidStr, name, desc, owner, creator, gazebo, sdf, ros2, &resPath)
}

func newProject(uuidStr, name, desc, owner, creator, gazebo, sdf, ros2, location *string) (Project, error) {
	// Default the location if the caller didn't override (tests sometimes do).
	loc := location
	if loc == nil {
		p := path.Join(globals.ResourceDir, *owner, projectsDir, *uuidStr)
		loc = &p
	}
	return Project{
		UUID: uuidStr, Name: name, Description: desc,
		Owner: owner, Creator: creator,
		GazeboVersion: gazebo, SDFVersion: sdf, ROS2Distro: ros2,
		Location: loc,
	}, nil
}

// QueryForProjects returns a gorm query scoped to Projects, ordered by id.
func QueryForProjects(q *gorm.DB) *gorm.DB {
	return q.Model(&Project{}).Order("id")
}

// GetProjectByName fetches one project by (owner, name). Returns
// gorm.ErrRecordNotFound if missing.
func GetProjectByName(tx *gorm.DB, name, owner string) (*Project, error) {
	var p Project
	if err := QueryForProjects(tx).
		Where("owner = ? AND name = ?", owner, name).
		First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// GetProjectByUUID fetches one project by its UUID.
func GetProjectByUUID(tx *gorm.DB, uuid string) (*Project, error) {
	var p Project
	if err := QueryForProjects(tx).Where("UUID = ?", uuid).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ListForOwner returns the caller's projects, most-recently-updated first.
func ListForOwner(tx *gorm.DB, owner string, limit, offset int) (Projects, error) {
	var out Projects
	q := QueryForProjects(tx).
		Where("owner = ?", owner).
		Order("updated_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
