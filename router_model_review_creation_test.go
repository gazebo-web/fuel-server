package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	gztest "github.com/gazebo-web/gz-go/v7/testhelpers"
	"github.com/stretchr/testify/assert"
)

func TestModelReviewCreateNewModel(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")
	user := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(user, jwt, t)

	extraParams := map[string]string{
		// the following are for models.CreateModel
		"name": "test",
		"tags": "test_tag_1, test_tag2",
		"description": "255aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"license":    "1",
		"permission": "0",

		// the following are for reviews.CreateModelReview
		"title":  "test title",
		"branch": "test branch",
	}

	okModelFiles := []gztest.FileDesc{
		{Path: "model.config", Contents: constModelConfigFileContents},
		{Path: "model.sdf", Contents: constModelSDFFileContents},
	}

	uri := "/1.0/models/reviews"

	createResourceWithArgs(
		"TestModelReviewCreateNewModel",
		uri,
		&jwt,
		extraParams,
		okModelFiles,
		t,
	)

	modelsURI := fmt.Sprintf("/1.0/%s/models", user)
	modelSearchTest := resourceSearchTest{
		uriTest{"TestModelReviewCreateNewModel", modelsURI, nil, nil, false},
		1, "test", ""}
	runSubtestWithModelSearchTestData(t, modelSearchTest)

	reqArgs := gztest.RequestArgs{
		Method:      "GET",
		Route:       uri,
		SignedToken: &jwt,
	}
	resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, http.StatusOK, ctJSON, t)

	body := *resp.BodyAsBytes
	respJSON := make([]map[string]interface{}, 0, 0)
	json.Unmarshal(body, &respJSON)
	assert.Len(t, respJSON, 1)
	review := respJSON[0]["review"].(map[string]interface{})
	assert.NotNil(t, review)
	assert.Equal(t, review["title"], "test title")
}

func TestModelReviewCreateExistingModel(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")
	user := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(user, jwt, t)
	createThreeTestModels(t, &jwt)

	createResourceWithArgs(
		"TestModelReviewCreateExistingModel",
		fmt.Sprintf("/1.0/%s/models/%s/reviews", user, "model1"),
		&jwt,
		map[string]string{"title": "test title", "branch": "test branch", "modelId": "0"},
		[]gztest.FileDesc{},
		t,
	)
}
