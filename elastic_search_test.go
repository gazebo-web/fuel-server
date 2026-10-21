package main

// Tests for ElasticSearch queries.

import (
	"encoding/json"
	"testing"

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
