package main

// Tests for ElasticSearch queries.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/gazebo-web/fuel-server/globals"
	fuel "github.com/gazebo-web/fuel-server/proto"
	gztest "github.com/gazebo-web/gz-go/v7/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddElasticVisibilityFilter checks the visibility filter added to
// ElasticSearch queries.
func TestAddElasticVisibilityFilter(t *testing.T) {
	search := map[string]interface{}{
		"query": map[string]interface{}{"match_all": map[string]interface{}{}},
	}

	// Anonymous requests only match resources that are not private.
	b, err := json.Marshal(addElasticVisibilityFilter(search, nil))
	require.NoError(t, err)
	assert.JSONEq(t, `{"query":{"bool":{
		"must":{"match_all":{}},
		"filter":{"bool":{"minimum_should_match":1,"should":[
			{"bool":{"must_not":{"term":{"private":true}}}}
		]}}}}}`, string(b))

	// Logged in users also match the private resources of their own groups.
	b, err = json.Marshal(addElasticVisibilityFilter(search, []string{"myOrg", "me"}))
	require.NoError(t, err)
	assert.JSONEq(t, `{"query":{"bool":{
		"must":{"match_all":{}},
		"filter":{"bool":{"minimum_should_match":1,"should":[
			{"bool":{"must_not":{"term":{"private":true}}}},
			{"terms":{"owner.keyword":["myOrg","me"]}}
		]}}}}}`, string(b))
}

// TestElasticErrorReason checks that ElasticSearch error bodies are parsed
// without panicking.
func TestElasticErrorReason(t *testing.T) {
	var errResult map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(`{"error":{"type":"x","reason":"bad query"},"status":400}`), &errResult))
	assert.Equal(t, "bad query", elasticErrorReason(errResult))

	require.NoError(t, json.Unmarshal([]byte(`{"error":"not found","status":404}`), &errResult))
	assert.Equal(t, "not found", elasticErrorReason(errResult))

	assert.Equal(t, "unknown error", elasticErrorReason(map[string]interface{}{}))
	assert.Equal(t, "unknown error", elasticErrorReason(map[string]interface{}{"error": map[string]interface{}{}}))
}

// TestElasticSearchVisibility runs model searches against an ElasticSearch
// server. It is skipped unless IGN_TEST_ELASTICSEARCH_URL is set (eg.
// http://localhost:9200). Note: the test deletes and recreates the Fuel indices
// of that server.
func TestElasticSearchVisibility(t *testing.T) {
	address := os.Getenv("IGN_TEST_ELASTICSEARCH_URL")
	if address == "" {
		t.Skip("IGN_TEST_ELASTICSEARCH_URL is not set")
	}
	// General test setup
	setup()

	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{address}})
	require.NoError(t, err)
	globals.ElasticSearch = client
	defer func() { globals.ElasticSearch = nil }()
	ctx := context.Background()
	deleteIndices(ctx)
	createIndices(ctx)

	myJWT := os.Getenv("IGN_TEST_JWT")
	username := createUser(t)
	defer removeUser(username, t)
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)
	memberJWT := createValidJWTForIdentity("another-user", t)
	member := createUserWithJWT(memberJWT, t)
	defer removeUserWithJWT(member, memberJWT, t)
	addUserToOrg(member, "member", testOrg, t)

	createTestModelWithOwner(t, &myJWT, "esPublicModel", username, false)
	createTestModelWithOwner(t, &myJWT, "esPrivateModel", username, true)

	// search returns the names of the found models and the X-Total-Count header.
	search := func(t *testing.T, q string, jwt *string) ([]string, string) {
		reqArgs := gztest.RequestArgs{Method: "GET", Route: "/1.0/models?q=" + q, SignedToken: jwt}
		resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, http.StatusOK, ctJSON, t)
		var found []*fuel.Model
		require.NoError(t, json.Unmarshal(*resp.BodyAsBytes, &found))
		names := []string{}
		for _, m := range found {
			names = append(names, *m.Name)
		}
		return names, resp.RespRecorder.Header().Get("X-Total-Count")
	}

	names, total := search(t, "esPublicModel", nil)
	assert.Equal(t, []string{"esPublicModel"}, names)
	assert.Equal(t, "1", total)

	// Private models are only found and counted for their owner.
	names, total = search(t, "esPrivateModel", &myJWT)
	assert.Equal(t, []string{"esPrivateModel"}, names)
	assert.Equal(t, "1", total)
	for desc, jwt := range map[string]*string{"anonymous": nil, "org member": &memberJWT} {
		names, total := search(t, "esPrivateModel", jwt)
		assert.Empty(t, names, desc)
		assert.Equal(t, "0", total, desc)
	}

	// After transferring the private model to the org, its members find it.
	b := new(bytes.Buffer)
	assert.NoError(t, json.NewEncoder(b).Encode(map[string]string{"destOwner": testOrg}))
	gztest.AssertRouteMultipleArgs("POST", "/1.0/"+username+"/models/esPrivateModel/transfer", b, http.StatusOK, &myJWT, ctJSON, t)

	names, total = search(t, "esPrivateModel", &memberJWT)
	assert.Equal(t, []string{"esPrivateModel"}, names)
	assert.Equal(t, "1", total)
	names, total = search(t, "esPrivateModel", nil)
	assert.Empty(t, names)
	assert.Equal(t, "0", total)
}
