package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gazebo-web/gz-go/v7"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"testing"

	mocket "github.com/Selvatico/go-mocket"
	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/globals"
	fuel "github.com/gazebo-web/fuel-server/proto"
	gztest "github.com/gazebo-web/gz-go/v7/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for models related routes

const constModelSDFFileContents = `<?xml version="1.0" ?>
   <sdf version="1.5">
     <model name="test_model">
       <link name="link">
       </link>
     </model>
   </sdf>
`

const constModelConfigFileContents = `<?xml version="1.0"?>
   <model>
     <name>test_model</name>
     <version>1.0</version>
     <sdf version="1.5">model.sdf</sdf>

     <author>
       <name>Carlos Aguero</name>
       <email>caguero@osrfoundation.org</email>
     </author>

     <description>
       A model used for testing.
     </description>
   </model>
`

type uriTest struct {
	// description of the test
	testDesc string
	// a url (eg. /1.0/models?q=aDescription)
	URL string
	// an optional JWT definition (can contain a plain jwt or a claims map)
	jwtGen *testJWT
	// optional expected gz.ErrMsg response. If the test case represents an error case
	// in such case, content type text/plain will be used
	expErrMsg *gz.ErrMsg
	// in case of error response, whether to parse the response body to get an gz.ErrMsg struct
	ignoreErrorBody bool
}

// creates an URL to get a model
func modelURL(owner, model, version string) string {
	encodedName := url.PathEscape(model)
	if version != "" {
		return fmt.Sprintf("/%s/%s/models/%s/%s/%s", apiVersion, owner,
			encodedName, version, encodedName)
	}
	return fmt.Sprintf("/%s/%s/models/%s", apiVersion, owner, encodedName)
}

// createTestModelWithOwner is a helper function to create model given an
// optional jwt, a model name, and an owner name (org or user).
func createTestModelWithOwner(t *testing.T, jwt *string, modelName, owner string,
	private bool) {
	// Each field in this map will be a separate field in the multipart form
	extraParams := map[string]string{
		"name":        modelName,
		"owner":       owner,
		"tags":        "test_tag_1, test_tag2",
		"description": "description",
		"license":     "1",
		"permission":  "0",
		"private":     strconv.FormatBool(private),
	}
	var withThumbnails = []gztest.FileDesc{
		{Path: "model.config", Contents: constModelConfigFileContents},
		{Path: "thumbnails/model.sdf", Contents: constModelSDFFileContents},
	}

	uri := "/1.0/models"
	testName := t.Name()
	createResourceWithArgs(testName, uri, jwt, extraParams, withThumbnails, t)
}

// createThreeTestModels is a helper function to create 3 models using the given
// optional jwt.
func createThreeTestModels(t *testing.T, jwt *string) {
	// Each field in this map will be a separate field in the multipart form
	extraParams := map[string]string{
		"name":        "model1",
		"tags":        "test_tag_1, test_tag2",
		"description": "description",
		"license":     "1",
		"permission":  "0",
	}
	var withThumbnails = []gztest.FileDesc{
		{Path: "model.config", Contents: constModelConfigFileContents},
		{Path: "thumbnails/model.sdf", Contents: constModelSDFFileContents},
	}
	// These model files are within a singleroot folder to always test the server
	// being able to handle single folder uploads.
	var modelFiles = []gztest.FileDesc{
		{Path: "singleroot/model.config", Contents: constModelConfigFileContents},
		{Path: "singleroot/model.sdf", Contents: constModelSDFFileContents},
		{Path: "singleroot/subfolder/test.txt", Contents: "test string"},
	}

	uri := "/1.0/models"
	testName := t.Name()
	createResourceWithArgs(testName, uri, jwt, extraParams, withThumbnails, t)
	extraParams["name"] = "model2"
	extraParams["description"] = "silly desc"
	createResourceWithArgs(testName, uri, jwt, extraParams, modelFiles, t)
	extraParams["name"] = "model3"
	extraParams["tags"] = "new one"
	createResourceWithArgs(testName, uri, jwt, extraParams, withThumbnails, t)
}

// compares a DB' model VS a Model response (fuel.Model)
func assertFuelModel(actual *fuel.Model, exp *models.Model, t *testing.T) {
	// Check required model fields
	assert.Equal(t, exp.Name, actual.Name)
	assert.Equal(t, exp.Owner, actual.Owner)
	assert.EqualValues(t, exp.Likes, *actual.Likes)
	assert.EqualValues(t, exp.Downloads, *actual.Downloads)
	assert.EqualValues(t, exp.Filesize, *actual.Filesize)
}

// Reads model from DB and checks that its folder exists.
func getOwnerModelFromDb(t *testing.T, owner, name string) *models.Model {
	// Get the created model
	var model models.Model
	err := globals.Server.Db.Preload("Tags").Where("owner = ? AND name = ?", owner, name).Find(&model).Error
	assert.NoError(t, err)
	require.NotNil(t, model.Location)
	// Sanity check: Make sure that the model file exists.
	_, err = os.Stat(*model.Location)
	assert.NoError(t, err, "Model Location does not exist in disk and it should", *model.Location)
	return &model
}

func getModelDownloadsFromDb(t *testing.T, owner, name string) *[]models.ModelDownload {
	model := getOwnerModelFromDb(t, owner, name)
	var mds []models.ModelDownload
	err := globals.Server.Db.Where("model_id = ?", model.ID).Find(&mds).Error
	assert.NoError(t, err, "Unable to read model downloads from db: %s %s", owner, name)
	return &mds
}

// resourceSearchTest defines a TestGetModels test case.
// It can be used to test model searches, get all models and get owner models routes, with pagination too.
// Also to test list of liked models.
type resourceSearchTest struct {
	uriTest
	// expected models count in response
	expCount int
	// expected model's name of this first returned model
	expFirstName string
	// expected Link headers
	expLink string
}

func TestGetModels(t *testing.T) {
	// General test setup
	setup()
	// Create a user and test model (the user will be removed later, as part of tests)
	testUser := createUser(t)
	createThreeTestModels(t, nil)
	// create another user, with no models for now
	jwt2 := createValidJWTForIdentity("another-user", t)
	testUser2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(testUser2, jwt2, t)

	uri := "/1.0/models"
	ownerURI := "/1.0/" + testUser + "/models"
	likedURI := "/1.0/" + testUser + "/likes/models"

	modelSearchTestsData := []resourceSearchTest{
		// MODELS
		{uriTest{"all", uri, nil, nil, false}, 3, "model3", ""},
		{uriTest{"all ASC order", uri + "?order=asc", nil, nil, false}, 3, "model1", ""},
		{uriTest{"a search", uri + "?q=model2", nil, nil, false}, 1, "model2", ""},
		{uriTest{"empty search query", uri + "?q=", nil, nil, false}, 3, "model3", ""},
		{uriTest{"match a tag", uri + "?q=new", nil, nil, false}, 1, "model3", ""},
		{uriTest{"match a tag and model name", uri + "?q=one model2&order=asc", nil, nil, false}, 2, "model2", ""},
		{uriTest{"match model description", uri + "?q=description", nil, nil, false}, 1, "model1", ""},
		// MODELS FROM OWNER
		{uriTest{"owner's models", ownerURI + "?order=asc", nil, nil, false}, 3, "model1", ""},
		// PAGINATION
		{uriTest{"get page #1", uri + "?order=asc&per_page=1&page=1", nil, nil, false}, 1, "model1",
			"</1.0/models?order=asc&page=2&per_page=1>; rel=\"next\", </1.0/models?order=asc&page=3&per_page=1>; rel=\"last\""},
		{uriTest{"get page #2", uri + "?order=asc&per_page=1&page=2", nil, nil, false}, 1, "model2",
			"</1.0/models?order=asc&page=3&per_page=1>; rel=\"next\", </1.0/models?order=asc&page=3&per_page=1>; rel=\"last\", </1.0/models?order=asc&page=1&per_page=1>; rel=\"first\", </1.0/models?order=asc&page=1&per_page=1>; rel=\"prev\""},
		{uriTest{"get page #3", uri + "?order=desc&per_page=1&page=3", nil, nil, false}, 1, "model1",
			"</1.0/models?order=desc&page=1&per_page=1>; rel=\"first\", </1.0/models?order=desc&page=2&per_page=1>; rel=\"prev\""},
		{uriTest{"invalid page", uri + "?per_page=1&page=7", nil, gz.NewErrorMessage(gz.ErrorPaginationPageNotFound), false}, 0, "", ""},
		// LIKED MODELS
		{uriTest{"liked models with non-existent user", "/1.0/invaliduser/likes/models", nil, gz.NewErrorMessage(gz.ErrorUserUnknown), false}, 0, "", ""},
		{uriTest{"liked models OK but empty", likedURI, nil, nil, false}, 0, "", ""},
	}

	user2NoModels := []resourceSearchTest{
		{uriTest{"user2 no models", "/1.0/" + testUser2 + "/models", nil, nil, false}, 0, "", ""},
	}

	myJWT := os.Getenv("IGN_TEST_JWT")
	defaultJWT := newJWT(myJWT)

	for _, test := range append(modelSearchTestsData, user2NoModels...) {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithModelSearchTestData(t, test)
		})
		// Now run the same test case but adding a JWT, if needed
		if test.jwtGen == nil {
			test.jwtGen = defaultJWT
			test.testDesc += "[with JWT]"
			t.Run(test.testDesc, func(t *testing.T) {
				runSubtestWithModelSearchTestData(t, test)
			})
		}
	}

	// Remove the user and run the tests again
	removeUser(testUser, t)
	for _, test := range modelSearchTestsData {
		test.testDesc += "[testUser removed]"
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithModelSearchTestData(t, test)
		})
	}

	// create some models for user2 too, and also perform some LIKE operations
	createThreeTestModels(t, &jwt2)

	// create another user since user1 was removed
	jwt3 := createValidJWTForIdentity("user3", t)
	testUser3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(testUser3, jwt3, t)

	m2Likes := "/1.0/" + testUser2 + "/models/model2/likes"
	gztest.AssertRouteMultipleArgs("POST", m2Likes, nil, http.StatusOK, &jwt3, "text/plain; charset=utf-8", t)

	user2ModelsTestsData := []resourceSearchTest{
		{uriTest{"all user2 models", "/1.0/" + testUser2 + "/models", nil, nil, false}, 3, "model3", ""},
		{uriTest{"liked models by testUser1 is empty", likedURI, nil, nil, false}, 0, "", ""},
		{uriTest{"liked models by testUser3 has model2", "/1.0/" + testUser3 + "/likes/models", nil, nil, false}, 1, "model2", ""},
	}

	for _, test := range user2ModelsTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithModelSearchTestData(t, test)
		})
	}
}

func TestGetPrivateModels(t *testing.T) {
	// General test setup
	setup()

	// create user 1
	jwt1 := createValidJWTForIdentity("user1", t)
	testUser1 := createUserWithJWT(jwt1, t)
	defer removeUserWithJWT(testUser1, jwt1, t)

	// create user 2
	jwt2 := createValidJWTForIdentity("user2", t)
	testUser2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(testUser2, jwt2, t)

	// create user 3
	jwt3 := createValidJWTForIdentity("user3", t)
	testUser3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(testUser3, jwt3, t)

	// create test user with default jwt
	jwt := os.Getenv("IGN_TEST_JWT")
	username := createUser(t)
	defer removeUser(username, t)

	// create test organization
	org := createOrganization(t)
	defer removeOrganization(org, t)

	// create a private model for user1
	createTestModelWithOwner(t, &jwt1, "private_model1", testUser1, true)

	// create public and private models for user2
	createTestModelWithOwner(t, &jwt2, "public_model2", testUser2, false)
	createTestModelWithOwner(t, &jwt2, "public_model2a", testUser2, false)
	createTestModelWithOwner(t, &jwt2, "private_model2", testUser2, true)
	createTestModelWithOwner(t, &jwt2, "private_model2a", testUser2, true)

	// create private model for org
	createTestModelWithOwner(t, &jwt, "private_org_model", org, true)
	addUserToOrg(testUser3, "member", org, t)

	userPrivateModelsTestsData := []resourceSearchTest{
		{uriTest{"anonymous user can see only public models", "/1.0/models", nil, nil, false}, 2, "public_model2a", ""},
		{uriTest{"user1 can see public models and own private model", "/1.0/models", newJWT(jwt1), nil, false}, 3, "public_model2a", ""},
		{uriTest{"user2 can see public models and own private models", "/1.0/models", newJWT(jwt2), nil, false}, 4, "private_model2a", ""},
		{uriTest{"member user3 can see public models and org private model", "/1.0/models", newJWT(jwt3), nil, false}, 3, "private_org_model", ""},
		{uriTest{"user1 can see own private model", "/1.0/" + testUser1 + "/models", newJWT(jwt1), nil, false}, 1, "private_model1", ""},
		{uriTest{"user2 can see own public and private models", "/1.0/" + testUser2 + "/models", newJWT(jwt2), nil, false}, 4, "private_model2a", ""},
		{uriTest{"member user3 can see org private model", "/1.0/" + org + "/models", newJWT(jwt3), nil, false}, 1, "private_org_model", ""},
		{uriTest{"member user3 has no models", "/1.0/" + testUser3 + "/models", newJWT(jwt3), nil, false}, 0, "", ""},
		{uriTest{"anonymous user can not see user1 private model", "/1.0/" + testUser1 + "/models", nil, nil, false}, 0, "", ""},
		{uriTest{"anonymous user can see user2 public models", "/1.0/" + testUser2 + "/models", nil, nil, false}, 2, "public_model2a", ""},
		{uriTest{"anonymous user can not see org private models", "/1.0/" + org + "/models", nil, nil, false}, 0, "", ""},
		{uriTest{"user2 can not see user1 private model", "/1.0/" + testUser1 + "/models", newJWT(jwt2), nil, false}, 0, "", ""},
		{uriTest{"user1 can see user2 public models", "/1.0/" + testUser2 + "/models", newJWT(jwt1), nil, false}, 2, "public_model2a", ""},
		{uriTest{"user2 can not see org private models", "/1.0/" + org + "/models", newJWT(jwt2), nil, false}, 0, "", ""},
	}

	for _, test := range userPrivateModelsTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithModelSearchTestData(t, test)
		})
	}
}

// runSubtestWithModelSearchTestData helper function that contains subtest code
func runSubtestWithModelSearchTestData(t *testing.T, test resourceSearchTest) {
	// FIXME(patricio): find a way to reuse this code in all tests (uriTest based)
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	reqArgs := gztest.RequestArgs{Method: "GET", Route: test.URL, Body: nil, SignedToken: jwt}
	resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	assert.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		gztest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var models []*fuel.Model
		assert.NoError(t, json.Unmarshal(*bslice, &models), "Unable to get all models: %s", string(*bslice))
		require.Len(t, models, test.expCount, "There should be %d Models. Got: %d", test.expCount, len(models))
		if test.expCount > 0 {
			firstModel := models[0]
			exp := test.expFirstName
			assert.Equal(t, exp, *firstModel.Name, "Model name [%s] is not the expected one [%s]", *firstModel.Name, exp)
		}
		// Link header should NOT be expected if the expected link was empty.
		// Note: Using Header().Get() returns an empty string if the Header is not present.
		// To verify if the header is present, we need to check the map directly.
		respRec := resp.RespRecorder
		assert.False(t, test.expLink == "" && len(respRec.Header()["Link"]) > 0, "Link header should not be present. Got: %s", respRec.Header()["Link"])
		assert.Equal(t, test.expLink, respRec.Header().Get("Link"), "Expected Link header[%s] != [%s]", test.expLink, respRec.Header().Get("Link"))
	}
}

// modelLikeTest defines a modelLike creation or deletion test case.
type modelLikeTest struct {
	uriTest
	// method: expected POST or DELETE
	method string
	// username and modelname are used to look for the model in DB.
	username  string
	modelname string
	// expected likes (after)
	expLikes int
}

// TestModelLikeCreate checks the model like route is valid
func TestModelLikeCreateAndDelete(t *testing.T) {
	setup()
	myJWT := os.Getenv("IGN_TEST_JWT")
	defaultJWT := newJWT(myJWT)

	// Create random user and some models
	username := createUser(t)
	defer removeUser(username, t)
	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	// Create another user and add to org
	jwt2 := createValidJWTForIdentity("another-user-2", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)
	addUserToOrg(user2, "member", testOrg, t)
	// create another user, non member
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)

	createThreeTestModels(t, nil)
	// create private asset owned by user
	createTestModelWithOwner(t, &myJWT, "user_private", username, true)
	// create private asset owned by org
	createTestModelWithOwner(t, &myJWT, "org_private", testOrg, true)

	m1URI := modelURL(username, "model1", "")
	puModel := modelURL(username, "user_private", "")
	orgModel := modelURL(testOrg, "org_private", "")
	jwt4 := createValidJWTForIdentity("unexistent-user", t)

	modelLikeTestData := []modelLikeTest{
		{uriTest{"like no jwt", m1URI + "/likes", nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "POST", "", "", 0},
		{uriTest{"invalid jwt", m1URI + "/likes", newJWT("invalid"), gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "POST", "", "", 0},
		{uriTest{"non-existent user jwt", m1URI + "/likes", newJWT(jwt4), gz.NewErrorMessage(gz.ErrorAuthNoUser), false}, "POST", "", "", 0},
		{uriTest{"non-existent model", modelURL(username, "non-existent-model", "") + "/likes", defaultJWT, gz.NewErrorMessage(gz.ErrorNameNotFound), false}, "POST", "", "", 0},
		{uriTest{"valid public asset like from another user ", m1URI + "/likes", newJWT(jwt3), nil, false}, "POST", username, "model1", 1},
		{uriTest{"user cannot like model twice", m1URI + "/likes", newJWT(jwt3), gz.NewErrorMessage(gz.ErrorDbSave), false}, "POST", "", "", 0},
		{uriTest{"cannot like user private asset with no jwt", puModel + "/likes", nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "POST", "", "", 0},
		{uriTest{"cannot like user private asset with another jwt", puModel + "/likes", newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "POST", "", "", 0},
		{uriTest{"cannot like org private asset with no jwt", orgModel + "/likes", nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "POST", "", "", 0},
		{uriTest{"valid private org asset like by member", orgModel + "/likes", newJWT(jwt2), nil, false}, "POST", testOrg, "org_private", 1},
		{uriTest{"cannot like org private asset by non member", orgModel + "/likes", newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "POST", "", "", 0},
		// DELETE tests
		{uriTest{"unlike no jwt", m1URI + "/likes", nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "DELETE", "", "", 0},
		{uriTest{"unlike invalid jwt", m1URI + "/likes", newJWT("invalid"), gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "DELETE", "", "", 0},
		{uriTest{"unlike with non-existent user jwt", m1URI + "/likes", newJWT(jwt4), gz.NewErrorMessage(gz.ErrorAuthNoUser), false}, "DELETE", "", "", 0},
		{uriTest{"unlike non-existent model", modelURL(username, "non-existent-model", "") + "/likes", defaultJWT, gz.NewErrorMessage(gz.ErrorNameNotFound), false}, "DELETE", "", "", 0},
		{uriTest{"valid public asset unlike", m1URI + "/likes", newJWT(jwt3), nil, false}, "DELETE", username, "model1", 0},
		{uriTest{"valid public asset unlike twice", m1URI + "/likes", newJWT(jwt3), nil, false}, "DELETE", username, "model1", 0},
		{uriTest{"valid unlike of model with no likes", modelURL(username, "model2", "") + "/likes", defaultJWT, nil, false}, "DELETE", username, "model2", 0},
		{uriTest{"cannot unlike user private asset with no jwt", puModel + "/likes", nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "DELETE", "", "", 0},
		{uriTest{"cannot unlike user private asset with another jwt", puModel + "/likes", newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "DELETE", "", "", 0},
		{uriTest{"cannot unlike org private asset with no jwt", orgModel + "/likes", nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "DELETE", "", "", 0},
		{uriTest{"valid unlike of private org asset by member", orgModel + "/likes", newJWT(jwt2), nil, false}, "DELETE", testOrg, "org_private", 0},
		{uriTest{"cannot unlike org private asset by non member", orgModel + "/likes", newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "DELETE", "", "", 0},
	}

	for _, test := range modelLikeTestData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctTextPlain)
			expStatus := expEm.StatusCode
			reqArgs := gztest.RequestArgs{Method: test.method, Route: test.URL, Body: nil, SignedToken: jwt}
			resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			require.Equal(t, expStatus, resp.RespRecorder.Code)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				gztest.AssertBackendErrorCode(t.Name()+" "+test.method, bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				// Verify that the database was updated to reflect the new number of likes
				m := getOwnerModelFromDb(t, test.username, test.modelname)
				assert.NotNil(t, m)
				assert.Equal(t, test.expLikes, m.Likes, "Model's like counter [%d] should be equal to exp: [%d]", m.Likes, test.expLikes)
			}
		})
	}
}

func TestModelLikeCreateDbMock(t *testing.T) {
	// General test setup
	setup()

	origDb := globals.Server.Db
	// Make sure to return back to real DB after running this test
	defer SetGlobalDB(origDb)

	// Setup DB mock
	mockDb := SetupDbMockCatcher()
	SetGlobalDB(mockDb)
	SetupCommonMockResponses("test user")

	// Make request as usual
	uri := "/1.0/testUser/models/testModel/likes"
	myJWT := os.Getenv("IGN_TEST_JWT")

	// Test bad connection at Begin() tx
	SetGlobalDB(NewFailAtBeginConn())
	expErr := gz.ErrorMessage(gz.ErrorNoDatabase)
	// Try to like the model with a valid a JWT token.
	bslice, _ := gztest.AssertRouteMultipleArgs("POST", uri, nil, expErr.StatusCode, &myJWT, "text/plain; charset=utf-8", t)
	gztest.AssertBackendErrorCode("TestModelLikeCreateDbMock", bslice, expErr.ErrCode, t)

	// Test failure at TX commit
	SetGlobalDB(mockDb)
	SetupMockCountModelLikes()
	SetupMockBadCommit()
	mocket.Catcher.NewMock().WithQuery("SELECT count(*) FROM \"model_likes\"  WHERE").WithRowsNum(1).WithReply([]map[string]interface{}{{"count": "1"}})
	expErr = gz.ErrorMessage(gz.ErrorDbSave)
	bslice, _ = gztest.AssertRouteMultipleArgs("POST", uri, nil, expErr.StatusCode, &myJWT, "text/plain; charset=utf-8", t)
	gztest.AssertBackendErrorCode("TestModelLikeCreateDbMock", bslice, expErr.ErrCode, t)

	// Test failure when updateModelLikeCounter returns error
	SetupCommonMockResponses("test user")
	ClearMockBadCommit()
	// Make the Count DB query fail
	mocket.Catcher.NewMock().WithQuery("SELECT count(*) FROM \"model_likes\"  WHERE").WithQueryException()
	expErr = gz.ErrorMessage(gz.ErrorDbSave)
	bslice, _ = gztest.AssertRouteMultipleArgs("POST", uri, nil, expErr.StatusCode, &myJWT, "text/plain; charset=utf-8", t)
	gztest.AssertBackendErrorCode("TestModelLikeCreateDbMock", bslice, expErr.ErrCode, t)
}

// TestAPIModel checks the route that describes the model API
func TestAPIModel(t *testing.T) {
	// General test setup
	setup()

	code := http.StatusOK
	if globals.Server.Db == nil {
		code = gz.ErrorMessage(gz.ErrorNoDatabase).StatusCode
	}

	uri := "/1.0/models"
	gztest.AssertRoute("OPTIONS", uri, code, t)
}

// modelIndexTest defines a TestGetOwnerModel test case.
type modelIndexTest struct {
	uriTest
	// expected model owner
	expOwner string
	// expected model name
	expName string
	// expected tags
	expTags []string
	// expected thumbnail url
	expThumbURL string
}

func TestGetOwnerModel(t *testing.T) {
	// General test setup
	setup()
	myJWT := os.Getenv("IGN_TEST_JWT")
	defaultJWT := newJWT(myJWT)

	// Create a user and test model
	testUser := createUser(t)
	defer removeUser(testUser, t)

	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	// Create another user and add to org
	jwt2 := createValidJWTForIdentity("another-user-2", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)
	addUserToOrg(user2, "member", testOrg, t)
	// create another user, non member
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	// Create another user and add to org
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)
	addUserToOrg(user4, "admin", testOrg, t)

	// create three standard models
	createThreeTestModels(t, nil)
	createTestModelWithOwner(t, &myJWT, "user_private", testUser, true)
	// create private asset owned by org
	createTestModelWithOwner(t, &myJWT, "private", testOrg, true)

	// create a model with name containing special characters
	modelSpecialCharName := "testmodel#hash"
	createTestModelWithOwner(t, nil, modelSpecialCharName, testUser, false)

	// standard model thumbnail url
	expThumbURL := fmt.Sprintf("/%s/models/%s/tip/files/%s", testUser, "model1",
		"thumbnails/model.sdf")

	// thumbnail url of model with special character name
	expSpecialCharThumbURL := fmt.Sprintf("/%s/models/%s/tip/files/%s", testUser,
		url.PathEscape(modelSpecialCharName), "thumbnails/model.sdf")

	expPrivateThumbURL := fmt.Sprintf("/%s/models/%s/tip/files/%s", testOrg,
		"private", "thumbnails/model.sdf")

	expTags := []string{"test_tag_1", "test_tag2"}
	modelIndexTestData := []modelIndexTest{
		{uriTest{"get model", modelURL(testUser, "model1", ""), nil, nil, false}, testUser, "model1", expTags, expThumbURL},
		{uriTest{"get model with no thumbnails", modelURL(testUser, "model2", ""), nil, nil, false}, testUser, "model2", expTags, ""},
		{uriTest{"invalid name", modelURL(testUser, "invalidname", ""), nil, gz.NewErrorMessage(gz.ErrorNameNotFound), false}, "", "", nil, ""},
		{uriTest{"get model with special char", modelURL(testUser, modelSpecialCharName, ""), nil, nil, false}, testUser, modelSpecialCharName, expTags, expSpecialCharThumbURL},
		{uriTest{"get private org model by org owner", modelURL(testOrg, "private", ""), defaultJWT, nil, false}, testOrg, "private", expTags, expPrivateThumbURL},
		{uriTest{"get private org model by admin", modelURL(testOrg, "private", ""), newJWT(jwt4), nil, false}, testOrg, "private", expTags, expPrivateThumbURL},
		{uriTest{"get private org model by member", modelURL(testOrg, "private", ""), newJWT(jwt2), nil, false}, testOrg, "private", expTags, expPrivateThumbURL},
		{uriTest{"get private org model by non member", modelURL(testOrg, "private", ""), newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, "", "", nil, ""},
		{uriTest{"get private user model with another jwt ", modelURL(testUser, "user_private", ""), newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, "", "", nil, ""},
	}

	for _, test := range modelIndexTestData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithModelIndexTestData(t, test)
		})
		// Now run the same test case but adding a JWT, if needed
		if test.jwtGen == nil {
			test.jwtGen = defaultJWT
			test.testDesc += "[with JWT]"
			t.Run(test.testDesc, func(t *testing.T) {
				runSubtestWithModelIndexTestData(t, test)
			})
		}
	}
}

// runSubtestWithModelSearchTestData helper function that contains subtest code
func runSubtestWithModelIndexTestData(t *testing.T, test modelIndexTest) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	reqArgs := gztest.RequestArgs{Method: "GET", Route: test.URL, Body: nil, SignedToken: jwt}
	resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		gztest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var gotModel fuel.Model
		assert.NoError(t, json.Unmarshal(*bslice, &gotModel), "Unable to unmarshal the model: %s", string(*bslice))
		// Also make sure the model's name is the one we expect
		assert.Equal(t, test.expOwner, *gotModel.Owner, "Got Model owner [%s] is not the expected one [%s]", *gotModel.Owner, test.expOwner)
		// check version info is also available and has value "1"
		assert.EqualValues(t, 1, *gotModel.Version, "Got Model version [%d] is not the expected version [%d]", *gotModel.Version, 1)
		// compare with db model
		model := getOwnerModelFromDb(t, test.expOwner, test.expName)
		assertFuelModel(&gotModel, model, t)
		actualTags := models.TagsToStrSlice(model.Tags)
		assert.True(t, gz.SameElements(test.expTags, actualTags), "Returned Tags are not the expected. Expected: %v. Got: %v", test.expTags, actualTags)
		// check expected thumbnails
		if test.expThumbURL == "" {
			assert.Nil(t, gotModel.ThumbnailUrl)
		} else {
			require.NotNil(t, gotModel.ThumbnailUrl)
			assert.Equal(t, test.expThumbURL, *gotModel.ThumbnailUrl, "Got thumbanil url [%s] is different than expected [%s]", *gotModel.ThumbnailUrl, test.expThumbURL)
		}
		// Test the model was stored at `IGN_FUEL_RESOURCE_DIR/{user}/models/{uuid}`
		expectedPath := path.Join(globals.ResourceDir, test.expOwner, "models", *model.UUID)
		assert.Equal(t, expectedPath, *model.Location, "Model Location [%s] is not the expected [%s]", *model.Location, expectedPath)
	}
}

// modelDownloadAsZipTest defines a download model as zip file test case.
type modelDownloadAsZipTest struct {
	uriTest
	owner string
	name  string
	// the expected returned model version, in the X-Ign-Resource-Version header. Must be a number
	ignVersionHeader int
	// a map containing files that should be present in the returned zip. Eg. {"model.sdf":true, "model.config":true}
	expZipFiles map[string]bool
	// expected downloads count for this zip (after downloading it). Note: this makes the test cases to be dependent among them.
	expDownloads int
	// expected username of the user that performed this download. Can be empty.
	expDownloadUsername string
}

// TestGetModelAsZip checks if we can get models as zip files
func TestGetModelAsZip(t *testing.T) {
	// General test setup
	setup()
	myJWT := os.Getenv("IGN_TEST_JWT")
	// Create a user and test model
	testUser := createUser(t)
	defer removeUser(testUser, t)
	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	// Create another user and add to org
	jwt2 := createValidJWTForIdentity("another-user-2", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)
	addUserToOrg(user2, "member", testOrg, t)
	// create another user, non member
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	// Create another user and add to org
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)
	addUserToOrg(user4, "admin", testOrg, t)

	// create assets
	createThreeTestModels(t, nil)
	createTestModelWithOwner(t, &myJWT, "user_private", testUser, true)
	// create private asset owned by org
	createTestModelWithOwner(t, &myJWT, "private", testOrg, true)

	// Get the created model
	model := getOwnerModelFromDb(t, testUser, "model1")
	files := map[string]bool{"thumbnails/": true, "thumbnails/model.sdf": true, "model.config": true}

	// Now check we can get the model as zip file using different uris
	modelDownloadAsZipTestsData := []modelDownloadAsZipTest{
		{uriTest{"/owner/models/name style", modelURL(testUser, *model.Name, ""), &testJWT{jwt: &myJWT}, nil, false}, testUser, *model.Name, 1, files, 1, testUser},
		{uriTest{"with explicit model version", modelURL(testUser, *model.Name, "1"), &testJWT{jwt: &myJWT}, nil, false}, testUser, *model.Name, 1, files, 2, testUser},
		{uriTest{"with no JWT", modelURL(testUser, *model.Name, "tip"), nil, nil, false}, testUser, *model.Name, 1, files, 3, ""},
		{uriTest{"invalid (negative) version", modelURL(testUser, *model.Name, "-4"), nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, testUser, *model.Name, 1, files, 3, ""},
		{uriTest{"invalid (alpha) version", modelURL(testUser, *model.Name, "a"), nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, testUser, *model.Name, 1, files, 3, ""},
		{uriTest{"0 version", modelURL(testUser, *model.Name, "0"), nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, testUser, *model.Name, 1, files, 3, ""},
		{uriTest{"version not found", modelURL(testUser, *model.Name, "5"), nil, gz.NewErrorMessage(gz.ErrorVersionNotFound), false}, testUser, *model.Name, 1, files, 3, ""},
		{uriTest{"get private org model by org owner", modelURL(testOrg, "private", ""), &testJWT{jwt: &myJWT}, nil, false}, testOrg, "private", 1, files, 1, testUser},
		{uriTest{"get private org model by admin", modelURL(testOrg, "private", ""), newJWT(jwt4), nil, false}, testOrg, "private", 1, files, 2, user4},
		{uriTest{"get private org model by member", modelURL(testOrg, "private", ""), newJWT(jwt2), nil, false}, testOrg, "private", 1, files, 3, user2},
		{uriTest{"get private org model by non member", modelURL(testOrg, "private", ""), newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, testOrg, "", 1, files, 2, ""},
		{uriTest{"get private org model with no jwt", modelURL(testOrg, "private", ""), nil, gz.NewErrorMessage(gz.ErrorUnauthorized), false}, testOrg, "", 1, files, 2, ""},
		{uriTest{"get private user model with no jwt", modelURL(testUser, "user_private", ""), nil, gz.NewErrorMessage(gz.ErrorUnauthorized), false}, testOrg, "", 1, files, 2, ""},
		{uriTest{"get private user model with another jwt", modelURL(testUser, "user_private", ""), newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, testOrg, "", 1, files, 2, ""},
	}

	for _, test := range modelDownloadAsZipTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctZip)
			expStatus := expEm.StatusCode
			reqArgs := gztest.RequestArgs{Method: "GET", Route: test.URL + ".zip", Body: nil, SignedToken: jwt}
			resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			assert.Equal(t, expStatus, resp.RespRecorder.Code)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				gztest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				assert.True(t, resp.Ok, "Model Zip Download request didn't succeed")
				ensureIgnResourceVersionHeader(resp.RespRecorder, test.ignVersionHeader, t)
				zSize := len(*bslice)
				zipReader, err := zip.NewReader(bytes.NewReader(*bslice), int64(zSize))
				assert.NoError(t, err, "Unable to read zip response")
				assert.NotEmpty(t, zipReader.File, "Got zip file did not have any files")
				for _, f := range zipReader.File {
					assert.True(t, test.expZipFiles[f.Name], "Got Zip file not included in expected files: %s", f.Name)
				}
				m := getOwnerModelFromDb(t, test.owner, test.name)
				assert.Equal(t, zSize, m.Filesize, "Zip file size (%d) is not equal to Model's Filesize field (%d)", zSize, m.Filesize)
				assert.Equal(t, test.expDownloads, m.Downloads, "Downloads counter should be %d. Got: %d", test.expDownloads, m.Downloads)
				mds := getModelDownloadsFromDb(t, test.owner, test.name)
				assert.Len(t, *mds, test.expDownloads, "Model Downloads length should be %d. Got %d", test.expDownloads, len(*mds))
				// get the user that made 'this' current download (the latest)
				pUserID := (*mds)[len(*mds)-1].UserID
				if test.expDownloadUsername == "" {
					assert.Nil(t, pUserID, "download user should be nil")
				} else {
					assert.NotNil(t, pUserID, "download user should NOT be nil. Expected username was: %s", test.expDownloadUsername)
					if pUserID != nil {
						us := dbGetUserByID(*pUserID)
						assert.Equal(t, test.expDownloadUsername, *us.Username, "download user [%s] was expected to be [%s]", *us.Username, test.expDownloadUsername)
					}
				}
				ua := (*mds)[len(*mds)-1].UserAgent
				assert.Empty(t, ua, "Model Download should have an empty UserAgent: %s", ua)
			}
		})
	}
}

// TestReportModelCreate checks the report model route is valid
func TestReportModelCreate(t *testing.T) {
	// General test setup
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	testUser := createUser(t)
	defer removeUser(testUser, t)
	createThreeTestModels(t, nil)
	// Sanity check: Get the created model to ensure it was created.
	model := getOwnerModelFromDb(t, testUser, "model1")

	uri := fmt.Sprintf("%s/report", modelURL(testUser, *model.Name, ""))

	body := map[string]string{"reason": "test"}

	// First, disable mail support and restore it in defer
	from := globals.FlagsEmailSender
	defer func() { globals.FlagsEmailSender = from }()
	globals.FlagsEmailSender = ""

	// Try to report a non-existent model.
	testURI := fmt.Sprintf("%s/report", modelURL(testUser, "non-existent-model", ""))
	expErr := gz.ErrorMessage(gz.ErrorNameNotFound)

	_, bslice, _ := gztest.SendMultipartPOST(t.Name(), t, testURI, nil, body, nil)
	gztest.AssertBackendErrorCode(t.Name(), bslice, expErr.ErrCode, t)

	_, bslice, _ = gztest.SendMultipartPOST(t.Name(), t, testURI, &jwt, body, nil)
	gztest.AssertBackendErrorCode(t.Name(), bslice, expErr.ErrCode, t)

	// Try to report the model
	gztest.SendMultipartPOST("ReportModelCreate", t, uri, nil, body, nil)
	gztest.SendMultipartPOST("ReportModelCreate", t, uri, &jwt, body, nil)
}
