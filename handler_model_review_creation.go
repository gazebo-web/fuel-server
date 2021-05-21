package main

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/models"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/reviews"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
)

// extract actual model review process
func reviewFn(cmr reviews.CreateModelReview, tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter, r *http.Request) (*reviews.ModelReview, *ign.ErrMsg) {
	// call review_service.CreateReview using cmr which already has modelID
	rs := &reviews.Service{}
	modelReview, em := rs.CreateModelReview(cmr, tx, jwtUser)
	if em != nil {
		return nil, em
	}

	return modelReview, nil
}

// Create a new model and a new review
func ModelReviewCreate(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
	// Parse form's values and files. https://golang.org/pkg/net/http/#Request.ParseMultipartForm
	if err := r.ParseMultipartForm(0); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorForm, err)
	}
	// Delete temporary files from r.ParseMultipartForm(0)
	defer r.MultipartForm.RemoveAll()

	// Extract the creator of the new modelReview from the request.
	jwtUser, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	// Create model
	var cm models.CreateModel
	if em := ParseStruct(&cm, r, true); em != nil {
		return nil, em
	}
	cm.Metadata = parseMetadata(r)

	// Call the model create function
	model, em := modelFn(cm, tx, jwtUser, w, r)
	if em != nil {
		return nil, em
	}

	// create CreateModelReview input from request
	var cmr reviews.CreateModelReview
	if em := ParseStruct(&cmr, r, true); em != nil {
		return nil, em
	}
	// Create model review with the newly created model
	// pass in newly created model id to create model review
	cmr.ModelID = &model.ID

	// create the review
	modelReview, em := reviewFn(cmr, tx, jwtUser, w, r)
	if em != nil {
		return nil, em
	}

	return modelReview, nil
}

// Create a new review for an existing model
func ReviewCreate(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
	// Parse form's values and files. https://golang.org/pkg/net/http/#Request.ParseMultipartForm
	if err := r.ParseMultipartForm(0); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorForm, err)
	}
	// Delete temporary files from r.ParseMultipartForm(0)
	defer r.MultipartForm.RemoveAll()

	// Extract the creator of the new modelReview from the request.
	jwtUser, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	// create and parse input form, modelID parsd into cmr
	var cmr reviews.CreateModelReview
	if em := ParseStruct(&cmr, r, true); em != nil {
		return nil, em
	}

	vars := mux.Vars(r)
	owner := vars["username"]
	modelName := vars["model"]
	model, err := models.GetModelByName(tx, modelName, owner)
	if err != nil {
		// how do we know what class of error it returns?
		errMsg := ign.ErrorMessage(ign.ErrorUnexpected)
		return nil, &errMsg
	}
	cmr.ModelID = &model.ID

	// create a new modelReview with prefilled modelID in cmr
	modelReview, em := reviewFn(cmr, tx, jwtUser, w, r)
	if em != nil {
		return nil, em
	}

	return modelReview, nil
}

func ReviewUpdate(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
	// Parse form's values and files. https://golang.org/pkg/net/http/#Request.ParseMultipartForm
	if err := r.ParseMultipartForm(0); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorForm, err)
	}
	// Delete temporary files from r.ParseMultipartForm(0)
	defer r.MultipartForm.RemoveAll()

	user, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	vars := mux.Vars(r)
	var updateReview reviews.UpdateReview
	if err := ParseStruct(&updateReview, r, true); err != nil {
		return nil, err
	}
	reviewId, err := strconv.ParseUint(vars["id"], 10, 0)
	if err != nil {
		return nil, ign.NewErrorMessageWithArgs(ign.ErrorIDWrongFormat, err, []string{"reviewId"})
	}
	updateReview.ID = uint(reviewId)

	s := reviews.Service{}
	review, ignerr := s.UpdateReview(tx, updateReview, user)
	if err != nil {
		return nil, ignerr
	}

	return review, nil
}
