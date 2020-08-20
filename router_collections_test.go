package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/collections"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	"gitlab.com/ignitionrobotics/web/fuelserver/proto"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"gitlab.com/ignitionrobotics/web/ign-go/testhelpers"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"
)

// Tests for collections related routes

// addAssetToCollection is a helper function that associates a named asset
// (model / world) to an existing collection.
func addAssetToCollection(t *testing.T, jwt, colOwner, colName, owner, name,
	aType string) {
	nameOwner := collections.NameOwnerPair{name, owner}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(nameOwner)
	uri := fmt.Sprintf("/1.0/%s/collections/%s/%ss", colOwner, colName, aType)
	igntest.AssertRouteMultipleArgs("POST", uri, b, http.StatusOK, &jwt, ctJSON, t)
}

// creates an URL to get a collection
func colURL(owner, name string) string {
	encodedName := url.PathEscape(name)
	return fmt.Sprintf("/%s/%s/collections/%s", apiVersion, owner, encodedName)
}

// creates an URL to get the list of collections associated to a model
func modelColsURI(owner, model string) string {
	encodedName := url.PathEscape(model)
	return fmt.Sprintf("/%s/%s/models/%s/collections", apiVersion, owner, encodedName)
}

// creates an URL to get the list of collections associated to a world
func worldColsURI(owner, world string) string {
	encodedName := url.PathEscape(world)
	return fmt.Sprintf("/%s/%s/worlds/%s/collections", apiVersion, owner, encodedName)
}

// createTestCollectionWithOwner is a helper function to create a collection given
// an optional jwt, a name, and an owner name (org or user). If owner is an empty
// string then the jwt user will be the owner.
func createTestCollectionWithOwner(t *testing.T, jwt *string, name,
	owner, desc string, private bool) {

	cc := collections.CreateCollection{Name: name, Owner: owner, Description: desc,
		Private: &private}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(cc)
	igntest.AssertRouteMultipleArgs("POST", "/1.0/collections", b, http.StatusOK, jwt, ctJSON, t)
}

// Create a random public collection under the default user.
// PRE-REQ: a user with the default JWT should have been created before.
func createCollection(t *testing.T) string {
	jwt := os.Getenv("IGN_TEST_JWT")
	name := ign.RandomString(8)
	description := "a random collection"
	createTestCollectionWithOwner(t, &jwt, name, "", description, false)
	return name
}

// Remove a collection
func removeCollection(t *testing.T, owner, name string, jwt *string) {

	// Find the collection
	col, _ := collections.ByName(globals.Server.Db, name, owner)
	require.NotNil(t, col, "removeCollection error: Unable to find collection[%s/%s] in DB", owner, name)

	uri := colURL(owner, name)
	igntest.AssertRouteMultipleArgs("DELETE", uri, nil, http.StatusOK, jwt, ctJSON, t)

	// make sure the collection is not in DB anymore
	col, _ = collections.ByName(globals.Server.Db, name, owner)
	require.Nil(t, col, "removeCollection error: collection[%s/%s] is still present in DB", owner, name)
}

type collectionsSearchTest struct {
	uriTest
	// expected count in response
	expCount int
	// expected name of this first returned collection
	expFirstName string
	// expected Link headers
	expLink string
	// expected Thumbnails of the first returned collection.
	// Values: nil (ignore), [] (no exp thubnails), full slice (exp values)
	expThumbnailUrls []string
}

// TODO(patricio): MERGE this with TestGetModels. Consider using an interface to unify some
// comparison.
func TestGetCollections(t *testing.T) {
	// General test setup
	setup()
	myJWT := os.Getenv("IGN_TEST_JWT")
	defaultJWT := newJWT(myJWT)

	// Create a user and collections
	testUser := createUser(t)
	cName1 := createCollection(t)
	// NOTE: we don't remove the collections because we will remove the user first
	// and thus, the removeCollection will fail with Not Authorized error.
	cName2 := createCollection(t)
	// create public collection
	createTestCollectionWithOwner(t, &myJWT, "test3", "", "new descr", false)

	// note: this creates models named model1, model2 and model3
	createThreeTestModels(t, nil)
	createThreeTestWorlds(t, nil)
	// manually add model1 and world2 to col1
	addAssetToCollection(t, myJWT, testUser, cName1, testUser, "model1", "model")
	addAssetToCollection(t, myJWT, testUser, cName1, testUser, "world2", "world")
	thumb1 := fmt.Sprintf("/%s/models/%s/tip/files/%s", testUser, "model1", "thumbnails/model.sdf")

	// createa another user
	jwt2 := createValidJWTForIdentity("another-user", t)
	testUser2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(testUser2, jwt2, t)

	uri := "/1.0/collections"
	ownerURI := "/1.0/" + testUser + "/collections"

	searchTestsData := []collectionsSearchTest{
		// Collections
		{uriTest{"get empty result", uri + "?q=thiswontmatch", nil, nil, false}, 0,
			"", "", nil},
		{uriTest{"get empty result with noft search", uri + "?q=:noft:thiswontmatch",
			nil, nil, false}, 0, "", "", nil},
		{uriTest{"all", uri, nil, nil, false}, 3, "test3", "", nil},
		{uriTest{"all ASC order", uri + "?order=asc", nil, nil, false}, 3, cName1, "",
			[]string{thumb1}},
		{uriTest{"a search", uri + "?q=" + cName2, nil, nil, false}, 1, cName2, "", nil},
		{uriTest{"empty search query", uri + "?q=", nil, nil, false}, 3, "test3", "", nil},
		{uriTest{"match description", uri + "?q=new descr", nil, nil, false}, 1, "test3", "", nil},
		{uriTest{"wrong position of :noft:", uri + "?q=new descr :noft:", nil, nil, false}, 1, "test3", "", nil},
		{uriTest{"with simple search #1", uri + "?q=:noft:test", nil, nil, false}, 1, "test3", "", nil},
		{uriTest{"with simple search #2", uri + "?q=:noft:st3", newJWT(jwt2),
			nil, false}, 1, "test3", "", nil},
		{uriTest{"with simple search #2 and extend", uri + "?q=:noft:st3&extend=true", newJWT(jwt2),
			nil, false}, 0, "", "", nil},
		{uriTest{"with simple search #3", uri + "?q=:noft:est4", nil, nil, false}, 0, "", "", nil},
		// Collections FROM OWNER
		{uriTest{"owner's collections", ownerURI + "?order=asc", nil, nil, false}, 3,
			cName1, "", []string{thumb1}},
		{uriTest{"owner's collections with other jwt", ownerURI + "?order=asc",
			newJWT(jwt2), nil, false}, 3, cName1, "", nil},
		// Collections associated to assets
		{uriTest{"model1's collections", modelColsURI(testUser, "model1"), nil, nil,
			false}, 1, cName1, "", []string{thumb1}},
		{uriTest{"inv model's collections", modelColsURI(testUser, "inv"), nil,
			ign.NewErrorMessage(ign.ErrorNameNotFound), true}, 1, "", "", nil},
		{uriTest{"world1's collections should be empty", worldColsURI(testUser, "world1"),
			nil, nil, false}, 0, "", "", nil},
		{uriTest{"world2's collections", worldColsURI(testUser, "world2"), nil, nil,
			false}, 1, cName1, "", []string{thumb1}},
		{uriTest{"inv world's collections", worldColsURI(testUser, "inv"), nil,
			ign.NewErrorMessage(ign.ErrorNameNotFound), true}, 1, "", "", nil},
		// PAGINATION
		{uriTest{"get page #1", uri + "?order=asc&per_page=1&page=1", nil, nil, false}, 1, cName1,
			"</1.0/collections?order=asc&page=2&per_page=1>; rel=\"next\", </1.0/collections?order=asc&page=3&per_page=1>; rel=\"last\"", nil},
		{uriTest{"get page #2", uri + "?order=asc&per_page=1&page=2", nil, nil, false}, 1, cName2,
			"</1.0/collections?order=asc&page=3&per_page=1>; rel=\"next\", </1.0/collections?order=asc&page=3&per_page=1>; rel=\"last\", </1.0/collections?order=asc&page=1&per_page=1>; rel=\"first\", </1.0/collections?order=asc&page=1&per_page=1>; rel=\"prev\"", nil},
		{uriTest{"get page #3", uri + "?order=desc&per_page=1&page=3", nil, nil, false}, 1, cName1,
			"</1.0/collections?order=desc&page=1&per_page=1>; rel=\"first\", </1.0/collections?order=desc&page=2&per_page=1>; rel=\"prev\"", nil},
		{uriTest{"invalid page", uri + "?per_page=1&page=7", nil, ign.NewErrorMessage(ign.ErrorPaginationPageNotFound), false}, 0, "", "", nil},
	}

	user2NoCollections := []collectionsSearchTest{
		{uriTest{"user2 with no collections", "/1.0/" + testUser2 + "/collections", nil, nil, false}, 0, "", "", nil},
	}

	for _, test := range append(searchTestsData, user2NoCollections...) {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithCollectionSearchTestData(t, test)
		})
		// Now run the same test case but adding a JWT, if needed
		if test.jwtGen == nil {
			test.jwtGen = defaultJWT
			test.testDesc += "[with JWT]"
			t.Run(test.testDesc, func(t *testing.T) {
				runSubtestWithCollectionSearchTestData(t, test)
			})
		}
	}

	// Remove the user and run the tests again
	removeUser(testUser, t)
	for _, test := range searchTestsData {
		test.testDesc += "[testUser removed]"
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithCollectionSearchTestData(t, test)
		})
	}

	// create some collections for user2
	createTestCollectionWithOwner(t, &jwt2, "test4", "", "user2 desc", false)
	defer removeCollection(t, testUser2, "test4", &jwt2)

	user2TestsData := []collectionsSearchTest{
		{uriTest{"all user2 collections", "/1.0/" + testUser2 + "/collections", nil, nil, false}, 1, "test4", "", nil},
	}

	for _, test := range user2TestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithCollectionSearchTestData(t, test)
		})
	}
}

// runSubtestWithCollectionSearchTestData helper function that contains subtest code
func runSubtestWithCollectionSearchTestData(t *testing.T, test collectionsSearchTest) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	igntest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	reqArgs := igntest.RequestArgs{Method: "GET", Route: test.URL, Body: nil, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var cols collections.Collections
		require.NotNil(t, bslice)
		assert.NoError(t, json.Unmarshal(*bslice, &cols), "Unable to get all resources: %s", string(*bslice))
		require.NotNil(t, cols)
		require.Len(t, cols, test.expCount, "There should be %d items. Got: %d", test.expCount, len(cols))
		if test.expCount > 0 {
			first := cols[0]
			exp := test.expFirstName
			assert.Equal(t, exp, *first.Name, "Resource name [%s] is not the expected one [%s]", *first.Name, exp)
			// also make sure private fields are not sent as json
			assert.Empty(t, first.ID)
			assert.Empty(t, first.UUID)
			assert.Empty(t, first.Creator)
		} else {
			require.Equal(t, collections.Collections{}, cols)
		}
		// Link header should NOT be expected if the expected link was empty.
		// Note: Using Header().Get() returns an empty string if the Header is not present.
		// To verify if the header is present, we need to check the map directly.
		respRec := resp.RespRecorder
		assert.False(t, test.expLink == "" && len(respRec.Header()["Link"]) > 0, "Link header should not be present. Got: %s", respRec.Header()["Link"])
		assert.Equal(t, test.expLink, respRec.Header().Get("Link"), "Expected Link header[%s] != [%s]", test.expLink, respRec.Header().Get("Link"))
		// check expected thumbnails
		if test.expThumbnailUrls != nil {
			first := cols[0]
			assert.Len(t, first.ThumbnailUrls, len(test.expThumbnailUrls))
			assert.ElementsMatch(t, test.expThumbnailUrls, first.ThumbnailUrls)
		}
	}
}

func TestGetPrivateCollections(t *testing.T) {
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

	org := createOrganization(t)
	defer removeOrganization(org, t)
	addUserToOrg(testUser3, "member", org, t)

	// create a private collection for user1
	createTestCollectionWithOwner(t, &jwt1, "private_col1", testUser1, "", true)

	// create public collection with an org as owner
	createTestCollectionWithOwner(t, &jwt, "public_org_collection", org, "", false)

	// create public and private collections for user2
	createTestCollectionWithOwner(t, &jwt2, "public_col2", testUser2, "", false)
	createTestCollectionWithOwner(t, &jwt2, "public_col2a", testUser2, "", false)
	createTestCollectionWithOwner(t, &jwt2, "private_col2", testUser2, "", true)
	createTestCollectionWithOwner(t, &jwt2, "private_col2a", testUser2, "", true)

	// create private collection with an org as owner
	createTestCollectionWithOwner(t, &jwt, "private_org_collection", org, "", true)

	// note: this creates models named model1, model2 and model3
	createThreeTestModels(t, nil)
	// manually add model1 to private collection col1
	addAssetToCollection(t, jwt, org, "private_org_collection", username, "model1", "model")
	thumb1 := fmt.Sprintf("/%s/models/%s/tip/files/%s", username, "model1", "thumbnails/model.sdf")
	// manually add a user2's private model to a public collection from user2
	// Test of to https://app.asana.com/0/750270806527182/885846144757573
	createTestModelWithOwner(t, &jwt2, "private_model", testUser2, true)
	addAssetToCollection(t, jwt2, testUser2, "public_col2a", testUser2, "private_model", "model")

	uri := "/1.0/collections"
	userPrivateCollectionsTestsData := []collectionsSearchTest{
		{uriTest{"anonymous user can see only public collection", uri, nil, nil, false}, 3, "public_col2a", "", nil},
		{uriTest{"user1 can see public collections and his own private ones", uri, newJWT(jwt1), nil, false}, 4, "public_col2a", "", []string{}},
		{uriTest{"user1 can 'extend' only own collections", uri + "?extend=true", newJWT(jwt1), nil, false}, 1, "private_col1", "", []string{}},
		{uriTest{"user2 can see public collections and his own private ones", uri, newJWT(jwt2), nil, false}, 5, "private_col2a", "", nil},
		{uriTest{"member user3 can see public collections and org private ones", uri, newJWT(jwt3), nil, false}, 4, "private_org_collection", "", []string{thumb1}},
		{uriTest{"member user3 can 'extend' collections from orgs he belongs", uri + "?extend=true", newJWT(jwt3), nil, false}, 2, "private_org_collection", "", []string{thumb1}},
		{uriTest{"user1 can see own public and private collection", "/1.0/" + testUser1 + "/collections", newJWT(jwt1), nil, false}, 1, "private_col1", "", nil},
		{uriTest{"user2 can see own public and private collections", "/1.0/" + testUser2 + "/collections", newJWT(jwt2), nil, false}, 4, "private_col2a", "", nil},
		{uriTest{"member user3 can see org private collection", "/1.0/" + org + "/collections", newJWT(jwt3), nil, false}, 2, "private_org_collection", "", []string{thumb1}},
		{uriTest{"user3 has no collections", "/1.0/" + testUser3 + "/collections", newJWT(jwt3), nil, false}, 0, "", "", nil},
		{uriTest{"anonymous user can not see user1 private collection", "/1.0/" + testUser1 + "/collections", nil, nil, false}, 0, "", "", nil},
		{uriTest{"anonymous user can see user2 public collections", "/1.0/" + testUser2 + "/collections", nil, nil, false}, 2, "public_col2a", "", nil},
		{uriTest{"anonymous user can not see org private collections", "/1.0/" + org + "/collections", nil, nil, false}, 1, "public_org_collection", "", nil},
		{uriTest{"user2 can not see user1 private collection", "/1.0/" + testUser1 + "/collections", newJWT(jwt2), nil, false}, 0, "", "", nil},
		{uriTest{"user1 can see user2 public collections", "/1.0/" + testUser2 + "/collections", newJWT(jwt1), nil, false}, 2, "public_col2a", "", nil},
		{uriTest{"user2 can not see org private collections", "/1.0/" + org + "/collections", newJWT(jwt2), nil, false}, 1, "public_org_collection", "", nil},
	}

	for _, test := range userPrivateCollectionsTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithCollectionSearchTestData(t, test)
		})
	}
}

// TestAPICollections checks the route that describes the collections API
func TestAPICollections(t *testing.T) {
	// General test setup
	setup()
	uri := "/1.0/collections"
	igntest.AssertRoute("OPTIONS", uri, http.StatusOK, t)
}

// TODO try to MERGE with TestGetOwnerModel.
// collectionIndexTest defines a TestGetCollection test case.
type collectionIndexTest struct {
	uriTest
	// expected owner
	expOwner string
	// expected name
	expName       string
	expThumbnails []string
}

func TestGetCollectionIndex(t *testing.T) {
	// General test setup
	setup()
	// Create test user
	testUser := createUser(t)
	defer removeUser(testUser, t)
	// create a separate user using a different jwt
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)
	// create a separate user using a different jwt
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	username3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(username3, jwt3, t)

	org := createOrganization(t)
	defer removeOrganization(org, t)
	addUserToOrg(username2, "member", org, t)

	jwt := os.Getenv("IGN_TEST_JWT")
	// create a public collection for user1
	cName1 := "col1"
	createTestCollectionWithOwner(t, &jwt, cName1, testUser, "", false)
	// create a private collection for user1
	cName2 := "col2"
	createTestCollectionWithOwner(t, &jwt, cName2, testUser, "", true)

	// create a public collection with name containing special characters
	colSpecialCharName := "test?question"
	createTestCollectionWithOwner(t, &jwt, colSpecialCharName, testUser, "", false)

	// create org public and private collections
	orgCol1 := "orgCol1"
	createTestCollectionWithOwner(t, &jwt, orgCol1, org, "", false)
	orgCol2 := "orgCol2Priv"
	createTestCollectionWithOwner(t, &jwt, orgCol2, org, "", true)

	// note: this creates models named model1, model2 and model3
	createThreeTestModels(t, nil)
	createThreeTestWorlds(t, nil)
	// manually add model1 and world2 to col1
	addAssetToCollection(t, jwt, testUser, cName1, testUser, "model1", "model")
	addAssetToCollection(t, jwt, testUser, cName1, testUser, "world2", "world")
	thumb1 := fmt.Sprintf("/%s/models/%s/tip/files/%s", testUser, "model1",
		"thumbnails/model.sdf")

	indexTestData := []collectionIndexTest{
		{uriTest{"no jwt - get public collection", colURL(testUser, cName1), nil,
			nil, false}, testUser, cName1, []string{thumb1}},
		{uriTest{"no jwt - get private collection", colURL(testUser, cName2), nil,
			ign.NewErrorMessage(ign.ErrorNameNotFound), false}, "", "", nil},
		{uriTest{"invalid name", colURL(testUser, "invalidname"), nil,
			ign.NewErrorMessage(ign.ErrorNameNotFound), false}, "", "", nil},
		{uriTest{"get collection with special char", colURL(testUser, colSpecialCharName),
			nil, nil, false}, testUser, colSpecialCharName, nil},
		{uriTest{"get public collection of another user", colURL(testUser, cName1),
			newJWT(jwt2), nil, true}, testUser, cName1, []string{thumb1}},
		{uriTest{"get private collection of another user", colURL(testUser, cName2),
			newJWT(jwt2), ign.NewErrorMessage(ign.ErrorNameNotFound), false}, "", "",
			nil},
		{uriTest{"get public org collection with no jwt", colURL(org, orgCol1), nil,
			nil, true}, org, orgCol1, nil},
		{uriTest{"get private org collection with no jwt", colURL(org, orgCol2), nil,
			ign.NewErrorMessage(ign.ErrorNameNotFound), false}, "", "", nil},
		{uriTest{"get public org collection from a member", colURL(org, orgCol1), newJWT(jwt2),
			nil, true}, org, orgCol1, nil},
		{uriTest{"get private org collection from a member", colURL(org, orgCol2),
			newJWT(jwt2), nil, true}, org, orgCol2, nil},
		{uriTest{"get public org collection from non member", colURL(org, orgCol1),
			newJWT(jwt3), nil, true}, org, orgCol1, nil},
		{uriTest{"get private org collection from non member", colURL(org, orgCol2),
			newJWT(jwt3), ign.NewErrorMessage(ign.ErrorNameNotFound), false}, "", "",
			nil},
	}

	for _, test := range indexTestData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithCollectionIndexTestData(t, test)
		})
	}
}

// runSubtestWithCollectionIndexTestData helper function that contains subtest code
func runSubtestWithCollectionIndexTestData(t *testing.T, test collectionIndexTest) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	igntest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	reqArgs := igntest.RequestArgs{Method: "GET", Route: test.URL, Body: nil, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	assert.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK && resp.RespRecorder.Code == http.StatusOK {
		var got collections.Collection
		assert.NoError(t, json.Unmarshal(*bslice, &got), "Unable to unmarshal resource: %s", string(*bslice))
		// first make sure non-serializable fields are NOT sent as json
		assert.Empty(t, got.ID)
		assert.Empty(t, got.UUID)
		assert.Empty(t, got.Creator)
		// Also make sure the owner is the one we expect
		assert.Equal(t, test.expOwner, *got.Owner, "Got owner [%s] is not the expected one [%s]", *got.Owner, test.expOwner)
		// Also make sure the name is the one we expect
		assert.Equal(t, test.expName, *got.Name, "Got name [%s] is not the expected one [%s]", *got.Name, test.expName)
		// compare with db item
		dbCol, err := collections.ByName(globals.Server.Db, test.expName, test.expOwner)
		require.NoError(t, err)
		assert.Equal(t, *dbCol.Name, *got.Name)
		assert.Equal(t, *dbCol.Private, *got.Private)
		assert.Equal(t, *dbCol.Description, *got.Description)
		// check expected thumbnails
		if test.expThumbnails == nil {
			assert.Nil(t, got.ThumbnailUrls)
		} else {
			assert.Equal(t, test.expThumbnails, got.ThumbnailUrls)
		}
	}
}

// createCollectionTest includes the input and expected output for a
// TestCollectionCreate test case.
type createCollectionTest struct {
	uriTest

	col collections.CreateCollection
	// should also delete the created resource as part of this test case?
	deleteAfter bool
	owner       string
}

// TestCollectionCreate tests the POST /collections route. It also optionally
// Deletes the Collection on each test
func TestCollectionCreate(t *testing.T) {
	setup()
	// get the tests JWT
	jwtDef := newJWT(os.Getenv("IGN_TEST_JWT"))
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)
	// create a separate JWT but do not create user using it.
	jwt2 := createValidJWTForIdentity("another-user", t)
	// create another user
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	// create another user
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)
	// create another user
	jwt5 := createValidJWTForIdentity("another-user-5", t)
	user5 := createUserWithJWT(jwt5, t)
	defer removeUserWithJWT(user5, jwt5, t)

	// Create a test organization.
	org := createOrganization(t)
	defer removeOrganization(org, t)
	addUserToOrg(user3, "member", org, t)
	addUserToOrg(user5, "admin", org, t)
	t.Logf("Org name: %s", org)

	name := "MyCollection"
	description := "a cool Collection"
	uri := "/1.0/collections"
	colCreateTestsData := []createCollectionTest{
		{uriTest{"no jwt", uri, nil, ign.NewErrorMessage(ign.ErrorUnauthorized), true},
			collections.CreateCollection{Name: name}, false, ""},
		{uriTest{"invalid jwt token", uri, &testJWT{jwt: sptr("invalid")},
			ign.NewErrorMessage(ign.ErrorUnauthorized), true},
			collections.CreateCollection{Name: name}, false, ""},
		{uriTest{"no user in backend", uri, newJWT(jwt2), ign.NewErrorMessage(ign.ErrorAuthNoUser),
			false}, collections.CreateCollection{Name: name, Description: description},
			false, ""},
		{uriTest{"no name", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue),
			false}, collections.CreateCollection{Description: description}, false, ""},
		{uriTest{"short name", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue),
			false}, collections.CreateCollection{Name: "no", Description: description}, false, ""},
		{uriTest{"no optional fields", uri, jwtDef, nil, false},
			collections.CreateCollection{Name: ign.RandomString(8)}, true, username},
		{uriTest{"with space underscore and dash", uri, jwtDef, nil, true},
			collections.CreateCollection{Name: "with- _space", Description: description},
			true, username},
		// // Note: the following test cases are inter-related, as the test for duplication.
		{uriTest{"with all fields", uri, jwtDef, nil, false},
			collections.CreateCollection{Name: name, Description: description}, false,
			username},
		{uriTest{"duplicate name for same owner", uri, jwtDef,
			ign.NewErrorMessage(ign.ErrorResourceExists), false},
			collections.CreateCollection{Name: name, Description: description}, true,
			username},
		// end of inter-related test cases
		{uriTest{"OK create for Org owner", uri, jwtDef, nil, false},
			collections.CreateCollection{Name: name, Owner: org}, true, org},
		{uriTest{"OK create and delete for Org admin", uri, newJWT(jwt5),
			nil, false}, collections.CreateCollection{Name: name, Owner: org}, true, org},
		{uriTest{"OK create for Org member but cannot delete", uri, newJWT(jwt3),
			nil, false}, collections.CreateCollection{Name: name, Owner: org}, false, org},
		{uriTest{"no write access from non org member", uri, newJWT(jwt4),
			ign.NewErrorMessage(ign.ErrorUnauthorized), false},
			collections.CreateCollection{Name: name, Owner: org}, false, org},
	}

	for _, test := range colCreateTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithCreateCollectionTestData(test, t)
		})
	}
}

// cloneCollectionTest includes the input and expected output for a
// TestCollectionClone test case.
type cloneCollectionTest struct {
	uriTest

	clone       collections.CloneCollection
	sourceOwner string
	sourceName  string
	// should also delete the created resource as part of this test case?
	deleteAfter bool
	owner       string
}

// TestCollectionTransfer tests transfering a collection
func TestCollectionTransfer(t *testing.T) {
	// General test setup
	setup()

	// get the tests JWT
	jwt := os.Getenv("IGN_TEST_JWT")
	jwtDef := newJWT(jwt)
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)

	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)

	// create another user
	jwt2 := createValidJWTForIdentity("another-user-3", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)

	// note: this creates models named model1, model2 and model3
	createThreeTestModels(t, nil)
	createTestModelWithOwner(t, nil, "orgModel", testOrg, false)
	// note: this creates worlds named world1, world2 and world3
	createThreeTestWorlds(t, nil)
	createTestWorldWithOwner(t, nil, "orgWorld", testOrg, false)

	collectionName := "MyCollection"
	description := "a cool Collection"
	createURI := "/1.0/collections"
	boolFalse := false

	// Create a collection
	colCreateTestsData := []createCollectionTest{
		{uriTest{"with all fields", createURI, jwtDef, nil, false},
			collections.CreateCollection{Name: collectionName, Description: description, Private: &boolFalse}, false,
			username}}

	for _, test := range colCreateTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithCreateCollectionTestData(test, t)
		})
	}

	// URL for world clone
	uri := "/1.0/" + username + "/collections/" + collectionName + "/transfer"

	transferTestsAnotherUser := []postTest{
		{"TestTransferInvalidUserPermissions", uri, &jwt2,
			map[string]string{"destOwner": "invalidOrg"}, nil,
			http.StatusBadRequest, -1, nil, nil},
		{"TestTransferInvalidDestinationName", uri, &jwt,
			map[string]string{"destOwner": "invalidOrg"}, nil,
			http.StatusBadRequest, -1, nil, nil},
	}
	// Run tests under different users
	testResourcePOST(t, transferTestsAnotherUser, false, nil)

	transferTestsMainUser := []postTest{
		{"TestTransferToUser", uri, &jwt,
			map[string]string{"destOwner": user2}, nil,
			http.StatusNotFound, -1, nil, nil},
		{"TestransferMissingJson", uri, &jwt,
			nil, nil, http.StatusNotFound, -1, nil, nil},
		{"TestransferValid", uri, &jwt,
			map[string]string{"destOwner": testOrg}, nil,
			http.StatusOK, -1, nil, nil},
	}

	// Run tests under main user
	for _, test := range transferTestsMainUser {
		t.Run(test.testDesc, func(t *testing.T) {

			b := new(bytes.Buffer)
			json.NewEncoder(b).Encode(test.postParams)

			if test.expStatus != http.StatusOK {
				igntest.AssertRouteMultipleArgs("POST", test.uri, b, test.expStatus, &jwt, "text/plain; charset=utf-8", t)
			} else {
				igntest.AssertRouteMultipleArgs("POST", test.uri, b, test.expStatus, &jwt, "application/json", t)
			}
		})
	}
}

// TestCollectionClone tests cloning a collection
func TestCollectionClone(t *testing.T) {
	setup()

	// get the tests JWT
	jwt := os.Getenv("IGN_TEST_JWT")
	jwtDef := newJWT(jwt)
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)

	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)

	// create another user
	jwt2 := createValidJWTForIdentity("another-user-3", t)
	user2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(user2, jwt2, t)

	// note: this creates models named model1, model2 and model3
	createThreeTestModels(t, nil)
	createTestModelWithOwner(t, nil, "orgModel", testOrg, false)
	// note: this creates worlds named world1, world2 and world3
	createThreeTestWorlds(t, nil)
	createTestWorldWithOwner(t, nil, "orgWorld", testOrg, false)

	collectionName := "MyCollection"
	description := "a cool Collection"
	uri := "/1.0/collections"
	boolFalse := false

	// Create a collection
	colCreateTestsData := []createCollectionTest{
		{uriTest{"with all fields", uri, jwtDef, nil, false},
			collections.CreateCollection{Name: collectionName, Description: description, Private: &boolFalse}, false,
			username}}

	for _, test := range colCreateTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithCreateCollectionTestData(test, t)
		})
	}

	// The clone URI
	cloneURI := fmt.Sprintf("/1.0/%s/collections/%s/clone", username, collectionName)

	// Test the we can successfully clone a collection
	t.Run("Good collection clone", func(t *testing.T) {
		runSubTestWithCloneCollectionTestData(
			cloneCollectionTest{
				uriTest{"Good collection clone", cloneURI, newJWT(jwt2), nil, false},
				collections.CloneCollection{
					Name:    "NewCollection",
					Owner:   user2,
					Private: &boolFalse},
				username, collectionName, false, user2},
			t)
		var confirmCollection collections.Collection
		err := globals.Server.Db.Where("name = 'NewCollection'").Find(&confirmCollection).Error
		assert.NoError(t, err)
		assert.Equal(t, *confirmCollection.Name, "NewCollection")
	})

	// Test that we can successfully clone a collection
	t.Run("Duplicate clone should create a unique name", func(t *testing.T) {
		runSubTestWithCloneCollectionTestData(
			cloneCollectionTest{
				uriTest{"Duplicate clone should create a unique name", cloneURI, newJWT(jwt2), nil, false},
				collections.CloneCollection{
					Name:    "NewCollection",
					Owner:   user2,
					Private: &boolFalse},
				username, collectionName, false, user2},
			t)
		var confirmCollection collections.Collection
		err := globals.Server.Db.Where("name = 'NewCollection 1'").Find(&confirmCollection).Error
		assert.NoError(t, err)
		assert.Equal(t, *confirmCollection.Name, "NewCollection 1")
	})

	// manually add model1 and world1 to col1
	addAssetToCollection(t, jwt, username, collectionName, username, "model1", "model")
	addAssetToCollection(t, jwt, username, collectionName, username, "world1", "world")

	// Test that a cloned collection also acquires the assets.
	t.Run("A cloned collection should copy the assets", func(t *testing.T) {
		runSubTestWithCloneCollectionTestData(
			cloneCollectionTest{
				uriTest{"A cloned collection should copy the assets", cloneURI, newJWT(jwt2), nil, false},
				collections.CloneCollection{
					Name:    "NewCollection With Assets",
					Owner:   user2,
					Private: &boolFalse},
				username, collectionName, false, user2},
			t)

		// Confirm that the new collection was created.
		var confirmCollection collections.Collection
		err := globals.Server.Db.Where("name = 'NewCollection With Assets'").Find(&confirmCollection).Error
		assert.NoError(t, err)
		assert.Equal(t, *confirmCollection.Name, "NewCollection With Assets")

		// Get the new collection's assets, and confirm that they are correct.
		var assets collections.CollectionAssets
		err = globals.Server.Db.Where("col_id = ?", confirmCollection.ID).Find(&assets).Error
		assert.NoError(t, err)
		assert.Equal(t, len(assets), 2)
		for _, asset := range assets {
			if asset.Type == "model" {
				assert.Equal(t, asset.AssetName, "model1")
			} else {
				assert.Equal(t, asset.AssetName, "world1")
			}
		}
	})
}

// runSubTestWithCreateCollectionTestData tries to create a collection based
// on the given test struct.
func runSubTestWithCreateCollectionTestData(test createCollectionTest, t *testing.T) {
	cc := test.col
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(cc)

	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	igntest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	bslice, _ := igntest.AssertRouteMultipleArgs("POST", test.URL, b, expStatus, jwt, expCt, t)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name()+" POST /collections", bslice, expEm.ErrCode, t)
	}
	if test.deleteAfter {
		removeCollection(t, test.owner, cc.Name, jwt)
	}
}

// runSubTestWithCloneCollectionTestData tries to clone a collection based
// on the given test struct.
func runSubTestWithCloneCollectionTestData(test cloneCollectionTest, t *testing.T) {
	cloneCollection := test.clone
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(cloneCollection)

	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	igntest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	bslice, _ := igntest.AssertRouteMultipleArgs("POST", test.URL, b, expStatus, jwt, expCt, t)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name()+" POST /{username}/collections/{collection_name}/clone", bslice, expEm.ErrCode, t)
	}
	if test.deleteAfter {
		removeCollection(t, test.owner, cloneCollection.Name, jwt)
	}
}

// updateCollectionTest includes the input and expected output for a
// TestCollectionUpdate test case.
type updateCollectionTest struct {
	uriTest
	// collection name
	name string
	// collection owner
	owner string
	// update data
	upd        *collections.UpdateCollection
	postFiles  []igntest.FileDesc
	expThumbCT *string
	expVersion int
}

// TestCollectionUpdate tests the PATCH owner/collections/collection route.
func TestCollectionUpdate(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")
	jwtDef := newJWT(jwt)
	// create a random user using the default test JWT
	username := createUser(t)
	defer removeUser(username, t)
	// create a separate user and remove it (ie. a non active user)
	jwt2 := createValidJWTForIdentity("another-user", t)
	user2 := createUserWithJWT(jwt2, t)
	removeUserWithJWT(user2, jwt2, t)
	// create another user
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	// create another user
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)

	// create another user
	jwt5 := createValidJWTForIdentity("another-user-5", t)
	user5 := createUserWithJWT(jwt5, t)
	defer removeUserWithJWT(user5, jwt5, t)

	// Create a test organization.
	org := createOrganization(t)
	defer removeOrganization(org, t)
	addUserToOrg(user3, "member", org, t)
	addUserToOrg(user5, "admin", org, t)
	t.Logf("Org name: %s", org)

	// create a test public collection
	cName := "col1"
	createTestCollectionWithOwner(t, &jwt, cName, username, "", false)
	defer removeCollection(t, username, cName, &jwt)
	orgCName := "org col"
	createTestCollectionWithOwner(t, &jwt, orgCName, org, "", false)
	defer removeCollection(t, org, orgCName, &jwt)
	orgCName2 := "org col2"
	// private collection
	createTestCollectionWithOwner(t, &jwt, orgCName2, org, "private", true)
	defer removeCollection(t, org, orgCName2, &jwt)

	description := "updated organization description"
	priv := true
	upd := collections.UpdateCollection{Description: &description, Private: &priv}
	updDescOnly := collections.UpdateCollection{Description: &description}
	emptyFiles := []igntest.FileDesc{}
	withLogo := []igntest.FileDesc{
		{"thumbnails/col.sdf", constModelSDFFileContents},
	}
	expThumbCT := "chemical/x-mdl-sdfile"

	uri := colURL(username, cName)
	unauth := ign.NewErrorMessage(ign.ErrorUnauthorized)
	colUpdateTestsData := []updateCollectionTest{
		{uriTest{"no jwt", uri, nil, unauth, true},
			cName, username, &upd, emptyFiles, nil, 1},
		{uriTest{"invalid jwt token", uri, &testJWT{jwt: sptr("invalid")},
			unauth, true}, cName, username, &upd, emptyFiles, nil, 1},
		{uriTest{"no fields", uri, jwtDef, ign.NewErrorMessage(ign.ErrorUnmarshalJSON),
			true}, cName, username, nil, nil, nil, 1},
		{uriTest{"no fields #2", uri, jwtDef, ign.NewErrorMessage(ign.ErrorFormInvalidValue),
			false}, cName, username, &collections.UpdateCollection{}, nil, nil, 1},
		{uriTest{"update OK", uri, jwtDef, nil, false}, cName, username, &upd, emptyFiles, nil, 1},
		{uriTest{"non active user", uri, newJWT(jwt2), ign.NewErrorMessage(ign.ErrorAuthNoUser),
			true}, cName, username, nil, emptyFiles, nil, 1},
		{uriTest{"non existent collection", colURL(username, "inv"), jwtDef,
			ign.NewErrorMessage(ign.ErrorNameNotFound), false}, cName, username,
			&upd, emptyFiles, nil, 1},
		{uriTest{"no write access for other user", uri, newJWT(jwt3),
			ign.NewErrorMessage(ign.ErrorNameNotFound), true}, cName, username,
			&upd, emptyFiles, nil, 1},
		{uriTest{"no write access for non org member", colURL(org, orgCName), newJWT(jwt4),
			unauth, true}, orgCName, org, &upd,
			emptyFiles, nil, 1},
		{uriTest{"Org admin can update if collection is public",
			colURL(org, orgCName), newJWT(jwt5), nil, true}, orgCName, org, &updDescOnly,
			withLogo, &expThumbCT, 2},
		{uriTest{"Org admin can update if collection is private",
			colURL(org, orgCName2), newJWT(jwt5), nil, true}, orgCName2, org, &updDescOnly,
			nil, nil, 1},
		{uriTest{"Org member can update if collection is public",
			colURL(org, orgCName), newJWT(jwt3), nil, true}, orgCName, org, &updDescOnly,
			withLogo, &expThumbCT, 3},
		{uriTest{"Org member can update if collection is private",
			colURL(org, orgCName2), newJWT(jwt3), nil, true}, orgCName2, org, &updDescOnly,
			nil, nil, 1},
		{uriTest{"Org member cannot update privacy setting", colURL(org, orgCName2),
			newJWT(jwt3), unauth, false}, orgCName2, org, &upd, nil, nil, 1},
		{uriTest{"Org admin can update privacy setting", colURL(org, orgCName2),
			newJWT(jwt5), nil, false}, orgCName2, org, &upd, nil, nil, 1},
	}

	for _, test := range colUpdateTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithUpdateCollectionTestData(test, t)
		})
	}
}

// runSubTestWithUpdateCollectionTestData tries to update a collection.
// It is used as the body of a subtest.
func runSubTestWithUpdateCollectionTestData(test updateCollectionTest, t *testing.T) {
	postParams := map[string]string{}
	if test.upd != nil {
		if test.upd.Description != nil {
			postParams["description"] = *test.upd.Description
		}
		if test.upd.Private != nil {
			postParams["private"] = strconv.FormatBool(*test.upd.Private)
		}
	}

	jwt := getJWTToken(t, test.jwtGen)
	expEm, _ := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	gotCode, bslice, ok := igntest.SendMultipartMethod(t.Name(), t, "PATCH", test.URL, jwt, postParams, test.postFiles)
	assert.True(t, ok, "Could not perform multipart request")
	if expStatus != http.StatusOK {
		require.Equal(t, expStatus, gotCode)
		if !test.ignoreErrorBody {
			igntest.AssertBackendErrorCode(t.Name()+" PATCH "+test.URL, bslice,
				expEm.ErrCode, t)
		}
	} else if expStatus == http.StatusOK {
		assert.Equal(t, http.StatusOK, gotCode, "Did not receive expected http code [%d] after sending PATCH. Got: [%d]. Response: %s", http.StatusOK, gotCode, string(*bslice))
		var got collections.Collection
		assert.NoError(t, json.Unmarshal(*bslice, &got), "Unable to unmarshal resource: %s", string(*bslice))
		require.NotNil(t, got)
		// first make sure non-serializable fields are NOT sent as json
		assert.Empty(t, got.ID)
		assert.Empty(t, got.UUID)
		assert.Empty(t, got.Creator)
		upd := test.upd
		require.NotNil(t, got.Name)
		assert.Equal(t, test.name, *got.Name, "Got name [%s] different than expected one [%s]",
			*got.Name, test.name)
		if upd.Description != nil {
			require.NotNil(t, got.Description)
			assert.Equal(t, *upd.Description, *got.Description,
				"Got description [%s] different than expected one [%s]", *got.Description,
				*upd.Description)
		}
		if upd.Private != nil {
			require.NotNil(t, got.Private)
			assert.Equal(t, *upd.Private, *got.Private, "Got private value", *got.Private,
				*upd.Private)
		}
		if len(test.postFiles) == 0 {
			assert.Nil(t, got.ThumbnailUrls)
		} else {
			require.NotEmpty(t, got.ThumbnailUrls)
			thumbnailURL := fmt.Sprintf("/%s/collections/%s/tip/files/%s", test.owner,
				url.PathEscape(test.name), test.postFiles[0].Path)
			assert.Equal(t, thumbnailURL, got.ThumbnailUrls[0])
			reqArgs := igntest.RequestArgs{Method: "GET", Route: "/1.0" + thumbnailURL,
				Body: nil, SignedToken: nil}
			resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, http.StatusOK, *test.expThumbCT, t)
			ensureIgnResourceVersionHeader(resp.RespRecorder, test.expVersion, t)
		}
	}
}

type collectionAssetAddTest struct {
	uriTest
	nameOwner collections.NameOwnerPair
}

func assetsURL(owner, name, aType string) string {
	encodedName := url.PathEscape(name)
	return fmt.Sprintf("/%s/%s/collections/%s/%ss", apiVersion, owner,
		encodedName, aType)
}

// TestCollectionAssetAdd tests associating assets to a collection
func TestCollectionAssetAdd(t *testing.T) {
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
	addUserToOrg(username3, "member", testOrg, t)

	// note: this creates models named model1, model2 and model3
	createThreeTestModels(t, nil)
	createTestModelWithOwner(t, nil, "orgModel", testOrg, false)
	// note: this creates worlds named world1, world2 and world3
	createThreeTestWorlds(t, nil)
	createTestWorldWithOwner(t, nil, "orgWorld", testOrg, false)

	cName := "col1"
	createTestCollectionWithOwner(t, &jwt, cName, username, "", false)
	defer removeCollection(t, username, cName, &jwt)
	orgCol := "orgCol1"
	createTestCollectionWithOwner(t, &jwt, orgCol, testOrg, "", false)
	defer removeCollection(t, testOrg, orgCol, &jwt)

	// manually add model1 and world1 to col1
	addAssetToCollection(t, jwt, username, cName, username, "model1", "model")
	addAssetToCollection(t, jwt, username, cName, username, "world1", "world")

	testListGen := func(aType string) []collectionAssetAddTest {
		return []collectionAssetAddTest{
			{uriTest{aType + "| no jwt", assetsURL(username, cName, aType), nil,
				ign.NewErrorMessage(ign.ErrorUnauthorized), true},
				collections.NameOwnerPair{aType + "1", username}},
			{uriTest{aType + "| collection doest not exist", assetsURL(username, "inv", aType),
				jwtDef, ign.NewErrorMessage(ign.ErrorNameNotFound), false},
				collections.NameOwnerPair{aType + "1", username}},
			{uriTest{aType + "| invalid jwt token", assetsURL(username, cName, aType),
				&testJWT{jwt: sptr("invalid")},
				ign.NewErrorMessage(ign.ErrorUnauthorized), true},
				collections.NameOwnerPair{aType + "1", username}},
			{uriTest{aType + " doest not exist", assetsURL(username, cName, aType), jwtDef,
				ign.NewErrorMessage(ign.ErrorNameNotFound), true},
				collections.NameOwnerPair{"inv", username}},
			{uriTest{aType + " already in collection", assetsURL(username, cName, aType), jwtDef,
				ign.NewErrorMessage(ign.ErrorResourceExists), true},
				collections.NameOwnerPair{aType + "1", username}},
			{uriTest{aType + "| non owner user should not be able to add asset to user collection",
				assetsURL(username, cName, aType), newJWT(jwt2),
				ign.NewErrorMessage(ign.ErrorUnauthorized), false},
				collections.NameOwnerPair{aType + "2", username}},
			{uriTest{aType + "| success adding asset to user collection",
				assetsURL(username, cName, aType), jwtDef, nil, false},
				collections.NameOwnerPair{aType + "2", username}},
			{uriTest{aType + "| non org member should not be able to add asset to org collection",
				assetsURL(testOrg, orgCol, aType), newJWT(jwt2),
				ign.NewErrorMessage(ign.ErrorUnauthorized), false},
				collections.NameOwnerPair{aType + "3", username}},
			{uriTest{aType + "| org member should be able to add asset to org collection",
				assetsURL(testOrg, orgCol, aType), newJWT(jwt3), nil, true},
				collections.NameOwnerPair{aType + "3", username}},
		}
	}

	assetTypes := []string{"model", "world"}
	for _, aType := range assetTypes {
		tests := testListGen(aType)
		for _, test := range tests {
			t.Run(test.testDesc, func(t *testing.T) {
				b := new(bytes.Buffer)
				json.NewEncoder(b).Encode(test.nameOwner)

				jwt := getJWTToken(t, test.jwtGen)
				expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
				expStatus := expEm.StatusCode
				igntest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
				reqArgs := igntest.RequestArgs{Method: "POST", Route: test.URL, Body: b, SignedToken: jwt}
				resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
				bslice := resp.BodyAsBytes
				require.Equal(t, expStatus, resp.RespRecorder.Code)
				if expStatus != http.StatusOK && !test.ignoreErrorBody {
					igntest.AssertBackendErrorCode(t.Name()+" POST "+test.URL, bslice, expEm.ErrCode, t)
				}
			})
		}
	}
}

type collectionAssetRemoveTest struct {
	uriTest
	nameOwner *collections.NameOwnerPair
}

// TestCollectionAssetRemove tests removing assets from collections
func TestCollectionAssetRemove(t *testing.T) {

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

	// create a separate user using a different jwt
	jwt4 := createValidJWTForIdentity("another-user-4", t)
	username4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(username4, jwt4, t)

	// Create an organization with the default jwt as owner.
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	addUserToOrg(username3, "member", testOrg, t)
	addUserToOrg(username4, "admin", testOrg, t)

	// note: this creates models named model1, model2 and model3
	createThreeTestModels(t, nil)
	createTestModelWithOwner(t, nil, "orgModel", testOrg, false)
	// note: this creates worlds named world1, world2 and world3
	createThreeTestWorlds(t, nil)
	createTestWorldWithOwner(t, nil, "orgWorld", testOrg, false)

	cName := "col1"
	createTestCollectionWithOwner(t, &jwt, cName, username, "", false)
	defer removeCollection(t, username, cName, &jwt)
	orgCol := "orgCol1"
	createTestCollectionWithOwner(t, &jwt, orgCol, testOrg, "", false)
	defer removeCollection(t, testOrg, orgCol, &jwt)
	// manually add models and worlds to both col1 and orgCol1
	addAssetToCollection(t, jwt, username, cName, username, "model1", "model")
	addAssetToCollection(t, jwt, username, cName, username, "world1", "world")

	addAssetToCollection(t, jwt, testOrg, orgCol, username, "model1", "model")
	addAssetToCollection(t, jwt, testOrg, orgCol, username, "model2", "model")
	addAssetToCollection(t, jwt, testOrg, orgCol, username, "model3", "model")
	addAssetToCollection(t, jwt, testOrg, orgCol, username, "world1", "world")
	addAssetToCollection(t, jwt, testOrg, orgCol, username, "world2", "world")
	addAssetToCollection(t, jwt, testOrg, orgCol, username, "world3", "world")

	testListGen := func(aType string) []collectionAssetRemoveTest {
		return []collectionAssetRemoveTest{
			{uriTest{aType + "| no jwt", assetsURL(username, cName, aType), nil,
				ign.NewErrorMessage(ign.ErrorUnauthorized), true},
				&collections.NameOwnerPair{aType + "1", username}},
			{uriTest{aType + "| col doest not exist", assetsURL(username, "inv", aType),
				jwtDef, ign.NewErrorMessage(ign.ErrorNameNotFound), false},
				&collections.NameOwnerPair{aType + "1", username}},
			{uriTest{aType + "| invalid jwt token", assetsURL(username, cName, aType),
				&testJWT{jwt: sptr("invalid")},
				ign.NewErrorMessage(ign.ErrorUnauthorized), true},
				&collections.NameOwnerPair{aType + "1", username}},
			{uriTest{aType + " empty asset owner", assetsURL(username, cName, aType), jwtDef,
				ign.NewErrorMessage(ign.ErrorFormInvalidValue), false},
				&collections.NameOwnerPair{"model1", ""}},
			{uriTest{aType + " empty asset name", assetsURL(username, cName, aType), jwtDef,
				ign.NewErrorMessage(ign.ErrorFormInvalidValue), false},
				&collections.NameOwnerPair{"", "username"}},
			{uriTest{aType + " no parameters", assetsURL(username, cName, aType), jwtDef,
				ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, nil},
			{uriTest{aType + " doest not exist", assetsURL(username, cName, aType), jwtDef,
				ign.NewErrorMessage(ign.ErrorNonExistentResource), false},
				&collections.NameOwnerPair{"inv", username}},
			{uriTest{aType + " does not belong to col", assetsURL(username, cName, aType),
				jwtDef, ign.NewErrorMessage(ign.ErrorNonExistentResource), true},
				&collections.NameOwnerPair{aType + "2", username}},
			{uriTest{aType + "| non owner user should not be able to remove asset from user col",
				assetsURL(username, cName, aType), newJWT(jwt2),
				ign.NewErrorMessage(ign.ErrorUnauthorized), true},
				&collections.NameOwnerPair{aType + "1", username}},
			{uriTest{aType + "| success removing asset from user col",
				assetsURL(username, cName, aType), jwtDef, nil, false},
				&collections.NameOwnerPair{aType + "1", username}},
			{uriTest{aType + "| non org member should not be able to remove asset from org col",
				assetsURL(testOrg, orgCol, aType), newJWT(jwt2),
				ign.NewErrorMessage(ign.ErrorUnauthorized), false},
				&collections.NameOwnerPair{aType + "1", username}},
			{uriTest{aType + "| org member should be able to remove asset from org col",
				assetsURL(testOrg, orgCol, aType), newJWT(jwt3),
				nil, false}, &collections.NameOwnerPair{aType + "1", username}},
			{uriTest{aType + "| org admin should be able to remove asset from org col",
				assetsURL(testOrg, orgCol, aType), newJWT(jwt4), nil, true},
				&collections.NameOwnerPair{aType + "2", username}},
			{uriTest{aType + "| org owner should be able to remove asset from org col",
				assetsURL(testOrg, orgCol, aType), jwtDef, nil, true},
				&collections.NameOwnerPair{aType + "3", username}},
		}
	}

	assetTypes := []string{"model", "world"}
	for _, aType := range assetTypes {
		tests := testListGen(aType)
		for _, test := range tests {
			t.Run(test.testDesc, func(t *testing.T) {
				jwt := getJWTToken(t, test.jwtGen)
				expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
				expStatus := expEm.StatusCode
				url := test.URL
				if test.nameOwner != nil {
					url += "?o=" + test.nameOwner.Owner + "&n=" + test.nameOwner.Name
				}
				igntest.AssertRoute("OPTIONS", url, http.StatusOK, t)
				reqArgs := igntest.RequestArgs{Method: "DELETE", Route: url, Body: nil, SignedToken: jwt}
				resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
				bslice := resp.BodyAsBytes
				require.Equal(t, expStatus, resp.RespRecorder.Code)
				if expStatus != http.StatusOK && !test.ignoreErrorBody {
					igntest.AssertBackendErrorCode(t.Name()+" DELETE "+url, bslice, expEm.ErrCode, t)
				}
			})
		}
	}
}

// collectionAssetListTest defines a GET assets from collection test case
type collectionAssetListTest struct {
	uriTest
	// the pagination query to append as suffix to the GET
	paginationQuery string
	// expected asset names in response
	expNames []string
	// model OR world
	assetType string
}

// TestGetCollectionAssets tests getting paginated collection's assets.
func TestGetCollectionAssets(t *testing.T) {
	setup()
	// get the tests JWT
	jwt := os.Getenv("IGN_TEST_JWT")
	jwtDef := newJWT(jwt)
	// create a random user using the default test JWT
	username := createUser(t)
	// NOTE: we don't "defer" remove the user, nor org nor collections because we will
	// manually remove the user as part of the tests and thus, the defer remove*
	// will fail with Not Authorized error.

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
	addUserToOrg(username3, "member", testOrg, t)

	// note: this creates models named model1, model2 and model3
	createThreeTestModels(t, nil)
	createTestModelWithOwner(t, nil, "orgModel", testOrg, false)
	// note: this creates worlds named world1, world2 and world3
	createThreeTestWorlds(t, nil)
	createTestWorldWithOwner(t, nil, "orgWorld", testOrg, false)

	cName := "col1"
	createTestCollectionWithOwner(t, &jwt, cName, username, "", false)
	orgCol := "orgCol1"
	createTestCollectionWithOwner(t, &jwt, orgCol, testOrg, "", false)
	cName2 := "col2"
	createTestCollectionWithOwner(t, &jwt, cName2, username, "", false)

	// manually add model1 and world1 to both col1 and orgCol1
	addAssetToCollection(t, jwt, username, cName, username, "model1", "model")
	addAssetToCollection(t, jwt, username, cName, username, "model3", "model")
	addAssetToCollection(t, jwt, testOrg, orgCol, username, "model1", "model")
	// worlds
	addAssetToCollection(t, jwt, username, cName, username, "world1", "world")
	addAssetToCollection(t, jwt, username, cName, username, "world3", "world")
	addAssetToCollection(t, jwt, testOrg, orgCol, username, "world1", "world")

	testListGen := func(aType string) []collectionAssetListTest {
		name1 := aType + "1"
		name3 := aType + "3"
		return []collectionAssetListTest{
			{uriTest{aType + "|get all public col assets", assetsURL(username, cName, aType), nil,
				nil, false}, "", []string{name1, name3}, aType},
			{uriTest{aType + "|public col also visible to non owners", assetsURL(username,
				cName, aType), newJWT(jwt2), nil, false}, "",
				[]string{name1, name3}, aType},
			{uriTest{aType + "|get all org's collection assets",
				assetsURL(testOrg, orgCol, aType), nil,
				nil, false}, "", []string{name1}, aType},
			{uriTest{aType + "|org collection visible to org members",
				assetsURL(testOrg, orgCol, aType),
				newJWT(jwt3), nil, false}, "", []string{name1}, aType},
			{uriTest{aType + "|public org col also visible to non members",
				assetsURL(testOrg, orgCol, aType),
				newJWT(jwt2), nil, false}, "", []string{name1}, aType},
			{uriTest{aType + "|col2 should not have assets", assetsURL(username, cName2, aType),
				nil, nil, false}, "", []string{}, aType},
		}
	}

	assetTypes := []string{"model", "world"}
	for _, aType := range assetTypes {
		tests := testListGen(aType)
		for _, test := range tests {
			t.Run(test.testDesc, func(t *testing.T) {
				runSubtestWithCollectionAssetListTestData(t, test, aType)
			})
			// Now run the same test case but adding a JWT, if needed
			if test.jwtGen == nil {
				test.jwtGen = jwtDef
				test.testDesc += "[with JWT]"
				t.Run(test.testDesc, func(t *testing.T) {
					runSubtestWithCollectionAssetListTestData(t, test, aType)
				})
			}
		}
	}

	// Remove the user and run the tests again
	removeUser(username, t)
	for _, aType := range assetTypes {
		tests := testListGen(aType)
		for _, test := range tests {
			test.testDesc += "[username removed]"
			t.Run(test.testDesc, func(t *testing.T) {
				runSubtestWithCollectionAssetListTestData(t, test, aType)
			})
		}
	}
}

func runSubtestWithCollectionAssetListTestData(t *testing.T, test collectionAssetListTest, aType string) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	igntest.AssertRoute("OPTIONS", test.URL+test.paginationQuery, http.StatusOK, t)
	bslice, _ := igntest.AssertRouteMultipleArgs("GET", test.URL+test.paginationQuery, nil, expStatus, jwt, expCt, t)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name()+" GET assets", bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		if aType == collections.TModel {
			var results []fuel.Model
			require.NoError(t, json.Unmarshal(*bslice, &results), "Unable to unmarshal response", string(*bslice))
			require.Len(t, results, len(test.expNames))
			names := []string{}
			for _, r := range results {
				names = append(names, r.GetName())
			}
			assert.ElementsMatch(t, test.expNames, names)
		} else if aType == collections.TWorld {
			var results []fuel.World
			require.NoError(t, json.Unmarshal(*bslice, &results), "Unable to unmarshal response", string(*bslice))
			require.Len(t, results, len(test.expNames))
			names := []string{}
			for _, r := range results {
				names = append(names, r.GetName())
			}
			assert.ElementsMatch(t, test.expNames, names)
		}
	}
}
