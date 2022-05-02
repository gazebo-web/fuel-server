package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/cmd/token-generator/generator"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/proto"
	"github.com/gazebo-web/fuel-server/vcs"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"gitlab.com/ignitionrobotics/web/ign-go/testhelpers"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
)

// Test utilities and some mocks

const (
	apiVersion  string = "1.0"
	ctTextPlain string = "text/plain; charset=utf-8"
	ctJSON      string = "application/json"
	ctZip       string = "application/zip"
)

// sptr returns a pointer to a given string.
// This function is specially useful when using string literals as argument.
func sptr(s string) *string {
	return &s
}

// iptr returns a pointer to a given int.
func iptr(i int) *int {
	return &i
}

// errMsgAndContentType is a helper that given an optional errMsg and a content type to use
// when OK (ie. http status code 200), it returns a tuple with the ErrMsg and contentType to use
// in a subsequent call to 'igntest.AssertRouteMultipleArgs'.
// It was created to reduce LOC.
func errMsgAndContentType(em *ign.ErrMsg, successCT string) (ign.ErrMsg, string) {
	if em != nil {
		return *em, ctTextPlain
	}
	return ign.ErrorMessageOK(), successCT
}

// setup helper function
func setup() {
	setupWithCustomInitalizer(nil)
}

type customInitializer func(ctx context.Context)

// setup helper function
func setupWithCustomInitalizer(customFn customInitializer) {
	logger := ign.NewLoggerNoRollbar("test", ign.VerbosityDebug)
	logCtx := ign.NewContextWithLogger(context.Background(), logger)
	// Make sure we don't have data from other tests.
	// For this we drop db tables and recreate them.
	// cleanDBTables(logCtx)
	packageTearDown(logCtx)
	DBAddDefaultData(logCtx, globals.Server.Db)

	if customFn != nil {
		customFn(logCtx)
	}

	// Check for auth0 environment variables.
	if os.Getenv("IGN_TEST_JWT") == "" {
		log.Printf("Missing IGN_TEST_JWT env variable." +
			"Authentication will not work.")
	}

	// Create the router, and indicate that we are testing
	igntest.SetupTest(globals.Server.Router)
}

//////////////////////////////
// Helper functions to test POSTing of file based resources to backend.
//////////////////////////////

// postWithArgs is an test helper function to POST resources to backend.
// posts a file-based resource for testing and returns the result.
func postWithArgs(t *testing.T, uri string, jwt *string,
	params map[string]string, files []igntest.FileDesc) (int, *[]byte) {
	code, bslice, _ := igntest.SendMultipartPOST(t.Name(), t, uri, jwt, params, files)
	return code, bslice
}

// createResourceWithArgs is an helper function to POST resources to backend.
// Create a file-based resource (model, world) for testing.
// extraParams and extraFiles args can be used to customize the resource that will be created.
func createResourceWithArgs(testName string, uri string, aJWT *string,
	extraParams map[string]string, extraFiles []igntest.FileDesc, t *testing.T) {

	var jwt string
	if aJWT != nil {
		jwt = *aJWT
	} else {
		jwt = os.Getenv("IGN_TEST_JWT")
	}
	code, bslice, ok := igntest.SendMultipartPOST(testName, t, uri, &jwt, extraParams, extraFiles)
	assert.True(t, ok, "Failed POST request %s %s", testName, string(*bslice))
	assert.Equal(t, http.StatusOK, code, "Did not receive expected http code after sending POST! %s %d %d %s", testName, http.StatusOK, code, string(*bslice))
}

// WithOwnerAndName is an interface for those objects that can return owner and
// name.
type WithOwnerAndName interface {
	GetName() *string
	GetOwner() *string
}

type FuelResource interface {
	WithOwnerAndName
	GetLikes() int64
	GetDownloads() int64
	GetFilesize() int64
}

// postTest is used to describe a Resource POST test case.
type postTest struct {
	testDesc   string
	uri        string
	jwt        *string
	postParams map[string]string
	postFiles  []igntest.FileDesc
	expStatus  int
	// optional: possible values are the code or -1 (to ignore)
	expErrCode int
	// optional. If present, returned resource will be compared against "name" expParam.
	expParams *map[string]string
	// the object used to unmarshal the returned http response and compare results (eg. models.Model)
	unmarshal WithOwnerAndName
}

// testResourcePOST is a helper function to POST a resource , compare its name, and optionally
// delete it.
// rmRoute argument should be a fmt string (ie. using %s) that will be formatted with the
// resource name and owner (eg. "/1.0/%s/worlds/%s")
func testResourcePOST(t *testing.T, testCases []postTest, shareUser bool, rmRoute *string) {

	jwt := os.Getenv("IGN_TEST_JWT")

	if shareUser {
		// Use a shared user for all tests
		sharedUser := createUser(t)
		defer removeUser(sharedUser, t)
		assert.NotEmpty(t, sharedUser, "Could not create shared user")
	}

	var testUser string
	for _, test := range testCases {
		testName := test.testDesc + "_SameUser:" + strconv.FormatBool(shareUser) + "_rm:" + strconv.FormatBool(rmRoute != nil)
		t.Run(testName, func(t *testing.T) {
			testJWT := jwt
			if test.jwt != nil {
				testJWT = *test.jwt
			} else if !shareUser {
				testJWT = jwt
				// Create and remove a user for each Test
				testUser = createUser(t)
				assert.NotEmpty(t, testUser, "Could not create shared user")
			}
			// Create model
			code, bslice, ok := igntest.SendMultipartPOST(t.Name(), t, test.uri, &testJWT, test.postParams, test.postFiles)
			assert.True(t, ok, "Failed POST request")
			require.Equal(t, test.expStatus, code, "Did not receive expected http code [%d] after sending POST. Got:[%d]. Response body [%s]", test.expStatus, code, string(*bslice))
			if test.expErrCode != -1 {
				igntest.AssertBackendErrorCode(t.Name(), bslice, test.expErrCode, t)
			}
			// Get the created Model
			if code == http.StatusOK {
				m := test.unmarshal
				assert.NoError(t, json.Unmarshal(*bslice, m), "Unable to decode the returned resource: [%s]", string(*bslice))
				assert.NotNil(t, m.GetName(), "Created resource does not have Name")
				if test.expParams != nil {
					expName := (*test.expParams)["name"]
					assert.Equal(t, expName, *m.GetName(), "Created resource does not have exp name [%s] field value. Got: [%s]", expName, *m.GetName())
				}
				if rmRoute != nil {
					uri := fmt.Sprintf(*rmRoute, *m.GetOwner(), *m.GetName())
					igntest.AssertRoute("DELETE", uri, http.StatusOK, t)
				}
			}
			if test.jwt == nil && !shareUser {
				removeUser(testUser, t)
			}
		})
	}
}

///////////////////////////////
// A Mock VCS repository used to test unexpected failures.
///////////////////////////////

// FailingVCS is a VCS repository implementation that always fails.
type FailingVCS struct{}

func (g *FailingVCS) CloneTo(ctx context.Context, target string) error {
	return errors.New("error")
}
func (g *FailingVCS) GetFile(ctx context.Context, rev string, pathFromRoot string) (*[]byte, error) {
	return nil, errors.New("error")
}
func (g *FailingVCS) InitRepo(ctx context.Context) error {
	return errors.New("error")
}
func (g *FailingVCS) ReplaceFiles(ctx context.Context, folder, owner string) error {
	return errors.New("error")
}
func (g *FailingVCS) Tag(ctx context.Context, tag string) error {
	return errors.New("error")
}
func (g *FailingVCS) Walk(ctx context.Context, rev string, includeFolders bool, fn vcs.WalkFn) error {
	return errors.New("error")
}
func (g *FailingVCS) Zip(ctx context.Context, rev, output string) (*string, error) {
	return nil, errors.New("error")
}
func (g *FailingVCS) RevisionCount(ctx context.Context, rev string) (int, error) {
	return 0, errors.New("error")
}

// origVCSFactory is a private variable to backup original server's VCS repo
// when a test switches the VCS factory to FailingVCS.
var origVCSFactory (func(ctx context.Context, dirpath string) vcs.VCS)

func SetFailingVCSFactory() {
	if origVCSFactory == nil {
		origVCSFactory = globals.VCSRepoFactory
	}
	globals.VCSRepoFactory = func(ctx context.Context, dirpath string) vcs.VCS {
		r := FailingVCS{}
		return &r
	}
}
func RestoreVCSFactory() {
	globals.VCSRepoFactory = origVCSFactory
	origVCSFactory = nil
}

//////////////
/// Utility functions to create and remove users and orgs
//////////////

// testJWT is either a explicit jwt token , or a map of jwtClaims
// used to generate a jwt token (using the TOKEN_GENERATOR_PRIVATE_RSA256_KEY env var)
type testJWT struct {
	jwt       *string
	jwtClaims *jwt.MapClaims
}

// newClaimsJWT creates a testJWT definition using a map of claims
func newClaimsJWT(cl *jwt.MapClaims) *testJWT {
	return &testJWT{jwtClaims: cl}
}

// newJWT creates a new testJWT definition based on a given string token.
func newJWT(tk string) *testJWT {
	return &testJWT{jwt: &tk}
}

// getTestJWT - given an optional testJWT it creates and returns a token (or nil).
func getJWTToken(t *testing.T, jwtDef *testJWT) *string {
	if jwtDef != nil {
		s := generateJWT(*jwtDef, t)
		return &s
	}
	return nil
}

// generateJWT creates a JWT given a testJWT struct.
func generateJWT(jwt testJWT, t *testing.T) string {
	testPrivateKey := os.Getenv("TOKEN_GENERATOR_PRIVATE_RSA256_KEY")
	testPrivateKeyAsPEM := []byte("-----BEGIN RSA PRIVATE KEY-----\n" + testPrivateKey + "\n-----END RSA PRIVATE KEY-----")
	if jwt.jwt != nil {
		return *jwt.jwt
	}

	token, err := generator.GenerateTokenRSA256(testPrivateKeyAsPEM, *jwt.jwtClaims)
	assert.NoError(t, err, "Error while generating token")
	return token
}

// Generate a new test JWT token with the given identity.
func createValidJWTForIdentity(identity string, t *testing.T) string {
	return generateJWT(testJWT{jwtClaims: &jwt.MapClaims{"sub": identity}}, t)
}

// Create a random user for testing purposes
func createUser(t *testing.T) string {
	myJWT := os.Getenv("IGN_TEST_JWT")
	return createUserWithJWT(myJWT, t)
}

// Create a user that will act as sysadmin during testing.
func createSysAdminUser(t *testing.T) string {
	myJWT := os.Getenv("IGN_TEST_JWT")
	return createNamedUserWithJWT("rootfortests", myJWT, t)
}

func createNamedUserWithJWT(username, jwt string, t *testing.T) string {
	name := "A random user"
	email := "username@example.com"
	org := "My organization"
	u := users.User{Name: &name, Username: &username, Email: &email, Organization: &org}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(u)

	req, _ := http.NewRequest("POST", "/1.0/users", b)
	req.Header.Add("Content-Type", "application/json")

	// Add the authorization token
	req.Header.Set("Authorization", "Bearer "+jwt)

	respRec := httptest.NewRecorder()
	globals.Server.Router.ServeHTTP(respRec, req)

	// Make sure the status code is correct
	assert.Equal(t, http.StatusOK, respRec.Code, "Server error: returned [%d] instead of [%d] with body [%s]", respRec.Code, http.StatusOK, respRec.Body)

	// Check CORS
	accessControlHeaders := respRec.Header().Get("Access-Control-Allow-Headers")
	assert.Contains(t, accessControlHeaders, "X-CSRF-Token", "Access-Control-Allow-Headers missing X-CSRF-Token")
	assert.Contains(t, accessControlHeaders, "Authorization", "Access-Control-Allow-Headers missing Authorization")

	accessControlOrigin := respRec.Header().Get("Access-Control-Allow-Origin")
	assert.Equal(t, "*", accessControlOrigin, "Access-Control-Allow-Origin != '*'")

	accessControlCredentials := respRec.Header().Get("Access-Control-Allow-Credentials")
	assert.Equal(t, "true", accessControlCredentials, "Access-Control-Allow-Credentials != 'true'")
	// end check CORS

	decoder := json.NewDecoder(respRec.Body)
	var userResponse users.UserResponse
	decoder.Decode(&userResponse)
	assert.Equal(t, username, userResponse.Username, "Expected username[%s] != response username[%s]", username, userResponse.Username)
	return username
}

// Create a random user with a given JWT for testing purposes
func createUserWithJWT(jwt string, t *testing.T) string {
	username := ign.RandomString(8)
	return createNamedUserWithJWT(username, jwt, t)
}

// Remove a user used for testing
func removeUser(username string, t *testing.T) {
	// Use default JWT
	myJWT := os.Getenv("IGN_TEST_JWT")
	removeUserWithJWT(username, myJWT, t)
}

func dbGetUserByID(id uint) *users.User {
	var user users.User
	globals.Server.Db.Where("id = ?", id).First(&user)
	if user.Username == nil {
		return nil
	}
	return &user
}

// TODO: merge with getUserFromDb func below.
func dbGetUser(username string) *users.User {
	var user users.User
	globals.Server.Db.Where("username = ?", username).First(&user)
	if user.Username == nil {
		return nil
	}
	return &user
}

// Reads user from DB
func getUserFromDb(username string, t *testing.T) (*users.User, *ign.ErrMsg) {
	// Get the created model
	return users.ByUsername(globals.Server.Db, username, false)
}

// Remove a user used for testing
func removeUserWithJWT(username string, jwt string, t *testing.T) {

	// Find the user
	var user *users.User
	user = dbGetUser(username)
	require.NotNil(t, user, "removeUser error: Unable to remove user [%s]", username)

	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(*user)
	req, _ := http.NewRequest("DELETE", "/1.0/users/"+username, b)
	// Add the authorization token
	req.Header.Set("Authorization", "Bearer "+jwt)
	respRec := httptest.NewRecorder()
	globals.Server.Router.ServeHTTP(respRec, req)
	// Make sure the status code is correct
	assert.Equal(t, http.StatusOK, respRec.Code, "Server error: returned [%d] instead of [%d]", respRec.Code, http.StatusOK)
	decoder := json.NewDecoder(respRec.Body)
	var userResponse users.UserResponse
	decoder.Decode(&userResponse)
	assert.Equal(t, username, userResponse.Username, "Expected username[%s] != response username[%s]", username, userResponse.Username)
	// Confirm the user deletion
	var aUser users.User
	globals.Server.Db.Where("username = ? AND deleted_at = ?", username, nil).First(&aUser)
	assert.Nil(t, aUser.Username, "The user is still in the database")
}

// Reads organization from DB
func getOrganizationFromDb(name string, t *testing.T) (*users.Organization, *ign.ErrMsg) {
	// Get the created organization
	return users.ByOrganizationName(globals.Server.Db, name, false)
}

// Create a random organization for testing purposes
// PRE-REQ: a user with the default JWT should have been created before.
func createOrganization(t *testing.T) string {
	name := ign.RandomString(8)
	createOrganizationWithName(t, name)
	return name
}

// Create a named organization for testing purposes
// PRE-REQ: a user with the default JWT should have been created before.
func createOrganizationWithName(t *testing.T, name string) string {
	jwt := os.Getenv("IGN_TEST_JWT")

	description := "a random organization"
	o := users.Organization{Name: &name, Description: &description}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(o)

	req, _ := http.NewRequest("POST", "/1.0/organizations", b)
	req.Header.Add("Content-Type", "application/json")

	// Add the authorization token
	req.Header.Set("Authorization", "Bearer "+jwt)

	respRec := httptest.NewRecorder()
	globals.Server.Router.ServeHTTP(respRec, req)

	// Make sure the status code is correct
	assert.Equal(t, http.StatusOK, respRec.Code, "Server error: returned [%d] instead of [%d] with body [%s]", respRec.Code, http.StatusOK, respRec.Body)

	// Check CORS
	accessControlHeaders := respRec.Header().Get("Access-Control-Allow-Headers")
	assert.Contains(t, accessControlHeaders, "X-CSRF-Token", "Access-Control-Allow-Headers missing X-CSRF-Token")
	assert.Contains(t, accessControlHeaders, "Authorization", "Access-Control-Allow-Headers missing Authorization")

	accessControlOrigin := respRec.Header().Get("Access-Control-Allow-Origin")
	assert.Equal(t, "*", accessControlOrigin, "Access-Control-Allow-Origin != '*'")

	accessControlCredentials := respRec.Header().Get("Access-Control-Allow-Credentials")
	assert.Equal(t, "true", accessControlCredentials, "Access-Control-Allow-Credentials != 'true'")
	// end check CORS

	decoder := json.NewDecoder(respRec.Body)
	var organizationResponse users.OrganizationResponse
	decoder.Decode(&organizationResponse)

	assert.Equal(t, name, organizationResponse.Name, "Expected organization name[%s] != response name[%s]", name, organizationResponse.Name)

	return name
}

// Remove an organization used for testing
func removeOrganization(name string, t *testing.T) {
	jwt := os.Getenv("IGN_TEST_JWT")

	// Find the organization
	organization, _ := users.ByOrganizationName(globals.Server.Db, name, false)
	require.NotNil(t, organization, "removeOrganization error: Unable to remove organization[%s]", name)

	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(*organization)

	req, _ := http.NewRequest("DELETE", "/1.0/organizations/"+name, b)

	// Add the authorization token
	req.Header.Set("Authorization", "Bearer "+jwt)
	respRec := httptest.NewRecorder()
	globals.Server.Router.ServeHTTP(respRec, req)

	// Make sure the status code is correct
	assert.Equal(t, http.StatusOK, respRec.Code, "server error was [%d] instead of expected [%d]",
		respRec.Code, http.StatusOK)

	decoder := json.NewDecoder(respRec.Body)
	var organizationResponse users.OrganizationResponse
	decoder.Decode(&organizationResponse)
	assert.Equal(t, name, organizationResponse.Name, "Expected Org name[%s] != got name[%s]",
		name, organizationResponse.Name)
}

// adds a user to an org with a role (owner/admin/member)
func addUserToOrg(user, role, org string, t *testing.T) {
	jwt := os.Getenv("IGN_TEST_JWT")
	add := users.AddUserToOrgInput{user, role}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(add)
	uri := fmt.Sprintf("/1.0/organizations/%s/users", org)
	igntest.AssertRouteMultipleArgs("POST", uri, b, http.StatusOK, &jwt, ctJSON, t)
}

// adds a team to an org
func addTeamToOrg(org, jwt string, team users.CreateTeamForm, t *testing.T) {
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(team)
	uri := fmt.Sprintf("/1.0/organizations/%s/teams", org)
	igntest.AssertRouteMultipleArgs("POST", uri, b, http.StatusOK, &jwt, ctJSON, t)
}

// updates a team of an org
func updateOrgTeam(org, team, jwt string, ut users.UpdateTeamForm, t *testing.T) {
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(ut)
	uri := fmt.Sprintf("/1.0/organizations/%s/teams/%s", org, team)
	igntest.AssertRouteMultipleArgs("PATCH", uri, b, http.StatusOK, &jwt, ctJSON, t)
}

// returns the len of a FileNode's children (recursively).
func getLenFileTreeChildren(node *fuel.FileTree_FileNode) int {
	if node == nil {
		return 0
	}
	len := len(node.GetChildren())
	for _, n := range node.GetChildren() {
		len += getLenFileTreeChildren(n)
	}
	return len
}

// asserts that the length of a FileTree (recursive) matches the given length.
func assertFileTreeLen(t *testing.T, ft *fuel.FileTree, length int, msgAndArgs ...interface{}) bool {
	l := len(ft.FileTree)
	for _, node := range ft.FileTree {
		l += getLenFileTreeChildren(node)
	}
	if l != length {
		return assert.Fail(t, fmt.Sprintf("FileTree \"%s\" should have %d item(s), but has %d", ft, length, l), msgAndArgs...)
	}
	return true
}

// internal func that checks for the existence of X-Ign-Resource-Version response header and compares it to
// given value.
func ensureIgnResourceVersionHeader(respRec *httptest.ResponseRecorder, expResourceVersion int, t *testing.T) {
	verStr := strconv.Itoa(expResourceVersion)
	assert.Len(t, respRec.Header()["X-Ign-Resource-Version"], 1, "X-Ign-Resource-Version header should be present")
	gotResVersion := respRec.Header().Get("X-Ign-Resource-Version")
	assert.Equal(t, verStr, gotResVersion, "Value of X-Ign-Resource-Version header is different. [%s] != [%s]", verStr, gotResVersion)
}
