package main

// Tests for access control of resources, teams and tokens.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	gztest "github.com/gazebo-web/gz-go/v7/testhelpers"
	"github.com/stretchr/testify/assert"
)

// TestTransferByNonOwner checks that only the owner of an asset can transfer
// it, even when the owner could write to the destination organization.
func TestTransferByNonOwner(t *testing.T) {
	// General test setup
	setup()

	// The owner of the assets is also the owner of the destination org.
	jwtDef := os.Getenv("IGN_TEST_JWT")
	username := createUser(t)
	defer removeUser(username, t)
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)

	// Another user that can read the public assets.
	anotherJwt := createValidJWTForIdentity("another-user", t)
	anotherUser := createUserWithJWT(anotherJwt, t)
	defer removeUserWithJWT(anotherUser, anotherJwt, t)

	createTestModelWithOwner(t, &jwtDef, "transferModel", username, false)
	createTestWorldWithOwner(t, &jwtDef, "transferWorld", username, false)
	createTestCollectionWithOwner(t, &jwtDef, "transferCollection", username, "", false)

	assets := []string{"models/transferModel", "worlds/transferWorld", "collections/transferCollection"}
	for _, asset := range assets {
		t.Run(asset, func(t *testing.T) {
			b := new(bytes.Buffer)
			assert.NoError(t, json.NewEncoder(b).Encode(map[string]string{"destOwner": testOrg}))
			uri := "/1.0/" + username + "/" + asset + "/transfer"
			gztest.AssertRouteMultipleArgs("POST", uri, b, http.StatusUnauthorized, &anotherJwt, "text/plain; charset=utf-8", t)

			// The asset is still owned by the original user.
			gztest.AssertRouteMultipleArgs("GET", "/1.0/"+username+"/"+asset, nil, http.StatusOK, &jwtDef, "application/json", t)
			gztest.AssertRouteMultipleArgs("GET", "/1.0/"+testOrg+"/"+asset, nil, http.StatusNotFound, &jwtDef, "text/plain; charset=utf-8", t)
		})
	}
}

// TestLikedPrivateResourcesVisibility checks that a user's liked models and
// worlds only include private resources the requestor can see.
func TestLikedPrivateResourcesVisibility(t *testing.T) {
	// General test setup
	setup()

	myJWT := os.Getenv("IGN_TEST_JWT")
	testUser := createUser(t)
	defer removeUser(testUser, t)
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)

	// A user that is not a member of the org.
	jwt2 := createValidJWTForIdentity("another-user", t)
	testUser2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(testUser2, jwt2, t)

	// The org owner likes a private model and a private world of the org.
	createTestModelWithOwner(t, &myJWT, "privateLiked", testOrg, true)
	createTestWorldWithOwner(t, &myJWT, "privateLiked", testOrg, true)
	gztest.AssertRouteMultipleArgs("POST", "/1.0/"+testOrg+"/models/privateLiked/likes", nil, http.StatusOK, &myJWT, "text/plain; charset=utf-8", t)
	gztest.AssertRouteMultipleArgs("POST", "/1.0/"+testOrg+"/worlds/privateLiked/likes", nil, http.StatusOK, &myJWT, "text/plain; charset=utf-8", t)

	modelsURI := "/1.0/" + testUser + "/likes/models"
	worldsURI := "/1.0/" + testUser + "/likes/worlds"
	modelTests := []resourceSearchTest{
		{uriTest{"liked models seen by the org owner", modelsURI, newJWT(myJWT), nil, false}, 1, "privateLiked", ""},
		{uriTest{"liked models seen by a non member", modelsURI, newJWT(jwt2), nil, false}, 0, "", ""},
		{uriTest{"liked models seen anonymously", modelsURI, nil, nil, false}, 0, "", ""},
	}
	for _, test := range modelTests {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithModelSearchTestData(t, test)
		})
	}
	worldTests := []resourceSearchTest{
		{uriTest{"liked worlds seen by the org owner", worldsURI, newJWT(myJWT), nil, false}, 1, "privateLiked", ""},
		{uriTest{"liked worlds seen by a non member", worldsURI, newJWT(jwt2), nil, false}, 0, "", ""},
		{uriTest{"liked worlds seen anonymously", worldsURI, nil, nil, false}, 0, "", ""},
	}
	for _, test := range worldTests {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithWorldSearchTestData(t, test)
		})
	}
}
