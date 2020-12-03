package main

import (
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/models"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"net/http"
)

// ModelCreate creates a new model based on input form. It return a model.Model or an error.
// You can request this method with the following cURL request:
//    curl -k -X POST -F name=my_model -F license=1
//      -F file=@<full-path-to-file>
//      https://localhost:4430/1.0/models --header 'authorization: Bearer <your-jwt-token-here>'
func ModelReviewCreate(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
	// TODO: consider limiting max form size (https://golang.org/pkg/net/http/#MaxBytesReader)

	// Parse form's values and files. https://golang.org/pkg/net/http/#Request.ParseMultipartForm
	if err := r.ParseMultipartForm(0); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorForm, err)
	}
	// Delete temporary files from r.ParseMultipartForm(0)
	defer r.MultipartForm.RemoveAll()

	// models.CreateModel is the input form
	var cm models.CreateModel
	if em := ParseStruct(&cm, r, true); em != nil {
		return nil, em
	}
	cm.Metadata = parseMetadata(r)

	createFn := func(tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter, r *http.Request) (*models.Model, *ign.ErrMsg) {
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

		// Create the model via the Models Service
		ms := &models.Service{}
		model, em := ms.CreateModel(r.Context(), tx, cm, uuidStr, modelPath, jwtUser)
		if em != nil {
			os.Remove(modelPath)
			return nil, em
		}
		return model, nil
	}

	return doCreateModel(tx, createFn, w, r)
}
