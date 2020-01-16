package subt

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"time"
)

// Portal contains the data for portals
// TODO: extract this into a "super" Resource struct.
type Portal struct {
	// Override default GORM Model fields
	ID        uint      `gorm:"primary_key" json:"-"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Adds deletedAt to the unique index to help disambiguate when soft deleted rows are involved.
	DeletedAt *time.Time `gorm:"type:timestamp(2) NULL; unique_index:idx_name_owner" sql:"index" json:"-"`
	// Unique identifier
	UUID *string `json:"-"`
	// Location on disk
	Location *string `json:"-"`
	// The name of the resource
	// Added to the name_owner unique index.
	Name *string `gorm:"unique_index:idx_name_owner" json:"name,omitempty"`
	// The owner of this resource (must exist in UniqueOwners). Can be user or org.
	// Also added to the name_owner unique index
	Owner *string `gorm:"unique_index:idx_name_owner" json:"owner,omitempty"`
	// The username of the User that created this resource (usually got from the JWT)
	Creator *string `json:"creator,omitempty"`
	// Private - True to make this a private resource
	Private *bool `json:"private,omitempty"`
	// A description of the model (max 65,535 chars)
	// Interesting post about TEXT vs VARCHAR(30000) performance:
	// https://nicj.net/mysql-text-vs-varchar-performance/
	Description *string `gorm:"type:text" json:"description,omitempty"`
}

// CompetitionParticipant contains the SubT participants extra fields
type CompetitionParticipant struct {
	// Override default GORM Model fields
	ID uint `gorm:"primary_key" json:"-"`
	// The participant name
	// Required. Manage the relationship with Organization.
	// Impl note: It is named Owner to support generic DB query modifiers that
	// expect the field Owner to represent organizations or users.
	Owner *string `gorm:"not null;unique_index:idx_active_owner" json:"owner"`
	// Competition name. Note: A competition is an Organization.
	Competition *string `json:"competition"`
	// GORM fields
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL" json:"created_at"`
	// Added 2 milliseconds to DeletedAt field
	DeletedAt *time.Time `gorm:"type:timestamp(2) NULL; unique_index:idx_active_owner" sql:"index" json:"-"`
	// Private - True to make this a private resource
	// Impl note: we added the Private field to support generic query modifiers that
	// expect the private field.
	Private *bool `json:"private,omitempty"`
}

// CompetitionParticipants is an slice of CompetitionParticipant
type CompetitionParticipants []CompetitionParticipant

// CompetitionScore contains scores for competition circuits.
type CompetitionScore struct {
	gorm.Model
	// Simulation unique identifier. For multisims, this should be the parent's group id.
	GroupId     *string `gorm:"not null" json:"group_id"`
	Competition *string `gorm:"not null" json:"competition"`
	Circuit     *string `gorm:"not null" json:"circuit"`
	Owner       *string `gorm:"not null" json:"owner"`
	// Simulation score
	Score *float64 `gorm:"not null" json:"score"`
	// Source includes the GroupIds of all simulations that produced this score entry
	Sources *string `gorm:"size:10000" json:"sources"`
}

// CompetitionScores is a list of CompetitionScore
type CompetitionScores []CompetitionScore

// RegStatus are possible status values of registrations
type RegStatus uint

// Registration operation related constants
const (
	RegOpPending RegStatus = iota
	RegOpDone
	RegOpRejected
)

// Registration models the table that tracks a team registration for SubT.
type Registration struct {
	// Override default GORM Model fields

	// ID is public as we want to use it in requests from clients
	ID        uint      `gorm:"primary_key" json:"-"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL" json:"created_at"`
	// Added 2 milliseconds to DeletedAt field
	DeletedAt *time.Time `gorm:"type:timestamp(2) NULL" sql:"index" json:"-"`
	// Competition name. Note: A competition is an Organization.
	Competition *string `json:"competition"`
	// Resolution Date
	ResolvedAt *time.Time `gorm:"type:timestamp(3) NULL" json:"resolved_at,omitempty"`
	// The status of the registration. Expects one of the RegOp* constants.
	Status *int `json:"status"`
	// Related Fuel Organization (by name). It is the "team".
	Participant *string `json:"participant"`
	// The username of the User that requested this
	Creator *string `json:"creator"`
}

func (r *Registration) regName() string {
	return fmt.Sprint("registration_", r.ID)
}

// Registrations is a list of registrations
type Registrations []Registration

// RegistrationCreate encapsulates data required to create a new pending registration.
type RegistrationCreate struct {
	Participant string `validate:"required,alphanumspace"`
}

// RegistrationUpdate encapsulates data required to resolve a pending registration
type RegistrationUpdate struct {
	Competition string    `json:"-"`
	Participant string    `json:"-"`
	Resolution  RegStatus `validate:"required,gte=1,lte=2"`
}

// SubmissionStatus are possible status values of LogFile submissions.
type SubmissionStatus uint

// Submission status related constants
const (
	StForReview SubmissionStatus = iota
	StDone
	StRejected
)

// LogFile represents a Log file submitted in a competition.
type LogFile struct {
	// Override default GORM Model fields
	ID        uint      `gorm:"primary_key" json:"id"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	// Added 2 milliseconds to DeletedAt field, and added it to the unique index to help disambiguate
	// when soft deleted rows are involved.
	DeletedAt *time.Time `gorm:"type:timestamp(2) NULL" sql:"index" json:"-"`
	// Unique identifier
	UUID *string `json:"-"`
	// Location in bucket
	Location *string `json:"location,omitempty"`
	// The owner of this resource (must exist in UniqueOwners). Can be user or org.
	Owner *string `json:"owner,omitempty"`
	// The username of the User that created this resource (usually got from the JWT)
	Creator *string `json:"creator,omitempty"`
	// Private - True to make this a private resource
	Private *bool `json:"private,omitempty"`

	// Specific fields

	// Competition name. A competition is an Organization
	Competition *string `json:"competition,omitempty"`
	// Submission status. A value from St* contants
	Status *int     `validate:"omitempty,gte=0,lte=2" json:"status"`
	Score  *float32 `json:"score,omitempty"`
	// Resolution Date
	ResolvedAt *time.Time `gorm:"type:timestamp(3) NULL" json:"resolved_at,omitempty"`
	// Optional comments
	// Interesting post about TEXT vs VARCHAR(30000) performance:
	// https://nicj.net/mysql-text-vs-varchar-performance/
	Comments *string `gorm:"type:text" json:"comments,omitempty"`
}

func (lf *LogFile) name() string {
	return fmt.Sprint("logfile", lf.ID)
}

// LogFiles is a list of LogFile
type LogFiles []LogFile

// LogSubmission encapsulates data required to submit a log file from a client
type LogSubmission struct {
	// Optional Owner. Must be a user or an org.
	// If not set, the current user will be used as owner
	Owner string `validate:"required" form:"owner"`
	// Optional description
	Description string `form:"description"`
	// One or more files
	// required: true
	File string `json:"file" validate:"omitempty,gt=0" form:"-"`
	// Optional privacy/visibility setting.
	Private *bool `validate:"omitempty" form:"private"`
}

// SubmissionUpdate encapsulates data required to score an existing a log file.
type SubmissionUpdate struct {
	// Submission status. A value from St* contants
	Status SubmissionStatus `validate:"gte=0,lte=2"`
	Score  float32
	// Optional comments
	// Interesting post about TEXT vs VARCHAR(30000) performance:
	// https://nicj.net/mysql-text-vs-varchar-performance/
	Comments *string `json:"comments,omitempty"`
}

// QueryForRegistrations returns a gorm query configured to query Registrations.
func QueryForRegistrations(q *gorm.DB) *gorm.DB {
	return q.Model(&Registration{})
}

// LeaderboardParticipant is a struct that contains participant data and their
// score
type LeaderboardParticipant struct {
	CompetitionParticipant
	Score   *float32 `json:"score,omitempty"`
	Circuit *string  `json:"circuit"`
}
