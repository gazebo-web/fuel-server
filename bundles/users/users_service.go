package users

import (
	"context"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/permissions"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
	"os"
	"path"
	"time"
)

// RemoveUser removes the given user. Returns a UserResponse with the removed user.
// The reqUser argument is the requesting user. It is used to check if the
// reqUser can perform the operation.
func RemoveUser(ctx context.Context, tx *gorm.DB, username string, reqUser *User) (*UserResponse, *gz.ErrMsg) {

	user, em := ByUsername(tx, username, false)
	if em != nil {
		return nil, em
	}

	// Make sure the JWT user is the same user to be removed
	if *user.Identity != *reqUser.Identity {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}

	// Sanity check: Make sure that the directory exists.
	dirPath := path.Join(globals.ResourceDir, *user.Username)
	if _, err := os.Stat(dirPath); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNonExistentResource, err)
	}

	// NOTE: we are not removing the user's folder.

	// Remove the user from the database (soft-delete).
	owner := UniqueOwner{Name: user.Username}
	if err := tx.Delete(user).Delete(&owner).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}

	ok, em := globals.Permissions.RemoveUser(*user.Username)
	if !ok {
		return nil, em
	}

	gz.LoggerFromContext(ctx).Info("User removed. Username=", *user.Username, " Email=", *user.Email)

	response := CreateUserResponse(tx, user, reqUser)
	return &response, nil
}

// UpdateUser updates an user.
// Fields that can be currently updated: name, email
// The reqUser argument is the requesting user. It is used to check if the
// reqUser can perform the operation.
func UpdateUser(ctx context.Context, tx *gorm.DB, username string,
	uu *UpdateUserInput, reqUser *User) (*UserResponse, *gz.ErrMsg) {

	// Sanity check: make sure the user exists
	user, em := ByUsername(tx, username, false)
	if em != nil {
		return nil, em
	}

	// Make sure the JWT user is the same user to be updated
	if *user.Identity != *reqUser.Identity {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}

	upd := tx.Model(user)
	// Edit the fields, if present.
	if uu.Name != nil {
		upd.Update("Name", *uu.Name)
	}
	if uu.Email != nil {
		upd.Update("Email", *uu.Email)
	}
	if uu.ExpFeatures != nil {
		upd.Update("ExpFeatures", *uu.ExpFeatures)
	}
	// Update the modification date.
	upd.Update("ModifyDate", time.Now())

	gz.LoggerFromContext(ctx).Info("User updated. Username=", *user.Username,
		" Email=", *user.Email)

	ur := CreateUserResponse(tx, user, reqUser)
	return &ur, nil
}

// UserList returns a list of paginated UserResponses.
func UserList(p *gz.PaginationRequest, tx *gorm.DB,
	reqUser *User) (*UserResponses, *gz.PaginationResult, *gz.ErrMsg) {
	// Get the users
	var us Users

	// Create the DB query
	q := tx.Model(&User{})

	pagination, err := gz.PaginateQuery(q, &us, *p)
	if err != nil {
		return nil, nil, gz.NewErrorMessageWithBase(gz.ErrorInvalidPaginationRequest, err)
	}
	if !pagination.PageFound {
		return nil, nil, gz.NewErrorMessage(gz.ErrorPaginationPageNotFound)
	}

	// Create UserReponse results
	responses := UserResponses{}
	for _, user := range us {
		responses = append(responses, CreateUserResponse(tx, &user, reqUser))
	}

	return &responses, pagination, nil
}

// GetUserByIdentity returns a user given an identity.
// This method will fail if the identify does not correspond to an active user.
func GetUserByIdentity(tx *gorm.DB, identity string) (*UserResponse, *gz.ErrMsg) {
	user, em := ByIdentity(tx, identity, false)
	if em != nil {
		return nil, em
	}

	ur := CreateUserResponse(tx, user, user)
	return &ur, nil
}

// CreateUserResponse creates a new UserResponse struct based on the given
// User object.
// The returned UserResponse will also include user private fields if the
// requestor can access those
func CreateUserResponse(tx *gorm.DB, user, requestor *User) UserResponse {
	var response UserResponse

	// Public info
	response.Username = *user.Username
	if user.Name != nil {
		response.Name = *user.Name
	}
	response.Organizations = make([]string, 0)

	blankQuery := tx.New()
	orgs, _ := GetOrganizationsAndRolesForUser(blankQuery, user, requestor)
	for g := range orgs {
		response.Organizations = append(response.Organizations, g)
	}

	response.SysAdmin = false

	// Private data should be included if the user is the same as the requestor or
	// if the requestor is a sysAdmin.
	if requestor != nil {
		privateAccess := false

		// Checks for System Admin and Same User.
		isSystemAdmin := globals.Permissions.IsSystemAdmin(*requestor.Username)
		isSameUser := *user.Identity == *requestor.Identity

		// Set the SysAdmin field only if both cases apply.
		if isSystemAdmin && isSameUser {
			response.SysAdmin = true
		}

		// only is system admin or self
		if isSystemAdmin || isSameUser {
			privateAccess = true
			if user.ExpFeatures != nil {
				response.ExpFeatures = *user.ExpFeatures
			}
		} else {
			// If the requestor has write access to any user's org, then
			// we can include private data.
			for _, r := range orgs {
				if r != "" {
					privateAccess = true
					break
				}
			}
		}

		if privateAccess {
			response.Email = *user.Email
			response.ID = user.ID
			response.OrgRoles = orgs
		}
	}

	return response
}

// CreateUser creates a new User in filesystem and DB using the data from
// the given User struct.
// Returns a UserResponse.
func CreateUser(ctx context.Context, tx *gorm.DB, u *User, failIfDirExist bool) (*UserResponse, *gz.ErrMsg) {
	// Sanity check: Make sure that the identity (JWT) is not already used by an active
	// user.
	aUser, em := ByIdentity(tx, *u.Identity, false)
	if em != nil && em.ErrCode != gz.ErrorAuthNoUser {
		return nil, em
	}
	if aUser != nil {
		return nil, gz.NewErrorMessage(gz.ErrorResourceExists)
	}
	// Sanity check: Make sure that the claimed username was not already used,
	// even with removed users or organizations.
	ownerName, em := OwnerByName(tx, *u.Username, true)
	if em != nil && em.ErrCode != gz.ErrorUserUnknown {
		return nil, em
	}
	if ownerName != nil {
		return nil, gz.NewErrorMessage(gz.ErrorResourceExists)
	}

	var aTeam Team

	tx.Where("name = ?", *u.Username).First(&aTeam)
	if aTeam.Name != nil && *aTeam.Name == *u.Username {
		return nil, gz.NewErrorMessage(gz.ErrorResourceExists)
	}

	_, em = CreateOwnerFolder(ctx, *u.Username, true)
	if em != nil {
		return nil, em
	}

	// Add the user to the database.
	// Note: we also need to add (before) a row to UniqueOwners
	owner := UniqueOwner{Name: u.Username, OwnerType: OwnerTypeUser}
	if err := tx.Create(&owner).Create(&u).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	ur := CreateUserResponse(tx, u, u)
	gz.LoggerFromContext(ctx).Info("A new user has been created. Username=", *u.Username,
		" Email=", *u.Email)

	return &ur, nil
}

// VerifyOwner checks to see if the 'owner' arg is an organization or a user. If the
// 'owner' is an organization, it verifies that the given 'user' arg has the expected
// permission in the organization. If the 'owner' is a user, it verifies that the
// 'user' arg is the same as the owner.
func VerifyOwner(tx *gorm.DB, owner, user string,
	per permissions.Action) (bool, *gz.ErrMsg) {
	// check if owner is an organization
	org, em := ByOrganizationName(tx, owner, false)
	if org != nil && em == nil {
		// check if user has write permission in that organization
		ok, em := globals.Permissions.IsAuthorized(user, *org.Name, per)
		if !ok {
			return false, em
		}
	} else {
		// Owner is a user. Make sure the owner is the same as the jwt user.
		if owner != user {
			// jwt user is different from owner field!
			return false, gz.NewErrorMessage(gz.ErrorUnauthorized)
		}
	}
	return true, nil
}

// CanPerformWithRole checks to see if the 'owner' arg is an organization or a
// user. If the 'owner' is an organization, it verifies that the given 'user' arg
// is authorized to act as the given Role (or above) in the organization.
// If the 'owner' is a user, it verifies that the 'user' arg is the same as
// the owner.
func CanPerformWithRole(tx *gorm.DB, owner, user string,
	role permissions.Role) (bool, *gz.ErrMsg) {
	// check if owner is an organization
	org, em := ByOrganizationName(tx, owner, false)
	if org != nil && em == nil {
		// check if user can act with the given role in the organization
		ok, em := globals.Permissions.IsAuthorizedForRole(user, *org.Name, role)
		if !ok {
			return false, em
		}
	} else {
		// Owner is a user. Make sure the owner is the same as the jwt user.
		if owner != user {
			return false, gz.NewErrorMessage(gz.ErrorUnauthorized)
		}
	}
	return true, nil
}

// CheckPermissions validates if the given user has the requested permission on
// the resource. The resource can be public or private, and that is extracted
// from the argument isPrivate.
func CheckPermissions(tx *gorm.DB, resource string, user *User, isPrivate bool,
	per permissions.Action) (bool, *gz.ErrMsg) {

	if !isPrivate && per == permissions.Read {
		return true, nil
	}

	if user == nil {
		if isPrivate || per != permissions.Read {
			return false, gz.NewErrorMessage(gz.ErrorUnauthorized)
		}
		// otherwise it should be public and with Read permission.
		return true, nil
	}
	// user is not nil
	// make sure the requesting user has the correct permissions
	if globals.Permissions.IsSystemAdmin(*user.Username) {
		return true, nil
	}
	// make sure the user requesting removal has the correct permissions
	if ok, em := globals.Permissions.IsAuthorized(*user.Username, resource, per); !ok {
		return false, em
	}
	return true, nil
}

// OwnerProfile stores information about a user OR an organization.
//
// swagger:model
type OwnerProfile struct {
	// The type: 'users' or 'organizations'
	OwnerType string
	// Optional UserResponse
	User *UserResponse
	// Optional OrganizationResponse
	Org *OrganizationResponse
}

// GetOwnerProfile returns the details of a user or an organization.
func GetOwnerProfile(tx *gorm.DB, owner string, user *User) (*OwnerProfile, *gz.ErrMsg) {

	o, em := OwnerByName(tx, owner, false)
	if em != nil {
		return nil, em
	}

	if o.OwnerType == OwnerTypeUser {
		u, em := ByUsername(tx, owner, false)
		if em != nil {
			return nil, em
		}
		ur := CreateUserResponse(tx, u, user)
		return &OwnerProfile{OwnerType: OwnerTypeUser, User: &ur}, nil
	}
	// Else , we assume it is an Organization
	org, em := ByOrganizationName(tx, owner, false)
	if em != nil {
		return nil, em
	}
	or := (&OrganizationService{}).CreateOrganizationResponse(org, user, false)
	return &OwnerProfile{OwnerType: OwnerTypeOrg, Org: &or}, nil
}

// AccessTokenList returns a list of paginated AccessTokens.
func AccessTokenList(p *gz.PaginationRequest, tx *gorm.DB,
	reqUser *User) (*gz.AccessTokens, *gz.PaginationResult, *gz.ErrMsg) {

	var accessTokens gz.AccessTokens

	q := tx.Model(&gz.AccessToken{}).Where("user_id = ?", reqUser.ID)

	pagination, err := gz.PaginateQuery(q, &accessTokens, *p)
	if err != nil {
		return nil, nil, gz.NewErrorMessageWithBase(gz.ErrorInvalidPaginationRequest, err)
	}
	if !pagination.PageFound {
		return nil, nil, gz.NewErrorMessage(gz.ErrorPaginationPageNotFound)
	}

	// Strip out the keys
	for i := range accessTokens {
		accessTokens[i].Key = ""
	}

	return &accessTokens, pagination, nil
}

// AccessTokenDelete removes a personal access token. This function requires the user's JWT, which
// means that a personal access token cannot be used to remove access token.
func AccessTokenDelete(jwtUser *User, tx *gorm.DB, accessToken gz.AccessToken) (interface{}, *gz.ErrMsg) {

	// Get the token.
	var token gz.AccessToken
	if err := tx.Model(jwtUser).Related(&jwtUser.AccessTokens).Where(
		"prefix = ? AND name = ?", accessToken.Prefix, accessToken.Name).First(&token).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}

	// Permanently delete the token
	tx.Unscoped().Delete(&token)
	return nil, nil
}

// AccessTokenCreate creates a new access token for a user.
func AccessTokenCreate(jwtUser *User, tx *gorm.DB, accessTokenCreateRequest gz.AccessTokenCreateRequest) (interface{}, *gz.ErrMsg) {

	newToken, saltedToken, err := accessTokenCreateRequest.Create(tx)

	if err != nil {
		return nil, err
	}

	tx.Model(jwtUser).Association("AccessTokens").Append(saltedToken)
	return newToken, nil
}
