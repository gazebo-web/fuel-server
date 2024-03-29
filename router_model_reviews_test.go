package main

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"os"
	"testing"

	gztest "github.com/gazebo-web/gz-go/v7/testhelpers"
	"github.com/stretchr/testify/assert"
)

// type reviewSearchTest defines a getReviewModels case
type reviewSearchTest struct {
	uriTest
	// expected models count in response
	expCount int
}

func TestGetModelReviews(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")
	user := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(user, jwt, t)

	uri := fmt.Sprintf("/1.0/%s/models/%s/reviews", user, "test")
	// need to create a review tied to a model
	createModelReviews(t, &jwt, user)

	//test the model
	testGetReviewData := reviewSearchTest{uriTest{"all", uri, nil, nil, false}, 1}

	t.Run(testGetReviewData.testDesc, func(t *testing.T) {
		runSubTestWithModelReviewData(t, testGetReviewData, &jwt)
	})
}

// create reviews tied to a model
func createModelReviews(t *testing.T, jwt *string, user string) {

	modelName := "test"

	// create a new model and review
	modelAndReview := map[string]string{
		// the following are for models.CreateModel
		"name": modelName,
		"tags": "test_tag_1, test_tag2",
		"description": "255aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"license":    "1",
		"permission": "0",

		// the following are for reviews.CreateModelReview
		"title":   "test title",
		"branch":  "test branch",
		"creator": user,
		"owner":   user,
	}

	okModelFiles := []gztest.FileDesc{
		{Path: "model.config", Contents: constModelConfigFileContents},
		{Path: "model.sdf", Contents: constModelSDFFileContents},
	}

	createNewModelReviewURI := "/1.0/models/reviews"
	testName := t.Name()
	createResourceWithArgs(
		testName,
		createNewModelReviewURI,
		jwt,
		modelAndReview,
		okModelFiles,
		t,
	)
}

func runSubTestWithModelReviewData(t *testing.T, test reviewSearchTest, jwt *string) {
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	reqArgs := gztest.RequestArgs{Method: "GET", Route: test.URL, SignedToken: jwt}
	gztest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	respJSON := make([]map[string]interface{}, 0)
	assert.NoError(t, json.Unmarshal(*resp.BodyAsBytes, &respJSON))
	review, ok := respJSON[0]["review"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, len(respJSON), 1)
	assert.Equal(t, review["title"], "test title")
}
