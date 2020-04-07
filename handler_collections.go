package main

import (
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/collections"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"github.com/jinzhu/gorm"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"
)

// CollectionList returns the list of collections from a team/user. The returned
// value will be of type "collections.Collections"
// It follows the func signature defined by type "searchHandler".
// You can request this method with the following curl request:
//     curl -k -X GET --url https://localhost:4430/1.0/collections
// or  curl -k -X GET --url https://localhost:4430/1.0/collections.json
// or  curl -k -X GET --url https://localhost:4430/1.0/{username}/collections with all the
// above format variants.
// func CollectionList(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
func CollectionList(p *ign.PaginationRequest, owner *string, order, search string,
	user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	var extend bool
	// Check if we need to only return collections that the user can extend
	v, ok := r.URL.Query()["extend"]
	if ok {
		extend, _ = strconv.ParseBool(v[0])
	}
	s := &collections.Service{}
	return s.CollectionList(p, tx, owner, order, search, extend, user)
}

// CollectionIndex returns a single Collection. The returned value will be of
// type "collections.Collection".
// You can request this method with the following curl request:
//  curl -k -H "Content-Type: application/json" -X GET https://localhost:4430/1.0/{username}/collections/{name}
func CollectionIndex(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	s := &collections.Service{}
	return s.GetCollection(tx, owner, name, user)
}

// CollectionRemove removes a Collection based on owner and name
// You can request this method with the following curl request:
//   curl -k -X DELETE --url https://localhost:4430/1.0/{username}/collections/{name}
func CollectionRemove(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	if em := (&collections.Service{}).RemoveCollection(tx, owner, name, user); em != nil {
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

// createCollectionFn is a callback func that "creation handlers" will pass to
// doCreateCollection. It is expected that createFn will have the real logic for
// the Collection creation.
type createCollectionFn func(tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter,
	r *http.Request) (*collections.Collection, *ign.ErrMsg)

// doCreateCollection provides the pre and post steps needed to create a collection.
// Handlers should invoke this function and pass a createCollectionFn callback.
func doCreateCollection(tx *gorm.DB, cb createCollectionFn, w http.ResponseWriter,
	r *http.Request) (*collections.Collection, *ign.ErrMsg) {
	// Extract the owner from the request.
	jwtUser, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	// invoke the actual createCollectionFn (the callback function)
	col, em := cb(tx, jwtUser, w, r)
	if em != nil {
		return nil, em
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorNoDatabase, err)
	}

	infoStr := "A new collection has been created:" +
		"\n\t name: " + *col.Name +
		"\n\t owner: " + *col.Owner +
		"\n\t creator: " + *col.Creator +
		"\n\t uuid: " + *col.UUID +
		"\n\t CreationDate: " + col.CreatedAt.UTC().Format(time.RFC3339)

	ign.LoggerFromRequest(r).Info(infoStr)
	// TODO: we should NOT be returning the DB record (including ID) to users.
	return col, nil
}

// CollectionCreate creates a new collection based on input form. It returns a
// collections.Collection or an error.
// You can request this method with the following cURL request:
//    curl -k -H "Content-Type: application/json" -X POST -d '{"name":"my collection",
//    "description":"a super cool collection", owner:"name"}'
//    https://localhost:4430/1.0/collections
//    --header 'authorization: Bearer <your-jwt-token-here>'
func CollectionCreate(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	var cc collections.CreateCollection
	if em := ParseStruct(&cc, r, false); em != nil {
		return nil, em
	}

	createFn := func(tx *gorm.DB, jwtUser *users.User, w http.ResponseWriter,
		r *http.Request) (*collections.Collection, *ign.ErrMsg) {

		// Create the collection via the Collections Service
		s := &collections.Service{}
		col, em := s.CreateCollection(r.Context(), tx, cc, jwtUser)
		if em != nil {
			return nil, em
		}
		return col, nil
	}

	return doCreateCollection(tx, createFn, w, r)
}

// CollectionUpdate modifies an existing collection.
// You can request this method with the following cURL request:
//    curl -k -X PATCH -F description="New Description"
//      -F file=@<full-path-to-file;filename=aFileName>
//      https://localhost:4430/1.0/{username}/collections/{name}
//      -H 'Authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func CollectionUpdate(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	r.ParseMultipartForm(0)
	// Delete temporary files from r.ParseMultipartForm(0)
	defer r.MultipartForm.RemoveAll()

	var uc collections.UpdateCollection
	if errMsg := ParseStruct(&uc, r, true); errMsg != nil {
		return nil, errMsg
	}

	bFiles := r.MultipartForm != nil && len(getRequestFiles(r)) > 0
	if uc.IsEmpty() && !bFiles {
		return nil, ign.NewErrorMessage(ign.ErrorFormInvalidValue)
	}

	// If the user has also sent files, then update them
	var newFilesPath *string
	if bFiles {
		// first, populate files into tmp dir to avoid overriding original
		// files in case of error.
		tmpDir, err := ioutil.TempDir("", name)
		defer os.Remove(tmpDir)
		if err != nil {
			return nil, ign.NewErrorMessageWithBase(ign.ErrorRepo, err)
		}
		if _, errMsg := populateTmpDir(r, false, tmpDir); errMsg != nil {
			return nil, errMsg
		}
		newFilesPath = &tmpDir

	}

	col, em := (&collections.Service{}).UpdateCollection(r.Context(), tx, owner,
		name, uc.Description, newFilesPath, uc.Private, user)
	if em != nil {
		return nil, em
	}

	infoStr := "Collection has been updated:" +
		"\n\t name: " + *col.Name +
		"\n\t owner: " + *col.Owner +
		"\n\t uuid: " + *col.UUID +
		"\n\t location: " + *col.Location +
		"\n\t CreatedAt: " + col.CreatedAt.UTC().Format(time.RFC3339) +
		"\n\t UpdatedAt: " + col.UpdatedAt.UTC().Format(time.RFC3339)

	ign.LoggerFromRequest(r).Info(infoStr)

	return &col, nil
}

// CollectionModelsList returns the list of models of a collection.
// You can request this method with the following cURL request:
//   curl -k https://localhost:4430/1.0/{username}/collections/{col_name}/models
func CollectionModelsList(colOwner, colName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	return collectionAssetList(colOwner, colName, collections.TModel, user, tx, w, r)
}

// CollectionModelAdd associates a model to a collection.
// You can request this method with the following cURL request:
//   curl -k -d '{"name":"model name", owner:"model owner"}'
//      -X POST https://localhost:4430/1.0/{username}/collections/{col_name}/models
//      --header 'authorization: Bearer <your-jwt-token-here>'
func CollectionModelAdd(colOwner, colName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
	return collectionAssetAdd(colOwner, colName, collections.TModel, user, tx, w, r)
}

// CollectionModelRemove removes a model from a collection.
// You can request this method with the following cURL request:
//   curl -k -d '{"name":"model name", owner:"model owner"}'
//      -X DELETE https://localhost:4430/1.0/{username}/collections/{col_name}/models
//      --header 'authorization: Bearer <your-jwt-token-here>'
func CollectionModelRemove(colOwner, colName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
	return collectionAssetRemove(colOwner, colName, collections.TModel, user, tx, w, r)
}

// CollectionWorldAdd associates a world to a collection.
// You can request this method with the following cURL request:
//   curl -k -d '{"name":"world name", owner:"world owner"}'
//      -X POST https://localhost:4430/1.0/{username}/collections/{col_name}/worlds
//      --header 'authorization: Bearer <your-jwt-token-here>'
func CollectionWorldAdd(colOwner, colName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
	return collectionAssetAdd(colOwner, colName, collections.TWorld, user, tx, w, r)
}

// CollectionWorldRemove removes a world from a collection.
// You can request this method with the following cURL request:
//   curl -k -d '{"name":"world name", owner:"world owner"}'
//      -X DELETE https://localhost:4430/1.0/{username}/collections/{col_name}/worlds
//      --header 'authorization: Bearer <your-jwt-token-here>'
func CollectionWorldRemove(colOwner, colName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
	return collectionAssetRemove(colOwner, colName, collections.TWorld, user, tx, w, r)
}

// CollectionWorldsList returns the list of worlds of a collection.
// You can request this method with the following cURL request:
//   curl -k https://localhost:4430/1.0/{username}/collections/{col_name}/worlds
func CollectionWorldsList(colOwner, colName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	return collectionAssetList(colOwner, colName, collections.TWorld, user, tx, w, r)
}

// collectionAssetAdd associates an asset to a collection. It requires the
// asset type as mandatory argument.
func collectionAssetAdd(colOwner, colName, assetType string, user *users.User,
	tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	var no collections.NameOwnerPair
	if em := ParseStruct(&no, r, false); em != nil {
		return nil, em
	}

	if _, em := (&collections.Service{}).AddAsset(r.Context(), tx, colOwner, colName,
		no, assetType, user); em != nil {
		return nil, em
	}

	// commit the DB transaction
	// Note: we commit the TX here on purpose, to be able to detect DB errors
	// before writing "data" to ResponseWriter. Once you write data (not headers)
	// into it the status code is set to 200 (OK).
	if err := tx.Commit().Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	return nil, nil
}

// collectionAssetRemove deletes an asset from a collection. It requires the
// asset type as mandatory argument.
func collectionAssetRemove(colOwner, colName, assetType string, user *users.User,
	tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	var no collections.NameOwnerPair
	// Read the name and owner from URL query. DELETE does not allow body.
	no.Owner = r.URL.Query().Get("o")
	no.Name = r.URL.Query().Get("n")
	// Validate struct values
	if em := ValidateStruct(&no); em != nil {
		return nil, em
	}

	if _, em := (&collections.Service{}).RemoveAsset(r.Context(), tx, colOwner, colName,
		no, assetType, user); em != nil {
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

// collectionAssetList returns the list of assets associated to a collection.
// The returned value will be paginated and will be of
// type "collections.CollectionAssets".
// The assetType argument can be used filter assets by type, eg: model|world.
func collectionAssetList(colOwner, colName, assetType string, user *users.User,
	tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	// Prepare pagination
	pr, em := ign.NewPaginationRequest(r)
	if em != nil {
		return nil, em
	}

	s := &collections.Service{}
	assets, pagination, em := s.GetCollectionAssets(pr, tx, colOwner, colName,
		assetType, user)
	if em != nil {
		return nil, em
	}

	ign.WritePaginationHeaders(*pagination, w, r)

	return assets, nil
}

// ModelCollections returns the list of collections associated to a given model.
// You can request this method with the following cURL request:
//   curl -k https://localhost:4430/1.0/{username}/models/{model_name}/collections
func ModelCollections(owner, modelName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	no := collections.NameOwnerPair{Name: modelName, Owner: owner}
	return associatedCollectionsList(collections.TModel, no, user, tx, w, r)
}

// WorldCollections returns the list of collections associated to a given world.
// You can request this method with the following cURL request:
//   curl -k https://localhost:4430/1.0/{username}/worlds/{world_name}/collections
func WorldCollections(owner, worldName string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	no := collections.NameOwnerPair{Name: worldName, Owner: owner}
	return associatedCollectionsList(collections.TWorld, no, user, tx, w, r)
}

// associatedCollectionsList returns the list of collections to which an asset, ie.
// a model or world, belongs to.
func associatedCollectionsList(assetType string, no collections.NameOwnerPair,
	user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	// Prepare pagination
	pr, em := ign.NewPaginationRequest(r)
	if em != nil {
		return nil, em
	}

	s := &collections.Service{}
	cols, pagination, em := s.GetAssociatedCollections(pr, tx, no, assetType, user)
	if em != nil {
		return nil, em
	}

	ign.WritePaginationHeaders(*pagination, w, r)

	return cols, nil
}

// CollectionIndividualFileDownload downloads an individual file from a collection.
// You can request this method with the following curl request:
//   curl -k -X GET --url https://localhost:4430/1.0/{username}/collections/{name}/{version}/files/{file-path}
// eg. curl -k -X GET --url https://localhost:4430/1.0/{username}/collections/{name}/tip/files/thumbnails/logo.png
func CollectionIndividualFileDownload(owner, name string, user *users.User,
	tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	s := &collections.Service{}
	return IndividualFileDownload(s, owner, name, user, tx, w, r)
}