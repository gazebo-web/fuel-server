package main

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/category"
	dtos "gitlab.com/ignitionrobotics/web/fuelserver/bundles/category/dtos"
	igntest "gitlab.com/ignitionrobotics/web/ign-go/testhelpers"
	"net/http"
	"os"
	"testing"
)

func TestCategoriesPost(t *testing.T) {
	setup()

	uri := "/1.0/categories"

	newName := "Example Category"
	newSlug := "example"
	newCategory := dtos.CreateCategory{
		Name: newName,
		Slug: &newSlug,
	}
	body, err := json.Marshal(newCategory)
	if err != nil {
		t.Fail()
	}

	buffer := bytes.NewBuffer(body)

	t.Run("User should not create categories", func(t *testing.T) {
		_, ok := igntest.AssertRouteMultipleArgs("POST", uri, buffer, http.StatusUnauthorized, nil, "text/plain; charset=utf-8", t)
		assert.True(t, ok)
	})
}

func TestCategoriesPostAdmin(t *testing.T) {
	setup()

	uri := "/1.0/categories"

	jwt := os.Getenv("IGN_TEST_JWT")
	admin := createSysAdminUser(t)
	defer removeUser(admin, t)

	newName := "Example Category"
	newSlug := "example"
	newCategory := dtos.CreateCategory{
		Name: newName,
		Slug: &newSlug,
	}
	body, err := json.Marshal(newCategory)
	if err != nil {
		t.Fail()
	}

	buffer := bytes.NewBuffer(body)

	result := category.Service{}
	t.Run("Admin should create categories", func(t *testing.T) {
		bslice, ok := igntest.AssertRouteMultipleArgs("POST", uri, buffer, http.StatusOK, &jwt, "application/json", t)
		assert.True(t, ok)
		assert.NoError(t, json.Unmarshal(*bslice, &result))
	})
}

func TestCategoriesErrorPostAdminDuplicated(t *testing.T) {
	setup()

	uri := "/1.0/categories"

	jwt := os.Getenv("IGN_TEST_JWT")
	admin := createSysAdminUser(t)
	defer removeUser(admin, t)

	newName := "Electronics"
	newSlug := "electronics"
	newCategory := dtos.CreateCategory{
		Name: newName,
		Slug: &newSlug,
	}
	body, err := json.Marshal(newCategory)
	if err != nil {
		t.Fail()
	}

	buffer := bytes.NewBuffer(body)

	t.Run("Admin should not create categories that already exist", func(t *testing.T) {
		_, ok := igntest.AssertRouteMultipleArgs("POST", uri, buffer, http.StatusConflict, &jwt, "text/plain; charset=utf-8", t)
		assert.True(t, ok)
	})
}

func TestCategoriesGetAll(t *testing.T) {
	setup()
	uri := "/1.0/categories"
	var cats []category.Category
	t.Run("Anyone should get the list of categories", func(t *testing.T) {
		result, ok := igntest.AssertRoute("GET", uri, http.StatusOK, t)
		assert.True(t, ok)
		assert.NoError(t, json.Unmarshal(*result, &cats))
		assert.True(t, len(cats) > 0)
	})
}

func TestCategoriesPatch(t *testing.T) {
	setup()
	uri := "/1.0/categories/electronics"
	newName := "Devices"
	newSlug := "devices"

	updatedCategory := dtos.UpdateCategory{
		Name: &newName,
		Slug: &newSlug,
	}

	body, err := json.Marshal(updatedCategory)
	if err != nil {
		t.Fail()
	}

	buffer := bytes.NewBuffer(body)

	t.Run("User should not update a category", func(t *testing.T) {
		_, ok := igntest.AssertRouteMultipleArgs("PATCH", uri, buffer, http.StatusUnauthorized, nil, "text/plain; charset=utf-8", t)
		assert.True(t, ok)
	})
}

func TestCategoriesPatchAdmin(t *testing.T) {
	setup()
	uri := "/1.0/categories/electronics"
	newName := "Devices"
	newSlug := "devices"

	jwt := os.Getenv("IGN_TEST_JWT")
	admin := createSysAdminUser(t)
	defer removeUser(admin, t)

	updatedCategory := dtos.UpdateCategory{
		Name: &newName,
		Slug: &newSlug,
	}

	body, err := json.Marshal(updatedCategory)
	if err != nil {
		t.Fail()
	}

	buffer := bytes.NewBuffer(body)

	result := category.Category{}
	t.Run("Admin should update a category", func(t *testing.T) {
		bslice, ok := igntest.AssertRouteMultipleArgs("PATCH", uri, buffer, http.StatusOK, &jwt, "application/json", t)
		assert.True(t, ok)
		assert.NoError(t, json.Unmarshal(*bslice, &result))
	})
}

func TestCategoriesDelete(t *testing.T) {
	setup()
	uri := "/1.0/categories/electronics"

	t.Run("User should not remove a category", func(t *testing.T) {
		_, ok := igntest.AssertRouteMultipleArgs("DELETE", uri, nil, http.StatusUnauthorized, nil, "text/plain; charset=utf-8", t)
		assert.True(t, ok)
	})
}

func TestCategoriesDeleteAdmin(t *testing.T) {
	setup()
	uri := "/1.0/categories/electronics"

	jwt := os.Getenv("IGN_TEST_JWT")
	admin := createSysAdminUser(t)
	defer removeUser(admin, t)
	result := category.Category{}

	t.Run("Admin should remove a category", func(t *testing.T) {
		count, _, ok := getCategoriesWithCount(t)
		assert.True(t, ok)

		bslice, ok := igntest.AssertRouteMultipleArgs("DELETE", uri, nil, http.StatusOK, &jwt, "application/json", t)
		assert.NoError(t, json.Unmarshal(*bslice, &result))
		assert.True(t, ok)

		postCount, _, ok := getCategoriesWithCount(t)
		assert.Equal(t, postCount, count-1)
	})
}

func TestCategoriesDeleteAdminRemoveParentId(t *testing.T) {
	setup()
	uri := "/1.0/categories/electronics"

	jwt := os.Getenv("IGN_TEST_JWT")
	admin := createSysAdminUser(t)
	defer removeUser(admin, t)
	result := category.Category{}
	categories := category.Categories{}

	t.Run("Admin should remove a category and the child categories parent ID should be removed", func(t *testing.T) {

		bslice, ok := igntest.AssertRouteMultipleArgs("DELETE", uri, nil, http.StatusOK, &jwt, "application/json", t)
		assert.NoError(t, json.Unmarshal(*bslice, &result))
		assert.True(t, ok)

		_, bslice, ok = getCategoriesWithCount(t)
		assert.NoError(t, json.Unmarshal(*bslice, &categories))
		for _, c := range categories {
			if c.ParentID == nil {
				continue
			}
			if *c.ParentID == result.ID {
				t.Fail()
			}
		}
	})
}

func getCategoriesWithCount(t *testing.T) (count int, bslice *[]byte, ok bool) {
	uri := "/1.0/categories"
	categories := category.Categories{}
	bslice, ok = igntest.AssertRoute("GET", uri, http.StatusOK, t)
	ok = assert.NoError(t, json.Unmarshal(*bslice, &categories))
	count = len(categories)
	return
}
