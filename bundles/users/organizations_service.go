package users

import (
	"context"
	"fmt"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	"gitlab.com/ignitionrobotics/web/fuelserver/permissions"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"regexp"
	"strings"
	"time"
)

// OrganizationService is the main struct exported by this Organization Service.
// It was meant as a way to structure code and help future extensions.
type OrganizationService struct{}

/*
IMPLEMENTATION NOTES / Design:
  We will have a Teams table (idem with Orgs) to store individual team's
  properties such as visibility and description.
  A team belongs to *one* Org.
  Teams and Orgs are soft deleted (same as users).

  The list of users of an Org/Team is tracked using casbin roles.

  In casbin, the organization role/group will represent the 'default' team.
*/

// RemoveOrganization removes the given organization. Returns a OrganizationResponse with the removed organization.
// The user argument is the requesting user. It is used to check if the user can perform the operation.
// NOTE: It does not remove the Group or its permissions from the Permissions
// DB (casbin), in case we want to revert.
func (ms *OrganizationService) RemoveOrganization(ctx context.Context, tx *gorm.DB, orgName string,
	user *User) (*OrganizationResponse, *ign.ErrMsg) {

	// Sanity check: make sure the org exists
	organization, em := ByOrganizationName(tx, orgName, false)
	if em != nil {
		return nil, em
	}

	// check if JWT user has permission to remove this organization.
	// Note, only Owners can remove an organization.
	if ok, em := globals.Permissions.IsAuthorizedForRole(*user.Username,
		*organization.Name, permissions.Owner); !ok {
		return nil, em
	}

	// First remove all organization teams (soft-delete)
	tx.Where("organization_id = ?", organization.ID).Delete(&Team{})
	// Remove the organization from the database (soft-delete).
	owner := UniqueOwner{Name: organization.Name}
	if err := tx.Delete(organization).Delete(&owner).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbDelete, err)
	}

	ign.LoggerFromContext(ctx).Info("Organization removed. Name=", *organization.Name)

	response := ms.CreateOrganizationResponse(organization, user, false)
	return &response, nil
}

// OrganizationList returns a list of paginated OrganizationResponses.
// forceShowPrivate forces returning Org private data regardless of the requestor's permissions.
func (ms *OrganizationService) OrganizationList(p *ign.PaginationRequest, tx *gorm.DB,
	requestor *User, forceShowPrivate bool) (*OrganizationResponses, *ign.PaginationResult, *ign.ErrMsg) {
	// Get the organizations
	var organizations Organizations

	// Create the DB query
	q := tx.Model(&Organization{})

	pagination, err := ign.PaginateQuery(q, &organizations, *p)
	if err != nil {
		return nil, nil, ign.NewErrorMessageWithBase(ign.ErrorInvalidPaginationRequest, err)
	}
	if !pagination.PageFound {
		return nil, nil, ign.NewErrorMessage(ign.ErrorPaginationPageNotFound)
	}

	// Create OrganizationReponse results
	responses := OrganizationResponses{}
	for _, organization := range organizations {
		responses = append(responses, ms.CreateOrganizationResponse(&organization, requestor, forceShowPrivate))
	}
	return &responses, pagination, nil
}

// CreateOrganizationResponse creates a new OrganizationResponse struct based on
// the given Organization object.
// The returned OrganizationResponse will also include organization private fields
// if the requestor has Write access to those. But forceShowPrivate arg forces returning
// private data regardless of the requestor's permissions.
func (ms *OrganizationService) CreateOrganizationResponse(organization *Organization,
	requestor *User, forceShowPrivate bool) OrganizationResponse {

	var response OrganizationResponse
	// Public info
	if organization.Name != nil {
		response.Name = *organization.Name
	}
	if organization.Description != nil {
		response.Description = *organization.Description
	}

	var canReadOrg bool
	if forceShowPrivate {
		canReadOrg = true
	} else if requestor != nil {
		canReadOrg, _ = globals.Permissions.IsAuthorized(*requestor.Username,
			*organization.Name, permissions.Read)
	}
	response.Private = canReadOrg

	// Private info
	if canReadOrg {
		if organization.Email != nil {
			response.Email = *organization.Email
		}
	}

	return response
}

// CreateOrganization creates a new Organization in DB using the data from
// the given Organization struct.
// Returns an OrganizationResponse.
func (ms *OrganizationService) CreateOrganization(ctx context.Context, tx *gorm.DB,
	co CreateOrganization, creator *User) (*OrganizationResponse, *ign.ErrMsg) {
	// Sanity check: Make sure that the organization name was not already used,
	// even with removed organizations or users.
	ownerName, em := OwnerByName(tx, co.Name, true)
	if em != nil && em.ErrCode != ign.ErrorUserUnknown {
		return nil, em
	}
	if ownerName != nil {
		return nil, ign.NewErrorMessage(ign.ErrorResourceExists)
	}

	// Create the organization in the permissions DB as a 'group' and set the
	// creator as the 'owner'.
	// This is the same as adding the user to the 'default' team of the Org.
	ok, em := globals.Permissions.AddUserGroupRole(*creator.Username, co.Name, permissions.Owner)
	if !ok {
		return nil, em
	}

	_, em = CreateOwnerFolder(ctx, co.Name, true)
	if em != nil {
		return nil, em
	}

	// Add the organization to the database.
	organization := Organization{Name: &co.Name, Description: &co.Description,
		Email: &co.Email, Creator: creator.Username}
	// Note: we also need to add (before) a row to UniqueOwners

	owner := UniqueOwner{Name: organization.Name, OwnerType: OwnerTypeOrg}
	if err := tx.Create(&owner).Create(&organization).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	or := ms.CreateOrganizationResponse(&organization, creator, false)
	ign.LoggerFromContext(ctx).Info("A new organization has been created. Name=", *organization.Name)
	return &or, nil
}

// UpdateOrganization updates an organization.
// Fields that can be currently updated: desc, email
// The user argument is the requesting user. It is used to check if the user can
// perform the operation.
func (ms *OrganizationService) UpdateOrganization(ctx context.Context, tx *gorm.DB,
	orgName string, uo *UpdateOrganization, user *User) (*Organization, *ign.ErrMsg) {

	// Sanity check: make sure the org exists
	organization, em := ByOrganizationName(tx, orgName, false)
	if em != nil {
		return nil, em
	}

	ok, em := globals.Permissions.IsAuthorized(*user.Username, *organization.Name, permissions.Write)
	if !ok {
		return nil, em
	}

	upd := tx.Model(organization)
	// Edit the description, if present.
	if uo.Description != nil {
		upd.Update("Description", *uo.Description)
	}
	// Edit email, if present.
	if uo.Email != nil {
		upd.Update("Email", *uo.Email)
	}

	// Update the modification date.
	upd.Update("ModifyDate", time.Now())

	return organization, nil
}

// GetOrganization returns the organization based on the name requested.
// param[in] The params key to look for.
// deleted[in] Whether to include deleted organizations in the search query.
func (ms *OrganizationService) GetOrganization(ctx context.Context, tx *gorm.DB,
	orgName string, deleted bool) (*Organization, *ign.ErrMsg) {
	org, em := ByOrganizationName(tx, orgName, deleted)
	if em != nil {
		return nil, em
	}

	errMsg := ign.ErrorMessageOK()
	return org, &errMsg
}

// AddUserToOrg adds an user to an organization, using the given role.
func (ms *OrganizationService) AddUserToOrg(ctx context.Context, tx *gorm.DB,
	orgName, username, role string, requestor *User) (*UserResponse, *ign.ErrMsg) {

	// Sanity check: make sure the org and user exist
	org, em := ByOrganizationName(tx, orgName, false)
	if em != nil {
		return nil, em
	}
	user, em := ByUsername(tx, username, false)
	if em != nil {
		return nil, em
	}

	// First check write permissions of the requesting user
	if ok, em := globals.Permissions.IsAuthorized(*requestor.Username, *org.Name,
		permissions.Write); !ok {
		return nil, em
	}
	// Now check if the requesting user can add other users using the given role.
	r, em := permissions.RoleFrom(role)
	if em != nil {
		return nil, em
	}
	if ok, em := globals.Permissions.IsAuthorizedForRole(*requestor.Username,
		*org.Name, r); !ok {
		return nil, em
	}

	// Add the user to the org. We do this by updating the permissions.
	// Note, adding the user to the org group means adding user to the default
	// team.
	ok, em := globals.Permissions.AddUserGroupRoleString(*user.Username, *org.Name, role)
	if !ok {
		return nil, em
	}

	ign.LoggerFromContext(ctx).Info(fmt.Sprintf("User [%s] added to Organization [%s]", username, *org.Name))

	response := CreateUserResponse(tx, user, requestor)
	return &response, nil
}

// RemoveUserFromOrg removes an user from an organization.
// NOTE: the owner of an Org cannot be removed (will return ErrorUnexpected)
func (ms *OrganizationService) RemoveUserFromOrg(ctx context.Context, tx *gorm.DB, orgName, username string,
	requestor *User) (*UserResponse, *ign.ErrMsg) {

	// Sanity check: make sure the org and user exist
	org, em := ByOrganizationName(tx, orgName, false)
	if em != nil {
		return nil, em
	}
	user, em := ByUsername(tx, username, false)
	if em != nil {
		return nil, em
	}
	// user should be able to remove self from organization
	// Otherwise check permissions of the requesting user
	if *requestor.Username != username {
		ok, em := globals.Permissions.IsAuthorized(*requestor.Username, *org.Name, permissions.Write)
		if !ok {
			return nil, em
		}
		// Now check if the requesting user can remove other users based on roles.
		role, em := globals.Permissions.GetUserRoleForGroup(*user.Username, *org.Name)
		if em != nil {
			return nil, ign.NewErrorMessage(ign.ErrorNameNotFound)
		}
		if ok, em := globals.Permissions.IsAuthorizedForRole(*requestor.Username,
			*org.Name, role); !ok {
			return nil, em
		}
	}

	// remove the user from the org. We do this by updating the permissions and roles.
	ok, em := globals.Permissions.RemoveUserFromGroup(*user.Username, *org.Name)
	if !ok {
		return nil, em
	}

	ign.LoggerFromContext(ctx).Info(fmt.Sprintf("User [%s] removed from Organization [%s]", username, *org.Name))

	response := CreateUserResponse(tx, user, requestor)
	return &response, nil
}

// GetOrgUsers returns the list of users of an Organization.
// The result will be paginated.
// user argument is the user requesting the operation.
func (ms *OrganizationService) GetOrgUsers(p *ign.PaginationRequest, tx *gorm.DB,
	orgName string, user *User) (*UserResponses, *ign.PaginationResult, *ign.ErrMsg) {

	// Sanity check: make sure the org exist
	_, em := ByOrganizationName(tx, orgName, false)
	if em != nil {
		return nil, nil, em
	}

	// an org is represented as a casbin group role.
	// Get all the users that have that role.
	usernames := globals.Permissions.GetUsersForGroup(orgName)
	q := tx.Where("username in (?)", usernames)
	return UserList(p, q, user)
}

// CreateTeam creates a new team within an organization. Returns a Team
func (ms *OrganizationService) CreateTeam(ctx context.Context, tx *gorm.DB,
	orgName string, t CreateTeamForm, creator *User) (*TeamResponse, *ign.ErrMsg) {

	// Sanity check: The JSON should have the required fields.
	// \todo: this should be moved to helper function that receives list of fields or field names
	if t.Name == "" {
		return nil, ign.NewErrorMessage(ign.ErrorMissingField)
	}
	// Sanity check (hack): teams cannot be named like roles (owner, admin, member).
	if _, em := permissions.RoleFrom(strings.ToLower(t.Name)); em == nil {
		extra := fmt.Sprintf("Team cannot be named [%s]", t.Name)
		return nil, ign.NewErrorMessageWithArgs(ign.ErrorFormInvalidValue, nil, []string{extra})
	}
	// Sanity check: make sure the org exist
	org, em := ByOrganizationName(tx, orgName, false)
	if em != nil {
		return nil, em
	}
	// check if user has permission to edit the organization
	ok, em := globals.Permissions.IsAuthorized(*creator.Username, orgName, permissions.Write)
	if !ok {
		return nil, em
	}

	// Sanity check: make sure the team does NOT exist
	if _, err := ByTeamName(tx, t.Name, true); err == nil {
		return nil, ign.NewErrorMessageWithArgs(ign.ErrorResourceExists, nil, []string{t.Name})
	}

	// Add the team to the database.
	team := Team{Name: &t.Name, Visible: *t.Visible, Description: t.Description,
		Organization: *org, Creator: creator.Username}
	if err := tx.Create(&team).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	ign.LoggerFromContext(ctx).Info(fmt.Sprintf("A new team has been created. Org:[%s] Team:[%s]",
		*org.Name, *team.Name))

	response := ms.CreateTeamResponse(orgName, &team)
	return &response, nil
}

// RemoveTeam removes the given team. Returns the removed Team
// The user argument is the requesting user. It is used to check if the user can perform the operation.
// NOTE: It does not remove the team role from the Permissions DB (casbin), in
// case we want to revert.
func (ms *OrganizationService) RemoveTeam(ctx context.Context, tx *gorm.DB,
	orgName, teamName string, user *User) (*TeamResponse, *ign.ErrMsg) {

	// Sanity check: make sure the org exists
	org, em := ByOrganizationName(tx, orgName, false)
	if em != nil {
		return nil, em
	}
	// Sanity check: make sure the team is valid
	team, em := ByTeamName(tx, teamName, false)
	if em != nil {
		return nil, em
	}

	// check if user has permission to edit the organization
	ok, em := globals.Permissions.IsAuthorized(*user.Username, *org.Name, permissions.Write)
	if !ok {
		return nil, em
	}

	// Remove the team from the database (soft-delete).
	if err := tx.Delete(team).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbDelete, err)
	}

	ign.LoggerFromContext(ctx).Info(fmt.Sprintf("Team removed. Org:[%s]. Team:[%s]", *org.Name, *team.Name))

	response := ms.CreateTeamResponse(orgName, team)
	return &response, nil
}

// GetOrganizationsAndRolesForUser returns a map with the Organizations
// and associated roles of a user. It only returns non-deleted organizations.
// If the requestor is the same user then it will include all details. Otherwise
// the returned organizations will include only those that the requestor can
// Read or are Public. Roles will be included for those that requestor can Write.
func GetOrganizationsAndRolesForUser(tx *gorm.DB, user,
	requestor *User) (map[string]string, *ign.ErrMsg) {

	orgNames := make([]string, 0)
	orgsWithRole := globals.Permissions.GetGroupsAndRolesForUser(*user.Username)
	// TODO should only include names of public orgs or those that the requestor
	// can read.
	for g := range orgsWithRole {
		orgNames = append(orgNames, g)
	}

	// Now, query the DB to filter out deleted organizations
	q := tx.Model(&Organization{}).Where("name in (?)", orgNames)
	var orgs Organizations
	if err := q.Find(&orgs).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}

	result := make(map[string]string, len(orgs))
	// filter again based on access from requestor
	if requestor != nil {
		sameUser := *requestor.Identity == *user.Identity
		reqName := *requestor.Username
		for _, o := range orgs {
			orgName := *o.Name
			hasAccess := sameUser
			if !sameUser {
				hasAccess, _ = globals.Permissions.IsAuthorized(reqName, orgName, permissions.Write)
			}
			if hasAccess {
				result[orgName] = orgsWithRole[orgName]
			} else {
				// Currently all orgs are public so no need to check Read access.
				result[orgName] = ""
			}
		}
	}

	return result, nil
}

// GetTeams returns the list of teams of an Organization.
// The result will be paginated.
// user argument is the user requesting the operation.
func (ms *OrganizationService) GetTeams(p *ign.PaginationRequest, tx *gorm.DB,
	orgName string, user *User) (*TeamResponses, *ign.PaginationResult, *ign.ErrMsg) {

	// Sanity check: make sure the org exist
	org, em := ByOrganizationName(tx, orgName, false)
	if em != nil {
		return nil, nil, em
	}
	// Check permissions of the requesting user
	ok, em := globals.Permissions.IsAuthorized(*user.Username, orgName, permissions.Read)
	if !ok {
		return nil, nil, em
	}

	q := QueryForTeams(tx).Where("organization_id = ?", org.ID)
	// if the user can edit the org then he can see ALL teams
	canEditOrg, _ := globals.Permissions.IsAuthorized(*user.Username, orgName, permissions.Write)
	if !canEditOrg {
		userTeams := getTeamsForUser(*org.Name, *user.Username)
		// for a non admin, only return visible teams AND teams which the user is member
		q = q.Where("(visible = ? OR name in (?))", true, userTeams)
	}

	var teams Teams
	pagination, err := ign.PaginateQuery(q, &teams, *p)
	if err != nil {
		return nil, nil, ign.NewErrorMessageWithBase(ign.ErrorInvalidPaginationRequest, err)
	}
	if !pagination.PageFound {
		return nil, nil, ign.NewErrorMessage(ign.ErrorPaginationPageNotFound)
	}

	responses := TeamResponses{}
	for _, t := range teams {
		responses = append(responses, ms.CreateTeamResponse(*org.Name, &t))
	}
	return &responses, pagination, nil
}

// GetTeamDetails returns a single team. The user argument is the requesting user.
func (ms *OrganizationService) GetTeamDetails(ctx context.Context, tx *gorm.DB,
	orgName, teamName string, user *User) (*TeamResponse, *ign.ErrMsg) {

	// Sanity check: make sure the org exist
	org, em := ByOrganizationName(tx, orgName, false)
	if em != nil {
		return nil, em
	}

	// Check permissions of the requesting user to access the org
	ok, em := globals.Permissions.IsAuthorized(*user.Username, *org.Name, permissions.Read)
	if !ok {
		return nil, em
	}

	// Sanity check: make sure the team exist
	team, em := ByTeamName(tx, teamName, false)
	if em != nil {
		return nil, em
	}

	// If it's a visible team then just return it
	if team.Visible {
		response := ms.CreateTeamResponse(*org.Name, team)
		return &response, nil
	}

	// If it's a private team then the user Must have access to read it
	canRead, em := canReadTeam(*user.Username, *org.Name, teamName)
	if !canRead {
		return nil, em
	}

	response := ms.CreateTeamResponse(*org.Name, team)
	return &response, nil
}

// canReadTeam returns true if the given user can read a team. Note: a user can
// read a team if he has read access to the team OR write access to the parent
// organization.
func canReadTeam(user, org, team string) (bool, *ign.ErrMsg) {
	canEditOrg, _ := globals.Permissions.IsAuthorized(user, org, permissions.Write)
	if canEditOrg {
		return true, nil
	}
	teamGroupName := getCasbinNameForTeam(org, team)
	canReadTeam, em := globals.Permissions.IsAuthorized(user, teamGroupName, permissions.Read)
	if canReadTeam {
		return true, nil
	}
	return false, em
}

// CreateTeamResponse creates a new TeamResponse struct based on the given
// Team object.
func (ms *OrganizationService) CreateTeamResponse(orgName string, team *Team) TeamResponse {
	var response TeamResponse

	// Public info
	response.Name = *team.Name
	response.Description = team.Description
	response.Visible = team.Visible
	response.Usernames = getUsersForTeam(orgName, *team.Name)
	return response
}

// UpdateTeam updates a team , and sets the list of users
// The user argument is the requesting user. It is used to check if the user can
// perform the operation.
func (ms *OrganizationService) UpdateTeam(ctx context.Context, tx *gorm.DB,
	orgName, teamName string, ut UpdateTeamForm, requestor *User) (*TeamResponse, *ign.ErrMsg) {

	// Sanity check: make sure the organization exist
	org, em := ByOrganizationName(tx, orgName, false)
	if em != nil {
		return nil, em
	}
	// Check permissions of the requesting user
	ok, em := globals.Permissions.IsAuthorized(*requestor.Username, *org.Name, permissions.Write)
	if !ok {
		return nil, em
	}
	// Sanity check: make sure the team exist
	team, em := ByTeamName(tx, teamName, false)
	if em != nil {
		return nil, em
	}
	// Sanity check: Validate that got usernames are valid Organization users.
	teamGroupName := getCasbinNameForTeam(orgName, teamName)
	for _, name := range ut.RmUsers {
		isInTeam := globals.Permissions.UserBelongsToGroup(name, teamGroupName)
		if !isInTeam {
			return nil, ign.NewErrorMessageWithArgs(ign.ErrorFormInvalidValue, nil,
				[]string{"Team does not have user:" + name})
		}
	}
	newUsers := make([]string, 0)
	for _, name := range ut.NewUsers {
		// the user should already belong to Organization (default team)
		belongsToOrg := globals.Permissions.UserBelongsToGroup(name, orgName)
		if !belongsToOrg {
			return nil, ign.NewErrorMessageWithArgs(ign.ErrorFormInvalidValue, nil,
				[]string{"user does not belong to Org:" + name})
		}
		teamGroupName := getCasbinNameForTeam(orgName, teamName)
		isInTeam := globals.Permissions.UserBelongsToGroup(name, teamGroupName)
		if !isInTeam {
			newUsers = append(newUsers, name)
		}
	}

	// add/remove users
	for _, uname := range ut.RmUsers {
		if em := removeUserFromTeam(ctx, tx, team, uname); em != nil {
			return nil, em
		}
	}
	for _, uname := range newUsers {
		if em := addUserToTeam(ctx, tx, team, uname); em != nil {
			return nil, em
		}
	}

	// Update the visibility
	if ut.Visible != nil {
		tx.Model(team).Update("Visible", *ut.Visible)
	}
	// Edit the description, if present.
	if ut.Description != nil {
		tx.Model(team).Update("Description", ut.Description)
	}

	ign.LoggerFromContext(ctx).Info(fmt.Sprintf("Team was updated. Org:[%s]. Team:[%s]",
		*org.Name, *team.Name))

	response := ms.CreateTeamResponse(orgName, team)
	return &response, nil
}

// getUsersForTeam gets the users that belong to a group's team.
func getUsersForTeam(org, team string) []string {
	teamGroupName := getCasbinNameForTeam(org, team)
	return globals.Permissions.GetUsersForGroup(teamGroupName)
}

// returns the organization teams of a user, by browsing the casbin roles.
func getTeamsForUser(org, user string) []string {
	teams := make([]string, 0)
	roles := globals.Permissions.GetGroupsAndRolesForUser(user)
	re := regexp.MustCompile(org + "_t_(.+)$")
	for g := range roles {
		s := re.FindStringSubmatch(g)
		if s != nil {
			team := s[1]
			teams = append(teams, team)
		}
	}
	return teams
}

// adds a user to a team, using casbin groups.
func addUserToTeam(ctx context.Context, tx *gorm.DB,
	team *Team, username string) *ign.ErrMsg {

	org := team.Organization

	// Add the user to the team in casbin too
	if org.Name != nil && team.Name != nil {
		teamGroupName := getCasbinNameForTeam(*org.Name, *team.Name)
		ok, em := globals.Permissions.AddUserGroupRole(username, teamGroupName, permissions.Member)
		if !ok {
			return em
		}
	} else {
		return ign.NewErrorMessage(ign.ErrorUnexpected)
	}
	return nil
}

// removes a user from a team, using casbin groups.
func removeUserFromTeam(ctx context.Context, tx *gorm.DB,
	team *Team, username string) *ign.ErrMsg {
	org := team.Organization

	// remove the user from the team. We do this by updating the permissions and roles.
	teamGroupName := getCasbinNameForTeam(*org.Name, *team.Name)
	ok, em := globals.Permissions.RemoveUserFromGroup(username, teamGroupName)
	if !ok {
		return em
	}

	return nil
}

// get the string representing a specific team of a group
func getCasbinNameForTeam(org, team string) string {
	return org + "_t_" + team
}
