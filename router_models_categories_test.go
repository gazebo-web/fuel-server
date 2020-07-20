package main

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/models"
	fuel "gitlab.com/ignitionrobotics/web/fuelserver/proto"
	igntest "gitlab.com/ignitionrobotics/web/ign-go/testhelpers"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestGetModelsSearchWihCategoriesFilterValid(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	testUser := createUser(t)
	defer removeUser(testUser, t)

	respCode, _, ok := createModelWithCategories(t, &jwt, "model1", []string{"Cars and Vehicles", "Toys"})
	assert.True(t, ok)
	assert.Equal(t, http.StatusOK, respCode)

	respCode, _, ok = createModelWithCategories(t, &jwt, "model2", []string{"Music", "Toys"})
	assert.True(t, ok)
	assert.Equal(t, http.StatusOK, respCode)

	respCode, _, ok = createModelWithCategories(t, &jwt, "model3", []string{"Animals", "Music"})
	assert.True(t, ok)
	assert.Equal(t, http.StatusOK, respCode)

	respCode, bslice, ok := searchModelWithCategories(t, "test_tag_1", "toys")

	var ms []fuel.Model
	assert.NoError(t, json.Unmarshal(*bslice, &ms))
	assert.Len(t, ms, 2)
	assert.True(t, ok)
	assert.Equal(t, http.StatusOK, respCode)
}

func TestCreateModelWithOneCategory(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	testUser := createUser(t)
	defer removeUser(testUser, t)

	respCode, bslice, ok := createModelWithCategories(t, &jwt, "model1", []string{"Cars and Vehicles"})
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

	respCode, bslice, ok := createModelWithCategories(t, &jwt, "model1", []string{"Cars and Vehicles", "Toys"})
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

	respCode, _, ok := createModelWithCategories(t, &jwt, "model1", []string{"Cars and Vehicles", "Toys", "Music"})
	assert.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, respCode)

}

func TestErrorCreateModelWithWrongCategory(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	testUser := createUser(t)
	defer removeUser(testUser, t)

	respCode, _, ok := createModelWithCategories(t, &jwt, "model1", []string{"sraC"})
	assert.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, respCode)

}

func TestUpdateModelWithNoCategories(t *testing.T) {
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")

	testUser := createUser(t)
	defer removeUser(testUser, t)

	_, bslice, ok := createModelWithCategories(t, &jwt, "model1", []string{"Cars and Vehicles", "Toys"})
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

	_, bslice, ok := createModelWithCategories(t, &jwt, "model1", []string{"Cars and Vehicles", "Toys"})
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

	_, bslice, ok := createModelWithCategories(t, &jwt, "model1", []string{"Cars and Vehicles", "Toys"})
	model := models.Model{}
	assert.NoError(t, json.Unmarshal(*bslice, &model))
	assert.True(t, ok)

	respCode, _, ok := updateModelWithCategories(t, &jwt, *model.Owner, *model.Name, []string{"Cars and Vehicles", "Toys", "Music"})
	assert.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, respCode)
}

func createModelWithCategories(t *testing.T, jwt *string, name string, categories []string) (respCode int, bslice *[]byte, ok bool) {
	cats := strings.Join(categories, ", ")
	extraParams := map[string]string{
		"name":        name,
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

func searchModelWithCategories(t *testing.T, search string, category string) (respCode int, bslice *[]byte, ok bool) {
	uri := fmt.Sprintf("/1.0/models?q=%s&category=%s", search, category)
	return igntest.SendMultipartMethod(t.Name(), t, "GET", uri, nil, nil, nil)
}
