package main

import (
	"encoding/json"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/models"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

// createFn is a callback func that "creation handlers" will pass to doCreateModel.
// It is expected that createFn will have the real logic for the model creation.
type createFn func(tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter, r *http.Request) (*models.Model, *ign.ErrMsg)

// parseMetadata will check if metadata exists in a request, and return a
// pointer to a models.ModelMetadata struct or nil.
func parseMetadata(r *http.Request) *models.ModelMetadata {
	var metadata *models.ModelMetadata

	// Check if "metadata" exists
	if _, valid := r.Form["metadata"]; valid {
		// Process each metadata line
		for _, meta := range r.Form["metadata"] {

			// Unmarshall the meta data
			var unmarshalled models.ModelMetadatum
			json.Unmarshal([]byte(meta), &unmarshalled)

			// Create the metadata array, if it is null.
			if metadata == nil {
				metadata = new(models.ModelMetadata)
			}

			// Store the meta data
			*metadata = append(*metadata, unmarshalled)
		}
	}
	return metadata
}

// doCreateModel provides the pre and post steps needed to create or clone a model.
// Handlers should invoke this function and pass a createFn callback.
func doCreateModel(tx *gorm.DB, cb createFn, w http.ResponseWriter, r *http.Request) (*models.Model, *ign.ErrMsg) {

	// Extract the creator of the new model from the request.
	jwtUser, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	// invoke the actual createFn (the callback function)
	model, em := cb(tx, jwtUser, w, r)
	if em != nil {
		return nil, em
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		os.Remove(*model.Location)
		return nil, ign.NewErrorMessageWithBase(ign.ErrorNoDatabase, err)
	}

	infoStr := "A new model has been created:" +
		"\n\t name: " + *model.Name +
		"\n\t owner: " + *model.Owner +
		"\n\t creator: " + *model.Creator +
		"\n\t uuid: " + *model.UUID +
		"\n\t location: " + *model.Location +
		"\n\t UploadDate: " + model.UploadDate.UTC().Format(time.RFC3339) +
		"\n\t Tags:"
	for _, t := range model.Tags {
		infoStr += *t.Name
	}

	ign.LoggerFromRequest(r).Info(infoStr)
	// TODO: we should NOT be returning the DB model (including ID) to users.
	return model, nil
}

// ModelCreate creates a new model based on input form. It return a model.Model or an error.
// You can request this method with the following cURL request:
//    curl -k -X POST -F name=my_model -F license=1
//      -F file=@<full-path-to-file>
//      https://localhost:4430/1.0/models --header 'authorization: Bearer <your-jwt-token-here>'
func ModelCreate(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
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

// ModelClone clones a model. Cloning a model means internally creating a new repository
// (git clone) under the current username.
// You can request this method with the following curl request:
//   curl -k -X POST --url https://localhost:4430/1.0/{other-username}/models/{model-name}/clone
//    --header 'authorization: Bearer <your-jwt-token-here>'
func ModelClone(owner, modelName string, ignored *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
	// Parse form's values and files. https://golang.org/pkg/net/http/#Request.ParseMultipartForm
	if err := r.ParseMultipartForm(0); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorForm, err)
	}
	// Delete temporary files from r.ParseMultipartForm(0)
	defer r.MultipartForm.RemoveAll()
	// models.CloneModel is the input form
	var cm models.CloneModel
	if em := ParseStruct(&cm, r, true); em != nil {
		return nil, em
	}

	createFn := func(tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter, r *http.Request) (*models.Model, *ign.ErrMsg) {
		// Ask the Models Service to clone the model
		ms := &models.Service{}
		clone, em := ms.CloneModel(r.Context(), tx, owner, modelName, cm, jwtUser)
		if em != nil {
			return nil, em
		}
		return clone, nil
	}

	return doCreateModel(tx, createFn, w, r)
}

// ModelUpdate modifies an existing model.
// You can request this method with the following cURL request:
//    curl -k -X PATCH -d '{"description":"New Description", "tags":"tag1,tag2"}'
//      https://localhost:4430/1.0/{username}/models/{model-name} -H "Content-Type: application/json"
//      -H 'Authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func ModelUpdate(owner, modelName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	r.ParseMultipartForm(0)
	// Delete temporary files from r.ParseMultipartForm(0)
	defer r.MultipartForm.RemoveAll()
	// models.UpdateModel is the input form
	var um models.UpdateModel
	if errMsg := ParseStruct(&um, r, true); errMsg != nil {
		return nil, errMsg
	}
	if um.IsEmpty() && r.MultipartForm == nil {
		return nil, ign.NewErrorMessage(ign.ErrorFormInvalidValue)
	}

	// If the user has also sent files, then update the model's version
	var newFilesPath *string
	if r.MultipartForm != nil && len(getRequestFiles(r)) > 0 {
		// first, populate files into tmp dir to avoid overriding model
		// files in case of error.
		tmpDir, err := ioutil.TempDir("", modelName)
		defer os.Remove(tmpDir)
		if err != nil {
			return nil, ign.NewErrorMessageWithBase(ign.ErrorRepo, err)
		}
		if _, errMsg := populateTmpDir(r, true, tmpDir); errMsg != nil {
			return nil, errMsg
		}
		newFilesPath = &tmpDir
	}

	um.Metadata = parseMetadata(r)

	model, em := (&models.Service{}).UpdateModel(r.Context(), tx, owner, modelName,
		um.Description, um.Tags, newFilesPath, um.Private, user, um.Metadata, um.Categories)
	if em != nil {
		return nil, em
	}

	infoStr := "Model has been updated:" +
		"\n\t name: " + *model.Name +
		"\n\t owner: " + *model.Owner +
		"\n\t uuid: " + *model.UUID +
		"\n\t location: " + *model.Location +
		"\n\t UploadDate: " + model.UploadDate.UTC().Format(time.RFC3339) +
		"\n\t Tags:"
	for _, t := range model.Tags {
		infoStr += *t.Name
	}
	ign.LoggerFromRequest(r).Info(infoStr)

	// Encode models into a protobuf message
	fuelModel := (&models.Service{}).ModelToProto(model)
	return &fuelModel, nil
}
