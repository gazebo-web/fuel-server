package main

import (
	"encoding/json"
	"fmt"
	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/bundles/worlds"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/proto"
	"github.com/gazebo-web/gz-go/v7"
	gztest "github.com/gazebo-web/gz-go/v7/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"net/http"
	"os"
	"path"
	"testing"
)

// Common Tests for worlds and models . Here we put such tests that work with
// both resourc types.

// TestInvalidServerKey checks what happens when the server is configured with an
// invalid auth key, and we try to POST new resources.
// NOTE: this is currently testing both models and worlds routes.
func TestInvalidServerKey(t *testing.T) {
	// General test setup
	setup()

	uris := []string{"/1.0/models", "/1.0/worlds"}

	// Each field in this map will be a separate field in the multipart form
	extraParams := map[string]string{
		"name":        "test",
		"tags":        "test_tag_1, test_tag2",
		"description": "description",
		"license":     "1",
		"permission":  "0",
	}
	var files = []gztest.FileDesc{
		{Path: "world.sdf", Contents: constModelSDFFileContents},
	}
	jwt := os.Getenv("IGN_TEST_JWT")

	for _, uri := range uris {
		t.Run(uri, func(t *testing.T) {
			// Use an invalid Auth key in the server and see what happens
			cleanFn := setRandomAuth0PublicKey()
			code, bslice, ok := gztest.SendMultipartPOST(t.Name(), t, uri, &jwt, extraParams, files)
			assert.True(t, ok, "Failed POST request %s %s", t.Name(), string(*bslice))
			assert.Equal(t, http.StatusUnauthorized, code,
				"Did not receive expected http code after sending POST! %s %d %d %s", t.Name(),
				http.StatusUnauthorized, code, string(*bslice))
			cleanFn()
		})
	}
}

type createResourceTest struct {
	uriTest
	params map[string]string
	files  []gztest.FileDesc
}

// TestCreateNamedResource tests if users can create resources based on owners
// and given permissions.
func TestCreateNamedResource(t *testing.T) {
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
	username3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(username3, jwt3, t)
	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	addUserToOrg(username2, "member", testOrg, t)

	var modelFiles = []gztest.FileDesc{
		{Path: "model.config", Contents: constModelConfigFileContents},
		{Path: "thumbnails/model.sdf", Contents: constModelSDFFileContents},
	}
	var worldFiles = []gztest.FileDesc{
		{Path: "world.world", Contents: constWorldMainFileContents},
	}

	uri := "/1.0/models"
	wURI := "/1.0/worlds"

	createResourceTestsData := []createResourceTest{
		// MODELS
		{uriTest{"no jwt", uri, nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true},
			map[string]string{"name": "mo1", "owner": username, "license": "1"},
			modelFiles},
		{uriTest{"OK to create model", uri, jwtDef, nil, false},
			map[string]string{"name": "mo1", "owner": username, "license": "1"},
			modelFiles},
		{uriTest{"Cannot create model with 2 chars", uri, jwtDef,
			gz.NewErrorMessage(gz.ErrorFormInvalidValue), false},
			map[string]string{"name": "no", "owner": username, "license": "1"},
			modelFiles},
		{uriTest{"invalid name", uri, jwtDef,
			gz.NewErrorMessage(gz.ErrorFormInvalidValue), false},
			map[string]string{"name": "forward/slash", "owner": username,
				"license": "1"}, modelFiles},
		{uriTest{"duplicate model", uri, jwtDef,
			gz.NewErrorMessage(gz.ErrorFormDuplicateModelName), false},
			map[string]string{"name": "mo1", "owner": username, "license": "1"},
			modelFiles},
		{uriTest{"models| org as owner, created by user1", uri, jwtDef, nil, false},
			map[string]string{"name": "mo2", "owner": testOrg, "license": "1"},
			modelFiles},
		{uriTest{"models| org as owner, created by member", uri, newJWT(jwt2), nil, false},
			map[string]string{"name": "mo3", "owner": testOrg, "license": "1"},
			modelFiles},
		{uriTest{"models| private model with org as owner, created by member", uri,
			newJWT(jwt2), nil, false}, map[string]string{"name": "mo4", "owner": testOrg,
			"license": "1", "permission": "1", "private": "true"}, modelFiles},
		{uriTest{"models| org as owner, created by non-member", uri, newJWT(jwt3),
			gz.NewErrorMessage(gz.ErrorUnauthorized), false},
			map[string]string{"name": "mo4", "owner": testOrg, "license": "1"},
			modelFiles},

		// WORLDS
		{uriTest{"no jwt", wURI, nil, gz.NewErrorMessage(gz.ErrorUnauthorized),
			true}, map[string]string{"name": "wo1", "owner": username, "license": "1"},
			worldFiles},
		{uriTest{"OK world", wURI, jwtDef, nil, false},
			map[string]string{"name": "wo1", "owner": username, "license": "1"},
			worldFiles},
		{uriTest: uriTest{"Cannot create world with 2 chars", wURI, jwtDef,
			gz.NewErrorMessage(gz.ErrorFormInvalidValue), false},
			params: map[string]string{"name": "no", "owner": username, "license": "1"},
			files:  worldFiles},
		{uriTest: uriTest{"invalid name", wURI, jwtDef,
			gz.NewErrorMessage(gz.ErrorFormInvalidValue), false},
			params: map[string]string{"name": "forward/slash", "owner": username,
				"license": "1"}, files: worldFiles},
		{uriTest: uriTest{testDesc: "dup world", URL: wURI, jwtGen: jwtDef,
			expErrMsg: gz.NewErrorMessage(gz.ErrorFormDuplicateWorldName)},
			params: map[string]string{"name": "wo1", "owner": username, "license": "1"},
			files:  worldFiles},
		{uriTest: uriTest{"worlds| org as owner, created by user1", wURI, jwtDef, nil, false},
			params: map[string]string{"name": "wo2", "owner": testOrg, "license": "1"},
			files:  worldFiles},
		{uriTest: uriTest{testDesc: "worlds| org as owner, created by member", URL: wURI, jwtGen: newJWT(jwt2)},
			params: map[string]string{"name": "wo3", "owner": testOrg, "license": "1"},
			files:  worldFiles},
		{uriTest: uriTest{"worlds| private model with org as owner, created by member", wURI,
			newJWT(jwt2), nil, false}, params: map[string]string{"name": "wo4", "owner": testOrg,
			"license": "1", "permission": "1", "private": "true"}, files: worldFiles},
		{uriTest: uriTest{testDesc: "worlds| org as owner, created by non-member", URL: wURI, jwtGen: newJWT(jwt3),
			expErrMsg: gz.NewErrorMessage(gz.ErrorUnauthorized)},
			params: map[string]string{"name": "wo4", "owner": testOrg, "license": "1"},
			files:  worldFiles},
	}

	for _, test := range createResourceTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, _ := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			gztest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
			code, bslice, _ := gztest.SendMultipartPOST(t.Name(), t, test.URL, jwt,
				test.params, test.files)
			assert.Equal(t, expStatus, code)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				gztest.AssertBackendErrorCode(t.Name()+" POST "+test.URL, bslice,
					expEm.ErrCode, t)
			}
		})
	}
}

// deleteNamedResourceTest defines a DELETE user/worlds/world test case.
type deleteNamedResourceTest struct {
	uriTest
	// username and name are used to look for the world in DB.
	username string
	name     string
	// the struct used to unmarshal the returned http response and compare results (eg. models.Model)
	unmarshal interface{}
}

// TestDeleteWorldsAndModels checks the delete route is valid. The same code
// works for models and worlds.
func TestDeleteWorldsAndModels(t *testing.T) {
	setup()

	jwtDef := os.Getenv("IGN_TEST_JWT")
	defaultJWT := newJWT(jwtDef)

	// Create two random users using different JWTs, some worlds and models
	username := createUser(t)
	defer removeUser(username, t)
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)
	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	// Create another user and make him member of the org
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	addUserToOrg(user3, "member", testOrg, t)
	// Create another user and make him admin of the org
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)
	addUserToOrg(user4, "admin", testOrg, t)

	createThreeTestWorlds(t, nil)
	// create public and private worlds owned by org
	createTestWorldWithOwner(t, &jwtDef, "public_world", testOrg, false)
	createTestWorldWithOwner(t, &jwtDef, "public_world2", testOrg, false)
	createTestWorldWithOwner(t, &jwtDef, "public_world3", testOrg, false)
	createTestWorldWithOwner(t, &jwtDef, "private_world", testOrg, true)
	createTestWorldWithOwner(t, &jwtDef, "private_world2", testOrg, true)
	createTestWorldWithOwner(t, &jwtDef, "private_world3", testOrg, true)
	createThreeTestModels(t, nil)
	// create public and private models owned by org
	createTestModelWithOwner(t, &jwtDef, "public_model", testOrg, false)
	createTestModelWithOwner(t, &jwtDef, "public_model2", testOrg, false)
	createTestModelWithOwner(t, &jwtDef, "public_model3", testOrg, false)
	createTestModelWithOwner(t, &jwtDef, "private_model", testOrg, true)
	createTestModelWithOwner(t, &jwtDef, "private_model2", testOrg, true)
	createTestModelWithOwner(t, &jwtDef, "private_model3", testOrg, true)

	w1URI := worldURL(username, "world1", "")
	w2URI := worldURL(testOrg, "public_world", "")
	pwURI := worldURL(testOrg, "private_world", "")
	m1URI := modelURL(username, "model1", "")
	m2URI := modelURL(testOrg, "public_model", "")
	pmURI := modelURL(testOrg, "private_model", "")

	deleteTestsData := []deleteNamedResourceTest{
		// TODO: make a single list of test cases that work for both models and worlds
		// WORLDS
		{uriTest{"cannot delete world with another jwt", w1URI, newJWT(jwt2), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, "", "", &worlds.World{}},
		{uriTest{"cannot delete world with no jwt", w1URI, nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "", "", &worlds.World{}},
		{uriTest{"a valid world delete from owner", w1URI, defaultJWT, nil, false}, username, "world1", &worlds.World{}},
		{uriTest{"org member can delete public org world", w2URI, newJWT(jwt3), nil, true}, testOrg, "public_world", &worlds.World{}},
		{uriTest{"org admin can delete public world", worldURL(testOrg, "public_world2", ""), newJWT(jwt4), nil, false}, testOrg, "public_world2", &worlds.World{}},
		{uriTest{"org owner can delete public world", worldURL(testOrg, "public_world3", ""), defaultJWT, nil, false}, testOrg, "public_world3", &worlds.World{}},
		{uriTest{"org member can delete private org world", pwURI, newJWT(jwt3), nil, true}, testOrg, "private_world", &worlds.World{}},
		{uriTest{"org admin can delete private world", worldURL(testOrg, "private_world2", ""), newJWT(jwt4), nil, false}, testOrg, "private_world2", &worlds.World{}},
		{uriTest{"org owner can delete private world", worldURL(testOrg, "private_world3", ""), defaultJWT, nil, false}, testOrg, "private_world3", &worlds.World{}},
		// MODELS
		{uriTest{"cannot delete model with another jwt", m1URI, newJWT(jwt2), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, "", "", &models.Model{}},
		{uriTest{"cannot delete model no jwt", m1URI, nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "", "", &models.Model{}},
		{uriTest{"a valid model delete", m1URI, defaultJWT, nil, false}, username, "model1", &models.Model{}},
		{uriTest{"org member can delete public org model", m2URI, newJWT(jwt3), nil, true}, testOrg, "public_model", &models.Model{}},
		{uriTest{"org admin can delete public org model", modelURL(testOrg, "public_model2", ""), newJWT(jwt4), nil, false}, testOrg, "public_model2", &models.Model{}},
		{uriTest{"org owner can delete public org model", modelURL(testOrg, "public_model3", ""), defaultJWT, nil, false}, testOrg, "public_model3", &models.Model{}},
		{uriTest{"org member can delete private org model", pmURI, newJWT(jwt3), nil, true}, testOrg, "private_model", &models.Model{}},
		{uriTest{"org admin can delete private org model", modelURL(testOrg, "private_model2", ""), newJWT(jwt4), nil, false}, testOrg, "private_model2", &models.Model{}},
		{uriTest{"org owner can delete private org model", modelURL(testOrg, "private_model3", ""), defaultJWT, nil, false}, testOrg, "private_model3", &models.Model{}},
	}

	for _, test := range deleteTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			gztest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
			reqArgs := gztest.RequestArgs{Method: "DELETE", Route: test.URL, Body: nil, SignedToken: jwt}
			resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			assert.Equal(t, expStatus, resp.RespRecorder.Code)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				gztest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				db := globals.Server.Db.Where("owner = ? AND name = ?", test.username, test.name).Find(test.unmarshal)
				assert.Error(t, db.Error)
				assert.True(t, db.RecordNotFound())
			}
		})
	}
}

// getFileTreeTest defines a TestGetFileTree test case.
type getFileTreeTest struct {
	uriTest
	// expected file tree length
	expLen int
	// expected x-ign-resource-version header
	expResVersion int
	// expected file paths in root nodes
	expRootPaths []string
}

// TestGetFileTree checks if we get the file tree for individual models and worlds.
func TestGetFileTree(t *testing.T) {
	// General test setup
	setup()
	// Create a user, test models and worlds
	testUser := createUser(t)
	defer removeUser(testUser, t)
	orgname := createOrganization(t)
	defer removeOrganization(orgname, t)
	// create another user
	jwt2 := createValidJWTForIdentity("another-user-2", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)
	// Create another user and make him member
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	addUserToOrg(user3, "member", orgname, t)

	createThreeTestModels(t, nil)
	createThreeTestWorlds(t, nil)
	// private org assets
	createTestModelWithOwner(t, nil, "orgModel", orgname, true)
	createTestWorldWithOwner(t, nil, "orgWorld", orgname, true)

	expModelPaths := []string{"/model.config", "/thumbnails"}
	expWorldPaths := []string{"/thumbnails", "/world.world"}
	defaultJWT := newJWT(os.Getenv("IGN_TEST_JWT"))

	fileTreeTestData := []getFileTreeTest{
		// MODELS
		{uriTest{"model1 filetree", modelURL(testUser, "model1", "") + "/tip/files", nil, nil, false}, 3, 1, expModelPaths},
		{uriTest{"model2 filetree with subfolder", modelURL(testUser, "model2", "") + "/tip/files", nil, nil, false}, 4, 1, []string{"/model.config", "/model.sdf", "/subfolder", "/subfolder/test.txt"}},
		{uriTest{"invalid model", worldURL(testUser, "invalidmodel", "") + "/tip/files.json", nil, gz.NewErrorMessage(gz.ErrorNameNotFound), true}, 0, 1, nil},
		{uriTest{"model version", modelURL(testUser, "model1", "") + "/1/files", nil, nil, false}, 3, 1, expModelPaths},
		{uriTest{"model invalid version format #1", modelURL(testUser, "model1", "") + "/-1.23/files", nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, 0, 1, nil},
		{uriTest{"model invalid version format #2", modelURL(testUser, "model1", "") + "/-a0/files", nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, 0, 1, nil},
		{uriTest{"model version 0", modelURL(testUser, "model2", "") + "/0/files", nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, 0, 1, nil},
		{uriTest{"unexistent model version", modelURL(testUser, "model2", "") + "/5/files", nil, gz.NewErrorMessage(gz.ErrorVersionNotFound), false}, 0, 1, nil},
		{uriTest{"get model with org owner", modelURL(orgname, "orgModel", "") + "/tip/files", defaultJWT, nil, false}, 3, 1, expModelPaths},
		{uriTest{"get private model with org member", modelURL(orgname, "orgModel", "") + "/tip/files", newJWT(jwt3), nil, false}, 3, 1, expModelPaths},
		{uriTest{"get private model with non member", modelURL(orgname, "orgModel", "") + "/tip/files", newJWT(jwt2), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, 0, 1, nil},
		//// WORLDS
		{uriTest{"world1 filetree", worldURL(testUser, "world1", "") + "/tip/files", nil, nil, false}, 3, 1, expWorldPaths},
		{uriTest{"world2 filetree with subfolder", worldURL(testUser, "world2", "") + "/tip/files", nil, nil, false}, 4, 1, []string{"/subfolder", "/world.sdf", "/world.world", "/subfolder/test.txt"}},
		{uriTest{"invalid world", worldURL(testUser, "invalidworld", "") + "/tip/files.json", nil, gz.NewErrorMessage(gz.ErrorNameNotFound), true}, 0, 1, nil},
		{uriTest{"world version", worldURL(testUser, "world1", "") + "/1/files", nil, nil, false}, 3, 1, expWorldPaths},
		{uriTest{"world invalid version format #1", worldURL(testUser, "world1", "") + "/-1.23/files", nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, 0, 1, nil},
		{uriTest{"world invalid version format #2", worldURL(testUser, "world1", "") + "/-a0/files", nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, 0, 1, nil},
		{uriTest{"world version 0", worldURL(testUser, "world2", "") + "/0/files", nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, 0, 1, nil},
		{uriTest{"unexistent world version", worldURL(testUser, "world2", "") + "/5/files", nil, gz.NewErrorMessage(gz.ErrorVersionNotFound), false}, 0, 1, nil},
		{uriTest{"get world with org owner", worldURL(orgname, "orgWorld", "") + "/tip/files", defaultJWT, nil, false}, 3, 1, expWorldPaths},
		{uriTest{"get private world with org member", worldURL(orgname, "orgWorld", "") + "/tip/files", newJWT(jwt3), nil, false}, 3, 1, expWorldPaths},
		{uriTest{"get private world with non member", worldURL(orgname, "orgWorld", "") + "/tip/files", newJWT(jwt2), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, 0, 1, nil},
	}

	for _, test := range fileTreeTestData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithFileTreeTestData(t, test)
		})
		// Now run the same test case but adding a JWT, if needed
		if test.jwtGen == nil {
			test.jwtGen = defaultJWT
			test.testDesc += "[with JWT]"
			t.Run(test.testDesc, func(t *testing.T) {
				runSubtestWithFileTreeTestData(t, test)
			})
		}
	}

	// remove some files in server and make it fail
	model := getOwnerModelFromDb(t, testUser, "model1")
	modelPath := path.Join(globals.ResourceDir, testUser, "models", fmt.Sprint(*model.UUID))
	assert.NoError(t, os.Rename(modelPath, modelPath+"-tmp"))
	brokenModel := getFileTreeTest{uriTest{"broken model1 filetree", modelURL(testUser, "model1", "") + "/tip/files", nil, gz.NewErrorMessage(gz.ErrorUnexpected), false}, 0, 1, nil}
	runSubtestWithFileTreeTestData(t, brokenModel)

	// remove files from world in server and make it fail
	w := getWorldFromDb(t, testUser, "world1")
	worldPath := path.Join(globals.ResourceDir, testUser, "worlds", fmt.Sprint(*w.UUID))
	assert.NoError(t, os.Rename(worldPath, worldPath+"-tmp"))
	brokenWorld := getFileTreeTest{uriTest{"broken world1 filetree", worldURL(testUser, "world1", "") + "/tip/files", nil, gz.NewErrorMessage(gz.ErrorUnexpected), false}, 0, 1, nil}
	runSubtestWithFileTreeTestData(t, brokenWorld)
}

func runSubtestWithFileTreeTestData(t *testing.T, test getFileTreeTest) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	gztest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	reqArgs := gztest.RequestArgs{Method: "GET", Route: test.URL, Body: nil, SignedToken: jwt}
	resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		gztest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var ft fuel.FileTree
		assert.NoError(t, json.Unmarshal(*bslice, &ft), "Unable to unmarshal the filetree")
		ok := assertFileTreeLen(t, &ft, test.expLen, "Invalid len in FileTree. URL: %s", test.URL)
		// validate resource version header was returned
		ensureIgnResourceVersionHeader(resp.RespRecorder, test.expResVersion, t)
		require.True(t, ok, "Invalid len in FileTree")
		// check root node paths
		for i, n := range ft.FileTree {
			assert.Equal(t, test.expRootPaths[i], *n.Path, "FileTreeNode (index %d) path should be [%s] but got [%s]", i, test.expRootPaths[i], *n.Path)
		}
	}
}

type protoFileTree struct {
	uri    string
	expLen int
}

// TestFileTreeProto checks if we get the file tree for
// individual worlds or models
func TestFileTreeProto(t *testing.T) {
	// General test setup
	setup()
	// Create a user and test world/models
	testUser := createUser(t)
	defer removeUser(testUser, t)
	createThreeTestModels(t, nil)
	createThreeTestWorlds(t, nil)
	myJWT := os.Getenv("IGN_TEST_JWT")

	tests := []protoFileTree{
		{uri: fmt.Sprintf("/1.0/%s/models/model1/tip/files.proto", testUser), expLen: 3},
		{uri: fmt.Sprintf("/1.0/%s/models/model1/1/files.proto", testUser), expLen: 3},
		{uri: fmt.Sprintf("/1.0/%s/worlds/world2/tip/files.proto", testUser), expLen: 4},
		{uri: fmt.Sprintf("/1.0/%s/worlds/world1/1/files.proto", testUser), expLen: 3},
	}

	for _, test := range tests {
		t.Run(test.uri, func(t *testing.T) {
			// Try the protobuf version
			gztest.AssertRoute("OPTIONS", test.uri, http.StatusOK, t)
			reqArgs := gztest.RequestArgs{Method: "GET", Route: test.uri, Body: nil, SignedToken: &myJWT}
			resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, http.StatusOK, "application/arraybuffer", t)
			assert.True(t, resp.Ok, "File tree request didn't succeed")
			bslice3 := resp.BodyAsBytes
			var ft fuel.FileTree
			assert.NoError(t, proto.Unmarshal(*bslice3, &ft), "Unable to get the filetree (proto)")
			assertFileTreeLen(t, &ft, test.expLen, "Invalid len in FileTree. URL: %s", test.uri)
			// validate resource version header was returned
			ensureIgnResourceVersionHeader(resp.RespRecorder, 1, t)
		})
	}
}

// individualFileTest defines a TestGetIndividualFile test case.
// it works with models, worlds, etc
type individualFileTest struct {
	uriTest
	// expected response content type
	expContentType string
	// expected x-ign-resource-version header
	expResVersion int
}

// TestGetIndividualFile tests downloading an individual file.
func TestGetIndividualFile(t *testing.T) {
	// General test setup
	setup()
	// Create a user and test models and worlds
	testUser := createUser(t)
	defer removeUser(testUser, t)

	orgname := createOrganization(t)
	defer removeOrganization(orgname, t)
	// create another user
	jwt2 := createValidJWTForIdentity("another-user-2", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)
	// Create another user and make him member
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	addUserToOrg(user3, "member", orgname, t)

	// create models and worlds owned by default user
	createThreeTestModels(t, nil)
	createTestModelWithOwner(t, nil, "user_private", testUser, true)
	createThreeTestWorlds(t, nil)
	createTestWorldWithOwner(t, nil, "user_private", testUser, true)
	// private org assets
	createTestModelWithOwner(t, nil, "orgModel", orgname, true)
	createTestWorldWithOwner(t, nil, "orgWorld", orgname, true)

	m1URI := modelURL(testUser, "model1", "")
	m2URI := modelURL(testUser, "model2", "")
	pmURI := modelURL(orgname, "orgModel", "")

	w1URI := worldURL(testUser, "world1", "")
	w2URI := worldURL(testUser, "world2", "")
	pwURI := worldURL(orgname, "orgWorld", "")

	defaultJWT := newJWT(os.Getenv("IGN_TEST_JWT"))

	individualFileTestsDaa := []individualFileTest{
		// MODELS
		{uriTest{"model config", m1URI + "/tip/files/model.config", nil, nil, false}, "text/xml; charset=utf-8", 1},
		{uriTest{"model sdf", m2URI + "/tip/files/model.sdf", nil, nil, false}, "chemical/x-mdl-sdfile", 1},
		{uriTest{"model invalid file", m1URI + "/tip/files/invalid.sdf", nil, gz.NewErrorMessage(gz.ErrorFileNotFound), false}, "", 1},
		{uriTest{"invalid model name", modelURL(testUser, "invalid", "") + "/tip/files/model.sdf", nil, gz.NewErrorMessage(gz.ErrorNameNotFound), false}, "", 1},
		{uriTest{"model file from subfolder", m2URI + "/tip/files/subfolder/test.txt", nil, nil, false}, "text/plain; charset=utf-8", 1},
		{uriTest{"model subfolder only", m2URI + "/tip/files/subfolder/", nil, gz.NewErrorMessage(gz.ErrorFileNotFound), false}, "", 1},
		{uriTest{"model subfolder no slash", m2URI + "/tip/files/subfolder", nil, gz.NewErrorMessage(gz.ErrorFileNotFound), false}, "", 1},
		{uriTest{"model explicit version", m1URI + "/1/files/model.config", nil, nil, false}, "text/xml; charset=utf-8", 1},
		{uriTest{"model explicit version subfolder", m2URI + "/1/files/subfolder/test.txt", nil, nil, false}, "text/plain; charset=utf-8", 1},
		{uriTest{"model invalid version", m1URI + "/0/files/model.config", nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, "", 1},
		{uriTest{"model version not found", m1URI + "/2/files/model.config", nil, gz.NewErrorMessage(gz.ErrorVersionNotFound), false}, "", 1},
		{uriTest{"get private model with org owner", pmURI + "/tip/files/model.config", defaultJWT, nil, false}, "text/xml; charset=utf-8", 1},
		{uriTest{"get private model with org member", pmURI + "/tip/files/model.config", newJWT(jwt3), nil, false}, "text/xml; charset=utf-8", 1},
		{uriTest{"get private model with non member", pmURI + "/tip/files/model.config", newJWT(jwt2), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, "", 1},
		{uriTest{"get private user model with another jwt", modelURL(testUser, "user_private", "") + "/tip/files/model.config", newJWT(jwt2), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, "", 1},
		// WORLDS
		{uriTest{"world sdf", w2URI + "/tip/files/world.world", nil, nil, false}, "text/xml; charset=utf-8", 1},
		{uriTest{"world invalid file", w1URI + "/tip/files/invalid.sdf", nil, gz.NewErrorMessage(gz.ErrorFileNotFound), false}, "", 1},
		{uriTest{"invalid world name", worldURL(testUser, "invalid", "") + "/tip/files/model.sdf", nil, gz.NewErrorMessage(gz.ErrorNameNotFound), false}, "", 1},
		{uriTest{"world file from subfolder", w2URI + "/tip/files/subfolder/test.txt", nil, nil, false}, "text/plain; charset=utf-8", 1},
		{uriTest{"world subfolder only", w2URI + "/tip/files/subfolder/", nil, gz.NewErrorMessage(gz.ErrorFileNotFound), false}, "", 1},
		{uriTest{"world subfolder no slash", w2URI + "/tip/files/subfolder", nil, gz.NewErrorMessage(gz.ErrorFileNotFound), false}, "", 1},
		{uriTest{"world explicit version", w1URI + "/1/files/world.world", nil, nil, false}, "text/xml; charset=utf-8", 1},
		{uriTest{"world explicit version subfolder", w2URI + "/1/files/subfolder/test.txt", nil, nil, false}, "text/plain; charset=utf-8", 1},
		{uriTest{"world invalid version", w1URI + "/0/files/world.world", nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, "", 1},
		{uriTest{"world version not found", w1URI + "/2/files/world.world", nil, gz.NewErrorMessage(gz.ErrorVersionNotFound), false}, "", 1},
		{uriTest{"get private world with org owner", pwURI + "/tip/files/world.world", defaultJWT, nil, false}, "text/xml; charset=utf-8", 1},
		{uriTest{"get private world with org member", pwURI + "/tip/files/world.world", newJWT(jwt3), nil, false}, "text/xml; charset=utf-8", 1},
		{uriTest{"get private world with non member", pwURI + "/tip/files/world.world", newJWT(jwt2), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, "", 1},
		{uriTest{"get private user world with another jwt", worldURL(testUser, "user_private", "") + "/tip/files/world.world", newJWT(jwt2), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, "", 1},
	}

	for _, test := range individualFileTestsDaa {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestIndividualFileDownload(t, test)
		})
		// Now run the same test case but adding a JWT, if needed
		if test.jwtGen == nil {
			test.jwtGen = defaultJWT
			test.testDesc += "[with JWT]"
			t.Run(test.testDesc, func(t *testing.T) {
				runSubtestIndividualFileDownload(t, test)
			})
		}
	}
}

func runSubtestIndividualFileDownload(t *testing.T, test individualFileTest) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, test.expContentType)
	expStatus := expEm.StatusCode
	reqArgs := gztest.RequestArgs{Method: "GET", Route: test.URL, Body: nil, SignedToken: jwt}
	gztest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		gztest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		ensureIgnResourceVersionHeader(resp.RespRecorder, test.expResVersion, t)
	}
}

// TestOptionsIndividualFile checks the OPTIONS route for downloading individual
// files.
func TestOptionsIndividualFile(t *testing.T) {
	// General test setup
	setup()

	wURI := worldURL("testuser", "world1", "") + "/1/files/file.file"
	wSubFolder := worldURL("testuser", "world1", "") + "/1/files/subfolder/file.file"
	gztest.AssertRoute("OPTIONS", wURI, http.StatusOK, t)
	gztest.AssertRoute("OPTIONS", wSubFolder, http.StatusOK, t)
}
