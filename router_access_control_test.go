package main

// Tests for access control of resources, teams and tokens.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/gz-go/v7"
	gztest "github.com/gazebo-web/gz-go/v7/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			gztest.AssertRouteMultipleArgs("POST", uri, b, http.StatusUnauthorized, &anotherJwt, ctTextPlain, t)

			// The asset is still owned by the original user.
			gztest.AssertRouteMultipleArgs("GET", "/1.0/"+username+"/"+asset, nil, http.StatusOK, &jwtDef, "application/json", t)
			gztest.AssertRouteMultipleArgs("GET", "/1.0/"+testOrg+"/"+asset, nil, http.StatusNotFound, &jwtDef, ctTextPlain, t)
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
	gztest.AssertRouteMultipleArgs("POST", "/1.0/"+testOrg+"/models/privateLiked/likes", nil, http.StatusOK, &myJWT, ctTextPlain, t)
	gztest.AssertRouteMultipleArgs("POST", "/1.0/"+testOrg+"/worlds/privateLiked/likes", nil, http.StatusOK, &myJWT, ctTextPlain, t)

	runners := map[string]func(*testing.T, resourceSearchTest){
		"models": runSubtestWithModelSearchTestData,
		"worlds": runSubtestWithWorldSearchTestData,
	}
	for kind, run := range runners {
		uri := "/1.0/" + testUser + "/likes/" + kind
		tests := []resourceSearchTest{
			{uriTest{"liked " + kind + " seen by the org owner", uri, newJWT(myJWT), nil, false}, 1, "privateLiked", ""},
			{uriTest{"liked " + kind + " seen by a non member", uri, newJWT(jwt2), nil, false}, 0, "", ""},
			{uriTest{"liked " + kind + " seen anonymously", uri, nil, nil, false}, 0, "", ""},
		}
		for _, test := range tests {
			t.Run(test.testDesc, func(t *testing.T) {
				run(t, test)
			})
		}
	}
}

// TestListWithUnknownCredentials checks list routes when the request has an
// invalid Private-Token, or a valid JWT for an identity without a user.
func TestListWithUnknownCredentials(t *testing.T) {
	// General test setup
	setup()

	// A bad access token is rejected.
	gztest.AssertRouteMultipleArgsStruct(gztest.RequestArgs{Method: "GET", Route: "/1.0/models",
		Headers: map[string]string{"Private-Token": "invalid-token"}},
		http.StatusUnauthorized, ctTextPlain, t)

	// A valid JWT for an identity that has no user is treated as anonymous.
	noUserJWT := createValidJWTForIdentity("identity-without-user", t)
	gztest.AssertRouteMultipleArgs("GET", "/1.0/models", nil, http.StatusOK, &noUserJWT, ctJSON, t)
}

// TestElasticSearchAdminWithoutUser checks the admin search routes when the
// JWT identity has no user.
func TestElasticSearchAdminWithoutUser(t *testing.T) {
	// General test setup
	setup()

	noUserJWT := createValidJWTForIdentity("identity-without-user", t)
	for _, method := range []string{"GET", "POST"} {
		gztest.AssertRouteMultipleArgs(method, "/1.0/admin/search", nil, http.StatusUnauthorized, &noUserJWT, ctTextPlain, t)
	}
	gztest.AssertRouteMultipleArgs("GET", "/1.0/admin/search/rebuild", nil, http.StatusUnauthorized, &noUserJWT, ctTextPlain, t)
	gztest.AssertRouteMultipleArgs("DELETE", "/1.0/admin/search/1", nil, http.StatusUnauthorized, &noUserJWT, ctTextPlain, t)
	gztest.AssertRouteMultipleArgs("GET", "/1.0/users", nil, http.StatusUnauthorized, &noUserJWT, ctTextPlain, t)
}

// TestTeamsAreScopedToTheirOrganization checks that the teams routes of an
// organization cannot reach a team with the same name in another organization.
func TestTeamsAreScopedToTheirOrganization(t *testing.T) {
	setup()
	jwt := os.Getenv("IGN_TEST_JWT")
	username := createUser(t)
	defer removeUser(username, t)

	// The default user owns an org with a private team.
	victimOrg := createOrganization(t)
	defer removeOrganization(victimOrg, t)
	notVisible := false
	addTeamToOrg(victimOrg, jwt, users.CreateTeamForm{Name: "devs", Visible: &notVisible}, t)

	// Another user owns a different org.
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)
	otherOrg := gz.RandomString(8)
	defer createOrganizationWithJWT(t, otherOrg, jwt2)()

	// The other org's routes do not reach the victim org's team.
	otherURI := "/1.0/organizations/" + otherOrg + "/teams/devs"
	gztest.AssertRouteMultipleArgs("GET", otherURI, nil, http.StatusNotFound, &jwt2, ctTextPlain, t)
	b := new(bytes.Buffer)
	desc := "changed"
	assert.NoError(t, json.NewEncoder(b).Encode(users.UpdateTeamForm{Description: &desc}))
	gztest.AssertRouteMultipleArgs("PATCH", otherURI, b, http.StatusNotFound, &jwt2, ctTextPlain, t)
	gztest.AssertRouteMultipleArgs("DELETE", otherURI, nil, http.StatusNotFound, &jwt2, ctTextPlain, t)

	// The victim org's team is unchanged.
	bslice, _ := gztest.AssertRouteMultipleArgs("GET", "/1.0/organizations/"+victimOrg+"/teams/devs", nil, http.StatusOK, &jwt, ctJSON, t)
	var team users.TeamResponse
	require.NoError(t, json.Unmarshal(*bslice, &team))
	assert.Nil(t, team.Description)

	// Team names only need to be unique within an organization.
	addTeamToOrg(otherOrg, jwt2, users.CreateTeamForm{Name: "devs", Visible: &notVisible}, t)
}

// TestRevokeAnotherUsersAccessToken checks that a user cannot revoke the
// access tokens of another user.
func TestRevokeAnotherUsersAccessToken(t *testing.T) {
	setup()

	myJWT := os.Getenv("IGN_TEST_JWT")
	username := createUser(t)
	defer removeUser(username, t)
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)

	body := new(bytes.Buffer)
	assert.NoError(t, json.NewEncoder(body).Encode(map[string]string{"name": "myName"}))
	response, _ := gztest.AssertRouteMultipleArgs("POST", "/1.0/users/"+username+"/access-tokens", body,
		http.StatusOK, &myJWT, ctJSON, t)
	var newToken gz.AccessToken
	require.NoError(t, json.Unmarshal(*response, &newToken))

	// The second user tries to revoke the first user's token through its own route.
	body = new(bytes.Buffer)
	assert.NoError(t, json.NewEncoder(body).Encode(newToken))
	gztest.AssertRouteMultipleArgs("POST", "/1.0/users/"+username2+"/access-tokens/revoke", body,
		http.StatusNotFound, &jwt2, ctTextPlain, t)

	// The token still exists.
	response, _ = gztest.AssertRouteMultipleArgs("GET", "/1.0/users/"+username+"/access-tokens", nil,
		http.StatusOK, &myJWT, ctJSON, t)
	var tokens gz.AccessTokens
	require.NoError(t, json.Unmarshal(*response, &tokens))
	assert.Equal(t, 1, len(tokens))

	// The owner can still revoke it.
	body = new(bytes.Buffer)
	assert.NoError(t, json.NewEncoder(body).Encode(newToken))
	gztest.AssertRouteMultipleArgs("POST", "/1.0/users/"+username+"/access-tokens/revoke", body,
		http.StatusOK, &myJWT, ctJSON, t)
}

// createOrganizationWithJWT creates an organization owned by the user of the
// given JWT, and returns a function that removes it.
func createOrganizationWithJWT(t *testing.T, name, jwt string) func() {
	b := new(bytes.Buffer)
	assert.NoError(t, json.NewEncoder(b).Encode(users.Organization{Name: &name}))
	gztest.AssertRouteMultipleArgs("POST", "/1.0/organizations", b, http.StatusOK, &jwt, ctJSON, t)
	return func() {
		gztest.AssertRouteMultipleArgs("DELETE", "/1.0/organizations/"+name, nil, http.StatusOK, &jwt, ctJSON, t)
	}
}

// TestPrivateTeamsOfSimilarlyNamedOrganizations checks that being in a team of
// an organization whose name ends with another organization's name does not
// reveal the private teams of that other organization.
func TestPrivateTeamsOfSimilarlyNamedOrganizations(t *testing.T) {
	setup()
	jwt := os.Getenv("IGN_TEST_JWT")
	username := createUser(t)
	defer removeUser(username, t)

	// The default user owns an org with a private team.
	victimOrg := gz.RandomString(8)
	createOrganizationWithName(t, victimOrg)
	defer removeOrganization(victimOrg, t)
	notVisible := false
	addTeamToOrg(victimOrg, jwt, users.CreateTeamForm{Name: "secret", Visible: &notVisible}, t)

	// A plain member of that org creates "x<org>" with a team of the same name.
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)
	addUserToOrg(username2, "member", victimOrg, t)
	otherOrg := "x" + victimOrg
	defer createOrganizationWithJWT(t, otherOrg, jwt2)()
	addTeamToOrg(otherOrg, jwt2, users.CreateTeamForm{Name: "secret", Visible: &notVisible}, t)
	updateOrgTeam(otherOrg, "secret", jwt2, users.UpdateTeamForm{NewUsers: []string{username2}}, t)

	// The member does not see the private team of the first org.
	bslice, _ := gztest.AssertRouteMultipleArgs("GET", "/1.0/organizations/"+victimOrg+"/teams", nil, http.StatusOK, &jwt2, ctJSON, t)
	var teams users.TeamResponses
	require.NoError(t, json.Unmarshal(*bslice, &teams))
	assert.Empty(t, teams)
}

// TestTeamPermissionGroupConflicts checks that a team cannot be created when
// its permissions group name is already used by a team of another
// organization.
func TestTeamPermissionGroupConflicts(t *testing.T) {
	setup()
	jwt := os.Getenv("IGN_TEST_JWT")
	username := createUser(t)
	defer removeUser(username, t)
	jwt2 := createValidJWTForIdentity("another-user", t)
	username2 := createUserWithJWT(jwt2, t)
	defer removeUserWithJWT(username2, jwt2, t)

	// Org "<a>" with team "b_t_c" uses the same group as org "<a>_t_b" with
	// team "c".
	orgA := gz.RandomString(8)
	createOrganizationWithName(t, orgA)
	defer removeOrganization(orgA, t)
	notVisible := false
	addTeamToOrg(orgA, jwt, users.CreateTeamForm{Name: "b_t_c", Visible: &notVisible}, t)

	orgB := orgA + "_t_b"
	defer createOrganizationWithJWT(t, orgB, jwt2)()
	b := new(bytes.Buffer)
	assert.NoError(t, json.NewEncoder(b).Encode(users.CreateTeamForm{Name: "c", Visible: &notVisible}))
	gztest.AssertRouteMultipleArgs("POST", "/1.0/organizations/"+orgB+"/teams", b, http.StatusConflict, &jwt2, ctTextPlain, t)

	// The same applies the other way around.
	addTeamToOrg(orgB, jwt2, users.CreateTeamForm{Name: "d", Visible: &notVisible}, t)
	b = new(bytes.Buffer)
	assert.NoError(t, json.NewEncoder(b).Encode(users.CreateTeamForm{Name: "b_t_d", Visible: &notVisible}))
	gztest.AssertRouteMultipleArgs("POST", "/1.0/organizations/"+orgA+"/teams", b, http.StatusConflict, &jwt, ctTextPlain, t)
}
