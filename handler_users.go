package main

import (
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"net/http"
)

// Login returns information about the user associated with a JWT
// You can request this method with the following cURL request:
//   curl -k -X GET --url https://localhost:4430/1.0/login
//     --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func Login(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {
	// Sanity check: Make sure that we have a user with the identity contained in
	// the JWT.
	identity, ok := ign.GetUserIdentity(r)
	if !ok {
		return nil, ign.NewErrorMessage(ign.ErrorAuthJWTInvalid)
	}

	return users.GetUserByIdentity(tx, identity)
}

// UserCreate creates a new user
// You can request this method with the following cURL request:
//  curl -k -H "Content-Type: application/json" -X POST -d '{"name":"John Doe",
//    "username":"test-username", "email":"johndoe@example.com", "org":"my org"}'
//    https://localhost:4430/1.0/users
//    --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func UserCreate(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	var u users.User
	if em := ParseStruct(&u, r, false); em != nil {
		return nil, em
	}

	if identity, ok := ign.GetUserIdentity(r); ok {
		u.Identity = &identity
	} else {
		return nil, ign.NewErrorMessage(ign.ErrorAuthJWTInvalid)
	}

	return users.CreateUser(r.Context(), tx, &u, true)
}

// UserList returns a list with all users.
func UserList(p *ign.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	user, ok, errMsg := getUserFromJWT(tx, r)

	if !ok && (errMsg.ErrCode != ign.ErrorAuthJWTInvalid &&
		errMsg.ErrCode != ign.ErrorAuthNoUser) {
		return nil, nil, &errMsg
	}

	if !globals.Permissions.IsSystemAdmin(*user.Username) {
		return nil, nil, ign.NewErrorMessage(ign.ErrorUnauthorized)
	}

	return users.UserList(p, tx, user)
}

// UserIndex returns a single user
// You can request this method with the following cURL request:
//   curl -k -X GET --url https://localhost:4430/1.0/users/{username}
//     --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
// Or you can use the following request for retrieving only the public data:
//   curl -k -X GET --url https://localhost:4430/1.0/users/{username}
func UserIndex(username string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	user, em := users.ByUsername(tx, username, false)
	if em != nil {
		return nil, em
	}

	response := users.CreateUserResponse(tx, user, jwtUser)
	return response, nil
}

// UserRemove deletes a user.
// You can request this method with the following cURL request:
//   curl -k -X DELETE --url https://localhost:4430/1.0/users/{username}
//     --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func UserRemove(username string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	return users.RemoveUser(r.Context(), tx, username, jwtUser)
}

// UserUpdate updates a user.
// You can request this method with the following cURL request:
//   curl -k -X PATCH -d '{"name":"New name", "email": "myemail@user.me"}'
//     --url https://localhost:4430/1.0/users/{username}
//     --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func UserUpdate(username string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	var uu users.UpdateUserInput
	if em := ParseStruct(&uu, r, false); em != nil {
		return nil, em
	}
	if uu.IsEmpty() {
		return nil, ign.NewErrorMessage(ign.ErrorFormInvalidValue)
	}

	return users.UpdateUser(r.Context(), tx, username, &uu, jwtUser)
}

// OwnerProfile returns the details of a User OR an Organization, based on
// the given owner name.
func OwnerProfile(username string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	return users.GetOwnerProfile(tx, username, jwtUser)
}

// AccessTokenList returns a paginated list with the user's access tokens.
func AccessTokenList(p *ign.PaginationRequest, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	params := mux.Vars(r)
	username, valid := params["username"]
	if !valid || username == "" {
		return nil, nil, ign.NewErrorMessage(ign.ErrorUnauthorized)
	}

	// Get requested user
	user, em := users.ByUsername(tx, username, false)
	if em != nil {
		return nil, nil, em
	}

	// Make sure the requested user matches the JWT.
	if *user.Identity != *jwtUser.Identity {
		return nil, nil, ign.NewErrorMessage(ign.ErrorUnauthorized)
	}

	return users.AccessTokenList(p, tx, jwtUser)
}

// AccessTokenDelete removes a personal access token. This function requires the user's JWT, which
// means that a personal access token cannot be used to remove access token.
func AccessTokenDelete(username string, jwtUser *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	// Get requested user
	user, em := users.ByUsername(tx, username, false)
	if em != nil {
		return nil, em
	}

	// Make sure the requested user matches the JWT.
	if *user.Identity != *jwtUser.Identity {
		return nil, ign.NewErrorMessage(ign.ErrorUnauthorized)
	}

	// Read the access token to delete.
	var accessToken ign.AccessToken
	if em := ParseStruct(&accessToken, r, false); em != nil {
		return nil, em
	}

	return users.AccessTokenDelete(jwtUser, tx, accessToken)
}

// AccessTokenCreate creates a personal access token for a user. This function requires a JWT
// which means a personal access token cannot be used to create more access tokens.
func AccessTokenCreate(username string, jwtUser *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	// Get requested user
	user, em := users.ByUsername(tx, username, false)
	if em != nil {
		return nil, em
	}

	// Make sure the requested user matches the JWT.
	if *user.Identity != *jwtUser.Identity {
		return nil, ign.NewErrorMessage(ign.ErrorUnauthorized)
	}

	// Parse the name of the token.
	var accessTokenCreateInfo ign.AccessTokenCreateRequest
	if em := ParseStruct(&accessTokenCreateInfo, r, false); em != nil {
		return nil, em
	}

	return users.AccessTokenCreate(jwtUser, tx, accessTokenCreateInfo)
}
