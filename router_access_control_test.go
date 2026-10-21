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
