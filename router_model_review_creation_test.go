package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	igntest "gitlab.com/ignitionrobotics/web/ign-go/testhelpers"
)

func TestModelReviewCRUD(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")
	user := createUserWithJWT(jwt, t)
	defer removeUserWithJWT(user, jwt, t)

	t.Run("create new model review", func(t *testing.T) {
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

		okModelFiles := []igntest.FileDesc{
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
	})

	t.Run("get newly created model", func(t *testing.T) {
		modelsURI := fmt.Sprintf("/1.0/%s/models", user)
		modelSearchTest := resourceSearchTest{
			uriTest{"TestModelReviewCreateNewModel", modelsURI, nil, nil, false},
			1, "test", ""}
		runSubtestWithModelSearchTestData(t, modelSearchTest)
	})

	t.Run("get newly created review", func(t *testing.T) {
		reqArgs := igntest.RequestArgs{
			Method:      "GET",
			Route:       "/1.0/models/reviews",
			SignedToken: &jwt,
		}
		resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, http.StatusOK, ctJSON, t)

		body := *resp.BodyAsBytes
		respJSON := make([]map[string]interface{}, 0, 0)
		json.Unmarshal(body, &respJSON)
		assert.Len(t, respJSON, 1)
		review := respJSON[0]["review"].(map[string]interface{})
		assert.NotNil(t, review)
		assert.Equal(t, review["title"], "test title")
	})

	t.Run("update review", func(t *testing.T) {
		respCode, respBody, ok := igntest.SendMultipartMethod(
			"update review",
			t,
			"PATCH",
			fmt.Sprintf("/1.0/%s/models/test/reviews/1", user),
			&jwt,
			map[string]string{
				"Status": "asjdlkasjdaksldj",
			},
			[]igntest.FileDesc{},
		)
		assert.True(t, ok)
		assert.Equal(t, http.StatusOK, respCode)
		var respJson map[string]interface{}
		json.Unmarshal(*respBody, &respJson)
		assert.Equal(t, "asjdlkasjdaksldj", respJson["status"])
	})
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
		[]igntest.FileDesc{},
		t,
	)

	t.Run("get newly created review", func(t *testing.T) {
		reqArgs := igntest.RequestArgs{
			Method:      "GET",
			Route:       "/1.0/models/reviews",
			SignedToken: &jwt,
		}
		resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, http.StatusOK, ctJSON, t)

		body := *resp.BodyAsBytes
		respJSON := make([]map[string]interface{}, 0, 0)
		json.Unmarshal(body, &respJSON)
		assert.Len(t, respJSON, 1)
		review := respJSON[0]["review"].(map[string]interface{})
		assert.NotNil(t, review)
		assert.Equal(t, review["title"], "test title")
	})

	t.Run("able to create multiple reviews for a model", func(t *testing.T) {
		createResourceWithArgs(
			"TestModelReviewCreateExistingModel",
			fmt.Sprintf("/1.0/%s/models/%s/reviews", user, "model1"),
			&jwt,
			map[string]string{"title": "test title2", "branch": "test branch", "modelId": "0"},
			[]igntest.FileDesc{},
			t,
		)

		reqArgs := igntest.RequestArgs{
			Method:      "GET",
			Route:       "/1.0/models/reviews",
			SignedToken: &jwt,
		}
		resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, http.StatusOK, ctJSON, t)

		body := *resp.BodyAsBytes
		respJSON := make([]map[string]interface{}, 0, 0)
		json.Unmarshal(body, &respJSON)
		assert.Len(t, respJSON, 2)
		review := respJSON[1]["review"].(map[string]interface{})
		assert.NotNil(t, review)
		assert.Equal(t, review["title"], "test title2")
	})
}
