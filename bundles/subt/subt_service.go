package subt

import (
	"context"
	"fmt"
	"github.com/gazebo-web/gz-go/v7"
	"time"

	res "github.com/gazebo-web/fuel-server/bundles/common_resources"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	p "github.com/gazebo-web/fuel-server/permissions"
	"github.com/jinzhu/gorm"
)

// SubTPortalName is the name of the Org that represents the competition.
const SubTPortalName = "subt"

// Service is the main struct exported by this service.
type Service struct{}

// iptr returns a pointer to a given int.
func iptr(i int) *int {
	return &i
}

func getRegistration(tx *gorm.DB, comp, participant string) (*Registration, *gz.ErrMsg) {
	var r Registration
	// Create query
	q := QueryForRegistrations(tx).Order("id desc", true)
	if err := q.Where("participant = ? AND competition = ?", participant, comp).First(&r).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNameNotFound, err)
	}
	return &r, nil
}

func getRegistrationByCreator(tx *gorm.DB, comp, requestingUser string) (*Registration, *gz.ErrMsg) {
	var r Registration
	// Create query
	q := QueryForRegistrations(tx).Order("id desc", true)
	if err := q.Where("creator = ? AND competition = ?", requestingUser, comp).First(&r).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNameNotFound, err)
	}
	return &r, nil
}

func getParticipant(tx *gorm.DB, comp,
	participant string) (*CompetitionParticipant, *gz.ErrMsg) {
	var p CompetitionParticipant
	// Create query
	q := tx.Model(&CompetitionParticipant{})
	if err := q.Where("owner = ? AND competition = ?", participant, comp).First(&p).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNameNotFound, err)
	}
	return &p, nil
}

// ApplyToSubT registers a new (pending) registration to join SubT.
// user argument is the active user requesting the operation.
// The orgName argument is the organization that will be registered as
// a 'participant team'.
func (s *Service) ApplyToSubT(ctx context.Context, tx *gorm.DB,
	orgName string, user *users.User) (*Registration, *gz.ErrMsg) {

	r, em := s.CreateRegistration(ctx, tx, SubTPortalName, orgName, user)
	if em != nil {
		return nil, em
	}

	return r, nil
}

// CreateRegistration registers a new (pending) registration to join a competition.
// user argument is the active user requesting the operation.
// The orgName argument is the organization that will be registered as a 'team'.
// TODO: this should be moved to generic a Registrations bundle.
func (s *Service) CreateRegistration(ctx context.Context, tx *gorm.DB,
	comp, orgName string, user *users.User) (*Registration, *gz.ErrMsg) {

	// Make sure the orgName to be registered as participant isn't the same
	// Competition org.
	if orgName == comp {
		return nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue)
	}

	// Sanity check: make sure the org exists
	org, em := users.ByOrganizationName(tx, orgName, false)
	if em != nil {
		return nil, em
	}
	// Check write permissions of the requesting user
	if ok, em := globals.Permissions.IsAuthorized(*user.Username, orgName, p.Write); !ok {
		return nil, em
	}

	// Now check there is no pending (or done) registration made by the requesting user.
	reg, em := getRegistrationByCreator(tx, comp, *user.Username)
	if em != nil && em.ErrCode != gz.ErrorNameNotFound {
		return nil, em
	} else if reg != nil && *reg.Status != int(RegOpRejected) {
		return nil, gz.NewErrorMessage(gz.ErrorResourceExists)
	}

	// Now check there is no pending registration already for that participant org.
	reg, em = getRegistration(tx, comp, orgName)
	if em != nil && em.ErrCode != gz.ErrorNameNotFound {
		return nil, em
	} else if reg != nil && *reg.Status != int(RegOpRejected) {
		return nil, gz.NewErrorMessage(gz.ErrorResourceExists)
	}

	registration := Registration{Status: iptr(int(RegOpPending)),
		Participant: &orgName, Competition: &comp, Creator: user.Username, Email: user.Email}
	if err := tx.Create(&registration).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	// Set read permissions to all Competition organizing team members (eg, SubT)
	// as well as the requesting user.
	// The Write permission will be only for admins of Competition (SubT).
	rName := registration.regName()
	if _, em := globals.Permissions.AddPermission(comp, rName, p.Read); em != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, em)
	}
	if _, em := globals.Permissions.AddPermission(*user.Username, rName, p.Read); em != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, em)
	}

	sendMail(fmt.Sprintf("New Registration [regName:%s] for [%s]. Team [%s]", rName, comp, orgName), &registration, org)
	return &registration, nil
}

// ResolveRegistration updates a registration. Usually to set a resolution
// (approve / reject).
// The requestor argument is the active user requesting the operation (an admin).
func (s *Service) ResolveRegistration(ctx context.Context, tx *gorm.DB,
	ru *RegistrationUpdate, requestor *users.User) (*Registration, *gz.ErrMsg) {

	// validate input data
	if ru.Resolution != RegOpDone && ru.Resolution != RegOpRejected {
		return nil, gz.NewErrorMessageWithArgs(gz.ErrorFormInvalidValue, nil, []string{"resolution"})
	}

	// Sanity check: make sure there is a pending registration
	reg, em := getRegistration(tx, ru.Competition, ru.Participant)
	if em != nil {
		return nil, em
	}
	if *reg.Status != int(RegOpPending) {
		return nil, gz.NewErrorMessage(gz.ErrorNameNotFound)
	}

	// Only admins of the competition can update registrations for that competition.
	if ok, em := globals.Permissions.IsAuthorized(*requestor.Username,
		*reg.Competition, p.Write); !ok {
		return nil, em
	}

	now := time.Now()
	up := tx.Model(reg).Update("ResolvedAt", &now)
	up.Update("Status", iptr(int(ru.Resolution)))

	if ru.Resolution == RegOpDone {
		// create the competition participant
		priv := true
		cu := &CompetitionParticipant{Owner: &ru.Participant,
			Competition: &ru.Competition, Private: &priv}
		if err := tx.Create(&cu).Error; err != nil {
			return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
		}
	}
	return reg, nil
}

// DeleteRegistration cancels a pending registration.
// The requestor argument is the active user requesting the operation.
// Returns the canceled registration
func (s *Service) DeleteRegistration(ctx context.Context, tx *gorm.DB,
	comp, orgName string, requestor *users.User) (*Registration, *gz.ErrMsg) {

	// Sanity check: make sure there is a pending registration
	reg, em := getRegistration(tx, comp, orgName)
	if em != nil {
		return nil, em
	}
	if *reg.Status != int(RegOpPending) {
		return nil, gz.NewErrorMessage(gz.ErrorNameNotFound)
	}

	// Only the same user or admins of the competition can cancel the registration.
	if *requestor.Username != *reg.Creator {
		if ok, em := globals.Permissions.IsAuthorized(*requestor.Username, comp, p.Write); !ok {
			return nil, em
		}
	}

	// Remove the registration from the database
	if err := tx.Delete(reg).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}

	return reg, nil
}

// DeleteParticipant removes a registered participant.
// The requestor argument is the active user requesting the operation.
// Returns the deleted participant
func (s *Service) DeleteParticipant(ctx context.Context, tx *gorm.DB,
	comp, orgName string, requestor *users.User) (*CompetitionParticipant, *gz.ErrMsg) {

	// Sanity check: make sure there is a participant
	part, em := getParticipant(tx, comp, orgName)
	if em != nil {
		return nil, em
	}

	// Sanity check: make sure there is a registration
	reg, em := getRegistration(tx, comp, orgName)
	if em != nil {
		return nil, em
	}

	// Only the same user or admins of the competition can remove the participant.
	if *requestor.Username != *part.Owner {
		if ok, em := globals.Permissions.IsAuthorized(*requestor.Username, comp, p.Write); !ok {
			return nil, em
		}
	}

	// Remove the participant from the participant table
	if err := tx.Delete(part).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}

	// Remove the registration from the registration table
	if err := tx.Delete(reg).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}

	return part, nil
}

// RegistrationList returns a list of paginated registrations.
// Only the admins of the competition and the user that applied the registration
// should be able to see registrations.
func (s *Service) RegistrationList(pr *gz.PaginationRequest, tx *gorm.DB, comp string,
	status RegStatus, reqUser *users.User) (*Registrations, *gz.PaginationResult, *gz.ErrMsg) {

	// Create the DB query
	q := QueryForRegistrations(tx)
	q = q.Where("status = ? AND competition = ?", int(status), comp)

	sysAdm := globals.Permissions.IsSystemAdmin(*reqUser.Username)
	if !sysAdm {
		// A system admin can see all registrations.
		// Otherwise, we need to filter registrations
		orgs := make([]string, 0)
		blankQuery := tx.New()
		orgRoles, _ := users.GetOrganizationsAndRolesForUser(blankQuery, reqUser, reqUser)
		// Keep only the orgs that the reqUser is Admin or Owner.
		for o := range orgRoles {
			if ok, _ := globals.Permissions.IsAuthorizedForRole(*reqUser.Username, o, p.Admin); ok {
				orgs = append(orgs, o)
			}
		}
		q = q.Where("creator = ? OR competition IN (?)", *reqUser.Username, orgs)
	}

	var regs Registrations
	pagination, err := gz.PaginateQuery(q, &regs, *pr)
	if err != nil {
		return nil, nil, gz.NewErrorMessageWithBase(gz.ErrorInvalidPaginationRequest, err)
	}
	if !pagination.PageFound {
		return nil, nil, gz.NewErrorMessage(gz.ErrorPaginationPageNotFound)
	}

	return &regs, pagination, nil
}

// ParticipantsList returns a list of paginated participants (organizations and users).
func (s *Service) ParticipantsList(pr *gz.PaginationRequest, tx *gorm.DB, comp string,
	reqUser *users.User) (*users.OrganizationResponses, *gz.PaginationResult, *gz.ErrMsg) {

	q := tx.Model(&users.OrganizationResponse{})
	q = q.Table("competition_participants")
	q = q.Select("owner as name, private, case when us.email is null then orgs.email else us.email end as email")
	q = q.Joins("left join users as us on us.username = competition_participants.owner")
	q = q.Joins("left join organizations as orgs on orgs.name = competition_participants.owner")

	// If reqUser belongs to the main competition group, then can see all participants.
	// Otherwise, only those participants the reqUser belongs to.
	if ok, _ := globals.Permissions.IsAuthorized(*reqUser.Username, comp, p.Read); !ok {
		// filter resources based on privacy setting
		q = res.QueryForResourceVisibility(tx, q, nil, reqUser)
	}

	var responses users.OrganizationResponses

	pagination, err := gz.PaginateQuery(q, &responses, *pr)
	if err != nil {
		return nil, nil, gz.NewErrorMessageWithBase(gz.ErrorInvalidPaginationRequest, err)
	}
	if !pagination.PageFound {
		return nil, nil, gz.NewErrorMessage(gz.ErrorPaginationPageNotFound)
	}

	return (*users.OrganizationResponses)(&responses), pagination, nil
}

// filterLeaderboard filters the results of a leaderboard query based on an array of values.
// The array of values is defined as a comma-separated environment variable.
// Both values and filters are converted to lowercase before being compared.
func (s *Service) filterLeaderboard(q *gorm.DB, field string, filters []string) *gorm.DB {
	// Apply a filter if any values were defined
	if len(filters) > 0 {
		return q.Where(fmt.Sprintf("LOWER(%s) NOT IN (?)", field), filters)
	}
	return q
}

// Leaderboard returns a paginated list with all competition participants sorted
// by their score.
func (s *Service) Leaderboard(pr *gz.PaginationRequest, tx *gorm.DB, comp string, circuit *string,
	owner *string) (interface{}, *gz.PaginationResult, *gz.ErrMsg) {

	// NOTE this feature is public. Anyone can see the leaderboard.

	// Create the DB query
	q := tx.Table("competition_participants").
		Select("competition_participants.*, scores.circuit, scores.score").
		Joins("LEFT JOIN (SELECT owner as sowner, circuit, MAX(score) as score"+
			"           FROM competition_scores "+
			"           GROUP BY owner, circuit) AS scores "+
			"ON competition_participants.owner = scores.sowner").
		Where("competition_participants.competition = ?", comp).
		Order("score DESC")

	// Include optional filtering clauses
	q = s.filterLeaderboard(q, "owner", globals.LeaderboardOrganizationFilter)
	if owner != nil {
		q = q.Where("LOWER(owner) = LOWER(?)", owner)
	}

	q = s.filterLeaderboard(q, "circuit", globals.LeaderboardCircuitFilter)
	if circuit != nil {
		q = q.Where("LOWER(circuit) = LOWER(?)", circuit)
	}

	// Get the organizations
	var lb []LeaderboardParticipant
	pagination, err := gz.PaginateQuery(q, &lb, *pr)
	if err != nil {
		return nil, nil, gz.NewErrorMessageWithBase(gz.ErrorInvalidPaginationRequest, err)
	}
	if !pagination.PageFound {
		return nil, nil, gz.NewErrorMessage(gz.ErrorPaginationPageNotFound)
	}

	return &lb, pagination, nil
}
