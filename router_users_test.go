package main

import (
	"bytes"
	"encoding/json"
	"github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"gitlab.com/ignitionrobotics/web/ign-go/testhelpers"
	"net/http"
	"os"
	"testing"
)

// Tests for user related routes

// loginUserTest represents the input and expected output for a TestUserLogin test case.
type loginUserTest struct {
	uriTest
	// expected username in user response
	expUsername string
	expOrgs     []string
	expOrgRoles map[string]string
}

// TestUserLogin tests the /login route.
func TestUserLogin(t *testing.T) {
	setup()
	// Create a random user
	username := createUser(t)
	defer removeUser(username, t)
	// Also create a random organization to test login with organization names
	// Should not be able to login using an organization name.
	orgName := createOrganization(t)
	defer removeOrganization(orgName, t)
	orgName2 := createOrganization(t)

	// create a separate user and add him to orgs
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	addUserToOrg(username2, "member", orgName, t)
	addUserToOrg(username2, "admin", orgName2, t)

	myJWT := os.Getenv("IGN_TEST_JWT")
	uri := "/1.0/login"
	loginUserTestsData := []loginUserTest{
		{uriTest{"valid login", uri, newJWT(myJWT), nil, false}, username,
			[]string{orgName, orgName2}, map[string]string{orgName: "owner",
				orgName2: "owner"}},
		{uriTest{"login user2", uri, newJWT(jwt2), nil, false}, username2,
			[]string{orgName, orgName2}, map[string]string{orgName: "member",
				orgName2: "admin"}},
		{uriTest{"invalid token", uri, newJWT("pahjtrkjfd"),
			ign.NewErrorMessage(ign.ErrorUnauthorized), true}, "", nil, nil},
		{uriTest{"invalid claims - no sub", uri,
			newClaimsJWT(&jwt.MapClaims{"invalid": "user"}),
			ign.NewErrorMessage(ign.ErrorAuthJWTInvalid), false}, "", nil, nil},
		{uriTest{"empty claims", uri, newClaimsJWT(&jwt.MapClaims{}),
			ign.NewErrorMessage(ign.ErrorAuthJWTInvalid), false}, "", nil, nil},
		{uriTest{"unexistent identity", uri,
			newClaimsJWT(&jwt.MapClaims{"sub": "non-existing-user"}),
			ign.NewErrorMessage(ign.ErrorAuthNoUser), false}, "", nil, nil},
	}

	for _, test := range loginUserTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithLoginUserTest(test, t)
		})
	}

	// remove the org 2 . And check returned data when user logs in.
	// Org2 should not be present in the list or user's organizations.
	removeOrganization(orgName2, t)

	loginUserTestsData = []loginUserTest{
		{uriTest{"should have fewer orgs", uri, newJWT(myJWT), nil, false}, username,
			[]string{orgName}, map[string]string{orgName: "owner"}},
		{uriTest{"should have fewer orgs #2", uri, newJWT(jwt2), nil, false},
			username2, []string{orgName}, map[string]string{orgName: "member"}},
	}

	for _, test := range loginUserTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithLoginUserTest(test, t)
		})
	}
	// now try to remove the 2nd user
	removeUserWithJWT(username2, jwt2, t)
}

// runSubTestWithLoginUserTest tries to login a user and check returned data.
func runSubTestWithLoginUserTest(test loginUserTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	bslice, _ := igntest.AssertRouteMultipleArgs("GET", test.URL, nil, expStatus, jwt, expCt, t)
	if expStatus == http.StatusOK {
		var ur users.UserResponse
		assert.NoError(t, json.Unmarshal(*bslice, &ur), "Unable to unmarshal user response %s", string(*bslice))
		assert.Equal(t, test.expUsername, ur.Username, "Got username [%s] different than expected [%s]", ur.Username, test.expUsername)
		assert.ElementsMatch(t, test.expOrgs, ur.Organizations)
		assert.Equal(t, test.expOrgRoles, ur.OrgRoles)
	} else if !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name()+" GET /login", bslice, expEm.ErrCode, t)
	}
}

// createUserTest includes the input and expected output for a TestUserCreate test case.
type createUserTest struct {
	uriTest
	// user data
	user users.User
	// should also delete the created user as part of this test case?
	deleteAfter bool
}

// TestUserCreate tests the POST /users route. It also optionally Deletes the user on each test
func TestUserCreate(t *testing.T) {
	setup()

	// create a user with the default JWT
	defaultUser := createUser(t)
	defer removeUser(defaultUser, t)
	// Also create a random organization to test duplicate names
	orgName := createOrganization(t)
	defer removeOrganization(orgName, t)

	// Now create a new JWT for the tests
	jwt := createValidJWTForIdentity("another-user", t)
	jwtDef := newJWT(jwt)

	uri := "/1.0/users"
	name := "A random user"
	email := "username@example.com"
	org := "My organization"
	username := ign.RandomString(8)
	invalidUsername := "d aaaa"
	// create and remove another user (ie. a non active user)
	jwt2 := createValidJWTForIdentity("another-user-2", t)
	username2 := createUserWithJWT(jwt2, t)
	removeUserWithJWT(username2, jwt2, t)

	userCreateTestsData := []createUserTest{
		{uriTest{"no username", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, users.User{Name: &name, Email: &email, Organization: &org}, false},
		{uriTest{"blacklisted username", uri, jwtDef,
			ign.NewErrorMessage(ign.ErrorFormInvalidValue), false},
			users.User{Username: sptr("settings"), Name: &name, Email: &email,
				Organization: &org}, false},
		{uriTest{"no email", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, users.User{Username: sptr(ign.RandomString(8)), Name: &name, Organization: &org}, false},
		{uriTest{"short username", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, users.User{Username: sptr("aa"), Email: &email}, false},
		{uriTest{"invalid username", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, users.User{Username: &invalidUsername, Email: &email}, false},
		{uriTest{"no optional fields", uri, jwtDef, nil, false}, users.User{Username: sptr(ign.RandomString(8)), Email: &email}, true},
		// Note: the following test cases are inter-related, as the test for duplication.
		{uriTest{"with all fields", uri, jwtDef, nil, false}, users.User{Username: &username, Name: &name, Email: &email, Organization: &org}, false},
		{uriTest{"another user using existent JWT", uri, jwtDef, ign.NewErrorMessage(ign.ErrorResourceExists), false}, users.User{Username: sptr(ign.RandomString(8)), Email: &email}, false},
		{uriTest{"dup username", uri, jwtDef, ign.NewErrorMessage(ign.ErrorResourceExists), false}, users.User{Username: &username, Name: &name, Email: &email, Organization: &org}, true},
		{uriTest{"should be able to reuse JWT after user deletion", uri, jwtDef, nil, false}, users.User{Username: sptr(ign.RandomString(8)), Email: &email}, true},
		{uriTest{"dup username - used by Org #1", uri, jwtDef, ign.NewErrorMessage(ign.ErrorResourceExists), false}, users.User{Username: &orgName, Name: &name, Email: &email, Organization: &org}, false},
		{uriTest{"dup username - used by Org, other JWT", uri, newJWT(jwt2), ign.NewErrorMessage(ign.ErrorResourceExists), false}, users.User{Username: &orgName, Name: &name, Email: &email, Organization: &org}, false},
		// end of inter-related test cases
	}

	for _, test := range userCreateTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithCreateUserTestData(test, t)
		})
	}
}

// runSubTestWithCreateUserTestData tries to create a user based on the given createUserTest struct.
// It is used as the body of a subtest.
func runSubTestWithCreateUserTestData(test createUserTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	u := test.user
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(u)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	bslice, _ := igntest.AssertRouteMultipleArgs("POST", test.URL, b, expStatus, jwt, expCt, t)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name()+" POST /users", bslice, expEm.ErrCode, t)
	}
	if test.deleteAfter {
		// Delete the user
		removeUserWithJWT(*u.Username, *jwt, t)
	}
}

// TestInvalidServerKeyCreateUser checks what happens when the server is configured with an invalid auth key.
func TestInvalidServerKeyCreateUser(t *testing.T) {
	setup()
	jwtDef := newJWT(os.Getenv("IGN_TEST_JWT"))

	// Now use an invalid Auth key in the server and see what happens
	cleanFn := setRandomAuth0PublicKey()
	defer cleanFn()

	test := createUserTest{
		uriTest{
			"should fail with unauthorized status",
			"/1.0/users",
			jwtDef,
			ign.NewErrorMessage(ign.ErrorUnauthorized),
			true,
		},
		users.User{Username: sptr("ShouldNotBeCreated"), Email: sptr("user@email.org")},
		false,
	}

	t.Run(test.testDesc, func(t *testing.T) {
		runSubTestWithCreateUserTestData(test, t)
	})
}

// setRandomAuth0PublicKey is a helper function that sets an invalid Auth key in the server.
// It returns a func that returns the server to its original key.
func setRandomAuth0PublicKey() func() {
	serverKey := globals.Server.Auth0RsaPublicKey()
	cleanFn := func() {
		globals.Server.SetAuth0RsaPublicKey(serverKey)
	}
	// Modified server key (just a little bit, to look real)
	globals.Server.SetAuth0RsaPublicKey("MIGfMA0GCSqGSIb4DQEBAQUAA4GNADCBiQKBgQDdlatRjRjogo3WojgGHFHYLugdUWAY9iR3fy4arWNA1KoS8kVw33cJibXr8bvwUAUparCwlvdbH6dvEOfou0/gCFQsHUfQrSDv+MuSUMAe8jzKE4qW+jK+xQU9a03GUnKHkkle+Q0pX/g6jXZ7r1/xAK5Do2kQ+X5xK9cipRgEKwIDAQAB")
	return cleanFn
}

// removeUserTest defines a DELETE /users/username test case.
type removeUserTest struct {
	uriTest
	// username to remove
	usernameToRemove string
}

// TestRemoveUser tests the DELETE /users/username route.
func TestRemoveUser(t *testing.T) {
	setup()

	myJWT := os.Getenv("IGN_TEST_JWT")
	// Create two random users using different JWTs
	username := createUser(t)
	defer removeUser(username, t)
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	uri := "/1.0/users/"

	removeUserTestsData := []removeUserTest{
		{uriTest{"try to delete from other jwt", uri + username2, newJWT(myJWT), ign.NewErrorMessage(ign.ErrorUnauthorized), false}, username2},
		{uriTest{"valid removal", uri + username2, newJWT(jwt2), nil, false}, username2},
	}

	for _, test := range removeUserTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			// Invoke DELETE user
			bslice, _ := igntest.AssertRouteMultipleArgs("DELETE", test.URL, nil, expStatus, jwt, expCt, t)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" DELETE "+test.URL, bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				dbu, _ := getUserFromDb(test.usernameToRemove, t)
				assert.Nil(t, dbu, "User was found in DB but should have been deleted: %s", test.usernameToRemove)
			}
		})
	}
}

// updateUserTest includes the input and expected output for a
// TestUserUpdate test case.
type updateUserTest struct {
	uriTest
	// The user to update
	username string
	// data to update
	uu *users.UpdateUserInput
}

// TestUserUpdate tests the PATCH /users route.
func TestUserUpdate(t *testing.T) {
	setup()
	// get the tests JWT
	jwtDef := newJWT(os.Getenv("IGN_TEST_JWT"))

	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)

	// create a separate user and remove it (ie. a non active user)
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	removeUserWithJWT(username2, jwt2, t)

	uri := "/1.0/users"

	name := "New Name"
	email := "test@email.org"
	userUpdateTestsData := []updateUserTest{
		{uriTest{"no jwt", uri, nil, ign.NewErrorMessage(ign.ErrorUnauthorized),
			true}, username, &users.UpdateUserInput{Name: &name, Email: &email}},
		{uriTest{"no fields", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue),
			false}, username, &users.UpdateUserInput{}},
		{uriTest{"no fields #2", uri, jwtDef, ign.NewErrorMessage(ign.ErrorUnmarshalJSON),
			false}, username, nil},
		{uriTest{"invalid email format", uri, jwtDef,
			ign.NewErrorMessage(ign.ErrorFormInvalidValue), true}, username,
			&users.UpdateUserInput{Name: &name, Email: sptr("inv")}},
		{uriTest{"invalid expFeatures", uri, jwtDef,
			ign.NewErrorMessage(ign.ErrorFormInvalidValue), true}, username,
			&users.UpdateUserInput{Name: &name, Email: &email, ExpFeatures: sptr("  inv")}},
		{uriTest{"with all fields", uri, jwtDef, nil, false}, username,
			&users.UpdateUserInput{Name: &name, Email: &email,
				ExpFeatures: sptr("  gzweb")}},
		{uriTest{"non active user", uri, newJWT(jwt2), ign.NewErrorMessage(ign.ErrorAuthNoUser),
			true}, username, &users.UpdateUserInput{Name: &name, Email: &email}},
	}

	for _, test := range userUpdateTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithUpdateUserTestData(test, t)
		})
	}
}

// runSubTestWithUpdateUserTestData tries to update an user based
// on the given update user test struct.
// It is used as the body of a subtest.
func runSubTestWithUpdateUserTestData(test updateUserTest, t *testing.T) {
	var b *bytes.Buffer
	if test.uu != nil {
		b = new(bytes.Buffer)
		json.NewEncoder(b).Encode(*test.uu)
	}

	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode

	reqArgs := igntest.RequestArgs{Method: "PATCH", Route: test.URL + "/" + test.username, Body: b, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	gotCode := resp.RespRecorder.Code
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name()+" PATCH /users/"+test.username, bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		require.Equal(t, http.StatusOK, gotCode, "Did not receive expected http code [%d]. Got: [%d]. Response: %s", http.StatusOK, gotCode, string(*bslice))
		var got users.UserResponse
		assert.NoError(t, json.Unmarshal(*bslice, &got), "Unable to unmarshal: %s", string(*bslice))
		assert.Equal(t, test.username, got.Username, "Got username [%s] different than expected one [%s]", got.Username, test.username)
		if test.uu.Name != nil {
			assert.Equal(t, *test.uu.Name, got.Name)
		}
		if test.uu.Email != nil {
			assert.Equal(t, *test.uu.Email, got.Email)
		}
		if test.uu.ExpFeatures != nil {
			assert.Equal(t, *test.uu.ExpFeatures, got.ExpFeatures)
		}
	}
}

// userIndexTest defines a GET /users/username test case.
type userIndexTest struct {
	uriTest
	// username to get
	username string
	// should the returned user data contain private data?
	privateData bool
}

// TestUserIndex tests the GET /users/{username} route.
func TestUserIndex(t *testing.T) {
	setup()
	myJWT := os.Getenv("IGN_TEST_JWT")
	// Create user
	username := createUser(t)
	defer removeUser(username, t)
	uri := "/1.0/users/"

	userIndexTestsData := []userIndexTest{
		{uriTest{"should get private data", uri, newJWT(myJWT), nil, false}, username, true},
		{uriTest{"no jwt. Should not get private data", uri, nil, nil, false}, username, false},
		{uriTest{"unexistent user", uri, nil, ign.NewErrorMessage(ign.ErrorUserUnknown), false}, "username2", false},
		{uriTest{"invalid jwt token", uri, newJWT("invalid"), ign.NewErrorMessage(ign.ErrorUnauthorized), true}, username, false},
	}

	for _, test := range userIndexTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			bslice, _ := igntest.AssertRouteMultipleArgs("GET", test.URL+test.username, nil, expStatus, jwt, expCt, t)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" GET /users/"+test.username, bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				var ur users.UserResponse
				assert.NoError(t, json.Unmarshal(*bslice, &ur), "Unable to unmarshal user response: %s", string(*bslice))
				// Check returned user is the expected one
				assert.Equal(t, username, ur.Username, "Got username [%s] different than expected one [%s]", ur.Username, username)
				if test.privateData {
					assert.NotEmpty(t, ur.Email, "UserResponse should contain private data: %s", ur)
				} else {
					assert.Empty(t, ur.Email, "UserResponse should NOT contain private data, but it does: %s", ur)
				}
			}
		})
	}
}

// ownerProfileTest defines a GET /profile/username test case.
type ownerProfileTest struct {
	uriTest
	// profile to get
	name string
	Type string
	// should the returned user data contain private data?
	privateData bool
}

// TestOwnerProfile tests the GET /profile/{username} route.
func TestOwnerProfile(t *testing.T) {
	setup()
	myJWT := os.Getenv("IGN_TEST_JWT")
	// create a random user allowed to create the org
	username := createUser(t)
	defer removeUser(username, t)
	// Create a random organization
	orgname := createOrganization(t)
	defer removeOrganization(orgname, t)
	// create a separate user and remove it (ie. a non active user)
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)
	uri := "/1.0/profile/"

	profileTestsData := []ownerProfileTest{
		{uriTest{"should get user private data", uri, newJWT(myJWT), nil, false},
			username, users.OwnerTypeUser, true},
		{uriTest{"no jwt. Should not get user private data", uri, nil, nil, false},
			username, users.OwnerTypeUser, false},
		{uriTest{"should get org data", uri, newJWT(myJWT), nil, false},
			orgname, users.OwnerTypeOrg, true},
		{uriTest{"unexistent owner", uri, nil, ign.NewErrorMessage(ign.ErrorUserUnknown),
			false}, "name2", "", false},
		{uriTest{"invalid jwt token", uri, newJWT("invalid"),
			ign.NewErrorMessage(ign.ErrorUnauthorized), true}, username, "", false},
		{uriTest{"should get org public data", uri, newJWT(jwt2), nil, false},
			orgname, users.OwnerTypeOrg, false},
		{uriTest{"should get user public data", uri, newJWT(jwt2), nil, false},
			username, users.OwnerTypeUser, false},
	}

	for _, test := range profileTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			bslice, _ := igntest.AssertRouteMultipleArgs("GET", test.URL+test.name, nil, expStatus, jwt, expCt, t)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" GET /profile/"+test.name, bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				var op users.OwnerProfile
				require.NoError(t, json.Unmarshal(*bslice, &op), "Unable to unmarshal response: %s", string(*bslice))
				if test.Type == users.OwnerTypeUser {
					assert.Nil(t, op.Org, "OwnerProfile Org should be nil")
					require.NotNil(t, op.User, "OwnerProfile User should not be nil")
					// Check returned user is the expected one
					assert.Equal(t, test.name, op.User.Username, "Names")
					if test.privateData {
						assert.NotEmpty(t, op.User.Email, "user email should not be empty")
					} else {
						assert.Empty(t, op.User.Email, "user email should be empty")
					}
				} else {
					// assume it is an org
					assert.Nil(t, op.User, "OwnerProfile User should be nil")
					require.NotNil(t, op.Org, "OwnerProfile Org should not be nil")
					// Check returned org is the expected one
					assert.Equal(t, test.name, op.Org.Name, "Names")
					assert.Equal(t, test.privateData, op.Org.Private, "Private field")
				}
			}
		})
	}
}

// TestAPIUser checks the route that describes the user API
func TestAPIUser(t *testing.T) {

	// General test setup
	setup()

	// Create a user
	testUser := createUser(t)
	defer removeUser(testUser, t)

	code := http.StatusOK
	if globals.Server.Db == nil {
		code = ign.ErrorMessage(ign.ErrorNoDatabase).StatusCode
	}

	uri := "/1.0/users/" + testUser
	igntest.AssertRoute("OPTIONS", uri, code, t)
}

type expResUser struct {
	username string
	orgs     []string
	// should include private info (eg. mail)?
	hasEmail bool
	orgRoles map[string]string
}

// userListTest defines a GET users list test case.
type userListTest struct {
	uriTest
	// the pagination query to append as suffix to the GET /users
	paginationQuery string
	// expected users
	expUsers []expResUser
}

// TestUserPagination tests the GET /users route.
func TestUserPagination(t *testing.T) {
	// General test setup
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")
	jwtDef := newJWT(jwt)
	// Create some harcoded users
	user1 := createUser(t)
	defer removeUser(user1, t)
	// Also create a random organization to check returned info.
	orgName := createOrganization(t)
	defer removeOrganization(orgName, t)
	// Note: need to use another JWT for new users
	jwt2 := createValidJWTForIdentity("another-user", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)
	// Create another user and make him member
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	addUserToOrg(user3, "member", orgName, t)

	invpage := ign.NewErrorMessage(ign.ErrorInvalidPaginationRequest)
	uri := "/1.0/users"
	// map[string]string{testOrg:"owner",testOrg2:"owner"}
	userListTestsData := []userListTest{
		{uriTest{"no jwt - get all users with only public info", uri, nil, nil, false},
			"", []expResUser{
				{user1, []string{}, false, nil},
				{user2, []string{}, false, nil},
				{user3, []string{}, false, nil},
			}},
		{uriTest{"no jwt - get pages of 1, page 1", uri, nil, nil, false}, "?per_page=1",
			[]expResUser{
				{user1, []string{}, false, nil},
			},
		},
		{uriTest{"no jwt - get pages of 1, page 2", uri, nil, nil, false}, "?per_page=1&page=2",
			[]expResUser{
				{user2, []string{}, false, nil},
			},
		},
		{uriTest{"org owner - get all users", uri, jwtDef, nil, false}, "", []expResUser{
			{user1, []string{orgName}, true, map[string]string{orgName: "owner"}},
			{user2, []string{}, false, nil},
			{user3, []string{orgName}, true, map[string]string{orgName: "member"}},
		}},
		{uriTest{"org member - get all users", uri, newJWT(jwt3), nil, false}, "", []expResUser{
			{user1, []string{orgName}, false, nil},
			{user2, []string{}, false, nil},
			{user3, []string{orgName}, true, map[string]string{orgName: "member"}},
		}},
		{uriTest{"get page beyond limit", uri, nil,
			ign.NewErrorMessage(ign.ErrorPaginationPageNotFound), false}, "?page=3", nil,
		},
		{uriTest{"get invalid page", uri, nil, invpage, false}, "?page=invalid", nil},
		{uriTest{"get invalid page #2", uri, nil, invpage, false}, "?page=-5", nil},
		{uriTest{"get invalid page #3", uri, nil, invpage, false}, "?page=1.2", nil},
	}

	for _, test := range userListTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			reqArgs := igntest.RequestArgs{Method: "GET", Route: test.URL + test.paginationQuery, Body: nil, SignedToken: jwt}
			resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			require.Equal(t, expStatus, resp.RespRecorder.Code)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" GET /users", bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				var users users.UserResponses
				assert.NoError(t, json.Unmarshal(*bslice, &users), "Unable to unmarshal list of users: %s", string(*bslice))
				// compare got users vs expected users
				require.Len(t, users, len(test.expUsers), "Got list does not have the expected count. Got: %d. Exp: %d", len(users), len(test.expUsers))
				for i, eu := range test.expUsers {
					assert.Equal(t, eu.username, users[i].Username,
						"Got Username [%s] at index [%d] is different than expected Username [%s]",
						users[i].Username, i, eu.username)
					assert.ElementsMatch(t, eu.orgs, users[i].Organizations,
						"Expected organization list is different at index [%d]", i)
					if eu.hasEmail {
						assert.NotEmpty(t, users[i].Email)
					}
					assert.Equal(t, eu.orgRoles, users[i].OrgRoles,
						"Expected (organization, role) MAP is different at index [%d]", i)
				}
			}
		})
	}
}

// TestPersonalAccessToken tests the /users/{username}/access-tokens and
// /users/{username}/access-tokens/revoke route.
func TestPersonalAccessToken(t *testing.T) {
	setup()

	myJWT := os.Getenv("IGN_TEST_JWT")

	// Create a random user
	username := createUser(t)
	defer removeUser(username, t)

	// Create another random user
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)

	type AccessTokenCreateInfo struct {
		Name string `json:"name"`
	}

	// Create a new personal access token
	accessTokenCreateInfo := AccessTokenCreateInfo{
		Name: "myName",
	}
	body := new(bytes.Buffer)
	json.NewEncoder(body).Encode(accessTokenCreateInfo)

	// A non-existant user should return an error.
	igntest.AssertRouteMultipleArgs("POST", "/1.0/users/BAD/access-tokens", body,
		400, &myJWT, "text/plain; charset=utf-8", t)

	// The username in the route should match the jwt username.
	igntest.AssertRouteMultipleArgs("POST", "/1.0/users/"+username2+"/access-tokens", body,
		401, &myJWT, "text/plain; charset=utf-8", t)

	response, _ := igntest.AssertRouteMultipleArgs("POST", "/1.0/users/"+username+"/access-tokens", body,
		200, &myJWT, "application/json", t)

	// Unmarshal the response, and check the name
	var newToken ign.AccessToken
	assert.NoError(t, json.Unmarshal(*response, &newToken), "Unable to unmarshal response.")
	assert.Equal(t, "myName", newToken.Name, "The new access token has an invalid name.")

	// A non-existant user should return an error.
	igntest.AssertRouteMultipleArgs("GET", "/1.0/users/BAD/access-tokens", nil,
		400, &myJWT, "text/plain; charset=utf-8", t)

	// The username in the route should match the jwt username.
	igntest.AssertRouteMultipleArgs("GET", "/1.0/users/"+username2+"/access-tokens", nil,
		401, &myJWT, "text/plain; charset=utf-8", t)

	// Get the list of access tokens
	response, _ = igntest.AssertRouteMultipleArgs("GET", "/1.0/users/"+username+"/access-tokens", nil,
		200, &myJWT, "application/json", t)
	var tokens ign.AccessTokens
	assert.NoError(t, json.Unmarshal(*response, &tokens), "Unable to unmarshal access token list.")
	assert.Equal(t, 1, len(tokens), "The number of access tokens was not equal to one.")
	assert.Empty(t, tokens[0].Key, "The key field should have been empty.")

	// Revoke the token
	body = new(bytes.Buffer)
	json.NewEncoder(body).Encode(newToken)

	// A non-existant user should return an error.
	igntest.AssertRouteMultipleArgs("POST", "/1.0/users/BAD/access-tokens/revoke", body,
		400, &myJWT, "text/plain; charset=utf-8", t)

	// The username in the route should match the jwt username.
	igntest.AssertRouteMultipleArgs("POST", "/1.0/users/"+username2+"/access-tokens/revoke", body,
		401, &myJWT, "text/plain; charset=utf-8", t)

	igntest.AssertRouteMultipleArgs("POST", "/1.0/users/"+username+"/access-tokens/revoke", body,
		200, &myJWT, "application/json", t)

	// Get the list of tokens, and make sure that the length is zero.
	response, _ = igntest.AssertRouteMultipleArgs("GET", "/1.0/users/"+username+"/access-tokens", nil,
		200, &myJWT, "application/json", t)
	json.Unmarshal(*response, &tokens)
	assert.Equal(t, 0, len(tokens), "There should be no token after the revoke.")

	// now try to remove the 2nd user
	removeUserWithJWT(username2, jwt2, t)
}
