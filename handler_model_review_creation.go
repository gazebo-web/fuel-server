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
func modelReviewFn(input interface{}, tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter, r *http.Request) (*reviews.ModelReview, *ign.ErrMsg) {
	owner := input.Owner
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
	review, em := rs.CreateReview(r.Context(), tx, input, uuidStr, jwtUser)
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

    // The input data structure for this handler should contain all fields required by both the
    // `ModelCreate` and `ModelReviewCreate` functions.
	// create CreateModelReview input from request
	var cmr := reviews.CreateModelReview
	if em := ParseStruct(&cm, r, true); em != nil {
		return nil, em
	}

	// construct CreateModel input form from CreateModelReview
	cm := models.CreateModel{
		cmr.ModelID // how to access all model info through just an ID?
	}
	// create the model
	model, em := modelFn(cm, tx, jwtUser, w, r)

	// Get CreateReview input from CreateModelReview
	rm := cmr.CreateReview
	review, em := reviewFn(rm, tx, jwtUser, w, r)

	return modelReview, nil
}
