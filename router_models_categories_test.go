package main

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/models"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	igntest "gitlab.com/ignitionrobotics/web/ign-go/testhelpers"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGetModelsSearchWihCategoriesFilterValid(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	testUser := createUser(t)
	defer removeUser(testUser, t)

	createModelWithCategories(t, &jwt, []string{"Cars and Vehicles", "Toys"})

	req, respRec := searchModelWithCategories("model1", "toys")

	globals.Server.Router.ServeHTTP(respRec, req)

	var ms []models.Model
	assert.Equal(t, http.StatusOK, respRec.Code)
	assert.NoError(t, json.Unmarshal(respRec.Body.Bytes(), &ms))
	assert.Len(t, ms, 1)
}

func TestCreateModelWithOneCategory(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	testUser := createUser(t)
	defer removeUser(testUser, t)

	respCode, bslice, ok := createModelWithCategories(t, &jwt, []string{"Cars and Vehicles"})
	model := models.Model{}
	assert.NoError(t, json.Unmarshal(*bslice, &model))
	assert.Len(t, model.Categories, 1)
	assert.True(t, ok)
	assert.Equal(t, http.StatusOK, respCode)

}

func TestCreateModelWithTwoCategories(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	testUser := createUser(t)
	defer removeUser(testUser, t)

	respCode, bslice, ok := createModelWithCategories(t, &jwt, []string{"Cars and Vehicles", "Toys"})
	model := models.Model{}
	assert.NoError(t, json.Unmarshal(*bslice, &model))
	assert.Len(t, model.Categories, 2)
	assert.True(t, ok)
	assert.Equal(t, http.StatusOK, respCode)
}

func TestErrorCreateModelWithMoreThanTwoCategories(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	testUser := createUser(t)
	defer removeUser(testUser, t)

	respCode, _, ok := createModelWithCategories(t, &jwt, []string{"Cars and Vehicles", "Toys", "Music"})
	assert.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, respCode)

}

func TestErrorCreateModelWithWrongCategory(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	testUser := createUser(t)
	defer removeUser(testUser, t)

	respCode, _, ok := createModelWithCategories(t, &jwt, []string{"sraC"})
	assert.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, respCode)

}

func TestUpdateModelWithNoCategories(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	testUser := createUser(t)
	defer removeUser(testUser, t)

	_, bslice, ok := createModelWithCategories(t, &jwt, []string{"Cars and Vehicles", "Toys"})
	model := models.Model{}
	assert.NoError(t, json.Unmarshal(*bslice, &model))
	assert.True(t, ok)

	respCode, bslice, ok := updateModelWithCategories(t, &jwt, *model.Owner, *model.Name, []string{})
	assert.True(t, ok)
	assert.Equal(t, http.StatusOK, respCode)

}

func TestUpdateModelWithLessThanTwoCategories(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	testUser := createUser(t)
	defer removeUser(testUser, t)

	_, bslice, ok := createModelWithCategories(t, &jwt, []string{"Cars and Vehicles", "Toys"})
	model := models.Model{}
	assert.NoError(t, json.Unmarshal(*bslice, &model))
	assert.True(t, ok)

	respCode, bslice, ok := updateModelWithCategories(t, &jwt, *model.Owner, *model.Name, []string{"Electronics"})
	assert.True(t, ok)
	assert.Equal(t, http.StatusOK, respCode)
}

func TestErrorUpdateModelWithMoreThanTwoCategories(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	testUser := createUser(t)
	defer removeUser(testUser, t)

	_, bslice, ok := createModelWithCategories(t, &jwt, []string{"Cars and Vehicles", "Toys"})
	model := models.Model{}
	assert.NoError(t, json.Unmarshal(*bslice, &model))
	assert.True(t, ok)

	respCode, _, ok := updateModelWithCategories(t, &jwt, *model.Owner, *model.Name, []string{"Cars and Vehicles", "Toys", "Music"})
	assert.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, respCode)
}

func createModelWithCategories(t *testing.T, jwt *string, categories []string) (respCode int, bslice *[]byte, ok bool) {
	cats := strings.Join(categories, ", ")
	extraParams := map[string]string{
		"name":        "model1",
		"tags":        "test_tag_1, test_tag2",
		"description": "description",
		"license":     "1",
		"permission":  "0",
		"categories":  cats,
	}
	withThumbnails := []igntest.FileDesc{
		{"model.config", constModelConfigFileContents},
		{"thumbnails/model.sdf", constModelSDFFileContents},
	}

	uri := "/1.0/models"
	testName := t.Name()

	return igntest.SendMultipartPOST(testName, t, uri, jwt, extraParams, withThumbnails)
}

func updateModelWithCategories(t *testing.T, jwt *string, owner, model string, categories []string) (respCode int, bslice *[]byte, ok bool) {
	uri := fmt.Sprintf("/1.0/%s/models/%s", owner, model)
	testName := t.Name()

	joinedCategories := strings.Join(categories, ", ")
	extraParams := map[string]string{
		"categories": joinedCategories,
	}
	withThumbnails := []igntest.FileDesc{
		{"model.config", constModelConfigFileContents},
		{"thumbnails/model.sdf", constModelSDFFileContents},
	}

	return igntest.SendMultipartMethod(testName, t, "PATCH", uri, jwt, extraParams, withThumbnails)
}

func searchModelWithCategories(search string, category string) (*http.Request, *httptest.ResponseRecorder) {
	uri := fmt.Sprintf("/1.0/models/?q=%s&category=%s", search, category)
	req, _ := http.NewRequest("GET", uri, nil)
	respRec := httptest.NewRecorder()
	return req, respRec
}