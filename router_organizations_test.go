package main

import (
	"bitbucket.org/ignitionrobotics/ign-fuelserver/bundles/users"
	"bitbucket.org/ignitionrobotics/ign-fuelserver/globals"
	"bitbucket.org/ignitionrobotics/ign-go"
	"bitbucket.org/ignitionrobotics/ign-go/testhelpers"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"os"
	"testing"
)

// Tests for organization related routes

// createOrganizationTest includes the input and expected output for a TestOrganizationCreate test case.
type createOrganizationTest struct {
	uriTest

	// organization data
	organization users.CreateOrganization

	// should also delete the created organization as part of this test case?
	deleteAfter bool
}

// TestOrganizationCreate tests the POST /organizations route. It also optionally Deletes the organization on each test
func TestOrganizationCreate(t *testing.T) {
	setup()
	// get the tests JWT
	jwtDef := newJWT(os.Getenv("IGN_TEST_JWT"))
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)
	// create a separate JWT but do not create user using it.
	jwt2 := createValidJWTForIdentity("another-user", t)

	name := "MyOrganization"
	email := "test@email.org"
	description := "a friendly organization"
	uri := "/1.0/organizations"
	organizationCreateTestsData := []createOrganizationTest{
		{uriTest{"no user in backend", uri, newJWT(jwt2), ign.NewErrorMessage(ign.ErrorAuthNoUser), false}, users.CreateOrganization{Name: name, Description: description}, false},
		{uriTest{"no name", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, users.CreateOrganization{Description: description}, false},
		{uriTest{"no optional fields", uri, jwtDef, nil, false}, users.CreateOrganization{Name: ign.RandomString(8)}, true},
		{uriTest{"blacklisted name", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false},
			users.CreateOrganization{Name: "home", Description: description}, false},
		{uriTest{"with space underscore and dash", uri, jwtDef, nil, true},
			users.CreateOrganization{Name: "with- _space", Description: description},
			false},
		{uriTest{"short name", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false},
			users.CreateOrganization{Name: "b", Description: description}, false},
		// Note: the following test cases are inter-related, as the test for duplication.
		{uriTest{"with all fields", uri, jwtDef, nil, false},
			users.CreateOrganization{Name: name, Email: email, Description: description},
			false},
		{uriTest{"duplicate name", uri, jwtDef, ign.NewErrorMessage(ign.ErrorResourceExists), false}, users.CreateOrganization{Name: name, Description: description}, true},
		{uriTest{"duplicate name even after org removal", uri, jwtDef, ign.NewErrorMessage(ign.ErrorResourceExists), false}, users.CreateOrganization{Name: name, Description: description}, false},
		{uriTest{"duplicate name - used for username", uri, jwtDef, ign.NewErrorMessage(ign.ErrorResourceExists), false}, users.CreateOrganization{Name: username, Description: description}, false},
		// end of inter-related test cases
	}

	for _, test := range organizationCreateTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithCreateOrganizationTestData(test, t)
		})
	}
}

// runSubTestWithCreateOrganizationTestData tries to create an organization based on the given createOrganizationTest struct.
// It is used as the body of a subtest.
func runSubTestWithCreateOrganizationTestData(test createOrganizationTest, t *testing.T) {
	o := test.organization
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(o)

	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	reqArgs := igntest.RequestArgs{Method: "POST", Route: test.URL, Body: b, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	gotCode := resp.RespRecorder.Code
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name()+" POST /organizations", bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		require.Equal(t, http.StatusOK, gotCode, "Did not receive expected http code [%d]. Got: [%d]. Response: %s", http.StatusOK, gotCode, string(*bslice))
		var gotOrg users.OrganizationResponse
		assert.NoError(t, json.Unmarshal(*bslice, &gotOrg), "Unable to unmarshal: %s", string(*bslice))
		assert.Equal(t, test.organization.Name, gotOrg.Name)
		if test.organization.Email == "" {
			assert.Empty(t, gotOrg.Email, "Should be empty but got: %s", gotOrg.Email)
		} else {
			assert.Equal(t, test.organization.Email, gotOrg.Email)
		}
		if test.organization.Description == "" {
			assert.Empty(t, gotOrg.Description, "Should be empty but got: %s", gotOrg.Description)
		} else {
			assert.Equal(t, test.organization.Description, gotOrg.Description)
		}
	}
	if test.deleteAfter {
		// Delete the organization
		removeOrganization(o.Name, t)
	}
}

// removeOrganizationTest defines a DELETE /organizations/name test case.
type removeOrganizationTest struct {
	uriTest

	// name to remove
	nameToRemove string
}

// TestOrganizationRemove tests the DELETE /organizations/name route.
func TestOrganizationRemove(t *testing.T) {
	setup()

	// get the tests JWT
	jwtDef := newJWT(os.Getenv("IGN_TEST_JWT"))
	// create a random user using the default test JWT
	// The user needs to exist and be active in order to create or remove organizations.
	username := createUser(t)
	defer removeUser(username, t)
	// create a separate JWT but do not create user using it.
	jwtUn := createValidJWTForIdentity("another-user", t)

	// create a test org
	// no need to defer removeOrganization as it will be removed as part of test
	org := createOrganization(t)
	// Create users and add to org
	jwt2 := createValidJWTForIdentity("another-user-2", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)
	addUserToOrg(user2, "member", org, t)
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	addUserToOrg(user3, "admin", org, t)
	// create another user, non member
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)

	uri := "/1.0/organizations"
	unauth := ign.NewErrorMessage(ign.ErrorUnauthorized)

	removeOrganizationTestsData := []removeOrganizationTest{
		{uriTest{"no user in backend", uri, newJWT(jwtUn), ign.NewErrorMessage(ign.ErrorAuthNoUser), false}, org},
		{uriTest{"org cannot be removed by non member", uri, newJWT(jwt4), unauth, false}, org},
		{uriTest{"org cannot be removed by a member", uri, newJWT(jwt2), unauth, false}, org},
		{uriTest{"org cannot be removed by an admin", uri, newJWT(jwt3), unauth, false}, org},
		{uriTest{"valid removal", uri, jwtDef, nil, false}, org},
	}

	for _, test := range removeOrganizationTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			reqArgs := igntest.RequestArgs{Method: "DELETE", Route: test.URL + "/" + test.nameToRemove, Body: nil, SignedToken: jwt}
			resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			require.Equal(t, expStatus, resp.RespRecorder.Code)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" DELETE /organizations/"+test.nameToRemove, bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				dbo, _ := getOrganizationFromDb(test.nameToRemove, t)
				assert.Nil(t, dbo, "Organization was found in DB but should have been deleted:", test.nameToRemove)
			}
		})
	}
}

// organizationIndexTest defines a GET /organizations/name test case.
type organizationIndexTest struct {
	uriTest
	// name to get
	name string
	// should get private data?
	private bool
}

// TestOrganizationIndex tests the GET /organizations/{name} route.
func TestOrganizationIndex(t *testing.T) {
	setup()

	// get the tests JWT
	jwtDef := newJWT(os.Getenv("IGN_TEST_JWT"))
	// create a random user allowed to create the org
	username := createUser(t)
	defer removeUser(username, t)
	// Create a random organization
	org := createOrganization(t)
	defer removeOrganization(org, t)
	// create a separate user and remove it (ie. a non active user)
	jwtNO := createValidJWTForIdentity("another-user", t)
	usernameNO := createUserWithJWT(jwtNO, t)
	removeUserWithJWT(usernameNO, jwtNO, t)
	// Create other users and add to org
	jwt2 := createValidJWTForIdentity("another-user-2", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)
	addUserToOrg(user2, "member", org, t)
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	addUserToOrg(user3, "admin", org, t)
	// create another user, non member
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)

	uri := "/1.0/organizations"
	organizationIndexTestsData := []organizationIndexTest{
		{uriTest{"org owner should get private data", uri, jwtDef, nil, false}, org, true},
		{uriTest{"org admin should get private data", uri, newJWT(jwt3), nil, false}, org, true},
		{uriTest{"org member should get private data", uri, newJWT(jwt2), nil, false}, org, true},
		{uriTest{"non member should get public data", uri, newJWT(jwt4), nil, false}, org, false},
		{uriTest{"no jwt - should get public data", uri, nil, nil, false}, org, false},
		{uriTest{"non existent org", uri, jwtDef, ign.NewErrorMessage(ign.ErrorNonExistentResource), false}, "name2", false},
		{uriTest{"invalid jwt token", uri, &testJWT{jwt: sptr("invalid")}, ign.NewErrorMessage(ign.ErrorUnauthorized), true}, org, false},
		// This one should return Unauthorized, if the jwt is passed in and its associated user is not valid anymore.
		// TODO: we should add a middleware to check passed in JWTs vs DB users.
		{uriTest{"non active user - should fail", uri, newJWT(jwtNO), nil /*ign.NewErrorMessage(ign.ErrorUnauthorized)*/, true}, org, false},
	}

	for _, test := range organizationIndexTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			reqArgs := igntest.RequestArgs{Method: "GET", Route: test.URL + "/" + test.name, Body: nil, SignedToken: jwt}
			resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			require.Equal(t, expStatus, resp.RespRecorder.Code)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" GET /organizations/"+test.name, bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				var or users.OrganizationResponse
				assert.NoError(t, json.Unmarshal(*bslice, &or), "Unable to unmarshal organization response", string(*bslice))
				// Check returned organization is the expected one
				assert.Equal(t, test.name, or.Name, "Got name [%s] different than expected one [%s]", or.Name, test.name)
				assert.Equal(t, test.private, or.Private, "Private field")
			}
		})
	}
}

// TestAPIOrganization checks the route that describes the organization API
func TestAPIOrganization(t *testing.T) {

	// General test setup
	setup()

	code := http.StatusOK
	if globals.Server.Db == nil {
		code = ign.ErrorMessage(ign.ErrorNoDatabase).StatusCode
	}

	uri := "/1.0/organizations/anOrg"
	igntest.AssertRoute("OPTIONS", uri, code, t)
}

type expOrg struct {
	name string
	// should get private data?
	private bool
}

// organizationListTest defines a GET organizations list test case.
type organizationListTest struct {
	uriTest
	// the pagination query to append as suffix to the GET /organizations
	paginationQuery string
	// expected names to be returned
	expOrgs []expOrg
}

// TestOrganizationPagination tests the GET /organizations route.
func TestOrganizationPagination(t *testing.T) {
	// General test setup
	setup()
	jwtDef := newJWT(os.Getenv("IGN_TEST_JWT"))
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)
	// Create some harcoded organizations
	testOrg1 := createOrganization(t)
	defer removeOrganization(testOrg1, t)
	testOrg2 := createOrganization(t)
	defer removeOrganization(testOrg2, t)
	// Create users and add to org
	jwt2 := createValidJWTForIdentity("another-user-2", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)
	addUserToOrg(user2, "member", testOrg1, t)
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	addUserToOrg(user3, "admin", testOrg1, t)
	// create another user, non member
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)

	uri := "/1.0/organizations"
	invpage := ign.NewErrorMessage(ign.ErrorInvalidPaginationRequest)
	organizationListTestsData := []organizationListTest{
		{uriTest{"no jwt - get all organizations, get only public date", uri, nil,
			nil, false}, "", []expOrg{{testOrg1, false}, {testOrg2, false}}},
		{uriTest{"orgs owner - get all organizations, get private data", uri, jwtDef,
			nil, false}, "", []expOrg{{testOrg1, true}, {testOrg2, true}}},
		{uriTest{"admin - get all organizations, get private data for some", uri,
			newJWT(jwt3), nil, false}, "", []expOrg{
			{testOrg1, true}, {testOrg2, false},
		},
		},
		{uriTest{"member - get all organizations, get private data for some", uri,
			newJWT(jwt2), nil, false}, "", []expOrg{
			{testOrg1, true}, {testOrg2, false},
		},
		},
		{uriTest{"non member of any - get public data only ", uri,
			newJWT(jwt4), nil, false}, "", []expOrg{
			{testOrg1, false}, {testOrg2, false},
		},
		},
		{uriTest{"no jwt - get pages of 1, page 1", uri, nil, nil, false}, "?per_page=1",
			[]expOrg{{testOrg1, false}}},
		{uriTest{"no jwt - get pages of 1, page 2", uri, nil, nil, false},
			"?per_page=1&page=2", []expOrg{{testOrg2, false}}},
		{uriTest{"get page beyond limit", uri, nil,
			ign.NewErrorMessage(ign.ErrorPaginationPageNotFound), false}, "?page=3", nil},
		{uriTest{"get invalid page", uri, nil, invpage, false}, "?page=invalid", nil},
		{uriTest{"get invalid page #2", uri, nil, invpage, false}, "?page=-5", nil},
		{uriTest{"get invalid page #3", uri, nil, invpage, false}, "?page=1.2", nil},
	}

	for _, test := range organizationListTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			reqArgs := igntest.RequestArgs{Method: "GET", Route: test.URL + test.paginationQuery, Body: nil, SignedToken: jwt}
			resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			require.Equal(t, expStatus, resp.RespRecorder.Code)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" GET /organizations", bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				var organizations users.OrganizationResponses
				assert.NoError(t, json.Unmarshal(*bslice, &organizations), "Unable to unmarshal list of organizations", string(*bslice))
				// compare got names vs expected names
				assert.Len(t, organizations, len(test.expOrgs), "Got list does not have the expected count. Got: %d. Exp: %d", len(organizations), len(test.expOrgs))
				for i, o := range test.expOrgs {
					assert.Equal(t, o.name, organizations[i].Name, "Got Name [%s] at index [%d] is different than expected Name [%s]", organizations[i].Name, i, o.name)
					if o.private {
						assert.True(t, organizations[i].Private, "Org should have Private data. Index %d", i)
					}
				}
			}
		})
	}
}

// updateOrganizationTest includes the input and expected output for a
// TestOrganizationUpdate test case.
type updateOrganizationTest struct {
	uriTest
	// organization name
	name string
	// new organization description
	description *string
	// new organization email
	email *string
}

// TestOrganizationUpdate tests the PATCH /organizations route.
func TestOrganizationUpdate(t *testing.T) {
	setup()
	// get the tests JWT
	jwtDef := newJWT(os.Getenv("IGN_TEST_JWT"))

	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)
	// Create an organization.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)

	// Create users and add to org
	jwt2 := createValidJWTForIdentity("another-user-2", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)
	addUserToOrg(user2, "member", testOrg, t)
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	addUserToOrg(user3, "admin", testOrg, t)
	// create another user, non member
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)

	// create a separate user and remove it (ie. a non active user)
	jwtNO := createValidJWTForIdentity("another-user", t)
	usernameNO := createUserWithJWT(jwtNO, t)
	removeUserWithJWT(usernameNO, jwtNO, t)

	uri := "/1.0/organizations"
	unauth := ign.NewErrorMessage(ign.ErrorUnauthorized)

	description := "updated organization description"
	email := "test@email.org"
	organizationUpdateTestsData := []updateOrganizationTest{
		{uriTest{"no jwt", uri, nil, unauth, true}, testOrg, &description, nil},
		{uriTest{"no fields", uri, jwtDef,
			ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, testOrg, nil, nil},
		{uriTest{"invalid email format", uri, jwtDef,
			ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, testOrg, nil, sptr("inv")},
		{uriTest{"only email", uri, jwtDef, nil, false}, testOrg, nil, &email},
		{uriTest{"with all fields", uri, jwtDef, nil, false}, testOrg, &description,
			&email},
		{uriTest{"non active user", uri, newJWT(jwtNO),
			ign.NewErrorMessage(ign.ErrorAuthNoUser), true}, testOrg, &description,
			&email},
		{uriTest{"org cannot be updated by non member", uri, newJWT(jwt4), unauth,
			false}, testOrg, &description, nil},
		{uriTest{"org member cannot update org", uri, newJWT(jwt2), unauth, false},
			testOrg, &description, nil},
		{uriTest{"org admin can update org", uri, newJWT(jwt3), nil, false}, testOrg,
			&description, nil},
	}

	for _, test := range organizationUpdateTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithUpdateOrganizationTestData(test, t)
		})
	}
}

// runSubTestWithUpdateOrganizationTestData tries to update an organization based
// on the given createOrganizationTest struct.
// It is used as the body of a subtest.
func runSubTestWithUpdateOrganizationTestData(test updateOrganizationTest, t *testing.T) {
	var o users.UpdateOrganization
	o.Description = test.description
	o.Email = test.email
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(o)

	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode

	reqArgs := igntest.RequestArgs{Method: "PATCH", Route: test.URL + "/" + test.name, Body: b, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	gotCode := resp.RespRecorder.Code
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name()+" PATCH /organizations/"+test.name, bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		require.Equal(t, http.StatusOK, gotCode, "Did not receive expected http code [%d]. Got: [%d]. Response: %s", http.StatusOK, gotCode, string(*bslice))
		var gotOrg users.OrganizationResponse
		assert.NoError(t, json.Unmarshal(*bslice, &gotOrg), "Unable to unmarshal: %s", string(*bslice))
		assert.Equal(t, test.name, gotOrg.Name, "Got name [%s] different than expected one [%s]", gotOrg.Name, test.name)
		if test.description != nil {
			assert.Equal(t, *test.description, gotOrg.Description)
		}
		if test.email != nil {
			assert.Equal(t, *test.email, gotOrg.Email, "Got email [%s] is different than expected one [%s]", gotOrg.Email, *test.email)
		}
	}
}

type userAddTest struct {
	uriTest
	username string
	role     string
}

func orgUsersRoute(org string) string {
	return fmt.Sprintf("/1.0/organizations/%s/users", org)
}

// TestOrganizationUserAdd tests adding users to an organization
func TestOrganizationUserAdd(t *testing.T) {
	setup()
	// get the tests JWT
	jwtDef := newJWT(os.Getenv("IGN_TEST_JWT"))
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)
	// create a separate user using a different jwt
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)
	// create a separate user using a different jwt
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	username3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(username3, jwt3, t)

	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	// Create users and add to org
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)
	addUserToOrg(user4, "member", testOrg, t)
	jwt5 := createValidJWTForIdentity("another-user-5", t)
	user5 := createUserWithJWT(jwt5, t)
	defer removeUserWithJWT(user5, jwt5, t)
	addUserToOrg(user5, "admin", testOrg, t)

	// create a separate user using a different jwt
	jwt6 := createValidJWTForIdentity("another-user-6", t)
	user6 := createUserWithJWT(jwt6, t)
	defer removeUserWithJWT(user6, jwt6, t)

	unauth := ign.NewErrorMessage(ign.ErrorUnauthorized)
	uri := orgUsersRoute(testOrg)
	userAddTestsData := []userAddTest{
		{uriTest{"no jwt", uri, nil, unauth, true}, username2, "member"},
		{uriTest{"org doest not exist", orgUsersRoute("inv"), jwtDef, ign.NewErrorMessage(ign.ErrorNonExistentResource), false}, username2, "member"},
		{uriTest{"invalid jwt token", uri, &testJWT{jwt: sptr("invalid")}, unauth, true}, username2, "member"},
		{uriTest{"invalid username format", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, "inv user", "member"},
		{uriTest{"user to add does not exist", uri, jwtDef, ign.NewErrorMessage(ign.ErrorUserUnknown), false}, "nope", "admin"},
		{uriTest{"invalid role format ", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, username2, "memb er"},
		{uriTest{"invalid role format #2", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, username2, "admin1"},
		{uriTest{"user already in org", uri, jwtDef, ign.NewErrorMessage(ign.ErrorResourceExists), false}, username, "admin"},
		{uriTest{"outside user should not be able to add", uri, newJWT(jwt2), unauth, false}, username2, "member"},
		{uriTest{"org owner - success adding user2 as member", uri, jwtDef, nil, false}, username2, "member"},
		{uriTest{"org member not authorized to add new user", uri, newJWT(jwt4), unauth, false}, username3, "member"},
		{uriTest{"org admin should be able to add new user", uri, newJWT(jwt5), nil, false}, username3, "admin"},
		{uriTest{"org admin should not be able to add new owner", uri, newJWT(jwt5), unauth, false}, user6, "owner"},
		{uriTest{"org owner should be able to add new owner", uri, jwtDef, nil, false}, user6, "owner"},
	}

	for _, test := range userAddTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			var add users.AddUserToOrgInput
			add.Username = test.username
			add.Role = test.role
			b := new(bytes.Buffer)
			json.NewEncoder(b).Encode(add)

			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			reqArgs := igntest.RequestArgs{Method: "POST", Route: test.URL, Body: b, SignedToken: jwt}
			resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			gotCode := resp.RespRecorder.Code
			require.Equal(t, expStatus, gotCode)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" POST "+test.URL, bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				require.Equal(t, http.StatusOK, gotCode, "Did not receive expected http code [%d]. Got: [%d]. Response: %s", http.StatusOK, gotCode, string(*bslice))
				var got users.UserResponse
				assert.NoError(t, json.Unmarshal(*bslice, &got), "Unable to unmarshal: %s", string(*bslice))
				assert.Equal(t, test.username, got.Username, "Got username [%s] different than expected one [%s]", got.Username, test.username)
			}
		})
	}
}

type userRemoveTest struct {
	uriTest
	expUsername string
}

func rmOrgUsersRoute(org, user string) string {
	return fmt.Sprintf("/1.0/organizations/%s/users/%s", org, user)
}

// TestOrganizationUserRemove tests removing users from an organization
func TestOrganizationUserRemove(t *testing.T) {
	// This test will create an org with user1 as owner and user2 as a member
	setup()
	// get the tests JWT
	jwt := os.Getenv("IGN_TEST_JWT")
	jwtDef := newJWT(jwt)
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)
	// create a separate user using a different jwt
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)

	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	addUserToOrg(username2, "member", testOrg, t)

	// create a separate user using a different jwt
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	username3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(username3, jwt3, t)

	// create a separate user using a different jwt
	// this user will remove it self from the org
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	username4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(username4, jwt4, t)
	addUserToOrg(username4, "member", testOrg, t)

	// create a separate user using a different jwt
	jwt5 := createValidJWTForIdentity("another-user-5", t)
	username5 := createUserWithJWT(jwt5, t)
	defer removeUserWithJWT(username5, jwt5, t)
	addUserToOrg(username5, "admin", testOrg, t)
	// create a separate user using a different jwt
	jwt6 := createValidJWTForIdentity("another-user-6", t)
	username6 := createUserWithJWT(jwt6, t)
	defer removeUserWithJWT(username6, jwt6, t)
	addUserToOrg(username6, "member", testOrg, t)
	// add another owner to the org
	jwt7 := createValidJWTForIdentity("another-user-7", t)
	user7 := createUserWithJWT(jwt7, t)
	defer removeUserWithJWT(user7, jwt7, t)
	addUserToOrg(user7, "owner", testOrg, t)

	unauth := ign.NewErrorMessage(ign.ErrorUnauthorized)
	uri := rmOrgUsersRoute(testOrg, username2)
	userRemoveTestData := []userRemoveTest{
		{uriTest{"no jwt", uri, nil, unauth, true}, ""},
		{uriTest{"org doest not exist", rmOrgUsersRoute("inv", username2), jwtDef,
			ign.NewErrorMessage(ign.ErrorNonExistentResource), false}, ""},
		{uriTest{"invalid jwt token", uri, &testJWT{jwt: sptr("invalid")}, unauth,
			true}, ""},
		{uriTest{"user does not exist", rmOrgUsersRoute(testOrg, "inv"),
			jwtDef, ign.NewErrorMessage(ign.ErrorUserUnknown), false}, ""},
		{uriTest{"user not in org", rmOrgUsersRoute(testOrg, username3), jwtDef,
			ign.NewErrorMessage(ign.ErrorNameNotFound), false}, ""},
		{uriTest{"outside user cannot remove users", uri, newJWT(jwt3),
			unauth, false}, ""},
		{uriTest{"org member cannot remove users", uri, newJWT(jwt4),
			unauth, false}, ""},
		{uriTest{"org member is able to remove himself", rmOrgUsersRoute(testOrg, username4),
			newJWT(jwt4), nil, false}, username4},
		{uriTest{"org owner - success removing user2", uri, jwtDef, nil, false}, username2},
		{uriTest{"should not be able to remove user2 again", uri, jwtDef,
			ign.NewErrorMessage(ign.ErrorNameNotFound), false}, ""},
		{uriTest{"org admin cannot remove an owner", rmOrgUsersRoute(testOrg, user7),
			newJWT(jwt5), unauth, false}, user7},
		{uriTest{"owner should be able to remove an owner", rmOrgUsersRoute(testOrg, user7),
			jwtDef, nil, false}, user7},
		{uriTest{"should not be able to remove last owner", rmOrgUsersRoute(testOrg, username),
			jwtDef, ign.NewErrorMessage(ign.ErrorUnexpected), true}, ""},
		{uriTest{"org admin can remove other members", rmOrgUsersRoute(testOrg, username6),
			newJWT(jwt5), nil, true}, username6},
	}

	for _, test := range userRemoveTestData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			reqArgs := igntest.RequestArgs{Method: "DELETE", Route: test.URL, Body: nil,
				SignedToken: jwt}
			resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			gotCode := resp.RespRecorder.Code
			require.Equal(t, expStatus, gotCode)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" DELETE "+test.URL, bslice,
					expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				require.Equal(t, http.StatusOK, gotCode, "Did not receive expected http code [%d]. Got: [%d]. Response: %s", http.StatusOK, gotCode, string(*bslice))
				var got users.UserResponse
				assert.NoError(t, json.Unmarshal(*bslice, &got), "Unable to unmarshal: %s", string(*bslice))
				assert.Equal(t, test.expUsername, got.Username, "Got username [%s] different than expected one [%s]", got.Username, test.expUsername)
			}
		})
	}
}

type expUser struct {
	username string
	orgs     []string
	orgRoles map[string]string
}

type orgUserListTest struct {
	uriTest
	expUsers []expUser
}

func TestOrganizationUserList(t *testing.T) {
	// This test will create an org with user1 as owner and user2 as a member
	setup()
	// get the tests JWT
	jwt := os.Getenv("IGN_TEST_JWT")
	jwtDef := newJWT(jwt)
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)
	// create a separate user using a different jwt
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)
	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	// Create an organization with the default jwt as owner.
	testOrg2 := createOrganization(t)
	// create a separate user using a different jwt
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	username3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(username3, jwt3, t)

	// add username2 as a member of the org
	addUserToOrg(username2, "member", testOrg, t)
	// add username3 as a member of the org2 and admin of org1
	addUserToOrg(username3, "member", testOrg2, t)
	addUserToOrg(username3, "admin", testOrg, t)

	uriOrg1 := orgUsersRoute(testOrg)
	uriOrg2 := orgUsersRoute(testOrg2)

	orgUserListTestData := []orgUserListTest{
		{uriTest{"with no jwt cannot see private data for org1", uriOrg1, nil, nil, true},
			[]expUser{
				{username, []string{}, nil},
				{username2, []string{}, nil},
				{username3, []string{}, nil},
			},
		},
		{uriTest{"org owner can see user roles for org1", uriOrg1, jwtDef, nil, true},
			[]expUser{
				{username, []string{testOrg, testOrg2}, map[string]string{testOrg: "owner", testOrg2: "owner"}},
				{username2, []string{testOrg}, map[string]string{testOrg: "member"}},
				{username3, []string{testOrg, testOrg2}, map[string]string{testOrg: "admin", testOrg2: "member"}},
			},
		},
		{uriTest{"org member jwt2 cannot see other members' roles for org1 (but can see own)", uriOrg1, newJWT(jwt2), nil, true},
			[]expUser{
				{username, []string{testOrg, testOrg2}, nil},
				{username2, []string{testOrg}, map[string]string{testOrg: "member"}},
				{username3, []string{testOrg, testOrg2}, nil},
			},
		},
		{uriTest{"with no jwt cannot see private data for org2", uriOrg2, nil, nil, true},
			[]expUser{
				{username, []string{}, nil},
				{username3, []string{}, nil},
			},
		},
		{uriTest{"org owner can see user roles for org2", uriOrg2, jwtDef, nil, true},
			[]expUser{
				{username, []string{testOrg, testOrg2}, map[string]string{testOrg: "owner", testOrg2: "owner"}},
				{username3, []string{testOrg, testOrg2}, map[string]string{testOrg: "admin", testOrg2: "member"}},
			},
		},
		{uriTest{"non member jwt2 can only see user's public data for org2", uriOrg2, newJWT(jwt2), nil, true},
			[]expUser{
				{username, []string{testOrg, testOrg2}, nil},
				{username3, []string{testOrg, testOrg2}, nil},
			},
		},
	}

	for _, test := range orgUserListTestData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithOrgUserListTestData(test, t)
		})
	}

	// Remove org2, and run some tests again
	removeOrganization(testOrg2, t)

	orgUserListTestData = []orgUserListTest{
		{uriTest{"after removing org2 - with jwt for org2", uriOrg2, jwtDef,
			ign.NewErrorMessage(ign.ErrorNonExistentResource), false}, nil},
		{uriTest{"after removing org2 - with owner jwt for org1", uriOrg1, jwtDef, nil, true},
			[]expUser{
				{username, []string{testOrg}, map[string]string{testOrg: "owner"}},
				{username2, []string{testOrg}, map[string]string{testOrg: "member"}},
				{username3, []string{testOrg}, map[string]string{testOrg: "admin"}},
			},
		},
		{uriTest{"after removing org2 - org member jwt2 cannot see other members' roles for org1 (but can see own)",
			uriOrg1, newJWT(jwt2), nil, true},
			[]expUser{
				{username, []string{testOrg}, nil},
				{username2, []string{testOrg}, map[string]string{testOrg: "member"}},
				{username3, []string{testOrg}, nil},
			},
		},
	}

	for _, test := range orgUserListTestData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithOrgUserListTestData(test, t)
		})
	}
}

// runSubTestWithOrgUserListTestData tries to get the list of users of an
// organization
// It is used as the body of a subtest.
func runSubTestWithOrgUserListTestData(test orgUserListTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	bslice, _ := igntest.AssertRouteMultipleArgs("GET", test.URL, nil, expStatus,
		jwt, expCt, t)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name()+" GET "+test.URL, bslice,
			expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var ur users.UserResponses
		assert.NoError(t, json.Unmarshal(*bslice, &ur),
			"Unable to unmarshal list ofusers: %s", string(*bslice))
		require.Len(t, ur, len(test.expUsers),
			"Got list does not have the expected count. Got: %d. Exp: %d", len(ur),
			len(test.expUsers))
		for i, eu := range test.expUsers {
			assert.Equal(t, eu.username, ur[i].Username,
				"Got Username [%s] at index [%d] is different than expected Username [%s]",
				ur[i].Username, i, eu.username)
			assert.ElementsMatch(t, eu.orgs, ur[i].Organizations,
				"Expected organization list is different at index [%d]", i)
			assert.Equal(t, eu.orgRoles, ur[i].OrgRoles,
				"Expected (organization, role) MAP is different at index [%d]", i)
		}
	}
}

type createTeamTest struct {
	uriTest
	orgName         string
	teamInput       users.CreateTeamForm
	expTeamResponse users.TeamResponse
}

func orgTeamsRoute(org string) string {
	return fmt.Sprintf("/1.0/organizations/%s/teams", org)
}

// TestOrganizationTeamCreate tests adding teams to an organization
func TestOrganizationTeamCreate(t *testing.T) {
	setup()
	// get the tests JWT
	jwtDef := newJWT(os.Getenv("IGN_TEST_JWT"))
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)
	// create a separate user using a different jwt
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)
	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	// Create users and add to org
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	addUserToOrg(user3, "member", testOrg, t)
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)
	addUserToOrg(user4, "admin", testOrg, t)
	// create another user, non member
	jwt5 := createValidJWTForIdentity("another-user-5", t)
	user5 := createUserWithJWT(jwt5, t)
	defer removeUserWithJWT(user5, jwt5, t)

	b := true
	t1Input := users.CreateTeamForm{Name: "team1", Description: sptr("a desc"), Visible: &b}
	t2Input := users.CreateTeamForm{Name: "team2", Visible: new(bool)}
	t1Response := users.TeamResponse{Name: t1Input.Name, Description: t1Input.Description,
		Visible: *t1Input.Visible, Usernames: nil}
	t2Response := users.TeamResponse{Name: t2Input.Name, Visible: false, Usernames: nil}

	uri := orgTeamsRoute(testOrg)

	teamCreateTestsData := []createTeamTest{
		{uriTest{"no jwt", uri, nil, ign.NewErrorMessage(ign.ErrorUnauthorized), true}, testOrg, t1Input, t1Response},
		{uriTest{"not authorized", uri, newJWT(jwt2), ign.NewErrorMessage(ign.ErrorUnauthorized), true}, testOrg, t1Input, t1Response},
		{uriTest{"invalid jwt token", uri, &testJWT{jwt: sptr("invalid")}, ign.NewErrorMessage(ign.ErrorUnauthorized), true}, testOrg, t1Input, t1Response},
		{uriTest{"org doest not exist", orgTeamsRoute("inv"), jwtDef, ign.NewErrorMessage(ign.ErrorNonExistentResource), false}, testOrg, t1Input, t1Response},
		{uriTest{"missing team name input", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, testOrg,
			users.CreateTeamForm{Name: "", Description: sptr("a desc"), Visible: new(bool)}, t1Response},
		{uriTest{"missing visible arg", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, testOrg,
			users.CreateTeamForm{Name: "aa", Description: nil, Visible: nil}, t1Response},
		{uriTest{"invalid team name #1", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, testOrg,
			users.CreateTeamForm{Name: "admin", Description: sptr("a desc"), Visible: &b}, t1Response},
		{uriTest{"invalid team name #2", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, testOrg,
			users.CreateTeamForm{Name: "member", Description: sptr("a desc"), Visible: &b}, t1Response},
		{uriTest{"invalid team name #3", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, testOrg,
			users.CreateTeamForm{Name: "owner", Description: sptr("a desc"), Visible: &b}, t1Response},
		{uriTest{"org owner - success adding team", uri, jwtDef, nil, false}, testOrg, t1Input, t1Response},
		{uriTest{"dup team", uri, jwtDef, ign.NewErrorMessage(ign.ErrorResourceExists), false}, testOrg, t1Input, t1Response},
		{uriTest{"org owner - success adding team #2", uri, jwtDef, nil, true}, testOrg, t2Input, t2Response},
		{uriTest{"org admin - success adding team", uri, newJWT(jwt4), nil, false},
			testOrg, users.CreateTeamForm{Name: "team3", Visible: new(bool)},
			users.TeamResponse{Name: "team3", Visible: false, Usernames: nil},
		},
		{uriTest{"org member - should not be able to add team", uri, newJWT(jwt3),
			ign.NewErrorMessage(ign.ErrorUnauthorized), false},
			testOrg, users.CreateTeamForm{Name: "team4", Visible: new(bool)}, users.TeamResponse{},
		},
		{uriTest{"non member - should not be able to add team", uri, newJWT(jwt5),
			ign.NewErrorMessage(ign.ErrorUnauthorized), false},
			testOrg, users.CreateTeamForm{Name: "team4", Visible: new(bool)}, users.TeamResponse{},
		},
	}

	for _, test := range teamCreateTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			b := new(bytes.Buffer)
			json.NewEncoder(b).Encode(test.teamInput)
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			reqArgs := igntest.RequestArgs{Method: "POST", Route: test.URL, Body: b, SignedToken: jwt}
			resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			gotCode := resp.RespRecorder.Code
			require.Equal(t, expStatus, gotCode)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" POST "+test.URL, bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				var tr users.TeamResponse
				require.NoError(t, json.Unmarshal(*bslice, &tr), "Unable to unmarshal response: %s", string(*bslice))
				assert.Equal(t, test.expTeamResponse, tr)
			}
		})
	}
}

type removeTeamTest struct {
	uriTest
	team     string
	expTeams users.TeamResponses
}

// TestOrganizationTeamRemove tests removing teams from an organization
func TestOrganizationTeamRemove(t *testing.T) {
	setup()
	// get the tests JWT
	jwt := os.Getenv("IGN_TEST_JWT")
	jwtDef := newJWT(jwt)
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)
	// create a separate user using a different jwt
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)
	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	testOrg2 := createOrganization(t)
	defer removeOrganization(testOrg2, t)
	// Add username2 to org1 as a member
	addUserToOrg(username2, "member", testOrg, t)
	// Create users and add to org
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	addUserToOrg(user3, "admin", testOrg, t)
	// create another user, non member
	jwt5 := createValidJWTForIdentity("another-user-5", t)
	user5 := createUserWithJWT(jwt5, t)
	defer removeUserWithJWT(user5, jwt5, t)

	// Add some teams to Org1
	b := true
	addTeamToOrg(testOrg, jwt, users.CreateTeamForm{Name: "team1", Visible: new(bool)}, t)
	addTeamToOrg(testOrg, jwt, users.CreateTeamForm{Name: "team2", Visible: &b}, t)
	addTeamToOrg(testOrg, jwt, users.CreateTeamForm{Name: "team3", Description: sptr("a"), Visible: new(bool)}, t)
	updateOrgTeam(testOrg, "team3", jwt, users.UpdateTeamForm{NewUsers: []string{username2}}, t)

	// Add teams to Org2
	addTeamToOrg(testOrg2, jwt, users.CreateTeamForm{Name: "team4", Description: sptr("test"), Visible: &b}, t)

	// Expected TeamResponses
	t1Resp := users.TeamResponse{Name: "team1", Description: nil, Visible: false, Usernames: nil}
	t2Resp := users.TeamResponse{Name: "team2", Description: nil, Visible: true, Usernames: nil}
	t3Resp := users.TeamResponse{Name: "team3", Description: sptr("a"), Visible: false, Usernames: []string{username2}}

	uri := orgTeamsRoute(testOrg)
	removeTeamTestsData := []removeTeamTest{
		{uriTest{"no jwt", uri, nil, ign.NewErrorMessage(ign.ErrorUnauthorized), true}, "team1", users.TeamResponses{t1Resp, t2Resp, t3Resp}},
		{uriTest{"no write access", uri, newJWT(jwt2), ign.NewErrorMessage(ign.ErrorUnauthorized), false}, "team1", users.TeamResponses{t1Resp, t2Resp, t3Resp}},
		{uriTest{"org doest not exist", orgTeamsRoute("inv"), jwtDef, ign.NewErrorMessage(ign.ErrorNonExistentResource), false}, "team1", nil},
		{uriTest{"invalid jwt token", uri, &testJWT{jwt: sptr("invalid")}, ign.NewErrorMessage(ign.ErrorUnauthorized), true}, "team1", nil},
		{uriTest{"missing team name", uri, jwtDef, ign.NewErrorMessage(ign.ErrorNameNotFound), true}, "", nil},
		{uriTest{"non member cannot delete team", uri, newJWT(jwt5),
			ign.NewErrorMessage(ign.ErrorUnauthorized), false}, "team2",
			users.TeamResponses{}},
		{uriTest{"member only cannot delete team", uri, newJWT(jwt2),
			ign.NewErrorMessage(ign.ErrorUnauthorized), false}, "team2",
			users.TeamResponses{}},
		{uriTest{"org admin can delete team2", uri, newJWT(jwt3), nil, false},
			"team2", users.TeamResponses{t1Resp, t3Resp}},
		{uriTest{"org owner can delete team4 OK", orgTeamsRoute(testOrg2), jwtDef, nil, false}, "team4", users.TeamResponses{}},
	}

	for _, test := range removeTeamTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			reqArgs := igntest.RequestArgs{Method: "DELETE", Route: test.URL + "/" + test.team, Body: nil, SignedToken: jwt}
			resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			gotCode := resp.RespRecorder.Code
			require.Equal(t, expStatus, gotCode)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" DELETE "+test.team, bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				bslice, _ = igntest.AssertRouteMultipleArgs("GET", test.URL, nil, http.StatusOK, jwt, ctJSON, t)
				var teams users.TeamResponses
				require.NoError(t, json.Unmarshal(*bslice, &teams), "Unable to unmarshal response", string(*bslice))
				assert.Equal(t, test.expTeams, teams)
			}
		})
	}
}

// teamListTest defines a GET organization teams list test case.
type teamListTest struct {
	uriTest
	// the pagination query to append as suffix to the GET
	paginationQuery string
	// expected teams to be returned
	expTeams users.TeamResponses
}

// TestTeamsPagination tests the GET organization teams route.
func TestTeamsPagination(t *testing.T) {
	// General test setup
	setup()
	// get the tests JWT
	jwt := os.Getenv("IGN_TEST_JWT")
	jwtDef := newJWT(jwt)
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)
	// create a separate user using a different jwt
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)
	// create a separate user using a different jwt
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	testOrg2 := createOrganization(t)
	defer removeOrganization(testOrg2, t)
	addUserToOrg(username2, "member", testOrg, t)
	addUserToOrg(user3, "admin", testOrg, t)
	// Add teams to Org1
	b := true
	addTeamToOrg(testOrg, jwt, users.CreateTeamForm{Name: "team1", Visible: new(bool)}, t)
	addTeamToOrg(testOrg, jwt, users.CreateTeamForm{Name: "team2", Visible: &b}, t)
	addTeamToOrg(testOrg, jwt, users.CreateTeamForm{Name: "team3", Description: sptr("a"), Visible: new(bool)}, t)
	updateOrgTeam(testOrg, "team3", jwt, users.UpdateTeamForm{NewUsers: []string{username2}}, t)
	// Add teams to Org2
	addTeamToOrg(testOrg2, jwt, users.CreateTeamForm{Name: "team4", Description: sptr("test"), Visible: &b}, t)

	t1Resp := users.TeamResponse{Name: "team1", Description: nil, Visible: false, Usernames: nil}
	t2Resp := users.TeamResponse{Name: "team2", Description: nil, Visible: true, Usernames: nil}
	t3Resp := users.TeamResponse{Name: "team3", Description: sptr("a"), Visible: false, Usernames: []string{username2}}
	t4Resp := users.TeamResponse{Name: "team4", Description: sptr("test"), Visible: true, Usernames: nil}

	uri := orgTeamsRoute(testOrg)
	uri2 := orgTeamsRoute(testOrg2)

	teamListTestsData := []teamListTest{
		{uriTest{"unauthorized", uri, nil, ign.NewErrorMessage(ign.ErrorUnauthorized), true}, "", users.TeamResponses{t1Resp, t2Resp, t3Resp}},
		{uriTest{"org owner can see all teams", uri, jwtDef, nil, false}, "", users.TeamResponses{t1Resp, t2Resp, t3Resp}},
		{uriTest{"org admin can see all teams", uri, newJWT(jwt3), nil, false}, "", users.TeamResponses{t1Resp, t2Resp, t3Resp}},
		{uriTest{"org member can only see visible teams or ones he is part of", uri, newJWT(jwt2), nil, false}, "", users.TeamResponses{t2Resp, t3Resp}},
		{uriTest{"teams from org2", uri2, jwtDef, nil, false}, "", users.TeamResponses{t4Resp}},
		{uriTest{"teams from org2 - unauthorized for non member", uri2, newJWT(jwt2), ign.NewErrorMessage(ign.ErrorUnauthorized), false}, "", users.TeamResponses{t4Resp}},
	}

	for _, test := range teamListTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			reqArgs := igntest.RequestArgs{Method: "GET", Route: test.URL + test.paginationQuery, Body: nil, SignedToken: jwt}
			resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			gotCode := resp.RespRecorder.Code
			require.Equal(t, expStatus, gotCode)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" GET teams", bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				var teams users.TeamResponses
				require.NoError(t, json.Unmarshal(*bslice, &teams), "Unable to unmarshal response", string(*bslice))
				assert.Equal(t, test.expTeams, teams)
			}
		})
	}
}

// teamIndexTest test GET of a single team
type teamIndexTest struct {
	uriTest
	// name to get
	name            string
	expTeamResponse users.TeamResponse
}

// TestGetTeamDetails test getting a single team of an org.
func TestGetTeamDetails(t *testing.T) {
	setup()
	// get the tests JWT
	jwt := os.Getenv("IGN_TEST_JWT")
	jwtDef := newJWT(jwt)
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)
	// create a separate user using a different jwt
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)
	// create a separate user using a different jwt
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	// Add teams to Org1
	b := true
	addTeamToOrg(testOrg, jwt, users.CreateTeamForm{Name: "team1", Visible: new(bool)}, t)
	addTeamToOrg(testOrg, jwt, users.CreateTeamForm{Name: "team2", Visible: &b}, t)
	addTeamToOrg(testOrg, jwt, users.CreateTeamForm{Name: "team3", Description: sptr("a"), Visible: new(bool)}, t)
	addUserToOrg(username2, "member", testOrg, t)
	addUserToOrg(user3, "admin", testOrg, t)
	updateOrgTeam(testOrg, "team3", jwt, users.UpdateTeamForm{NewUsers: []string{username2}}, t)

	t1Resp := users.TeamResponse{Name: "team1", Description: nil, Visible: false, Usernames: nil}
	t2Resp := users.TeamResponse{Name: "team2", Description: nil, Visible: true, Usernames: nil}
	t3Resp := users.TeamResponse{Name: "team3", Description: sptr("a"), Visible: false, Usernames: []string{username2}}

	uri := orgTeamsRoute(testOrg)
	teamIndexTestsData := []teamIndexTest{
		{uriTest{"no jwt - unauth", uri, nil, ign.NewErrorMessage(ign.ErrorUnauthorized), true}, "team1", t1Resp},
		{uriTest{"inexistent org", orgTeamsRoute("inv"), jwtDef, ign.NewErrorMessage(ign.ErrorNonExistentResource), false}, "team1", t1Resp},
		{uriTest{"invalid jwt token", uri, &testJWT{jwt: sptr("invalid")}, ign.NewErrorMessage(ign.ErrorUnauthorized), true}, "team1", t1Resp},
		{uriTest{"org owner should get team1 data", uri, jwtDef, nil, false}, "team1", t1Resp},
		{uriTest{"org admin should get any team", uri, newJWT(jwt3), nil, false}, "team1", t1Resp},
		{uriTest{"user2 should NOT get team1 (non team member)", uri, newJWT(jwt2), ign.NewErrorMessage(ign.ErrorUnauthorized), false}, "team1", t1Resp},
		{uriTest{"user2 should get a visible team", uri, newJWT(jwt2), nil, false}, "team2", t2Resp},
		{uriTest{"user2 should get team3 (team member)", uri, newJWT(jwt2), nil, false}, "team3", t3Resp},
	}

	for _, test := range teamIndexTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			bslice, _ := igntest.AssertRouteMultipleArgs("GET", test.URL+"/"+test.name, nil, expStatus, jwt, expCt, t)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" GET "+test.name, bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				var tr users.TeamResponse
				require.NoError(t, json.Unmarshal(*bslice, &tr), "Unable to unmarshal response: %s", string(*bslice))
				assert.Equal(t, test.expTeamResponse, tr)
			}
		})
	}
}

type updateTeamTest struct {
	uriTest
	name            string
	teamInput       users.UpdateTeamForm
	readJwt         *string
	readError       *ign.ErrMsg
	expTeamResponse users.TeamResponse
}

// TestOrganizationTeamUpdate tests updating org teams
func TestOrganizationTeamUpdate(t *testing.T) {
	setup()
	// get the tests JWT
	jwt := os.Getenv("IGN_TEST_JWT")
	jwtDef := newJWT(jwt)
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)
	// create a separate user using a different jwt
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	username3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(username3, jwt3, t)
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)
	jwt5 := createValidJWTForIdentity("another-user-5", t)
	user5 := createUserWithJWT(jwt5, t)
	defer removeUserWithJWT(user5, jwt5, t)
	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	// Add teams to Org1
	b := true
	addTeamToOrg(testOrg, jwt, users.CreateTeamForm{Name: "team1", Visible: new(bool)}, t)
	addTeamToOrg(testOrg, jwt, users.CreateTeamForm{Name: "team2", Visible: &b}, t)
	addTeamToOrg(testOrg, jwt, users.CreateTeamForm{Name: "team3", Description: sptr("a"), Visible: new(bool)}, t)
	addUserToOrg(username2, "member", testOrg, t)
	addUserToOrg(user4, "admin", testOrg, t)
	updateOrgTeam(testOrg, "team3", jwt, users.UpdateTeamForm{NewUsers: []string{username2}}, t)
	// team update input
	t1Update := users.UpdateTeamForm{Description: sptr("new desc"), NewUsers: []string{username2}}
	t2Update := users.UpdateTeamForm{Visible: new(bool)}
	t3Update := users.UpdateTeamForm{RmUsers: []string{username2}}
	// Team responses
	t1Resp := users.TeamResponse{Name: "team1", Description: sptr("new desc"), Visible: false, Usernames: []string{username2}}
	t2Resp := users.TeamResponse{Name: "team2", Description: nil, Visible: false, Usernames: nil}
	t3Resp := users.TeamResponse{Name: "team3", Description: sptr("a"), Visible: false, Usernames: nil}

	uri := orgTeamsRoute(testOrg)
	updateTeamTestsData := []updateTeamTest{
		{uriTest{"no jwt", uri, nil, ign.NewErrorMessage(ign.ErrorUnauthorized),
			true}, "team1", t1Update, nil, nil, t1Resp},
		{uriTest{"inexistent org", orgTeamsRoute("inv"), jwtDef,
			ign.NewErrorMessage(ign.ErrorNonExistentResource), true}, "team1",
			t1Update, nil, nil, t1Resp},
		{uriTest{"invalid jwt token", uri, &testJWT{jwt: sptr("invalid")},
			ign.NewErrorMessage(ign.ErrorUnauthorized), true}, "team1", t1Update, nil,
			nil, t1Resp},
		{uriTest{"non org member cannot update teams", uri, newJWT(jwt5),
			ign.NewErrorMessage(ign.ErrorUnauthorized), false}, "team1", t1Update, nil,
			nil, t1Resp},
		{uriTest{"org member doesn't have write access for team1", uri, newJWT(jwt2),
			ign.NewErrorMessage(ign.ErrorUnauthorized), true}, "team1", t1Update,
			nil, nil, t1Resp},
		{uriTest{"org member doesn't have write access for team3", uri, newJWT(jwt2),
			ign.NewErrorMessage(ign.ErrorUnauthorized), false}, "team3", t3Update,
			nil, nil, t3Resp},
		{uriTest{"invalid team name", uri, jwtDef,
			ign.NewErrorMessage(ign.ErrorNameNotFound), true}, "a1", t1Update, nil,
			nil, t1Resp},
		{uriTest{"org owner can add user2 to team1. Then user2 can Read team data", uri,
			jwtDef, nil, true}, "team1", t1Update, &jwt2, nil, t1Resp},
		{uriTest{"org owner can make team2 invisible. Then user2 cannot read invisible team", uri,
			jwtDef, nil, true}, "team2", t2Update, &jwt2,
			ign.NewErrorMessage(ign.ErrorUnauthorized), t2Resp},
		{uriTest{"org owner can update team2", uri, jwtDef, nil, true}, "team2", t2Update, nil,
			nil, t2Resp},
		{uriTest{"org admin can update team3", uri, newJWT(jwt4), nil, true}, "team3",
			t3Update, nil, nil, t3Resp},
		{uriTest{"Org owner can be added to subteam", uri, jwtDef, nil, true},
			"team3", users.UpdateTeamForm{NewUsers: []string{username}}, nil, nil,
			users.TeamResponse{Name: "team3", Description: sptr("a"), Visible: false,
				Usernames: []string{username}}},
	}

	for _, test := range updateTeamTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			b := new(bytes.Buffer)
			json.NewEncoder(b).Encode(test.teamInput)
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			reqArgs := igntest.RequestArgs{Method: "PATCH", Route: test.URL + "/" + test.name, Body: b, SignedToken: jwt}
			resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			gotCode := resp.RespRecorder.Code
			require.Equal(t, expStatus, gotCode)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name()+" PATCH "+test.URL+"/"+test.name, bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				if test.readJwt != nil {
					jwt = test.readJwt
				}
				expEm, expCt = errMsgAndContentType(test.readError, ctJSON)
				expStatus = expEm.StatusCode
				bslice, _ = igntest.AssertRouteMultipleArgs("GET", test.URL+"/"+test.name, nil, expStatus, jwt, expCt, t)
				if expStatus != http.StatusOK && !test.ignoreErrorBody {
					igntest.AssertBackendErrorCode(t.Name()+" GET "+test.URL+"/"+test.name, bslice, expEm.ErrCode, t)
				} else if expStatus == http.StatusOK {
					var tr users.TeamResponse
					require.NoError(t, json.Unmarshal(*bslice, &tr), "Unable to unmarshal response: %s", string(*bslice))
					require.Equal(t, test.expTeamResponse, tr)
				}
			}
		})
	}
}
