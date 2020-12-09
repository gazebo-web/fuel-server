package main

import (
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/models"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"net/http"
	"os"
	"time"
)

// createReviewFn is a callback func that "creation handlers" will pass to doCreateModelReview.
// It is expected that createReviewFn will have the real logic for model review creation.
type createReviewFn func(tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter, r *http.Request) (*models.ModelReview, *ign.ErrMsg)

// doCreateModelReview provides the pre and post steps needed to create a modelReview.
// Handlers should invoke this function and pass a createReviewFn callback.
func doCreateModelReview(tx *gorm.DB, cb createReviewFn, w http.ResponseWriter, r *http.Request) (*models.ModelReview, *ign.ErrMsg) {

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

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		os.Remove(*modelReview.Location)
		return nil, ign.NewErrorMessageWithBase(ign.ErrorNoDatabase, err)
	}

	infoStr := "A new model review has been created:" +
		"\n\t name: " + *modelReview.Name +
		"\n\t owner: " + *modelReview.Model.Owner+
		"\n\t creator: " + *modelReview.Model.Creator +
		"\n\t Reviews: " + modelReview.Reviewers +
		"\n\t Branch: " + modelReview.Branch +
		"\n\t Approvals: " + modelReview.Approvals +
		"\n\t uuid: " + *modelReview.UUID +
		"\n\t location: " + *modelReview.Location +
		"\n\t UploadDate: " + modelReview.UploadDate.UTC().Format(time.RFC3339) +
		"\n\t Tags:"
	for _, t := range modelReview.Tags {
		infoStr += *t.Name
	}

	ign.LoggerFromRequest(r).Info(infoStr)
	// TODO: we should NOT be returning the DB model (including ID) to users.
	return modelReview, nil
}

func ModelReviewCreate(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
	// Parse form's values and files. https://golang.org/pkg/net/http/#Request.ParseMultipartForm
	if err := r.ParseMultipartForm(0); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorForm, err)
	}
	// Delete temporary files from r.ParseMultipartForm(0)
	defer r.MultipartForm.RemoveAll()

	// reviews.CreateModelReview is the input form
	var cmr models.CreateModelReview
	if em := ParseStruct(&cmr, r, true); em != nil {
		return nil, em
	}
	cmr.Metadata = parseMetadata(r)
	createModelReviewFn := func(tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter, r *http.Request) (*models.ModelReview, *ign.ErrMsg) {
		owner := cmr.CreateModel.Owner
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
		uuidStr, modelPath, err := users.NewUUID(owner, "models")
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


		return modelReview, nil
	}

	return doCreateModelReview(tx, createReviewFn, w, r)
}
