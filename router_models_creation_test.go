package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/proto"
	"github.com/gazebo-web/gz-go/v7"
	gztest "github.com/gazebo-web/gz-go/v7/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"net/http"
	"os"
	"strconv"
	"testing"
)

// TestModelCreateVariants tests CreateModel with different scenarios.
func TestModelCreateVariants(t *testing.T) {
	// General test setup
	setup()

	uri := "/1.0/models"
	rmRoute := "/1.0/%s/models/%s"

	// Each field in this map will be a separate field in the multipart form
	extraParams := map[string]string{
		"name": "test",
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

	// Each field in this map will be a separate field in the multipart form
	invalidNamePercentParams := map[string]string{
		"name": "test%",
		"tags": "test_tag_1, test_tag2",
		"description": "255aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"license":    "1",
		"permission": "0",
	}

	// Files to upload
	var dupModelFiles = []gztest.FileDesc{
		{Path: "model.config", Contents: constModelConfigFileContents},
		{Path: "model.sdf", Contents: constModelSDFFileContents},
		{Path: "model.config", Contents: constModelConfigFileContents},
	}

	var okModelFiles = []gztest.FileDesc{
		{Path: "model.config", Contents: constModelConfigFileContents},
		{Path: "model.sdf", Contents: constModelSDFFileContents},
	}

	var invalidHgFiles = []gztest.FileDesc{
		{Path: ".hg/test.txt", Contents: constModelConfigFileContents},
	}
	var invalidGitFiles = []gztest.FileDesc{
		{Path: ".git/test.txt", Contents: constModelConfigFileContents},
	}

	modelTests := []postTest{
		{testDesc: "TestFilesPostOK", uri: uri, postParams: extraParams, postFiles: okModelFiles, expStatus: http.StatusOK, expErrCode: -1, unmarshal: &models.Model{}},
		// We should be able to save the exact same Model if the previous one was removed.
		{testDesc: "TestFilesPostOK2", uri: uri, postParams: extraParams, postFiles: okModelFiles, expStatus: http.StatusOK, expErrCode: -1, unmarshal: &models.Model{}},
		{testDesc: "TestFilesPostOK3", uri: uri, postParams: extraParams, postFiles: okModelFiles, expStatus: http.StatusOK, expErrCode: -1, unmarshal: &models.Model{}},
		{testDesc: "TestInvalidGitFile", uri: uri, postParams: extraParams, postFiles: invalidGitFiles, expStatus: http.StatusBadRequest,
			expErrCode: gz.ErrorFormInvalidValue, unmarshal: &models.Model{}},
		{testDesc: "TestInvalidHgFile", uri: uri, postParams: extraParams, postFiles: invalidHgFiles, expStatus: http.StatusBadRequest,
			expErrCode: gz.ErrorFormInvalidValue, unmarshal: &models.Model{}},
		{testDesc: "TestDuplicateFilesPost", uri: uri, postParams: extraParams, postFiles: dupModelFiles, expStatus: http.StatusBadRequest,
			expErrCode: gz.ErrorFormDuplicateFile, unmarshal: &models.Model{}},
		{testDesc: "TestEmptyFilesInPost", uri: uri, postParams: extraParams, postFiles: []gztest.FileDesc{}, expStatus: http.StatusBadRequest,
			expErrCode: gz.ErrorFormMissingFiles, unmarshal: &models.Model{}},
		// TestCreateModelInvalidData checks the model creation route fails when an incomplete post is sent.
		{testDesc: "TestCreateModelMissingData", uri: uri, postParams: map[string]string{}, postFiles: []gztest.FileDesc{}, expStatus: http.StatusBadRequest,
			expErrCode: gz.ErrorFormInvalidValue, unmarshal: &models.Model{}},
		{testDesc: "TestCreateModelInvalidValueLicense", uri: uri, postParams: map[string]string{"name": "test", "tags": "",
			"license": "a", "permission": "0"}, postFiles: okModelFiles, expStatus: http.StatusBadRequest, expErrCode: gz.ErrorFormInvalidValue, unmarshal: &models.Model{}},
		{testDesc: "TestCreateModelNonExistentLicense", uri: uri, postParams: map[string]string{"name": "test", "tags": "",
			"license": "1000", "permission": "0"}, postFiles: okModelFiles, expStatus: http.StatusBadRequest, expErrCode: gz.ErrorFormInvalidValue, unmarshal: &models.Model{}},
		{testDesc: "TestCreateModelInvalidValuePermission", uri: uri, postParams: map[string]string{"name": "test", "tags": "",
			"license": "2", "permission": "public"}, postFiles: okModelFiles, expStatus: http.StatusBadRequest, expErrCode: gz.ErrorFormInvalidValue, unmarshal: &models.Model{}},
		{testDesc: "TestCreateModelInvalidRangePermission", uri: uri, postParams: map[string]string{"name": "test", "tags": "",
			"license": "2", "permission": "2"}, postFiles: okModelFiles, expStatus: http.StatusBadRequest, expErrCode: gz.ErrorFormInvalidValue, unmarshal: &models.Model{}},
		{testDesc: "TestCreateModelInvalidRangePermission2", uri: uri, postParams: map[string]string{"name": "test", "tags": "",
			"license": "2", "permission": "-1"}, postFiles: okModelFiles, expStatus: http.StatusBadRequest, expErrCode: gz.ErrorFormInvalidValue, unmarshal: &models.Model{}},
		{testDesc: "TestDescriptionMoreThan255Chars", uri: uri, postParams: longDescriptionParams, postFiles: okModelFiles, expStatus: http.StatusOK, expErrCode: -1, unmarshal: &models.Model{}},
		{testDesc: "TestNameContainsPercent", uri: uri, postParams: invalidNamePercentParams, postFiles: okModelFiles, expStatus: http.StatusBadRequest, expErrCode: gz.ErrorFormInvalidValue, unmarshal: &models.Model{}},
	}
	// Run all tests under different users, and removing each model after creation
	testResourcePOST(t, modelTests, false, &rmRoute)
	// Now Run all tests under different users, but keeping the created models
	testResourcePOST(t, modelTests, false, nil)
	// Now Run all tests under the same user, but removing each model after creation
	testResourcePOST(t, modelTests, true, &rmRoute)

	// Now test for duplicate model name
	dupModelNameTests := []postTest{
		{testDesc: "TestFilesPostOK", uri: uri, postParams: extraParams, postFiles: okModelFiles, expStatus: http.StatusOK,
			expErrCode: -1, unmarshal: &models.Model{}},
		{testDesc: "TestDuplicateModelName", uri: uri, postParams: extraParams, postFiles: okModelFiles,
			expStatus: http.StatusBadRequest, expErrCode: gz.ErrorFormDuplicateModelName, unmarshal: &models.Model{}},
	}

	testResourcePOST(t, dupModelNameTests, true, nil)

	// create victim user for impersonation test
	jwt := createValidJWTForIdentity("another-user", t)
	testUser := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(testUser, jwt, t)

	ownerTest := []postTest{
		{"TestCreatorOwnerMismatch", uri, nil, map[string]string{"name": "test", "tags": "", "owner": testUser, "license": "1", "permission": "0"}, okModelFiles, http.StatusUnauthorized, gz.ErrorUnauthorized, nil, &models.Model{}},
	}

	// Now run test with owner that is different from creator
	testResourcePOST(t, ownerTest, false, &rmRoute)
}

// TestModelTransfer tests transfering a model
func TestModelTransfer(t *testing.T) {
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

	// Create source models
	createThreeTestModels(t, &jwtDef)

	// Sanity check: Get the created model to ensure it was created
	model := getOwnerModelFromDb(t, username, "model1")

	// URL for model clone
	uri := "/1.0/" + username + "/models/" + *model.Name + "/transfer"

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
			assert.NoError(t, json.NewEncoder(b).Encode(test.postParams))
			if test.expStatus != http.StatusOK {
				gztest.AssertRouteMultipleArgs("POST", test.uri, b, test.expStatus, &jwtDef, "text/plain; charset=utf-8", t)
			} else {
				gztest.AssertRouteMultipleArgs("POST", test.uri, b, test.expStatus, &jwtDef, "application/json", t)
			}
		})
	}
}

// TestModelClone tests cloning a model
func TestModelClone(t *testing.T) {
	// General test setup
	setup()

	// Create a user
	jwt := createValidJWTForIdentity("another-user", t)
	testUser := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(testUser, jwt, t)

	// Create source models
	createThreeTestModels(t, &jwt)

	// Sanity check: Get the created model to ensure it was created
	model := getOwnerModelFromDb(t, testUser, "model1")

	// URL for model clone
	uri := "/1.0/" + testUser + "/models/" + *model.Name + "/clone"

	// Each field in this map will be a separate field in the multipart form
	expParams := map[string]string{
		"name": *model.Name,
	}

	emptyParams := map[string]string{}
	postFiles := []gztest.FileDesc{}
	otherName := map[string]string{
		"name": "test",
	}

	modelTests := []postTest{
		{"TestCloneInvalidName", uri, nil,
			map[string]string{"name": "forward/slash"}, postFiles,
			http.StatusBadRequest, -1, &expParams, &models.Model{}},
		{"TestCloneOK no name", uri, nil, emptyParams, postFiles, http.StatusOK, -1,
			&expParams, &models.Model{}},
		{"TestClone short name not valid", uri, nil, map[string]string{"name": "no"},
			postFiles, http.StatusBadRequest, -1, &expParams, &models.Model{}},
		{"TestCloneOtherNameOK", uri, nil, otherName, postFiles, http.StatusOK, -1,
			&map[string]string{"name": "test"}, &models.Model{}},
	}

	rmRoute := "/1.0/%s/models/%s"

	// Run all tests under different users, and removing each model after creation
	testResourcePOST(t, modelTests, false, &rmRoute)
	// Now Run all tests under different users, but keeping the created models
	testResourcePOST(t, modelTests, false, nil)
	// Now Run all tests under the same user, but removing each model after creation
	testResourcePOST(t, modelTests, true, &rmRoute)
	// Now test name handling when duplicate name after clone
	expParamsDupName := map[string]string{
		"name": *model.Name + " 1",
	}
	modelTestsDupName := []postTest{
		{"TestCloneOK", uri, nil, emptyParams, postFiles, http.StatusOK, -1, &expParams, &models.Model{}},
		// We should be able to save the exact same Model if the previous one was removed.
		{"TestCloneInvalidName", uri, nil, emptyParams, postFiles, http.StatusOK, -1, &expParamsDupName, &models.Model{}},
	}
	testResourcePOST(t, modelTestsDupName, true, nil)

	// Get the last cloned model
	clonedModelName := expParamsDupName["name"]
	db := globals.Server.Db
	var m models.Model
	err := models.QueryForModels(db).Where("name = ?", clonedModelName).First(&m).Error
	assert.NoError(t, err, "Cloned Model not found")

	// test that the files are also cloned and we can retrieve them using the versioned routes
	getURI := "/1.0/" + *m.Owner + "/models/" + clonedModelName + "/tip/files/model.config"
	gztest.AssertRouteMultipleArgs("GET", getURI, nil, http.StatusOK, &jwt, "text/xml; charset=utf-8", t)

	getURI = "/1.0/" + *m.Owner + "/models/" + clonedModelName + "/1/files/model.config"
	gztest.AssertRouteMultipleArgs("GET", getURI, nil, http.StatusOK, &jwt, "text/xml; charset=utf-8", t)

	getURI = "/1.0/" + *m.Owner + "/models/" + clonedModelName + "/1/" + clonedModelName
	reqArgs := gztest.RequestArgs{Method: "GET", Route: getURI + ".zip", Body: nil, SignedToken: &jwt}
	resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, http.StatusOK, "application/zip", t)
	assert.True(t, resp.Ok, "Model Zip Download request didn't succeed")

	// Now test with a failing VCS repository mock
	SetFailingVCSFactory()
	serverErrorTests := []postTest{
		{"TestCloneWithServerVCSError", uri, nil, otherName, postFiles, http.StatusInternalServerError,
			gz.ErrorCreatingDir, nil, &models.Model{}},
	}
	testResourcePOST(t, serverErrorTests, true, nil)
	RestoreVCSFactory()

	// test cloning private model

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

	// create private model for default user
	// default user should be able to clone this model but not other users
	createTestModelWithOwner(t, &jwtDef, "private_model", username, true)
	// create org owned model
	createTestModelWithOwner(t, &jwtDef, "private2", testOrg, true)

	clonePrivateParam := map[string]string{
		"name": "private-clone",
	}
	expClonePrivateParam := map[string]string{
		"name": "private-clone",
	}
	expCloneOrgPrivateParam := map[string]string{
		"name": "private2",
	}

	modelTestsPrivateClone := []postTest{
		{"Test clone private ok", "/1.0/" + username + "/models/private_model/clone", &jwtDef, clonePrivateParam, postFiles, http.StatusOK, -1, &expClonePrivateParam, &models.Model{}},
		{"Test clone org private model by member", "/1.0/" + testOrg + "/models/private2/clone", &jwt3, emptyParams, postFiles, http.StatusOK, -1, &expCloneOrgPrivateParam, &models.Model{}},
		{"Test clone private unauthorized", "/1.0/" + username + "/models/private_model/clone", &jwt, emptyParams, postFiles, http.StatusUnauthorized, gz.ErrorUnauthorized, nil, &models.Model{}},
	}
	testResourcePOST(t, modelTestsPrivateClone, false, nil)
}

// modelUpdateTest is used to describe a Model Update test case.
type modelUpdateTest struct {
	uriTest
	postParams map[string]string
	postFiles  []gztest.FileDesc
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

// TestModelUpdate checks the model update route is valid.
func TestModelUpdate(t *testing.T) {
	// General test setup.
	setup()
	// Create user and models
	testUser := createUser(t)
	defer removeUser(testUser, t)
	myJWT := os.Getenv("IGN_TEST_JWT")
	defaultJWT := newJWT(myJWT)
	createThreeTestModels(t, &myJWT)
	// Get the created model to ensure it was created.
	model := getOwnerModelFromDb(t, testUser, "model1")

	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	// Create another user and add to org
	jwt2 := createValidJWTForIdentity("another-user-2", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)
	addUserToOrg(user2, "member", testOrg, t)

	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	// Create another user and add to org
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)
	addUserToOrg(user4, "admin", testOrg, t)

	// create private model owned by org
	createTestModelWithOwner(t, &myJWT, "private_model", testOrg, true)

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
	var emptyFiles []gztest.FileDesc
	var okModelFiles = []gztest.FileDesc{
		{Path: "model.config", Contents: constModelConfigFileContents},
		{Path: "model.sdf", Contents: "test changed contents\n"},
		{Path: "model1.sdf", Contents: constModelSDFFileContents},
		{Path: "model2.sdf", Contents: constModelSDFFileContents},
	}
	okModelRootPaths := []string{"/model.config", "/model.sdf", "/model1.sdf", "/model2.sdf"}

	newTags := "newTag1"
	tagsParams := map[string]string{
		"tags": newTags,
	}

	var otherFiles = []gztest.FileDesc{
		{Path: "model1.config", Contents: constModelConfigFileContents},
	}

	privacyParams := map[string]string{
		"private": strconv.FormatBool(true),
	}

	// model1 filetree root paths
	origRootPaths := []string{"/model.config", "/thumbnails"}
	uri := "/1.0/" + testUser + "/models/" + fmt.Sprint(*model.Name)
	orgURI := "/1.0/" + testOrg + "/models/private_model"

	modelUpdateTestData := []modelUpdateTest{
		{uriTest{"update with no JWT", uri, nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, nil, nil, "", nil, 0, nil, false},
		{uriTest{"edit only tags", uri, defaultJWT, nil, false}, tagsParams, emptyFiles, "description", []string{newTags}, 3, origRootPaths, false},
		{uriTest{"edit only desc", uri, defaultJWT, nil, false}, descParams, emptyFiles, newDescription, []string{newTags}, 3, origRootPaths, false},
		{uriTest{"edit model desc and tags", uri, defaultJWT, nil, false}, extraParams, emptyFiles, "edit-description", extraTags, 3, origRootPaths, false},
		{uriTest{"model desc and files", uri, defaultJWT, nil, false}, descParams, okModelFiles, newDescription, extraTags, 4, okModelRootPaths, false},
		{uriTest{"remove files", uri, defaultJWT, nil, false}, extraParams, otherFiles, "edit-description", extraTags, 1, []string{"/model1.config"}, false},
		{uriTest{"edit only privacy", uri, defaultJWT, nil, false}, privacyParams, otherFiles, "edit-description", extraTags, 1, []string{"/model1.config"}, true},
		{uriTest{"edit org model by owner", orgURI, defaultJWT, nil, false}, extraParams, otherFiles, "edit-description", extraTags, 1, []string{"/model1.config"}, true},
		{uriTest{"edit org model by admin", orgURI, newJWT(jwt4), nil, false}, extraParams, otherFiles, "edit-description", extraTags, 1, []string{"/model1.config"}, true},
		{uriTest{"edit org model by member", orgURI, newJWT(jwt2), nil, false}, extraParams, otherFiles, "edit-description", extraTags, 1, []string{"/model1.config"}, true},
		{uriTest{"non member cannot edit org model", orgURI, newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, nil, nil, "", nil, 0, nil, false},
		{uriTest{"member only cannot edit privacy setting", orgURI, newJWT(jwt2), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, privacyParams, otherFiles, "edit-description", extraTags, 1, []string{"/model1.config"}, true},
		{uriTest{"admin can edit privacy setting", orgURI, newJWT(jwt4), nil, false}, privacyParams, otherFiles, "edit-description", extraTags, 1, []string{"/model1.config"}, true},
		{uriTest{"owner can edit privacy setting", orgURI, defaultJWT, nil, false}, privacyParams, otherFiles, "edit-description", extraTags, 1, []string{"/model1.config"}, true},
	}

	for _, test := range modelUpdateTestData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, _ := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			gotCode, bslice, ok := gztest.SendMultipartMethod(t.Name(), t, "PATCH", test.URL, jwt, test.postParams, test.postFiles)
			assert.True(t, ok, "Could not perform multipart request")
			require.Equal(t, expStatus, gotCode)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				gztest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				assert.Equal(t, http.StatusOK, gotCode, "Did not receive expected http code [%d] after sending PATCH. Got: [%d]. Response: %s", http.StatusOK, gotCode, string(*bslice))
				var gotModel fuel.Model
				assert.NoError(t, json.Unmarshal(*bslice, &gotModel), "Unable to unmarshal the model: %s", string(*bslice))
				// get the updated model from DB and compare
				m := getOwnerModelFromDb(t, *gotModel.Owner, *gotModel.Name)
				assertFuelModel(&gotModel, m, t)
				if test.expDesc != "" {
					assert.Equal(t, test.expDesc, *gotModel.Description)
				}
				if test.expTags != nil {
					actualTags := models.TagsToStrSlice(m.Tags)
					assert.Len(t, actualTags, len(test.expTags), "Tags length is not the expected")
					assert.True(t, gz.SameElements(test.expTags, actualTags), "Returned Tags are not the expected. Expected: %v. Got: %v", test.expTags, actualTags)
				}
				if test.expRootPaths != nil {
					filesURI := fmt.Sprintf("/1.0/%s/models/%s/tip/files", *gotModel.Owner, *gotModel.Name)
					bslice2, _ := gztest.AssertRoute("GET", filesURI, http.StatusOK, t)
					var m2 fuel.FileTree
					assert.NoError(t, json.Unmarshal(*bslice2, &m2), "Unable to get the model filetree: %s", string(*bslice2))
					assertFileTreeLen(t, &m2, test.expFileTreeLen, "Invalid len in FileTree. URL: %s", filesURI)
					// check root node paths
					for i, n := range m2.FileTree {
						assert.Equal(t, test.expRootPaths[i], *n.Path, "FileTreeNode (index %d) path should be [%s] but got [%s]", i, test.expRootPaths[i], *n.Path)
					}
				}
				// check resource privacy
				assert.Equal(t, test.expPrivacy, *gotModel.Private)
			}
		})
	}
}
