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

// extract actual model review process
func modelReviewFn(cm models.CreateModel, tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter, r *http.Request) (*reviews.ModelReview, *ign.ErrMsg) {
	owner := cm.Owner
	if owner != "" {
		// Ensure the passed in name exists before moving forward
		_, em := users.OwnerByName(tx, owner, true)
		if em != nil {
			return nil, em
		}
	} else {
		owner = *jwtUser.Username
	}

	// Create the review via the Reviews Service
	rs := &reviews.Service{}
	review, em := rs.CreateReview(r.Context(), tx, cm, uuidStr, jwtUser)
	if em != nil {
		os.Remove()
		return nil, em
	}
	return model, nil
}

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

	// Block 1: Create model
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

	// Block 2: Create model review with the newly created model
	// The input data structure for this handler should contain all fields required by both the
	// `ModelCreate` and `ModelReviewCreate` functions.
	// create CreateModelReview input from request
	var cmr reviews.CreateModelReview
	if em := ParseStruct(&cm, r, true); em != nil {
		return nil, em
	}
	cmr.ModelID = &model.ID

	// create the model
	model, em := modelFn(cm, tx, jwtUser, w, r)

	return modelReview, nil
}
