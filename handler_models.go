package main

import (
	"fmt"
	"github.com/gazebo-web/fuel-server/bundles/category"
	"github.com/gazebo-web/fuel-server/bundles/collections"
	"github.com/gazebo-web/fuel-server/bundles/generics"
	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
)

// ModelList returns the list of models from a team/user. The returned value
// will be of type "fuel.Models"
// It follows the func signature defined by type "searchHandler".
// You can request this method with the following curl request:
//
//	curl -k -X GET --url https://localhost:4430/1.0/models
//
// or  curl -k -X GET --url https://localhost:4430/1.0/models.proto
// or  curl -k -X GET --url https://localhost:4430/1.0/models.json
// or  curl -k -X GET --url https://localhost:4430/1.0/{username}/models with all the
// above format variants.
func ModelList(p *gz.PaginationRequest, owner *string, order, search string,
	user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *gz.PaginationResult, *gz.ErrMsg) {
	ms := &models.Service{Storage: globals.Storage}

	var categories category.Categories

	if categoryFilters, ok := r.URL.Query()["category"]; ok {
		for _, f := range categoryFilters {
			categories = modelListCategoryHelper(tx, f, categories)
		}
	}
	return ms.ModelList(p, tx, owner, order, search, nil, user, &categories)
}

// modelListCategoryHelper append a category to filter in model list
func modelListCategoryHelper(tx *gorm.DB, filter string, categories category.Categories) category.Categories {
	if cat, err := category.BySlug(tx, filter); err == nil {
		categories = append(categories, *cat)
	}
	return categories
}

// ModelLikeList returns the list of models liked by a certain user. The returned value
// will be of type "fuel.Models".
// It follows the func signature defined by type "searchHandler".
// You can request this method with the following curl request:
//
//	curl -k -X GET --url https://localhost:4430/1.0/{username}/likes/models
//
// func ModelLikeList(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {
func ModelLikeList(p *gz.PaginationRequest, owner *string, order, search string,
	user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *gz.PaginationResult, *gz.ErrMsg) {

	likedBy, em := users.ByUsername(tx, *owner, true)
	if em != nil {
		return nil, nil, em
	}
	ms := &models.Service{Storage: globals.Storage}
	return ms.ModelList(p, tx, owner, order, search, likedBy, user, nil)
}

// ModelOwnerVersionFileTree returns the file tree of a single model. The returned value
// will be of type "fuel.ModelFileTree".
// You can request this method with the following curl request:
//
//	curl -k -X GET --url https://localhost:4430/1.0/{username}/models/{model_name}/{version}/files
func ModelOwnerVersionFileTree(owner, modelName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	// Get the model version
	modelVersion, valid := mux.Vars(r)["version"]
	// If the version does not exist
	if !valid {
		return nil, gz.NewErrorMessage(gz.ErrorModelNotInRequest)
	}

	modelProto, em := (&models.Service{Storage: globals.Storage}).ModelFileTree(r.Context(), tx, owner,
		modelName, modelVersion, user)
	if em != nil {
		return nil, em
	}

	_, err := writeIgnResourceVersionHeader(strconv.Itoa(int(*modelProto.Version)), w, r)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	return modelProto, nil
}

// ModelOwnerIndex returns a single model. The returned value will be of
// type "fuel.Model".
// You can request this method with the following curl request:
//
//	curl -k -H "Content-Type: application/json" -X GET https://localhost:4430/1.0/{username}/models/{model_name}
func ModelOwnerIndex(owner, modelName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	ms := &models.Service{Storage: globals.Storage}
	fuelModel, em := ms.GetModelProto(r.Context(), tx, owner, modelName, user)
	if em != nil {
		return nil, em
	}

	_, err := writeIgnResourceVersionHeader(strconv.Itoa(int(*fuelModel.Version)), w, r)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	return fuelModel, nil
}

// ModelOwnerRemove removes a model based on owner and name
// You can request this method with the following curl request:
//
//	curl -k -X DELETE --url https://localhost:4430/1.0/{username}/models/{model_name}
func ModelOwnerRemove(owner, modelName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	// Get the model
	model, em := (&models.Service{Storage: globals.Storage}).GetModel(tx, owner, modelName, user)
	if em != nil {
		return nil, em
	}

	// Remove the model from the models table
	if em = (&models.Service{Storage: globals.Storage}).RemoveModel(r.Context(), tx, owner, modelName, user); em != nil {
		return nil, em
	}

	// Remove the model from collections
	if err := (&collections.Service{}).RemoveAssetFromAllCollections(tx, model.ID); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return nil, nil
}

// ModelOwnerLikeCreate likes a model from an owner
// You can request this method with the following cURL request:
//
//	curl -k -X POST https://localhost:4430/1.0/{username}/models/{model_name}/likes
//	  --header 'authorization: Bearer <your-jwt-token-here>'
func ModelOwnerLikeCreate(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	_, count, em := (&models.Service{Storage: globals.Storage}).CreateModelLike(tx, owner, name, user)
	if em != nil {
		return nil, em
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, count)
	return nil, nil
}

// ModelOwnerLikeRemove removes a like from a model.
// You can request this method with the following cURL request:
//
//	curl -k -X DELETE https://localhost:4430/1.0/{username}/models/{model_name}/likes
//	  --header 'authorization: Bearer <your-jwt-token-here>'
func ModelOwnerLikeRemove(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	_, count, em := (&models.Service{Storage: globals.Storage}).RemoveModelLike(tx, owner, name, user)
	if em != nil {
		return nil, em
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, count)
	return nil, nil
}

// ModelOwnerVersionIndividualFileDownload downloads an individual model file
// based on owner, model name, and version.
// You can request this method with the following curl request:
//
//	curl -k -X GET --url https://localhost:4430/1.0/{username}/models/{model_name}/{version}/files/{file-path}
//
// eg. curl -k -X GET --url https://localhost:4430/1.0/{username}/models/{model_name}/tip/files/model.config
func ModelOwnerVersionIndividualFileDownload(owner, name string, user *users.User,
	tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {
	s := &models.Service{Storage: globals.Storage}
	return IndividualFileDownload(s, owner, name, user, tx, w, r)
}

// ModelOwnerVersionZip returns a single model as a zip file
// You can request this method with the following curl request:
//
//	curl -k -X GET --url https://localhost:4430/1.0/{username}/models/{model-name}/{version}/{model-name}.zip
func ModelOwnerVersionZip(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	// Get the model version
	modelVersion, valid := mux.Vars(r)["version"]
	// If the version does not exist
	if !valid {
		modelVersion = ""
	}
	svc := &models.Service{Storage: globals.Storage}
	_, zipPath, ver, em := svc.DownloadZip(r.Context(), tx,
		owner, name, modelVersion, user, r.UserAgent())
	if em != nil {
		return nil, em
	}

	// Set zip response headers
	w.Header().Set("Content-Type", "application/zip")
	_, err := writeIgnResourceVersionHeader(strconv.Itoa(ver), w, r)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorZipNotAvailable, err)
	}

	// Redirect to the cloud storage
	http.Redirect(w, r, *zipPath, http.StatusOK)
	return nil, nil
}

// ReportModelCreate reports a model.
// You can request this method with the following curl request:
//
//	curl -k -X POST --url https://localhost:4430/1.0/{username}/models/{model-name}/report
func ReportModelCreate(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	// Parse form's values
	if err := r.ParseMultipartForm(0); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorForm, err)
	}

	// Delete temporary files from r.ParseMultipartForm(0)
	defer func(form *multipart.Form) {
		err := form.RemoveAll()
		if err != nil {
			log.Println("Failed to close form:", err)
		}
	}(r.MultipartForm)

	var createModelReport models.CreateReport

	if em := ParseStruct(&createModelReport, r, true); em != nil {
		return nil, em
	}

	if _, em := (&models.Service{Storage: globals.Storage}).CreateModelReport(tx, owner, name, createModelReport.Reason); em != nil {
		return nil, em
	}

	if err := tx.Commit().Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	if _, em := generics.SendReportEmail(name, owner, "models", createModelReport.Reason, r); em != nil {
		return nil, em
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	return nil, nil
}
