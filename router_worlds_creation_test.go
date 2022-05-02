package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/bundles/worlds"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/proto"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"gitlab.com/ignitionrobotics/web/ign-go/testhelpers"
	"net/http"
	"os"
	"strconv"
	"testing"
)

const constWorldMainFileContents = `<?xml version="1.0" ?>
<sdf version="1.5">
  <world name="default">
    <include>
      <uri>http://myserver:8000/1.0/testuser/models/test_model/1</uri>
    </include>
    <include>
      <uri>model://ground_plane</uri>
    </include>
    <include>
      <uri>model://sun</uri>
    </include>
  </world>
</sdf>
`

const constInvalidWorldModelIncludes = `<?xml version="1.0" ?>
<sdf version="1.5">
  <world name="default">
    <include>
      <uri>this is invalid</uri>
    </include>
  </world>
</sdf>
`

// TODO MERGE consider merging using an interface in order to unify some comparison
// utilities in models and worlds.
// compares a DB' world VS a world response (fuel.World)
func assertFuelWorld(actual *fuel.World, exp *worlds.World, t *testing.T) {
	// Check required model fields
	assert.Equal(t, exp.Name, actual.Name)
	assert.Equal(t, exp.Owner, actual.Owner)
	assert.EqualValues(t, exp.Likes, *actual.Likes)
	assert.EqualValues(t, exp.Downloads, *actual.Downloads)
	assert.EqualValues(t, exp.Filesize, *actual.Filesize)
}

// Reads a world from DB and checks its folder exists.
func getWorldFromDb(t *testing.T, owner, name string) *worlds.World {
	// Get the created world
	var world worlds.World
	err := globals.Server.Db.Preload("Tags").Where("owner = ? AND name = ?", owner, name).Find(&world).Error
	assert.NoError(t, err)
	assert.NotNil(t, world.Location)
	// Sanity check: Make sure that the world file exists.
	_, err = os.Stat(*world.Location)
	assert.NoError(t, err, "World Location does not exist in disk but it should", *world.Location)
	return &world
}

func getWorldDownloadsFromDb(t *testing.T, owner, name string) *[]worlds.WorldDownload {
	world := getWorldFromDb(t, owner, name)
	var wds []worlds.WorldDownload
	err := globals.Server.Db.Where("world_id = ?", world.ID).Find(&wds).Error
	assert.NoError(t, err, "Unable to read world downloads from db: %s %s", owner, name)
	return &wds
}

// createTestWorldWithOwner is a helper function to create world given an
// optional jwt, a name, and an owner name (org or user).
func createTestWorldWithOwner(t *testing.T, jwt *string, wName, owner string, private bool) {
	// Each field in this map will be a separate field in the multipart form
	extraParams := map[string]string{
		"name":        wName,
		"owner":       owner,
		"tags":        "test_tag_1, test_tag2",
		"description": "description",
		"license":     "1",
		"permission":  "0",
		"private":     strconv.FormatBool(private),
	}
	var withThumbnails = []igntest.FileDesc{
		{"world.world", constWorldMainFileContents},
		{"thumbnails/world.sdf", constModelSDFFileContents},
	}

	uri := "/1.0/worlds"
	testName := t.Name()
	createResourceWithArgs(testName, uri, jwt, extraParams, withThumbnails, t)
}

// createThreeTestWorlds is a helper function to create 3 worlds the given JWT (optional)
func createThreeTestWorlds(t *testing.T, jwt *string) {
	// Each field in this map will be a separate field in the multipart form
	extraParams := map[string]string{
		"name":        "world1",
		"tags":        "test_tag_1, test_tag2",
		"description": "description",
		"license":     "1",
		"permission":  "0",
	}
	var withThumbnails = []igntest.FileDesc{
		{"world.world", constWorldMainFileContents},
		{"thumbnails/world.sdf", constModelSDFFileContents},
	}
	// These world files are within a singleroot folder to always test the server
	// being able to handle single folder uploads.
	var files = []igntest.FileDesc{
		{"singleroot/world.world", constWorldMainFileContents},
		{"singleroot/world.sdf", constModelSDFFileContents},
		{"singleroot/subfolder/test.txt", "test string"},
	}
	uri := "/1.0/worlds"
	testName := t.Name()
	createResourceWithArgs(testName, uri, jwt, extraParams, withThumbnails, t)
	extraParams["name"] = "world2"
	extraParams["description"] = "silly desc"
	createResourceWithArgs(testName, uri, jwt, extraParams, files, t)
	extraParams["name"] = "world3"
	extraParams["tags"] = "new one"
	createResourceWithArgs(testName, uri, jwt, extraParams, withThumbnails, t)
}

func shouldParseModelIncludes() bool {
	parseWorldModels, _ := ign.ReadEnvVar(worlds.ParseWorldContentsEnvVar)
	flag, err := strconv.ParseBool(parseWorldModels)
	return err == nil && flag
}

// TestWorldCreateVariants tests CreateWorld with different scenarios.
func TestWorldCreateVariants(t *testing.T) {
	// General test setup
	setup()

	uri := "/1.0/worlds"
	rmRoute := "/1.0/%s/worlds/%s"

	// Each field in this map will be a separate field in the multipart form
	extraParams := map[string]string{
		"name": "aWorld",
		"tags": "test_tag_1, test_tag2",
		"description": "255aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"license":    "1",
		"permission": "0",
	}

	longDescriptionParams := map[string]string{
		"name": "test",
		"tags": "test_tag_1, test_tag2",
		"description": "256aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"license":    "1",
		"permission": "0",
	}

	// Files to upload
	var dupFiles = []igntest.FileDesc{
		{"world.config", constModelConfigFileContents},
		{"world.sdf", constModelSDFFileContents},
		{"world.sdf", constModelSDFFileContents},
	}

	var okFiles = []igntest.FileDesc{
		{"world.world", constWorldMainFileContents},
		{"world.sdf", constModelSDFFileContents},
	}

	var noWorldFiles = []igntest.FileDesc{
		{"world.conf", constWorldMainFileContents},
		{"world.sdf", constModelSDFFileContents},
	}

	var invalidWorldContents = []igntest.FileDesc{
		{"world.world", constInvalidWorldModelIncludes},
	}

	var invalidHgFiles = []igntest.FileDesc{
		{"single/a.txt", constModelConfigFileContents},
		{"single/.hg/test.txt", constModelConfigFileContents},
	}
	var invalidGitFiles = []igntest.FileDesc{
		{"a.txt", constModelConfigFileContents},
		{".git", constModelConfigFileContents},
	}

	worldTests := []postTest{
		{"TestFilesPostOK", uri, nil, extraParams, okFiles, http.StatusOK, -1, &extraParams, &worlds.World{}},
		// We should be able to save the exact same World if the previous one was removed.
		{"TestFilesPostOK2", uri, nil, extraParams, okFiles, http.StatusOK, -1, &extraParams, &worlds.World{}},
		{"TestFilesPostOK3", uri, nil, extraParams, okFiles, http.StatusOK, -1, &extraParams, &worlds.World{}},
		{"TestInvalidHgFilesPost", uri, nil, extraParams, invalidHgFiles, http.StatusBadRequest,
			ign.ErrorFormInvalidValue, nil, nil},
		{"TestInvalidGitFilesPost", uri, nil, extraParams, invalidGitFiles, http.StatusBadRequest,
			ign.ErrorFormInvalidValue, nil, nil},
		{"TestDuplicateFilesPost", uri, nil, extraParams, dupFiles, http.StatusBadRequest,
			ign.ErrorFormDuplicateFile, nil, nil},
		{"TestEmptyFilesInPost", uri, nil, extraParams, []igntest.FileDesc{}, http.StatusBadRequest,
			ign.ErrorFormMissingFiles, nil, &worlds.World{}},
		// TestCreateInvalidData checks the world creation route fails when an incomplete post is sent.
		{"TestCreateMissingData", uri, nil, map[string]string{}, []igntest.FileDesc{}, http.StatusBadRequest,
			ign.ErrorFormInvalidValue, nil, &worlds.World{}},
		{"TestCreateInvalidValueLicense", uri, nil, map[string]string{"name": "test", "tags": "",
			"license": "a", "permission": "0"}, okFiles, http.StatusBadRequest, ign.ErrorFormInvalidValue, nil, &worlds.World{}},
		{"TestCreateNonExistentLicense", uri, nil, map[string]string{"name": "test", "tags": "",
			"license": "1000", "permission": "0"}, okFiles, http.StatusBadRequest, ign.ErrorFormInvalidValue, nil, &worlds.World{}},
		{"TestCreateInvalidValuePermission", uri, nil, map[string]string{"name": "test", "tags": "",
			"license": "2", "permission": "public"}, okFiles, http.StatusBadRequest, ign.ErrorFormInvalidValue, nil, &worlds.World{}},
		{"TestCreateInvalidRangePermission", uri, nil, map[string]string{"name": "test", "tags": "",
			"license": "2", "permission": "2"}, okFiles, http.StatusBadRequest, ign.ErrorFormInvalidValue, nil, &worlds.World{}},
		{"TestCreateInvalidRangePermission2", uri, nil, map[string]string{"name": "test", "tags": "",
			"license": "2", "permission": "-1"}, okFiles, http.StatusBadRequest, ign.ErrorFormInvalidValue, nil, &worlds.World{}},
		{"TestDescriptionMoreThan255Chars", uri, nil, longDescriptionParams, okFiles, http.StatusOK, -1, nil, &worlds.World{}},
	}

	if shouldParseModelIncludes() {
		tc1 := postTest{"TestInvalidWorldContents", uri, nil, extraParams, invalidWorldContents, http.StatusBadRequest, ign.ErrorFormInvalidValue, nil, nil}
		tc2 := postTest{"TestMissingWorldFile", uri, nil, extraParams, noWorldFiles, http.StatusBadRequest, ign.ErrorFormInvalidValue, nil, nil}
		worldTests = append(worldTests, tc1, tc2)
	}

	// Run all tests under different users, and removing each world after creation
	testResourcePOST(t, worldTests, false, &rmRoute)
	// Now Run all tests under different users, but keeping the created worlds
	testResourcePOST(t, worldTests, false, nil)
	// Now Run all tests under the same user, but removing each world after creation
	testResourcePOST(t, worldTests, true, &rmRoute)

	// Now test for duplicate world name
	dupNameTests := []postTest{
		{"TestFilesPostOK", uri, nil, extraParams, okFiles, http.StatusOK,
			-1, nil, &worlds.World{}},
		{"TestDuplicateName", uri, nil, extraParams, okFiles,
			http.StatusBadRequest, ign.ErrorFormDuplicateWorldName, nil, &worlds.World{}},
	}
	testResourcePOST(t, dupNameTests, true, nil)
}

// TestWorldTransfer tests transfering a world
func TestWorldTransfer(t *testing.T) {
	// General test setup
	setup()

	// create test user with default jwt
	jwtDef := os.Getenv("IGN_TEST_JWT")
	username := createUser(t)
	defer removeUser(username, t)

	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)

	// Create another user
	anotherJwt := createValidJWTForIdentity("another-user", t)
	testUser := createUserWithJWT(anotherJwt, t)
	defer removeUserWithJWT(testUser, anotherJwt, t)

	// Create source worlds
	createThreeTestWorlds(t, &jwtDef)

	// Sanity check: Get the created world to ensure it was created
	world := getWorldFromDb(t, username, "world1")

	// URL for world clone
	uri := "/1.0/" + username + "/worlds/" + *world.Name + "/transfer"

	transferTestsAnotherUser := []postTest{
		{"TestTransferInvalidUserPermissions", uri, &anotherJwt,
			map[string]string{"destOwner": "invalidOrg"}, nil,
			http.StatusBadRequest, -1, nil, nil},
		{"TestTransferInvalidDestinationName", uri, &jwtDef,
			map[string]string{"destOwner": "invalidOrg"}, nil,
			http.StatusBadRequest, -1, nil, nil},
	}
	// Run tests under different users
	testResourcePOST(t, transferTestsAnotherUser, false, nil)

	transferTestsMainUser := []postTest{
		{"TestTransferToUser", uri, &jwtDef,
			map[string]string{"destOwner": testUser}, nil,
			http.StatusNotFound, -1, nil, nil},
		{"TestransferMissingJson", uri, &jwtDef,
			nil, nil, http.StatusNotFound, -1, nil, nil},
		{"TestransferValid", uri, &jwtDef,
			map[string]string{"destOwner": testOrg}, nil,
			http.StatusOK, -1, nil, nil},
	}

	// Run tests under main user
	for _, test := range transferTestsMainUser {
		t.Run(test.testDesc, func(t *testing.T) {

			b := new(bytes.Buffer)
			json.NewEncoder(b).Encode(test.postParams)

			if test.expStatus != http.StatusOK {
				igntest.AssertRouteMultipleArgs("POST", test.uri, b, test.expStatus, &jwtDef, "text/plain; charset=utf-8", t)
			} else {
				igntest.AssertRouteMultipleArgs("POST", test.uri, b, test.expStatus, &jwtDef, "application/json", t)
			}
		})
	}
}

// TestWorldClone tests cloning a world
func TestWorldClone(t *testing.T) {
	// General test setup
	setup()

	// Create a user
	jwt := createValidJWTForIdentity("another-user", t)
	testUser := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(testUser, jwt, t)
	// Create source world
	createThreeTestWorlds(t, &jwt)

	// Sanity check: Get the created world to ensure it was created
	world := getWorldFromDb(t, testUser, "world1")

	// URL for world clone
	uri := "/1.0/" + testUser + "/worlds/" + *world.Name + "/clone"

	// Each field in this map will be a separate field in the multipart form
	extraParams := map[string]string{
		"name": *world.Name,
	}
	emptyParams := map[string]string{}
	postFiles := []igntest.FileDesc{}
	otherName := map[string]string{
		"name": "test",
	}

	tests := []postTest{
		{"TestCloneInvalidName", uri, nil,
			map[string]string{"name": "forward/slash"}, postFiles,
			http.StatusBadRequest, -1, &extraParams, &worlds.World{}},
		{"TestCloneOK no name", uri, nil, emptyParams, postFiles, http.StatusOK, -1, &extraParams, &worlds.World{}},
		{"TestClone short name not valid", uri, nil, map[string]string{"name": "no"},
			postFiles, http.StatusBadRequest, -1, &extraParams, &worlds.World{}},
		{"TestCloneOtherNameOK", uri, nil, otherName, postFiles, http.StatusOK, -1, &otherName, &worlds.World{}},
	}

	deleteRoute := "/1.0/%s/worlds/%s"

	// Run all tests under different users, and removing each world after creation
	testResourcePOST(t, tests, false, &deleteRoute)
	// Now Run all tests under different users, but keeping the created worlds
	testResourcePOST(t, tests, false, nil)
	// Now Run all tests under the same user, but removing each world after creation
	testResourcePOST(t, tests, true, &deleteRoute)
	// Now test name handling when duplicate name after clone
	extraParamsDupName := map[string]string{
		"name": *world.Name + " 1",
	}
	testsDupName := []postTest{
		{"TestCloneOK", uri, nil, emptyParams, postFiles, http.StatusOK, -1, &extraParams, &worlds.World{}},
		// We should be able to save the exact same world if the previous one was removed.
		{"TestCloneInvalidName", uri, nil, emptyParams, postFiles, http.StatusOK, -1, &extraParamsDupName, &worlds.World{}},
	}
	testResourcePOST(t, testsDupName, true, nil)

	// Get the last cloned world
	clonedName := extraParamsDupName["name"]
	db := globals.Server.Db
	var w worlds.World
	err := worlds.QueryForWorlds(db).Where("name = ?", clonedName).First(&w).Error
	assert.NoError(t, err, "Cloned world not found")

	// test that the files are also cloned and we can retrieve them using the versioned routes
	getURI := "/1.0/" + *w.Owner + "/worlds/" + clonedName + "/tip/files/world.world"
	igntest.AssertRouteMultipleArgs("GET", getURI, nil, http.StatusOK, &jwt, "text/xml; charset=utf-8", t)

	getURI = "/1.0/" + *w.Owner + "/worlds/" + clonedName + "/1/files/world.world"
	igntest.AssertRouteMultipleArgs("GET", getURI, nil, http.StatusOK, &jwt, "text/xml; charset=utf-8", t)

	getURI = "/1.0/" + *w.Owner + "/worlds/" + clonedName + "/1/" + clonedName
	reqArgs := igntest.RequestArgs{Method: "GET", Route: getURI + ".zip", Body: nil, SignedToken: &jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, http.StatusOK, "application/zip", t)
	assert.True(t, resp.Ok, "World Zip Download request didn't succeed")

	// Now test with a failing VCS repository mock
	SetFailingVCSFactory()
	serverErrorTests := []postTest{
		{"TestCloneWithServerVCSError", uri, nil, extraParams, postFiles, http.StatusInternalServerError,
			ign.ErrorCreatingDir, nil, &worlds.World{}},
	}
	testResourcePOST(t, serverErrorTests, true, nil)
	RestoreVCSFactory()

	// test cloning private world

	// create test user with default jwt
	jwtDef := os.Getenv("IGN_TEST_JWT")
	username := createUser(t)
	defer removeUser(username, t)
	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	// Create another user
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	addUserToOrg(user3, "member", testOrg, t)

	// create private world for default user
	// default user should be able to clone this world but not other users
	createTestWorldWithOwner(t, &jwtDef, "private_world", username, true)
	// create org owned world
	createTestWorldWithOwner(t, &jwtDef, "private2", testOrg, true)

	// clone its own private model but not the other jwt user's private model
	clonePrivateParam := map[string]string{
		"name": "private-clone",
	}
	expClonePrivateParam := map[string]string{
		"name": "private-clone",
	}
	expCloneOrgPrivateParam := map[string]string{
		"name": "private2",
	}

	worldTestsPrivateClone := []postTest{
		{"Test clone private ok", "/1.0/" + username + "/worlds/private_world/clone", &jwtDef, clonePrivateParam, postFiles, http.StatusOK, -1, &expClonePrivateParam, &worlds.World{}},
		{"Test clone org private world by member", "/1.0/" + testOrg + "/worlds/private2/clone", &jwt3, emptyParams, postFiles, http.StatusOK, -1, &expCloneOrgPrivateParam, &worlds.World{}},
		{"Test clone private unauthorized", "/1.0/" + username + "/worlds/private_world/clone", &jwt, emptyParams, postFiles, http.StatusUnauthorized, ign.ErrorUnauthorized, nil, &worlds.World{}},
	}
	testResourcePOST(t, worldTestsPrivateClone, false, nil)

}

// udateTest is used to describe a file-based resource Update test case.
type resUpdateTest struct {
	uriTest
	postParams map[string]string
	postFiles  []igntest.FileDesc
	// expected model description after update.
	expDesc string
	// expected tags
	expTags []string
	// expected file tree length
	expFileTreeLen int
	// expected paths in filetree's root nodes
	expRootPaths []string
	// expected resource privacy setting
	expPrivacy bool
}

// TestWorldUpdate checks the world update route is valid.
// MERGE with TestModelUpdate. Will need to make unmarshal more generic
func TestWorldUpdate(t *testing.T) {
	// General test setup.
	setup()
	// Create user and worlds
	testUser := createUser(t)
	defer removeUser(testUser, t)
	myJWT := os.Getenv("IGN_TEST_JWT")
	defaultJWT := newJWT(myJWT)
	createThreeTestWorlds(t, &myJWT)
	// Get the created world to ensure it was created.
	world := getWorldFromDb(t, testUser, "world1")

	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	// Create another user and add to org
	jwt2 := createValidJWTForIdentity("another-user-2", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)
	addUserToOrg(user2, "member", testOrg, t)
	// Create another user and add to org
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)
	addUserToOrg(user4, "admin", testOrg, t)

	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)

	// create private world owned by org
	createTestWorldWithOwner(t, &myJWT, "private_world", testOrg, true)

	// Each field in this map will be a separate field in the multipart form
	extraTags := []string{"editTag1", "editTag2"}
	extraParams := map[string]string{
		"tags":        "editTag1,editTag2",
		"description": "edit-description",
	}
	newDescription := "new-description"
	descParams := map[string]string{
		"description": newDescription,
	}
	emptyFiles := []igntest.FileDesc{}
	var okFiles = []igntest.FileDesc{
		{"world.world", constWorldMainFileContents},
		{"world.sdf", "test changed contents\n"},
		{"world1.sdf", constModelSDFFileContents},
		{"world2.sdf", constModelSDFFileContents},
	}
	okRootPaths := []string{"/world.sdf", "/world.world", "/world1.sdf", "/world2.sdf"}

	newTags := "newTag1"
	tagsParams := map[string]string{
		"tags": newTags,
	}

	var otherFiles = []igntest.FileDesc{
		{"world1.world", constWorldMainFileContents},
	}

	var noWorldFiles = []igntest.FileDesc{
		{"world.conf", constWorldMainFileContents},
		{"world.sdf", constModelSDFFileContents},
	}

	var invalidWorldContents = []igntest.FileDesc{
		{"world.world", constInvalidWorldModelIncludes},
	}

	newPrivacy := true
	privacyParams := map[string]string{
		"private": strconv.FormatBool(newPrivacy),
	}

	// world1 filetree root paths
	origRootPaths := []string{"/thumbnails", "/world.world"}
	uri := "/1.0/" + testUser + "/worlds/" + fmt.Sprint(*world.Name)
	orgURI := "/1.0/" + testOrg + "/worlds/private_world"

	updateTestData := []resUpdateTest{
		{uriTest{"update with no JWT", uri, nil, ign.NewErrorMessage(ign.ErrorUnauthorized), true}, nil, nil, "", nil, 0, nil, false},
		{uriTest{"edit only tags", uri, defaultJWT, nil, false}, tagsParams, emptyFiles, "description", []string{newTags}, 3, origRootPaths, false},
		{uriTest{"edit only desc", uri, defaultJWT, nil, false}, descParams, emptyFiles, newDescription, []string{newTags}, 3, origRootPaths, false},
		{uriTest{"edit desc and tags", uri, defaultJWT, nil, false}, extraParams, emptyFiles, "edit-description", extraTags, 3, origRootPaths, false},
		{uriTest{"edit desc and files", uri, defaultJWT, nil, false}, descParams, okFiles, newDescription, extraTags, 4, okRootPaths, false},
		{uriTest{"remove files", uri, defaultJWT, nil, false}, extraParams, otherFiles, "edit-description", extraTags, 1, []string{"/world1.world"}, false},
		{uriTest{"edit only privacy", uri, defaultJWT, nil, false}, privacyParams, otherFiles, "edit-description", extraTags, 1, []string{"/world1.world"}, true},
		{uriTest{"edit org world by owner", orgURI, defaultJWT, nil, false}, extraParams, otherFiles, "edit-description", extraTags, 1, []string{"/world1.world"}, true},
		{uriTest{"edit org world by admin", orgURI, newJWT(jwt4), nil, false}, extraParams, otherFiles, "edit-description", extraTags, 1, []string{"/world1.world"}, true},
		{uriTest{"edit org world by member", orgURI, newJWT(jwt2), nil, false}, extraParams, otherFiles, "edit-description", extraTags, 1, []string{"/world1.world"}, true},
		{uriTest{"edit org world by non member", orgURI, newJWT(jwt3), ign.NewErrorMessage(ign.ErrorUnauthorized), false}, nil, nil, "", nil, 0, nil, false},
		{uriTest{"member only cannot edit privacy setting", orgURI, newJWT(jwt2), ign.NewErrorMessage(ign.ErrorUnauthorized), false}, privacyParams, otherFiles, "edit-description", extraTags, 1, []string{"/world1.world"}, true},
		{uriTest{"admin can edit privacy setting", orgURI, newJWT(jwt4), nil, false}, privacyParams, otherFiles, "edit-description", extraTags, 1, []string{"/world1.world"}, true},
		{uriTest{"owner can edit privacy setting", orgURI, defaultJWT, nil, false}, privacyParams, otherFiles, "edit-description", extraTags, 1, []string{"/world1.world"}, true},
	}

	if shouldParseModelIncludes() {
		tc1 := resUpdateTest{uriTest{"missing main world file", uri, defaultJWT,
			ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, descParams, noWorldFiles,
			newDescription, nil, 0, nil, false}
		tc2 := resUpdateTest{uriTest{"invalid main world file contents", uri, defaultJWT,
			ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, descParams,
			invalidWorldContents, newDescription, nil, 0, nil, false}
		updateTestData = append(updateTestData, tc1, tc2)
	}

	for _, test := range updateTestData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, _ := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			gotCode, bslice, ok := igntest.SendMultipartMethod(t.Name(), t, "PATCH", test.URL, jwt, test.postParams, test.postFiles)
			assert.True(t, ok, "Could not perform multipart request")
			require.Equal(t, expStatus, gotCode)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				require.Equal(t, http.StatusOK, gotCode, "Did not receive expected http code [%d] after sending PATCH. Got: [%d]. Response: %s", http.StatusOK, gotCode, string(*bslice))
				var got fuel.World
				require.NoError(t, json.Unmarshal(*bslice, &got), "Unable to unmarshal the world: %s", string(*bslice))
				// get the updated world from DB and compare
				w := getWorldFromDb(t, *got.Owner, *got.Name)
				assertFuelWorld(&got, w, t)
				if test.expDesc != "" {
					assert.Equal(t, test.expDesc, *got.Description)
				}
				if test.expTags != nil {
					actualTags := models.TagsToStrSlice(w.Tags)
					assert.Len(t, actualTags, len(test.expTags), "Tags length is not the expected")
					assert.True(t, ign.SameElements(test.expTags, actualTags), "Returned Tags are not the expected. Expected: %v. Got: %v", test.expTags, actualTags)
				}
				if test.expRootPaths != nil {
					filesURI := fmt.Sprintf("/1.0/%s/worlds/%s/tip/files", *got.Owner, *got.Name)
					bslice2, _ := igntest.AssertRoute("GET", filesURI, http.StatusOK, t)
					var w2 fuel.FileTree
					require.NoError(t, json.Unmarshal(*bslice2, &w2), "Unable to get the world filetree: %s", string(*bslice2))
					ok := assertFileTreeLen(t, &w2, test.expFileTreeLen, "Invalid len in FileTree. URL: %s", filesURI)
					require.True(t, ok, "Filetree check failed")
					// check root node paths
					for i, n := range w2.FileTree {
						assert.Equal(t, test.expRootPaths[i], *n.Path, "FileTreeNode (index %d) path should be [%s] but got [%s]", i, test.expRootPaths[i], *n.Path)
					}
				}
				// check resource privacy
				assert.Equal(t, test.expPrivacy, *got.Private)
			}
		})
	}
}
