package main

import (
	"fmt"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/category"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/generics"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/models"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
)

// ModelList returns the list of models from a team/user. The returned value
// will be of type "fuel.Models"
// It follows the func signature defined by type "searchHandler".
// You can request this method with the following curl request:
//     curl -k -X GET --url https://localhost:4430/1.0/models
// or  curl -k -X GET --url https://localhost:4430/1.0/models.proto
// or  curl -k -X GET --url https://localhost:4430/1.0/models.json
// or  curl -k -X GET --url https://localhost:4430/1.0/{username}/models with all the
// above format variants.
func ModelList(p *ign.PaginationRequest, owner *string, order, search string,
	user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	ms := &models.Service{}

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
//     curl -k -X GET --url https://localhost:4430/1.0/{username}/likes/models
// func ModelLikeList(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
func ModelLikeList(p *ign.PaginationRequest, owner *string, order, search string,
	user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	likedBy, em := users.ByUsername(tx, *owner, true)
	if em != nil {
		return nil, nil, em
	}
	ms := &models.Service{}
	return ms.ModelList(p, tx, owner, order, search, likedBy, user, nil)
}

// ModelOwnerVersionFileTree returns the file tree of a single model. The returned value
// will be of type "fuel.ModelFileTree".
// You can request this method with the following curl request:
//   curl -k -X GET --url https://localhost:4430/1.0/{username}/models/{model_name}/{version}/files
func ModelOwnerVersionFileTree(owner, modelName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	// Get the model version
	modelVersion, valid := mux.Vars(r)["version"]
	// If the version does not exist
	if !valid {
		return nil, ign.NewErrorMessage(ign.ErrorModelNotInRequest)
	}

	modelProto, em := (&models.Service{}).ModelFileTree(r.Context(), tx, owner,
		modelName, modelVersion, user)
	if em != nil {
		return nil, em
	}

	writeIgnResourceVersionHeader(strconv.Itoa(int(*modelProto.Version)), w, r)

	return modelProto, em
}

// ModelOwnerIndex returns a single model. The returned value will be of
// type "fuel.Model".
// You can request this method with the following curl request:
//  curl -k -H "Content-Type: application/json" -X GET https://localhost:4430/1.0/{username}/models/{model_name}
func ModelOwnerIndex(owner, modelName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	ms := (&models.Service{})
	fuelModel, em := ms.GetModelProto(r.Context(), tx, owner, modelName, user)
	if em != nil {
		return nil, em
	}

	writeIgnResourceVersionHeader(strconv.Itoa(int(*fuelModel.Version)), w, r)

	return fuelModel, nil
}

// ModelOwnerRemove removes a model based on owner and name
// You can request this method with the following curl request:
//   curl -k -X DELETE --url https://localhost:4430/1.0/{username}/models/{model_name}
func ModelOwnerRemove(owner, modelName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	if em := (&models.Service{}).RemoveModel(tx, owner, modelName, user); em != nil {
		return nil, em
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbDelete, err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return nil, nil
}

// ModelOwnerLikeCreate likes a model from an owner
// You can request this method with the following cURL request:
//    curl -k -X POST https://localhost:4430/1.0/{username}/models/{model_name}/likes
//      --header 'authorization: Bearer <your-jwt-token-here>'
func ModelOwnerLikeCreate(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	_, count, em := (&models.Service{}).CreateModelLike(tx, owner, name, user)
	if em != nil {
		return nil, em
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, count)
	return nil, nil
}

// ModelOwnerLikeRemove removes a like from a model.
// You can request this method with the following cURL request:
//    curl -k -X DELETE https://localhost:4430/1.0/{username}/models/{model_name}/likes
//      --header 'authorization: Bearer <your-jwt-token-here>'
func ModelOwnerLikeRemove(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	_, count, em := (&models.Service{}).RemoveModelLike(tx, owner, name, user)
	if em != nil {
		return nil, em
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, count)
	return nil, nil
}

// ModelOwnerVersionIndividualFileDownload downloads an individual model file
// based on owner, model name, and version.
// You can request this method with the following curl request:
//   curl -k -X GET --url https://localhost:4430/1.0/{username}/models/{model_name}/{version}/files/{file-path}
// eg. curl -k -X GET --url https://localhost:4430/1.0/{username}/models/{model_name}/tip/files/model.config
func ModelOwnerVersionIndividualFileDownload(owner, name string, user *users.User,
	tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
	s := &models.Service{}
	return IndividualFileDownload(s, owner, name, user, tx, w, r)
}

// ModelOwnerVersionZip returns a single model as a zip file
// You can request this method with the following curl request:
//   curl -k -X GET --url https://localhost:4430/1.0/{username}/models/{model-name}/{version}/{model-name}.zip
func ModelOwnerVersionZip(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	// Get the model version
	modelVersion, valid := mux.Vars(r)["version"]
	// If the version does not exist
	if !valid {
		modelVersion = ""
	}

	model, zipPath, ver, em := (&models.Service{}).DownloadZip(r.Context(), tx,
		owner, name, modelVersion, user, r.UserAgent())
	if em != nil {
		return nil, em
	}

	zipFileName := fmt.Sprintf("model-%s.zip", *model.UUID)

	// Remove request header to always serve fresh
	r.Header.Del("If-Modified-Since")
	// Set zip response headers
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipFileName))
	writeIgnResourceVersionHeader(strconv.Itoa(ver), w, r)

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorZipNotAvailable, err)
	}

	// Serve the zip file contents
	// Note: ServeFile should be always last line, after all headers were set.
	http.ServeFile(w, r, *zipPath)
	return nil, nil
}

// ReportModelCreate reports a model.
// You can request this method with the following curl request:
//   curl -k -X POST --url https://localhost:4430/1.0/{username}/models/{model-name}/report
func ReportModelCreate(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	// Parse form's values
	if err := r.ParseMultipartForm(0); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorForm, err)
	}

	// Delete temporary files from r.ParseMultipartForm(0)
	defer r.MultipartForm.RemoveAll()

	var createModelReport models.CreateReport

	if em := ParseStruct(&createModelReport, r, true); em != nil {
		return nil, em
	}

	if _, em := (&models.Service{}).CreateModelReport(tx, owner, name, createModelReport.Reason); em != nil {
		return nil, em
	}

	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	if _, em := generics.SendReportEmail(name, owner, "models", createModelReport.Reason, r); em != nil {
		return nil, em
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	return nil, nil
}
