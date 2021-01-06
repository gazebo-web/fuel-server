package main

import (
	"net/http"
	"os"
	"time"

	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/models"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/reviews"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
)

// createFn is a callback func that "creation handlers" will pass to doCreateModel.
// It is expected that createFn will have the real logic for the model creation.
type createReviewFn func(tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter, r *http.Request) (*reviews.ModelReviews, *ign.ErrMsg)

// doCreateModelReview provides the pre and post steps needed to create a modelReview.
// Handlers should invoke this function and pass a createReviewFn callback.
func doCreateModelReview(tx *gorm.DB, cb createReviewFn, w http.ResponseWriter, r *http.Request) (*reviews.ModelReview, *ign.ErrMsg) {

	// Extract the creator of the new model from the request.
	jwtUser, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	// invoke the actual createFn (the callback function)
	modelReview, em := cb(tx, jwtUser, w, r)
	if em != nil {
		return nil, em
	}

	infoStr := "A new model review has been created:" +
		"\n\t name: " + *modelReview.Review.Name +
		"\n\t owner: " + *modelReview.Review.Owner+
		"\n\t creator: " + *modelReview.Review.Creator +
		"\n\t branch: " + *modelReview.Review.Branch +
		"\n\t tags:"
	for _, t := range modelReview.Review.Tags {
		infoStr += *t.Name
	}
	infoStr +=	"\n\t reviewers: "
	for _, r := range modelReview.Review.Reviewers {
		infoStr += r
	}
	infoStr += "\n\t approvals: "
	for _, a := range modelReview.Review.Approvals {
		infoStr += a
	}

	ign.LoggerFromRequest(r).Info(infoStr)
	// TODO: we should NOT be returning the DB model (including ID) to users.
	return modelReview, nil
}

func createModelReviewFn(tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter, r *http.Request) (*reviews.ModelReview, *ign.ErrMsg) {
	// Parse form's values and files. https://golang.org/pkg/net/http/#Request.ParseMultipartForm
	if err := r.ParseMultipartForm(0); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorForm, err)
	}
	// Delete temporary files from r.ParseMultipartForm(0)
	defer r.MultipartForm.RemoveAll()

	// reviews.CreateModelReview is the input form
	var cmr reviews.CreateModelReview
	if em := ParseStruct(&cmr, r, true); em != nil {
		return nil, em
	}

	owner := cmr.CreateReview.Owner
	if owner != "" {
		// Ensure the passed in name exists before moving forward
		_, em := users.OwnerByName(tx, owner, true)
		if em != nil {
			return nil, em
		}
	} else {
		owner = *jwtUser.Username
	}

	// Get a new UUID and model folder
	_, modelPath, err := users.NewUUID(owner, "models")
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorCreatingDir, err)
	}

	// move files from multipart form into new model's folder
	_, em := populateTmpDir(r, true, modelPath)
	if em != nil {
		os.Remove(modelPath)
		return nil, em
	}

	// Create review via the reviews Service
	// rs := &reviews.Service{}
	// review, rem := rs.CreateReview(cmr)
	// model, rem := ?
	// if rem != nil {
	// 	os.Remove(modelPath)
	// 	return nil, rem
	// }
	modelReview, _ := reviews.CreateModelReview(review, model.GetID())
	return &modelReview, nil
}

func ModelReviewCreate(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
	createReviewFn := createModelReviewFn
	return doCreateModelReview(tx, createReviewFn, w, r)
}
