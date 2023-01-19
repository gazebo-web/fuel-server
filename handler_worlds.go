package main

import (
	"encoding/json"
	"fmt"
	"github.com/gazebo-web/gz-go/v7"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gazebo-web/fuel-server/bundles/collections"
	"github.com/gazebo-web/fuel-server/bundles/generics"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/bundles/worlds"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
)

// parseMetadata will check if metadata exists in a request, and return a
// pointer to a worlds.WorldMetadata struct or nil.
func parseWorldMetadata(r *http.Request) *worlds.WorldMetadata {
	var metadata *worlds.WorldMetadata

	// Check if "metadata" exists
	if _, valid := r.Form["metadata"]; valid {
		// Process each metadata line
		for _, meta := range r.Form["metadata"] {

			// Unmarshall the meta data
			var unmarshalled worlds.WorldMetadatum
			err := json.Unmarshal([]byte(meta), &unmarshalled)
			if err != nil {
				continue
			}
			// Create the metadata array, if it is null.
			if metadata == nil {
				metadata = new(worlds.WorldMetadata)
			}

			// Store the meta data
			*metadata = append(*metadata, unmarshalled)
		}
	}
	return metadata
}

// WorldList returns the list of worlds from a team/user. The returned value
// will be of type "fuel.Worlds".
// It follows the func signature defined by type "searchHandler".
// You can request this method with the following curl request:
//
//	curl -k -X GET --url https://localhost:4430/1.0/worlds
//
// or  curl -k -X GET --url https://localhost:4430/1.0/worlds.proto
// or  curl -k -X GET --url https://localhost:4430/1.0/worlds.json
// or  curl -k -X GET --url https://localhost:4430/1.0/{username}/worlds with all the
// above format variants.
func WorldList(p *gz.PaginationRequest, owner *string, order, search string,
	user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *gz.PaginationResult, *gz.ErrMsg) {

	ws := &worlds.Service{}
	return ws.WorldList(p, tx, owner, order, search, nil, user)
}

// WorldLikeList returns the list of worlds liked by a certain user. The returned value
// will be of type "fuel.Worlds".
// It follows the func signature defined by type "searchHandler".
// You can request this method with the following curl request:
//
//	curl -k -X GET --url https://localhost:4430/1.0/{username}/likes/worlds
func WorldLikeList(p *gz.PaginationRequest, owner *string, order, search string,
	user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *gz.PaginationResult, *gz.ErrMsg) {

	likedBy, em := users.ByUsername(tx, *owner, true)
	if em != nil {
		return nil, nil, em
	}
	ws := &worlds.Service{}
	return ws.WorldList(p, tx, owner, order, search, likedBy, user)
}

// WorldFileTree returns the file tree of a single world. The returned value
// will be of type "fuel.WorldFileTree".
// You can request this method with the following curl request:
//
//	curl -k -X GET --url https://localhost:4430/1.0/{username}/worlds/{world_name}/{version}/files
func WorldFileTree(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	// Get the version
	version, valid := mux.Vars(r)["version"]
	// If the version does not exist
	if !valid {
		return nil, gz.NewErrorMessage(gz.ErrorWorldNotInRequest)
	}

	worldProto, em := (&worlds.Service{}).FileTree(r.Context(), tx, owner, name, version, user)
	if em != nil {
		return nil, em
	}

	_, err := writeIgnResourceVersionHeader(strconv.Itoa(int(*worldProto.Version)), w, r)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	return worldProto, em
}

// WorldIndex returns a single world. The returned value will be of
// type "fuel.World".
// You can request this method with the following curl request:
//
//	curl -k -H "Content-Type: application/json" -X GET https://localhost:4430/1.0/{username}/worlds/{world_name}
func WorldIndex(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	ws := &worlds.Service{}
	fuelWorld, em := ws.GetWorldProto(r.Context(), tx, owner, name, user)
	if em != nil {
		return nil, em
	}

	_, err := writeIgnResourceVersionHeader(strconv.Itoa(int(*fuelWorld.Version)), w, r)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	return fuelWorld, nil
}

// WorldRemove removes a world based on owner and name
// You can request this method with the following curl request:
//
//	curl -k -X DELETE --url https://localhost:4430/1.0/{username}/worlds/{world_name}
func WorldRemove(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	// Get the world
	world, em := (&worlds.Service{}).GetWorld(tx, owner, name, user)
	if em != nil {
		return nil, em
	}

	// Remove the world from the worlds table
	if em := (&worlds.Service{}).RemoveWorld(r.Context(), tx, owner, name, user); em != nil {
		return nil, em
	}

	// Remove the world from collections
	if err := (&collections.Service{}).RemoveAssetFromAllCollections(tx, world.ID); err != nil {
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

// WorldLikeCreate likes a world from an owner
// You can request this method with the following cURL request:
//
//	curl -k -X POST https://localhost:4430/1.0/{username}/worlds/{world_name}/likes
//	  --header 'authorization: Bearer <your-jwt-token-here>'
func WorldLikeCreate(owner, worldName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	_, count, em := (&worlds.Service{}).CreateWorldLike(tx, owner, worldName, user)
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

// WorldLikeRemove removes a like from a world.
// You can request this method with the following cURL request:
//
//	curl -k -X DELETE https://localhost:4430/1.0/{username}/worlds/{world_name}/likes
//	  --header 'authorization: Bearer <your-jwt-token-here>'
func WorldLikeRemove(owner, worldName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	_, count, em := (&worlds.Service{}).RemoveWorldLike(tx, owner, worldName, user)
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

// WorldIndividualFileDownload downloads an individual world file
// based on owner, world name, and version.
// You can request this method with the following curl request:
//
//	curl -k -X GET --url https://localhost:4430/1.0/{username}/worlds/{world_name}/{version}/files/{file-path}
//
// eg. curl -k -X GET --url https://localhost:4430/1.0/{username}/worlds/{world_name}/tip/files/model.config
func WorldIndividualFileDownload(owner, worldName string, user *users.User,
	tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	s := &worlds.Service{}
	return IndividualFileDownload(s, owner, worldName, user, tx, w, r)
}

// WorldZip returns a single world as a zip file
// You can request this method with the following curl request:
//
//	curl -k -X GET --url https://localhost:4430/1.0/{username}/worlds/{world-name}/{version}/{world-name}.zip
func WorldZip(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	// Get the world version
	version, valid := mux.Vars(r)["version"]
	// If the version does not exist
	if !valid {
		version = ""
	}

	world, zipPath, ver, em := (&worlds.Service{}).DownloadZip(r.Context(), tx,
		owner, name, version, user, r.UserAgent())
	if em != nil {
		return nil, em
	}

	zipFileName := fmt.Sprintf("world-%s.zip", *world.UUID)

	// Remove request header to always serve fresh
	r.Header.Del("If-Modified-Since")
	// Set zip response headers
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipFileName))
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

	// Serve the zip file contents
	// Note: ServeFile should be always last line, after all headers were set.
	http.ServeFile(w, r, *zipPath)
	return nil, nil
}

// ReportWorldCreate reports a model.
// You can request this method with the following curl request:
//
//	curl -k -X POST --url https://localhost:4430/1.0/{username}/worlds/{model-name}/report
func ReportWorldCreate(owner, name string, user *users.User, tx *gorm.DB,
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

	var createWorldReport worlds.CreateReport

	if em := ParseStruct(&createWorldReport, r, true); em != nil {
		return nil, em
	}

	if _, em := (&worlds.Service{}).CreateWorldReport(tx, owner, name, createWorldReport.Reason); em != nil {
		return nil, em
	}

	if err := tx.Commit().Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	if _, em := generics.SendReportEmail(name, owner, "worlds", createWorldReport.Reason, r); em != nil {
		return nil, em
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	return nil, nil
}

// createWorldFn is a callback func that "creation handlers" will pass to doCreateWorld.
// It is expected that createFn will have the real logic for the world creation.
type createWorldFn func(tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter, r *http.Request) (*worlds.World, *gz.ErrMsg)

// doCreateWorld provides the pre and post steps needed to create or clone a world.
// Handlers should invoke this function and pass a createWorldFn callback.
func doCreateWorld(tx *gorm.DB, cb createWorldFn, w http.ResponseWriter, r *http.Request) (*worlds.World, *gz.ErrMsg) {
	// Extract the owner of the new world from the request.
	jwtUser, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	// invoke the actual createWorldFn (the callback function)
	world, em := cb(tx, jwtUser, w, r)
	if em != nil {
		return nil, em
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		os.Remove(*world.Location)
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNoDatabase, err)
	}

	infoStr := "A new world has been created:" +
		"\n\t name: " + *world.Name +
		"\n\t owner: " + *world.Owner +
		"\n\t creator: " + *world.Creator +
		"\n\t uuid: " + *world.UUID +
		"\n\t location: " + *world.Location +
		"\n\t UploadDate: " + world.UploadDate.UTC().Format(time.RFC3339) +
		"\n\t Tags:"
	for _, t := range world.Tags {
		infoStr += *t.Name
	}

	gz.LoggerFromRequest(r).Info(infoStr)
	// TODO: we should NOT be returning the DB world (including ID) to users.
	return world, nil
}

// WorldCreate creates a new world based on input form. It return a world.World or an error.
// You can request this method with the following cURL request:
//
//	curl -k -X POST -F name=my_world -F license=1
//	  -F file=@<full-path-to-file>
//	  https://localhost:4430/1.0/worlds --header 'authorization: Bearer <your-jwt-token-here>'
func WorldCreate(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {
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
	// worlds.CreateWorld is the input form
	var cw worlds.CreateWorld
	if em := ParseStruct(&cw, r, true); em != nil {
		return nil, em
	}

	createFn := func(tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter, r *http.Request) (*worlds.World, *gz.ErrMsg) {
		owner := cw.Owner
		if owner != "" {
			// Ensure the passed in name exists before moving forward
			_, em := users.OwnerByName(tx, owner, true)
			if em != nil {
				return nil, em
			}
		} else {
			owner = *jwtUser.Username
		}

		// Get a new UUID and world folder
		uuidStr, worldPath, err := users.NewUUID(owner, "worlds")
		if err != nil {
			return nil, gz.NewErrorMessageWithBase(gz.ErrorCreatingDir, err)
		}

		// move files from multipart form into new world's folder
		_, em := populateTmpDir(r, true, worldPath)
		if em != nil {
			os.Remove(worldPath)
			return nil, em
		}

		// Create the world via the Worlds Service
		ws := &worlds.Service{}
		world, em := ws.CreateWorld(r.Context(), tx, cw, uuidStr, worldPath, jwtUser)
		if em != nil {
			os.Remove(worldPath)
			return nil, em
		}
		return world, nil
	}

	return doCreateWorld(tx, createFn, w, r)
}

// WorldClone clones a world. Cloning a world means internally creating a new repository
// (git clone) under the current username.
// You can request this method with the following curl request:
//
//	curl -k -X POST --url https://localhost:4430/1.0/{other-username}/worlds/{world-name}/clone
//	 --header 'authorization: Bearer <your-jwt-token-here>'
func WorldClone(owner, name string, ignored *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {
	// Parse form's values and files. https://golang.org/pkg/net/http/#Request.ParseMultipartForm
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
	// worlds.CloneWorld is the input form
	var cw worlds.CloneWorld
	if em := ParseStruct(&cw, r, true); em != nil {
		return nil, em
	}

	createFn := func(tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter, r *http.Request) (*worlds.World, *gz.ErrMsg) {
		// Ask the Models Service to clone the model
		ws := &worlds.Service{}
		clone, em := ws.CloneWorld(r.Context(), tx, owner, name, cw, jwtUser)
		if em != nil {
			return nil, em
		}
		return clone, nil
	}

	return doCreateWorld(tx, createFn, w, r)
}

// WorldUpdate modifies an existing world.
// You can request this method with the following cURL request:
//
//	curl -k -X PATCH -d '{"description":"New Description", "tags":"tag1,tag2"}'
//	  https://localhost:4430/1.0/{username}/worlds/{world-name} -H "Content-Type: application/json"
//	  -H 'Authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func WorldUpdate(owner, worldName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	err := r.ParseMultipartForm(0)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}
	// Delete temporary files from r.ParseMultipartForm(0)
	defer func(form *multipart.Form) {
		err := form.RemoveAll()
		if err != nil {
			log.Println("Failed to close form:", err)
		}
	}(r.MultipartForm)
	// worlds.UpdateWorld is the input form
	var uw worlds.UpdateWorld
	if errMsg := ParseStruct(&uw, r, true); errMsg != nil {
		return nil, errMsg
	}
	if uw.IsEmpty() && r.MultipartForm == nil {
		return nil, gz.NewErrorMessage(gz.ErrorFormInvalidValue)
	}

	// If the user has also sent files, then update the world's version
	var newFilesPath *string
	if r.MultipartForm != nil && len(getRequestFiles(r)) > 0 {
		// first, populate files into tmp dir to avoid overriding world
		// files in case of error.
		tmpDir, err := ioutil.TempDir("", worldName)
		defer os.Remove(tmpDir)
		if err != nil {
			return nil, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
		}
		if _, errMsg := populateTmpDir(r, true, tmpDir); errMsg != nil {
			return nil, errMsg
		}
		newFilesPath = &tmpDir
	}

	uw.Metadata = parseWorldMetadata(r)

	world, em := (&worlds.Service{}).UpdateWorld(r.Context(), tx, owner, worldName,
		uw.Description, uw.Tags, newFilesPath, uw.Private, user, uw.Metadata)
	if em != nil {
		return nil, em
	}

	infoStr := "World has been updated:" +
		"\n\t name: " + *world.Name +
		"\n\t owner: " + *world.Owner +
		"\n\t uuid: " + *world.UUID +
		"\n\t location: " + *world.Location +
		"\n\t UploadDate: " + world.UploadDate.UTC().Format(time.RFC3339) +
		"\n\t Tags:"
	for _, t := range world.Tags {
		infoStr += *t.Name
	}
	gz.LoggerFromRequest(r).Info(infoStr)

	// Encode world into a protobuf message
	fuelWorld := (&worlds.Service{}).WorldToProto(world)
	return &fuelWorld, nil
}

// WorldModelReferences returns the list of external models referenced by a world.
// The returned value will be of type "worlds.ModelIncludes"
// You can request this method with the following curl request:
//
//	curl -k --url https://localhost:4430/1.0/{username}/worlds/{world_name}/{version}/{world_name}/modelrefs
func WorldModelReferences(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	// Get the world version
	version, valid := mux.Vars(r)["version"]
	// If the version does not exist
	if !valid {
		version = ""
	}

	// Prepare pagination
	pr, em := gz.NewPaginationRequest(r)
	if em != nil {
		return nil, em
	}

	ws := &worlds.Service{}
	refs, pagination, em := ws.GetModelReferences(r.Context(), pr, tx, owner, name,
		version, user)
	if em != nil {
		return nil, em
	}

	err := gz.WritePaginationHeaders(*pagination, w, r)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}
	return refs, nil
}

// WorldTransfer transfer ownership of a world to an organization. The source
// owner must have write permissions on the destination organization
//
//	curl -k -X POST -H "Content-Type: application/json" http://localhost:8000/1.0/{username}/worlds/{worldname}/transfer --header "Private-Token: {private-token}" -d '{"destOwner":"{destination_owner_name"}'
//
// \todo Support transfer of worlds to owners other users and organizations.
// This will require some kind of email notifcation to the destination and
// acceptance form.
func WorldTransfer(sourceOwner, worldName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	// Read the request and check permissions.
	transferAsset, em := processTransferRequest(sourceOwner, tx, r)
	if em != nil {
		return nil, em
	}

	// Get the world
	ws := &worlds.Service{}
	world, em := ws.GetWorld(tx, sourceOwner, worldName, user)
	if em != nil {
		extra := fmt.Sprintf("World [%s] not found", worldName)
		return nil, gz.NewErrorMessageWithArgs(gz.ErrorNameNotFound, em.BaseError, []string{extra})
	}

	if em := transferMoveResource(tx, world, sourceOwner, transferAsset.DestOwner); em != nil {
		return nil, em
	}
	tx.Save(&world)

	return &world, nil
}
