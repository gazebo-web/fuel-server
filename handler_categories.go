package main

import (
	"github.com/gorilla/mux"
	"github.com/gosimple/slug"
	"github.com/jinzhu/gorm"
	"github.com/gazebo-web/fuel-server/bundles/category"
	dtos "github.com/gazebo-web/fuel-server/bundles/category/dtos"
	"github.com/gazebo-web/fuel-server/globals"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"net/http"
)

// CategoryList returns a list with all available categories.
// You can request this method with the following curl command:
//  curl -k -X GET http://localhost:8000/1.0/categories
func CategoryList(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {
	s := &category.Service{}
	return s.List(tx)
}

// CategoryCreate creates a new category. Only system admins can create
// a new category.
// You can request this method with the following curl command:
//  curl -k -H "Content-Type: application/json" -X POST -d '{"name":"CATEGORY"}'
//    http://localhost:8000/1.0/categories
//    --header 'private-token: <A_VALID_ACCESS_TOKEN>'
func CategoryCreate(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	// Parse the request.
	var createCategory dtos.CreateCategory
	if em := ParseStruct(&createCategory, r, false); em != nil {
		return nil, em
	}

	// Sanity check: Find the user and fail if the user is not a system admin.
	user, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	if !globals.Permissions.IsSystemAdmin(*user.Username) {
		return nil, ign.NewErrorMessage(ign.ErrorUnauthorized)
	}

	// Create the new category.
	s := &category.Service{}
	response, em := s.Create(r.Context(), tx, createCategory)
	if em != nil {
		return nil, em
	}

	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	return response, nil
}

// CategoryDelete deletes an existing category. Only system admins can delete
// a category.
// You can request this method with the following curl command:
//  curl -k -H "Content-Type: application/json" -X DELETE
//    https://localhost:4430/1.0/categories/{slug}
//    --header 'private-token: <A_VALID_ACCESS_TOKEN>'
func CategoryDelete(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	categorySlug, ok := mux.Vars(r)["slug"]
	if !ok && !slug.IsSlug(categorySlug) {
		return nil, ign.NewErrorMessage(ign.ErrorIDNotInRequest)
	}

	// Sanity check: Find the user and fail if the user is not a system admin.
	user, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}
	if !globals.Permissions.IsSystemAdmin(*user.Username) {
		return nil, ign.NewErrorMessage(ign.ErrorUnauthorized)
	}

	// Delete the category
	s := &category.Service{}
	response, em := s.Delete(r.Context(), tx, categorySlug)
	if em != nil {
		return nil, em
	}

	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	return response, nil
}

// CategoryUpdate updates an existing category. Only system admins can update
// a category.
// You can request this method with the following curl command:
//  curl -k -H "Content-Type: application/json" -X PATCH -d '{"name":"NEW_CATEGORY_NAME", "parent_id":"NEW_CATEGORY_PARENT_ID"}'
//    http://localhost:8000/1.0/categories/{slug}
//    --header 'private-token: <A_VALID_ACCESS_TOKEN>'
func CategoryUpdate(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	categorySlug, ok := mux.Vars(r)["slug"]
	if !ok && !slug.IsSlug(categorySlug) {
		return nil, ign.NewErrorMessage(ign.ErrorIDNotInRequest)
	}
	var cat dtos.UpdateCategory
	if em := ParseStruct(&cat, r, false); em != nil {
		return nil, em
	}

	// Sanity check: Find the user and fail if the user is not a system admin.
	user, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	if !globals.Permissions.IsSystemAdmin(*user.Username) {
		return nil, ign.NewErrorMessage(ign.ErrorUnauthorized)
	}

	// Update the category.
	s := &category.Service{}
	response, em := s.Update(r.Context(), tx, categorySlug, cat)
	if em != nil {
		return nil, em
	}

	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	return response, nil
}
