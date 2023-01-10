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

	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/bundles/worlds"
	"github.com/gazebo-web/fuel-server/globals"
	fuel "github.com/gazebo-web/fuel-server/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gztest "github.com/gazebo-web/gz-go/v7/testhelpers"
)

// Tests for worlds related routes

// creates an URL to get a world
func worldURL(owner, name, version string) string {
	encodedName := url.PathEscape(name)
	if version != "" {
		return fmt.Sprintf("/%s/%s/worlds/%s/%s/%s", apiVersion, owner,
			encodedName, version, encodedName)
	}
	return fmt.Sprintf("/%s/%s/worlds/%s", apiVersion, owner, encodedName)
}

// TODO MERGE this with TestGetModels. Consider using an interface to unify some
// comparison.
func TestGetWorlds(t *testing.T) {
	// General test setup
	setup()
	// Create a user and worlds
	testUser := createUser(t)
	createThreeTestWorlds(t, nil)
	// createa another user
	jwt2 := createValidJWTForIdentity("another-user", t)
	testUser2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(testUser2, jwt2, t)

	uri := "/1.0/worlds"
	ownerURI := "/1.0/" + testUser + "/worlds"
	likedURI := "/1.0/" + testUser + "/likes/worlds"

	searchTestsData := []resourceSearchTest{
		// MODELS
		{uriTest{"all", uri, nil, nil, false}, 3, "world3", ""},
		{uriTest{"all ASC order", uri + "?order=asc", nil, nil, false}, 3, "world1", ""},
		{uriTest{"a search", uri + "?q=world2", nil, nil, false}, 1, "world2", ""},
		{uriTest{"empty search query", uri + "?q=", nil, nil, false}, 3, "world3", ""},
		{uriTest{"match a tag", uri + "?q=new", nil, nil, false}, 1, "world3", ""},
		{uriTest{"match a tag and world name", uri + "?q=one world2&order=asc", nil, nil, false}, 2, "world2", ""},
		{uriTest{"match world description", uri + "?q=description", nil, nil, false}, 1, "world1", ""},
		// MODELS FROM OWNER
		{uriTest{"owner's worlds", ownerURI + "?order=asc", nil, nil, false}, 3, "world1", ""},
		// PAGINATION
		{uriTest{"get page #1", uri + "?order=asc&per_page=1&page=1", nil, nil, false}, 1, "world1",
			"</1.0/worlds?order=asc&page=2&per_page=1>; rel=\"next\", </1.0/worlds?order=asc&page=3&per_page=1>; rel=\"last\""},
		{uriTest{"get page #2", uri + "?order=asc&per_page=1&page=2", nil, nil, false}, 1, "world2",
			"</1.0/worlds?order=asc&page=3&per_page=1>; rel=\"next\", </1.0/worlds?order=asc&page=3&per_page=1>; rel=\"last\", </1.0/worlds?order=asc&page=1&per_page=1>; rel=\"first\", </1.0/worlds?order=asc&page=1&per_page=1>; rel=\"prev\""},
		{uriTest{"get page #3", uri + "?order=desc&per_page=1&page=3", nil, nil, false}, 1, "world1",
			"</1.0/worlds?order=desc&page=1&per_page=1>; rel=\"first\", </1.0/worlds?order=desc&page=2&per_page=1>; rel=\"prev\""},
		{uriTest{"invalid page", uri + "?per_page=1&page=7", nil, gz.NewErrorMessage(gz.ErrorPaginationPageNotFound), false}, 0, "", ""},
		// LIKED MODELS
		{uriTest{"liked worlds with non-existent user", "/1.0/invaliduser/likes/worlds", nil, gz.NewErrorMessage(gz.ErrorUserUnknown), false}, 0, "", ""},
		{uriTest{"liked world OK but empty", likedURI, nil, nil, false}, 0, "", ""},
	}

	user2NoWorlds := []resourceSearchTest{
		{uriTest{"user2 with no worlds", "/1.0/" + testUser2 + "/worlds", nil, nil, false}, 0, "", ""},
	}

	myJWT := os.Getenv("IGN_TEST_JWT")
	defaultJWT := newJWT(myJWT)

	for _, test := range append(searchTestsData, user2NoWorlds...) {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithWorldSearchTestData(t, test)
		})
		// Now run the same test case but adding a JWT, if needed
		if test.jwtGen == nil {
			test.jwtGen = defaultJWT
			test.testDesc += "[with JWT]"
			t.Run(test.testDesc, func(t *testing.T) {
				runSubtestWithWorldSearchTestData(t, test)
			})
		}
	}
	// Remove the user and run the tests again
	removeUser(testUser, t)
	for _, test := range searchTestsData {
		test.testDesc += "[testUser removed]"
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithWorldSearchTestData(t, test)
		})
	}

	// create some worlds for user2, and also perform some LIKE operations
	createThreeTestWorlds(t, &jwt2)

	// create another user as user1 was removed
	jwt3 := createValidJWTForIdentity("user3", t)
	testUser3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(testUser3, jwt3, t)

	m2Likes := "/1.0/" + testUser2 + "/worlds/world2/likes"
	gztest.AssertRouteMultipleArgs("POST", m2Likes, nil, http.StatusOK, &jwt3, "text/plain; charset=utf-8", t)

	user2TestsData := []resourceSearchTest{
		{uriTest{"all user2 worlds", "/1.0/" + testUser2 + "/worlds", nil, nil, false}, 3, "world3", ""},
		{uriTest{"liked worlds by testUser1 is empty", likedURI, nil, nil, false}, 0, "", ""},
		{uriTest{"liked worlds by testUser3 has world2", "/1.0/" + testUser3 + "/likes/worlds", nil, nil, false}, 1, "world2", ""},
	}

	for _, test := range user2TestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithWorldSearchTestData(t, test)
		})
	}
}

func TestGetPrivateWorlds(t *testing.T) {
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

	// create a private world for user1
	createTestWorldWithOwner(t, &jwt1, "private_world1", testUser1, true)

	// create public and private worldfor user2
	createTestWorldWithOwner(t, &jwt2, "public_world2", testUser2, false)
	createTestWorldWithOwner(t, &jwt2, "public_world2a", testUser2, false)
	createTestWorldWithOwner(t, &jwt2, "private_world2", testUser2, true)
	createTestWorldWithOwner(t, &jwt2, "private_world2a", testUser2, true)

	// create private world for org
	createTestWorldWithOwner(t, &jwt, "private_org_world", org, true)
	addUserToOrg(testUser3, "member", org, t)

	userPrivateWorldsTestsData := []resourceSearchTest{
		{uriTest{"anonymous user can see only public world", "/1.0/worlds", nil, nil, false}, 2, "public_world2a", ""},
		{uriTest{"user1 can see public worlds and own private world", "/1.0/worlds", newJWT(jwt1), nil, false}, 3, "public_world2a", ""},
		{uriTest{"user2 can see public worlds and own private worlds", "/1.0/worlds", newJWT(jwt2), nil, false}, 4, "private_world2a", ""},
		{uriTest{"member user3 can see public worlds and org private world", "/1.0/worlds", newJWT(jwt3), nil, false}, 3, "private_org_world", ""},
		{uriTest{"user1 can see own private world", "/1.0/" + testUser1 + "/worlds", newJWT(jwt1), nil, false}, 1, "private_world1", ""},
		{uriTest{"user2 can see own public and private worlds", "/1.0/" + testUser2 + "/worlds", newJWT(jwt2), nil, false}, 4, "private_world2a", ""},
		{uriTest{"member user3 can see org private world", "/1.0/" + org + "/worlds", newJWT(jwt3), nil, false}, 1, "private_org_world", ""},
		{uriTest{"member user3 has no worlds", "/1.0/" + testUser3 + "/worlds", newJWT(jwt3), nil, false}, 0, "", ""},
		{uriTest{"anonymous user can not see user1 private world", "/1.0/" + testUser1 + "/worlds", nil, nil, false}, 0, "", ""},
		{uriTest{"anonymous user can see user2 public worlds", "/1.0/" + testUser2 + "/worlds", nil, nil, false}, 2, "public_world2a", ""},
		{uriTest{"anonymous user can not see org private worlds", "/1.0/" + org + "/worlds", nil, nil, false}, 0, "", ""},
		{uriTest{"user2 can not see user1 private world", "/1.0/" + testUser1 + "/worlds", newJWT(jwt2), nil, false}, 0, "", ""},
		{uriTest{"user1 can see user2 public worlds", "/1.0/" + testUser2 + "/worlds", newJWT(jwt1), nil, false}, 2, "public_world2a", ""},
		{uriTest{"user2 can not see org private worlds", "/1.0/" + org + "/worlds", newJWT(jwt2), nil, false}, 0, "", ""},
	}

	for _, test := range userPrivateWorldsTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithWorldSearchTestData(t, test)
		})
	}
}

// runSubtestWithWorldSearchTestData helper function that contains subtest code
func runSubtestWithWorldSearchTestData(t *testing.T, test resourceSearchTest) {
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
		var worlds []*fuel.World
		assert.NoError(t, json.Unmarshal(*bslice, &worlds), "Unable to get all resources: %s", string(*bslice))
		require.Len(t, worlds, test.expCount, "There should be %d items. Got: %d", test.expCount, len(worlds))
		if test.expCount > 0 {
			first := worlds[0]
			exp := test.expFirstName
			assert.Equal(t, exp, *first.Name, "Resource name [%s] is not the expected one [%s]", *first.Name, exp)
		}
		// Link header should NOT be expected if the expected link was empty.
		// Note: Using Header().Get() returns an empty string if the Header is not present.
		// To verify if the header is present, we need to check the map directly.
		respRec := resp.RespRecorder
		assert.False(t, test.expLink == "" && len(respRec.Header()["Link"]) > 0, "Link header should not be present. Got: %s", respRec.Header()["Link"])
		assert.Equal(t, test.expLink, respRec.Header().Get("Link"), "Expected Link header[%s] != [%s]", test.expLink, respRec.Header().Get("Link"))
	}
}

// resourceLikeTest defines a Like creation or deletion test case.
// TODO MERGE this with TesModelLikeCreateAndDelete. Consider using an interface
// to unify some comparisons.
type resourceLikeTest struct {
	uriTest
	// method: expected POST or DELETE
	method string
	// username and name are used to look for the resource in DB.
	username string
	name     string
	// expected likes (after)
	expLikes int
}

// TestWorldLikeCreateAndDelete checks the world like route is valid
func TestWorldLikeCreateAndDelete(t *testing.T) {
	setup()
	myJWT := os.Getenv("IGN_TEST_JWT")
	defaultJWT := newJWT(myJWT)

	// Create random user and some worlds
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

	createThreeTestWorlds(t, nil)
	// create private asset owned by user
	createTestWorldWithOwner(t, &myJWT, "user_private", username, true)
	// create private asset owned by org
	createTestWorldWithOwner(t, &myJWT, "org_private", testOrg, true)

	w1URI := worldURL(username, "world1", "")
	puWorld := worldURL(username, "user_private", "")
	orgWorld := worldURL(testOrg, "org_private", "")
	jwt4 := createValidJWTForIdentity("unexistent-user", t)

	likeTestData := []resourceLikeTest{
		{uriTest{"like no jwt", w1URI + "/likes", nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "POST", "", "", 0},
		{uriTest{"invalid jwt", w1URI + "/likes", newJWT("invalid"), gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "POST", "", "", 0},
		{uriTest{"non-existent user jwt", w1URI + "/likes", newJWT(jwt4), gz.NewErrorMessage(gz.ErrorAuthNoUser), false}, "POST", "", "", 0},
		{uriTest{"non-existent world", worldURL(username, "non-existent", "") + "/likes", defaultJWT, gz.NewErrorMessage(gz.ErrorNameNotFound), false}, "POST", "", "", 0},
		{uriTest{"valid public asset like from another user", w1URI + "/likes", newJWT(jwt3), nil, false}, "POST", username, "world1", 1},
		{uriTest{"user cannot like world twice", w1URI + "/likes", newJWT(jwt3), gz.NewErrorMessage(gz.ErrorDbSave), false}, "POST", "", "", 0},
		{uriTest{"cannot like user private asset with no jwt", puWorld + "/likes", nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "POST", "", "", 0},
		{uriTest{"cannot like user private asset with another jwt", puWorld + "/likes", newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "POST", "", "", 0},
		{uriTest{"cannot like org private asset with no jwt", orgWorld + "/likes", nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "POST", "", "", 0},
		{uriTest{"valid private org asset like by member", orgWorld + "/likes", newJWT(jwt2), nil, false}, "POST", testOrg, "org_private", 1},
		{uriTest{"cannot like org private asset by non member", orgWorld + "/likes", newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "POST", "", "", 0},
		// DELETE tests
		{uriTest{"unlike no jwt", w1URI + "/likes", nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "DELETE", "", "", 0},
		{uriTest{"unlike invalid jwt", w1URI + "/likes", newJWT("invalid"), gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "DELETE", "", "", 0},
		{uriTest{"unlike with non-existent user jwt", w1URI + "/likes", newJWT(jwt4), gz.NewErrorMessage(gz.ErrorAuthNoUser), false}, "DELETE", "", "", 0},
		{uriTest{"unlike non-existent world", worldURL(username, "non-existent", "") + "/likes", defaultJWT, gz.NewErrorMessage(gz.ErrorNameNotFound), false}, "DELETE", "", "", 0},
		{uriTest{"valid public asset unlike", w1URI + "/likes", newJWT(jwt3), nil, false}, "DELETE", username, "world1", 0},
		{uriTest{"valid public asset unlike twice", w1URI + "/likes", newJWT(jwt3), nil, false}, "DELETE", username, "world1", 0},
		{uriTest{"valid unlike of world with no likes", worldURL(username, "world2", "") + "/likes", defaultJWT, nil, false}, "DELETE", username, "world2", 0},
		{uriTest{"cannot unlike user private asset with no jwt", puWorld + "/likes", nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "DELETE", "", "", 0},
		{uriTest{"cannot unlike user private asset with another jwt", puWorld + "/likes", newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "DELETE", "", "", 0},
		{uriTest{"cannot unlike org private asset with no jwt", orgWorld + "/likes", nil, gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "DELETE", "", "", 0},
		{uriTest{"valid unlike of private org asset by member", orgWorld + "/likes", newJWT(jwt2), nil, false}, "DELETE", testOrg, "org_private", 0},
		{uriTest{"cannot unlike org private asset by non member", orgWorld + "/likes", newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), true}, "DELETE", "", "", 0},
	}

	for _, test := range likeTestData {
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
				// Verify that the response contains the new number of likes
				likesCounter, err := strconv.Atoi(string(*bslice))
				assert.NoError(t, err, "Couldn't convert the received likes counter to int.")
				assert.Equal(t, test.expLikes, likesCounter, "Response Likes count [%d] should be equal to [%d]", likesCounter, test.expLikes)
				w := getWorldFromDb(t, test.username, test.name)
				assert.NotNil(t, w)
				assert.Equal(t, test.expLikes, w.Likes, "World's like counter [%d] should be equal to exp: [%d]", w.Likes, test.expLikes)
			}
		})
	}
}

// TestAPIWorld checks the route that describes the worlds API
func TestAPIWorld(t *testing.T) {
	// General test setup
	setup()
	uri := "/1.0/worlds"
	gztest.AssertRoute("OPTIONS", uri, http.StatusOK, t)
}

// TODO try to MERGE with TestGetOwnerModel.
// worldIndexTest defines a TestGetOwnerWorld test case.
type worldIndexTest struct {
	uriTest
	// expected world owner
	expOwner string
	// expected world name
	expName string
	// expected tags
	expTags []string
	// expected thumbnail url
	expThumbURL string
}

func TestGetOwnerWorld(t *testing.T) {
	// General test setup
	setup()

	myJWT := os.Getenv("IGN_TEST_JWT")
	defaultJWT := newJWT(myJWT)

	// Create a user and test world
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

	// create three standard worlds
	createThreeTestWorlds(t, nil)
	createTestWorldWithOwner(t, &myJWT, "user_private", testUser, true)
	// create private world owned by org
	createTestWorldWithOwner(t, &myJWT, "private", testOrg, true)

	// create a model with name containing special characters
	worldSpecialCharName := "testworld?question"
	createTestWorldWithOwner(t, nil, worldSpecialCharName, testUser, false)

	// standard world thumbnail url
	expThumbURL := fmt.Sprintf("/%s/worlds/%s/tip/files/%s", testUser, "world1",
		"thumbnails/world.sdf")

	// thumbnail url of world with special character name
	expSpecialCharThumbURL := fmt.Sprintf("/%s/worlds/%s/tip/files/%s", testUser,
		url.PathEscape(worldSpecialCharName), "thumbnails/world.sdf")

	expPrivateThumbURL := fmt.Sprintf("/%s/worlds/%s/tip/files/%s", testOrg,
		"private", "thumbnails/world.sdf")

	expTags := []string{"test_tag_1", "test_tag2"}
	indexTestData := []worldIndexTest{
		{uriTest{"get world", worldURL(testUser, "world1", ""), nil, nil, false}, testUser, "world1", expTags, expThumbURL},
		{uriTest{"get world with no thumbnails", worldURL(testUser, "world2", ""), nil, nil, false}, testUser, "world2", expTags, ""},
		{uriTest{"invalid name", worldURL(testUser, "invalidname", ""), nil, gz.NewErrorMessage(gz.ErrorNameNotFound), false}, "", "", nil, ""},
		{uriTest{"get world with special char", worldURL(testUser, worldSpecialCharName, ""), nil, nil, false}, testUser, worldSpecialCharName, expTags, expSpecialCharThumbURL},
		{uriTest{"get private org world by org owner", worldURL(testOrg, "private", ""), defaultJWT, nil, false}, testOrg, "private", expTags, expPrivateThumbURL},
		{uriTest{"get private org world by admin", worldURL(testOrg, "private", ""), newJWT(jwt4), nil, false}, testOrg, "private", expTags, expPrivateThumbURL},
		{uriTest{"get private org world by member", worldURL(testOrg, "private", ""), newJWT(jwt2), nil, false}, testOrg, "private", expTags, expPrivateThumbURL},
		{uriTest{"get private org world by non member", worldURL(testOrg, "private", ""), newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, "", "", nil, ""},
		{uriTest{"get private user world with another jwt ", worldURL(testUser, "user_private", ""), newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, "", "", nil, ""},
	}

	for _, test := range indexTestData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithWorldIndexTestData(t, test)
		})
		// Now run the same test case but adding a JWT, if needed
		if test.jwtGen == nil {
			test.jwtGen = defaultJWT
			test.testDesc += "[with JWT]"
			t.Run(test.testDesc, func(t *testing.T) {
				runSubtestWithWorldIndexTestData(t, test)
			})
		}
	}
}

// runSubtestWithWorldSearchTestData helper function that contains subtest code
func runSubtestWithWorldIndexTestData(t *testing.T, test worldIndexTest) {
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
		var got fuel.World
		assert.NoError(t, json.Unmarshal(*bslice, &got), "Unable to unmarshal the world: %s", string(*bslice))
		// Also make sure the world's owner is the one we expect
		assert.Equal(t, test.expOwner, *got.Owner, "Got World owner [%s] is not the expected one [%s]", *got.Owner, test.expOwner)
		// Also make sure the world's name is the one we expect
		assert.Equal(t, test.expName, *got.Name, "Got World name [%s] is not the expected one [%s]", *got.Name, test.expName)
		// check version info is also available and has value "1"
		assert.EqualValues(t, 1, *got.Version, "Got version [%d] is not the expected version [%d]", *got.Version, 1)
		// compare with db world
		world := getWorldFromDb(t, test.expOwner, test.expName)
		assertFuelWorld(&got, world, t)
		actualTags := models.TagsToStrSlice(world.Tags)
		assert.True(t, gz.SameElements(test.expTags, actualTags), "Returned Tags are not the expected. Expected: %v. Got: %v", test.expTags, actualTags)
		// check expected thumbnails
		if test.expThumbURL == "" {
			assert.Nil(t, got.ThumbnailUrl)
		} else {
			assert.Equal(t, test.expThumbURL, *got.ThumbnailUrl, "Got thumbanil url [%s] is different than expected [%s]", *got.ThumbnailUrl, test.expThumbURL)
		}
		// Test the world was stored at `IGN_FUEL_RESOURCE_DIR/{user}/worlds/{uuid}`
		expectedPath := path.Join(globals.ResourceDir, test.expOwner, "worlds", *world.UUID)
		assert.Equal(t, expectedPath, *world.Location, "World Location [%s] is not the expected [%s]", *world.Location, expectedPath)
	}
}

// TODO try to MERGE this with TestGetModelAsZip
// worldDownloadAsZipTest defines a download world as zip file test case.
type worldDownloadAsZipTest struct {
	uriTest
	owner string
	name  string
	// the expected resource version, in the X-Ign-Resource-Version header. Must be a number
	ignVersionHeader int
	// a map containing files that should be present in the returned zip. Eg. {"model.sdf":true, "model.config":true}
	expZipFiles map[string]bool
	// expected downloads count for this zip (after downloading it). Note: this makes the test cases to be dependent among them.
	expDownloads int
	// expected username of the user that performed this download. Can be empty.
	expDownloadUsername string
}

// TestGetWorldAsZip checks if we can get worlds as zip files
func TestGetWorldAsZip(t *testing.T) {
	// General test setup
	setup()
	myJWT := os.Getenv("IGN_TEST_JWT")
	// Create a user and test world
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

	createThreeTestWorlds(t, nil)
	createTestWorldWithOwner(t, &myJWT, "user_private", testUser, true)
	// create private world owned by org
	createTestWorldWithOwner(t, &myJWT, "private", testOrg, true)

	// Get the created world
	world := getWorldFromDb(t, testUser, "world1")
	files := map[string]bool{"thumbnails/": true, "thumbnails/world.sdf": true, "world.world": true}

	// Now check we can get the world as zip file using different uris
	downloadAsZipTestsData := []worldDownloadAsZipTest{
		{uriTest{"/owner/worlds/name style", worldURL(testUser, *world.Name, ""), &testJWT{jwt: &myJWT}, nil, false}, testUser, *world.Name, 1, files, 1, testUser},
		{uriTest{"with explicit world version", worldURL(testUser, *world.Name, "1"), &testJWT{jwt: &myJWT}, nil, false}, testUser, *world.Name, 1, files, 2, testUser},
		{uriTest{"with no JWT", worldURL(testUser, *world.Name, "tip"), nil, nil, false}, testUser, *world.Name, 1, files, 3, ""},
		{uriTest{"invalid (negative) version", worldURL(testUser, *world.Name, "-4"), nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, testUser, *world.Name, 1, files, 3, ""},
		{uriTest{"invalid (alpha) version", worldURL(testUser, *world.Name, "a"), nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, testUser, *world.Name, 1, files, 3, ""},
		{uriTest{"0 version", worldURL(testUser, *world.Name, "0"), nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, testUser, *world.Name, 1, files, 3, ""},
		{uriTest{"version not found", worldURL(testUser, *world.Name, "5"), nil, gz.NewErrorMessage(gz.ErrorVersionNotFound), false}, testUser, *world.Name, 1, files, 3, ""},
		{uriTest{"get private org world by org owner", worldURL(testOrg, "private", ""), &testJWT{jwt: &myJWT}, nil, false}, testOrg, "private", 1, files, 1, testUser},
		{uriTest{"get private org world by admin", worldURL(testOrg, "private", ""), newJWT(jwt4), nil, false}, testOrg, "private", 1, files, 2, user4},
		{uriTest{"get private org world by member", worldURL(testOrg, "private", ""), newJWT(jwt2), nil, false}, testOrg, "private", 1, files, 3, user2},
		{uriTest{"get private org world by non member", worldURL(testOrg, "private", ""), newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, testOrg, "", 1, files, 2, ""},
		{uriTest{"get private org world with no jwt", worldURL(testOrg, "private", ""), nil, gz.NewErrorMessage(gz.ErrorUnauthorized), false}, testOrg, "", 1, files, 2, ""},
		{uriTest{"get private user world with no jwt", worldURL(testUser, "user_private", ""), nil, gz.NewErrorMessage(gz.ErrorUnauthorized), false}, testOrg, "", 1, files, 2, ""},
		{uriTest{"get private user world with another jwt", worldURL(testUser, "user_private", ""), newJWT(jwt3), gz.NewErrorMessage(gz.ErrorUnauthorized), false}, testOrg, "", 1, files, 2, ""},
	}

	for _, test := range downloadAsZipTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctZip)
			expStatus := expEm.StatusCode
			reqArgs := gztest.RequestArgs{Method: "GET", Route: test.URL + ".zip", Body: nil, SignedToken: jwt}
			resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			require.Equal(t, expStatus, resp.RespRecorder.Code)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				gztest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				assert.True(t, resp.Ok, "World Zip Download request didn't succeed")
				ensureIgnResourceVersionHeader(resp.RespRecorder, test.ignVersionHeader, t)
				zSize := len(*bslice)
				zipReader, err := zip.NewReader(bytes.NewReader(*bslice), int64(zSize))
				assert.NoError(t, err, "Unable to read zip response")
				assert.NotEmpty(t, zipReader.File, "Got zip file did not have any files")
				for _, f := range zipReader.File {
					assert.True(t, test.expZipFiles[f.Name], "Got Zip file not included in expected files: %s", f.Name)
				}

				w := getWorldFromDb(t, test.owner, test.name)
				assert.Equal(t, zSize, w.Filesize, "Zip file size (%d) is not equal to world's Filesize field (%d)", zSize, w.Filesize)
				assert.Equal(t, test.expDownloads, w.Downloads, "Downloads counter should be %d. Got: %d", test.expDownloads, w.Downloads)
				wds := getWorldDownloadsFromDb(t, test.owner, test.name)
				assert.Len(t, *wds, test.expDownloads, "World Downloads length should be %d. Got %d", test.expDownloads, len(*wds))
				// get the user that made 'this' current download (the latest)
				pUserID := (*wds)[len(*wds)-1].UserID
				if test.expDownloadUsername == "" {
					assert.Nil(t, pUserID, "download user should be nil")
				} else {
					assert.NotNil(t, pUserID, "download user should NOT be nil. Expected username was: %s", test.expDownloadUsername)
					if pUserID != nil {
						us := dbGetUserByID(*pUserID)
						assert.Equal(t, test.expDownloadUsername, *us.Username, "download user [%s] was expected to be [%s]", *us.Username, test.expDownloadUsername)
					}
				}
				ua := (*wds)[len(*wds)-1].UserAgent
				assert.Empty(t, ua, "World Download should have an empty UserAgent: %s", ua)
			}
		})
	}
}

// TestReportWorldCreate checks the world flag route is valid
func TestReportWorldCreate(t *testing.T) {
	// General test setup
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	// Create a user
	testUser := createUser(t)
	defer removeUser(testUser, t)
	createThreeTestWorlds(t, nil)

	// Sanity check: Get the created world to ensure it was created.
	world := getWorldFromDb(t, testUser, "world2")

	uri := fmt.Sprintf("%s/report", worldURL(testUser, *world.Name, ""))

	body := map[string]string{"reason": "test"}

	from := globals.FlagsEmailSender
	defer func() { globals.FlagsEmailSender = from }()
	globals.FlagsEmailSender = ""

	// Try to report a non-existent model.
	testURI := fmt.Sprintf("%s/report", worldURL(testUser, "non-existent-model", ""))
	expErr := gz.ErrorMessage(gz.ErrorNameNotFound)

	_, bslice, _ := gztest.SendMultipartPOST(t.Name(), t, testURI, nil, body, nil)
	gztest.AssertBackendErrorCode(t.Name(), bslice, expErr.ErrCode, t)

	_, bslice, _ = gztest.SendMultipartPOST(t.Name(), t, testURI, &jwt, body, nil)
	gztest.AssertBackendErrorCode(t.Name(), bslice, expErr.ErrCode, t)

	// Try to report the world
	gztest.SendMultipartPOST(t.Name(), t, uri, nil, body, nil)
	gztest.SendMultipartPOST(t.Name(), t, uri, &jwt, body, nil)
}

type worldModelIncludesTest struct {
	uriTest
	// a slice containing the expected model includes
	expModelIncludes []worlds.ModelInclude
}

// TestGetModelReferences test the WorldModelReferences route.
func TestGetModelReferences(t *testing.T) {
	// General test setup
	setup()
	// Create a user and test world
	testUser := createUser(t)
	defer removeUser(testUser, t)
	createThreeTestWorlds(t, nil)

	// Get the created world
	world := getWorldFromDb(t, testUser, "world1")
	myJWT := os.Getenv("IGN_TEST_JWT")

	var includes []worlds.ModelInclude
	if shouldParseModelIncludes() {
		includes = []worlds.ModelInclude{
			{ModelName: sptr("test_model"), ModelVersion: iptr(1)},
			{ModelName: sptr("ground_plane"), ModelVersion: iptr(-1)},
			{ModelName: sptr("sun"), ModelVersion: iptr(-1)},
		}
	}

	worldModelIncludesTestsData := []worldModelIncludesTest{
		{uriTest{"OK", worldURL(testUser, *world.Name, "1"),
			&testJWT{jwt: &myJWT}, nil, false}, includes},
		{uriTest{"with no JWT", worldURL(testUser, *world.Name, "tip"), nil, nil,
			false}, includes},
		{uriTest{"invalid (negative) version", worldURL(testUser, *world.Name, "-4"),
			nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, includes},
		{uriTest{"invalid (alpha) version", worldURL(testUser, *world.Name, "a"), nil,
			gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, includes},
		{uriTest{"0 version", worldURL(testUser, *world.Name, "0"), nil,
			gz.NewErrorMessage(gz.ErrorFormInvalidValue), false}, includes},
		{uriTest{"version not found", worldURL(testUser, *world.Name, "5"), nil,
			gz.NewErrorMessage(gz.ErrorVersionNotFound), false}, includes},
	}

	for _, test := range worldModelIncludesTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			jwt := getJWTToken(t, test.jwtGen)
			expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
			expStatus := expEm.StatusCode
			reqArgs := gztest.RequestArgs{Method: "GET", Route: test.URL + "/modelrefs", Body: nil, SignedToken: jwt}
			resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
			bslice := resp.BodyAsBytes
			require.Equal(t, expStatus, resp.RespRecorder.Code)
			if expStatus != http.StatusOK && !test.ignoreErrorBody {
				gztest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
			} else if expStatus == http.StatusOK {
				require.Equal(t, http.StatusOK, resp.RespRecorder.Code)
				var mis []worlds.ModelInclude
				assert.NoError(t, json.Unmarshal(*bslice, &mis), "Unable to unmarshal model includes: %s", string(*bslice))
				require.Len(t, mis, len(test.expModelIncludes))
				for i, expMi := range test.expModelIncludes {
					assert.Equal(t, expMi.ModelName, mis[i].ModelName, "ModelName")
					assert.Equal(t, expMi.ModelVersion, mis[i].ModelVersion, "ModelVersion")
				}
			}
		})
	}
}
