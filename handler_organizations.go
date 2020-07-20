package main

import (
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"net/http"
)

// OrganizationCreate creates a new organization
// You can request this method with the following cURL request:
//  curl -k -H "Content-Type: application/json" -X POST -d '{"name":"OSRF",
//    "description":"non-profit", "email":"myemail@org.org"}'
//    https://localhost:4430/1.0/organizations
//    --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func OrganizationCreate(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	var organization users.CreateOrganization
	if em := ParseStruct(&organization, r, false); em != nil {
		return nil, em
	}

	// Sanity check: Find the user associated to the given JWT. Fail if no user.
	user, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	response, em := (&users.OrganizationService{}).CreateOrganization(r.Context(),
		tx, organization, user)
	if em != nil {
		return nil, em
	}

	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	return response, nil
}

// OrganizationList returns a list with all organizations.
// You can request this method with the following cURL request:
//   curl -k -X GET --url https://localhost:4430/1.0/organizations
//     --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func OrganizationList(p *ign.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	return (&users.OrganizationService{}).OrganizationList(p, tx, user, false)
}

// OrganizationUserList returns a paginated list with the users of an organization.
func OrganizationUserList(p *ign.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	orgName, em := getName(tx, r)
	if em != nil {
		return nil, nil, em
	}
	orgSvc := &users.OrganizationService{}
	return orgSvc.GetOrgUsers(p, tx, *orgName, user)
}

// OrganizationIndex returns a single organization
// You can request this method with the following cURL request:
//   curl -k -X GET --url https://localhost:4430/1.0/organizations/{name}
//     --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
// Or you can use the following request for retrieving only the public data:
//   curl -k -X GET --url https://localhost:4430/1.0/organizations/{name}
func OrganizationIndex(orgName string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	orgSvc := &users.OrganizationService{}
	org, errMsg := orgSvc.GetOrganization(r.Context(), tx, orgName, false)
	if org == nil {
		return nil, errMsg
	}

	response := orgSvc.CreateOrganizationResponse(org, jwtUser, false)
	return response, nil
}

// OrganizationRemove deletes an organization.
// You can request this method with the following cURL request:
//   curl -k -X DELETE --url https://localhost:4430/1.0/organizations/{name}
//     --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func OrganizationRemove(orgName string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	orgSvc := &users.OrganizationService{}
	response, em := orgSvc.RemoveOrganization(r.Context(), tx, orgName, jwtUser)
	if em != nil {
		return nil, em
	}

	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbDelete, err)
	}

	return response, nil
}

// getName returns the value of the "name" parameter from the HTTP route.
// Returns an ign.ErrMsg if not present
func getName(tx *gorm.DB, r *http.Request) (*string, *ign.ErrMsg) {
	// Extract the organization name from the request.
	params := mux.Vars(r)
	// Get the organization
	orgName, present := params["name"]
	// If the key does not exist
	if !present {
		return nil, ign.NewErrorMessage(ign.ErrorUserNotInRequest)
	}

	return &orgName, nil
}

// getTeamName returns the value of the "teamname" parameter from the HTTP route.
// Returns an ign.ErrMsg if not present
func getTeamName(r *http.Request) (string, *ign.ErrMsg) {
	// get team name from request
	params := mux.Vars(r)
	teamName, present := params["teamname"]
	// If the key does not exist
	if !present {
		return "", ign.NewErrorMessage(ign.ErrorIDNotInRequest)
	}
	return teamName, nil
}

// OrganizationUpdate modifies an existing organization.
// You can request this method with the following cURL request:
//    curl -k -X PATCH -d '{"description":"New Description"}'
//      https://localhost:4430/1.0/organizations/{name} -H "Content-Type: application/json"
//      -H 'Authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func OrganizationUpdate(orgName string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	var uo users.UpdateOrganization
	if em := ParseStruct(&uo, r, false); em != nil {
		return nil, em
	}
	if uo.IsEmpty() {
		return nil, ign.NewErrorMessage(ign.ErrorFormInvalidValue)
	}

	orgSvc := &users.OrganizationService{}
	org, em := orgSvc.UpdateOrganization(r.Context(), tx, orgName, &uo, jwtUser)
	if em != nil {
		return nil, em
	}

	// Commit the DB transaction.
	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	infoStr := "Organization has been updated:" +
		"\n\t name: " + *org.Name +
		"\n\t description: " + *org.Description
	ign.LoggerFromRequest(r).Info(infoStr)

	// If the user can update the org, then it can see its private info
	response := (&users.OrganizationService{}).CreateOrganizationResponse(org, jwtUser, false)
	return response, nil
}

// OrganizationUserCreate adds a user to an organization with a given role.
// You can request this method with the following cURL request:
//    curl -k -X POST https://localhost:4430/1.0/organizations/{orgName}/users
//      -H "Content-Type: application/json"
//      -d '{"username":"theUserToAdd", "role":"owner|admin|member"}'
//      --header 'authorization: Bearer <your-jwt-token-here>'
// It returns the added user
func OrganizationUserCreate(orgName string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	var orgUser users.AddUserToOrgInput
	if em := ParseStruct(&orgUser, r, false); em != nil {
		return nil, em
	}

	resp, em := (&users.OrganizationService{}).AddUserToOrg(r.Context(), tx, orgName, orgUser.Username, orgUser.Role, jwtUser)
	if em != nil {
		return nil, em
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	return resp, nil
}

// OrganizationUserRemove removes a user from an organization.
// You can request this method with the following cURL request:
//    curl -k -X DELETE https://localhost:4430/1.0/organizations/{orgName}/users/{username}
//      --header 'authorization: Bearer <your-jwt-token-here>'
// It returns the added user
func OrganizationUserRemove(orgName string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	// Extract the username of the user to remove from the request.
	params := mux.Vars(r)
	userToRemove, present := params["username"]
	// If the key does not exist
	if !present {
		return nil, ign.NewErrorMessage(ign.ErrorUserNotInRequest)
	}

	resp, em := (&users.OrganizationService{}).RemoveUserFromOrg(r.Context(), tx,
		orgName, userToRemove, jwtUser)
	if em != nil {
		return nil, em
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	return resp, nil
}

// OrganizationTeamsList returns a paginated list with the teams of an organization.
func OrganizationTeamsList(p *ign.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	orgName, em := getName(tx, r)
	if em != nil {
		return nil, nil, em
	}
	orgSvc := &users.OrganizationService{}
	return orgSvc.GetTeams(p, tx, *orgName, user)
}

// OrganizationTeamCreate adds a team to an organization.
// You can request this method with the following cURL request:
//    curl -k -X POST https://localhost:4430/1.0/organizations/{orgName}/teams
//      -H "Content-Type: application/json"
//      -d '{"name":"teamName", "visible":"aBool", "description":"desc"}'
//      --header 'authorization: Bearer <your-jwt-token-here>'
// It returns the created team
func OrganizationTeamCreate(orgName string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	var teamInput users.CreateTeamForm
	if em := ParseStruct(&teamInput, r, false); em != nil {
		return nil, em
	}

	response, em := (&users.OrganizationService{}).CreateTeam(r.Context(), tx, orgName, teamInput, jwtUser)
	if em != nil {
		return nil, em
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	return response, nil
}

// OrganizationTeamRemove removes a team from an organization.
// You can request this method with the following cURL request:
//    curl -k -X DELETE https://localhost:4430/1.0/organizations/{orgName}/teams/{teamname}
//      --header 'authorization: Bearer <your-jwt-token-here>'
// It returns the team
func OrganizationTeamRemove(orgName string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	// Extract the team name from the request.
	teamName, em := getTeamName(r)
	if em != nil {
		return nil, em
	}

	response, em := (&users.OrganizationService{}).RemoveTeam(r.Context(), tx,
		orgName, teamName, jwtUser)
	if em != nil {
		return nil, em
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	return response, nil
}

// OrganizationTeamUpdate modifies an existing team.
// You can request this method with the following cURL request:
//    curl -k -X PATCH -d '{"description":"New Description"}'
//      https://localhost:4430/1.0/organizations/{name}/teams/{teamname} -H "Content-Type: application/json"
//      -H 'Authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
// It returns the updated team
func OrganizationTeamUpdate(orgName string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	// Extract the team name from the request.
	teamName, em := getTeamName(r)
	if em != nil {
		return nil, em
	}

	var ut users.UpdateTeamForm
	if em := ParseStruct(&ut, r, false); em != nil {
		return nil, em
	}

	orgSvc := &users.OrganizationService{}
	response, em := orgSvc.UpdateTeam(r.Context(), tx, orgName, teamName, ut, jwtUser)
	if em != nil {
		return nil, em
	}

	// Commit the DB transaction.
	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	infoStr := "Organization Team has been updated: \n\t name: " + teamName
	ign.LoggerFromRequest(r).Info(infoStr)

	return response, nil
}

// OrganizationTeamIndex returns a single team.
// You can request this method with the following cURL request:
//   curl -k -X GET --url https://localhost:4430/1.0/organizations/{name}/teams/{teamname}
//     --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func OrganizationTeamIndex(orgName string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	// Extract the team name from the request.
	teamName, em := getTeamName(r)
	if em != nil {
		return nil, em
	}

	orgSvc := &users.OrganizationService{}
	return orgSvc.GetTeamDetails(r.Context(), tx, orgName, teamName, jwtUser)
}
