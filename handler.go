package main

import (
	"encoding/json"
	"fmt"
	res "github.com/gazebo-web/fuel-server/bundles/common_resources"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/go-playground/form"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"gopkg.in/go-playground/validator.v9"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// NoResult is a middleware that adapts a gz.HandlerWithResult into a gz.Handler.
func NoResult(handler gz.HandlerWithResult) gz.Handler {
	return func(tx *gorm.DB, w http.ResponseWriter, r *http.Request) *gz.ErrMsg {
		_, em := handler(tx, w, r)
		return em
	}
}

// searchFnHandler defines the signature for handlers that accept
// search arguments and return paginated results.
// Arguments:
// p: a pagination request to use.
// owner: optional , for routes that start with a username. eg /{username}/collections.
// order: asc or desc (eg. order=)
// search: the search query in the router (eg. q=)
// user: the user requesting the operation (based on JWT).
// Returns: The searchFnHandler is expected to return paginated results.
type searchFnHandler func(p *gz.PaginationRequest, owner *string, order,
	search string, user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *gz.PaginationResult, *gz.ErrMsg)

// SearchHandler is a middleware handler that wraps a searchFnHandler and
// invokes it with the following extra arguments:
// - p: a configured pagination request
// - owner: got from the route, if any.
// - order and search: got from the URL Query parameters.
// - user: the user requesting the operation. Got from the JWT.
// It returns the list of resources from an owner, and also writes the pagination
// headers into the HTTP response.
func SearchHandler(handler searchFnHandler) gz.HandlerWithResult {
	return func(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {
		// Prepare pagination
		pr, errMsg := gz.NewPaginationRequest(r)
		if errMsg != nil {
			return nil, errMsg
		}

		// Get JWT user
		// it is ok for user to be nil
		user, ok, em2 := getUserFromJWT(tx, r)
		if !ok && (em2.ErrCode != gz.ErrorAuthJWTInvalid &&
			errMsg.ErrCode != gz.ErrorAuthNoUser) {
			return nil, &em2
		}

		// Note: A search query can be composed of multiple parts separated by an
		// encoded ampersand (%26). For example:
		// ?q=name:robot%26tags:drc
		owner, order, search, valid, em3 := readListParams(r, tx)
		if !valid {
			return nil, em3
		}

		var list interface{}
		var pagination *gz.PaginationResult
		var eMsg *gz.ErrMsg

		// Assume that we will need to use the backup search.
		backupSearch := true

		// Do we have a search term and Elastic Search? If so, then let's use it.
		if len(search) > 0 && globals.ElasticSearch != nil {
			if strings.Contains(r.URL.Path, "/models") {
				list, pagination, eMsg = elasticSearch("fuel_models", pr, owner, order, search, user, tx, w, r)

				// Do we need to fallback on our backup search?
				backupSearch = eMsg != nil
			} else if strings.Contains(r.URL.Path, "/worlds") {
				list, pagination, eMsg = elasticSearch("fuel_worlds", pr, owner, order, search, user, tx, w, r)

				// Do we need to fallback on our backup search?
				backupSearch = eMsg != nil
			}
		}

		// Fallback on SQL based search if Elastic Search failed or Elastic Search
		// is not present.
		if backupSearch {
			list, pagination, eMsg = handler(pr, owner, order, search, user, tx, w, r)
		}

		if eMsg != nil {
			return nil, eMsg
		}

		if pagination != nil {
			err := gz.WritePaginationHeaders(*pagination, w, r)
			if err != nil {
				return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
			}
		}

		return list, nil
	}
}

type pagHandler func(p *gz.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.PaginationResult, *gz.ErrMsg)

// PaginationHandlerWithUser is a middleware handler that wraps a pageHandler
// function and invokes it with the following extra arguments:
// - p: a configured pagination request
// - user: the user requesting the operation. Got from the JWT.
// If failIfNoUser is true the the middleware will fail if the JWT does not
// represent a valid user. Otherwise will pass 'nil' to the inner handler.
// It returns the list of resources from an owner, and also writes the pagination
// headers into the HTTP response.
func PaginationHandlerWithUser(handler pagHandler, failIfNoUser bool) gz.HandlerWithResult {
	return func(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

		// Prepare pagination
		pr, em := gz.NewPaginationRequest(r)
		if em != nil {
			return nil, em
		}

		// Get JWT user
		user, ok, errMsg := getUserFromJWT(tx, r)
		if !ok && (failIfNoUser || (errMsg.ErrCode != gz.ErrorAuthJWTInvalid &&
			errMsg.ErrCode != gz.ErrorAuthNoUser)) {
			return nil, &errMsg
		}

		list, pagination, em := handler(pr, user, tx, w, r)
		if em != nil {
			return nil, em
		}

		err := gz.WritePaginationHeaders(*pagination, w, r)
		if err != nil {
			return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
		}
		return list, nil
	}
}

// PaginationHandler is a middleware handler that wraps a pageHandler function and
// invokes it with the following extra arguments:
// - p: a configured pagination request
// - user: the user requesting the operation. Got from the JWT.
// It returns the list of resources from an owner, and also writes the pagination
// headers into the HTTP response.
func PaginationHandler(handler pagHandler) gz.HandlerWithResult {
	return PaginationHandlerWithUser(handler, false)
}

type nameAndOwner func(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg)

// NameOwnerHandler is a middleware handler that wraps a nameAndOwner function and
// invokes it with the following extra arguments:
// - owner: the owner name got from the route.
// - name: a resource name.
// - user: the user requesting the operation. Can be nil. Got from the JWT.
// Note: if the failIfNoUser is true , this handler will return errors if the JWT
// is invalid or does not exist in DB. Otherwise, if false, the user will be nil.
// It returns the result from invoking the inner handler.
func NameOwnerHandler(nameArg string, failIfNoUser bool,
	handler nameAndOwner) gz.HandlerWithResult {

	return func(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

		// Extract the user associated with the JWT, if any.
		user, ok, errMsg := getUserFromJWT(tx, r)
		if !ok && ((errMsg.ErrCode != gz.ErrorAuthJWTInvalid &&
			errMsg.ErrCode != gz.ErrorAuthNoUser) || failIfNoUser) {
			return nil, &errMsg
		}

		name, owner, em := readOwnerNameParams(nameArg, tx, r)
		if em != nil {
			return nil, em
		}

		result, em := handler(owner, name, user, tx, w, r)
		if em != nil {
			return nil, em
		}
		return result, nil
	}
}

// readOwnerNameParams is a helper function that reads the owner's name,
// and resource name from the url.
func readOwnerNameParams(nameArg string, tx *gorm.DB,
	r *http.Request) (name, owner string, em *gz.ErrMsg) {

	// Get the parameters
	params := mux.Vars(r)
	// Get the owner
	var valid bool
	var uniqueOwner *string
	uniqueOwner, valid, em = readOwner(tx, r, "username", true)
	// If the owner does not exist
	if !valid {
		if em.ErrCode == gz.ErrorUserNotInRequest {
			// override the error if user not present in request
			em = gz.NewErrorMessage(gz.ErrorOwnerNotInRequest)
		}
		return
	}

	// Get the resource name
	name, valid = params[nameArg]
	// If the name does not exist
	if !valid {
		em = gz.NewErrorMessage(gz.ErrorNameWrongFormat)
		return
	}

	owner = *uniqueOwner
	em = nil
	return
}

// readListParams is a helper function that reads the "owner", the "order" and "q"
// parameters used to get a list of resources.
// The order parameter can be asc or desc.
// The q parameter is the search query.
func readListParams(r *http.Request, tx *gorm.DB) (owner *string, order, search string, valid bool, em *gz.ErrMsg) {
	// Get the requested owner, if any
	owner, valid, em = readOwner(tx, r, "username", true)
	if !valid && em.ErrCode != gz.ErrorUserNotInRequest {
		// Return the error if it's different than ErrorUserNotInRequest
		return
	}
	// Get the parameters
	queryP := r.URL.Query()
	orderParam, ok := queryP["order"]
	if ok {
		order = orderParam[0]
	}
	sc, ok := queryP["q"]
	if ok {
		search = sc[0]
	}
	valid = true
	return
}

type nameFn func(name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg)

// NameHandler is a middleware handler that wraps a nameFn function and
// invokes it with the following extra arguments:
// - name: the name got from the route.
// - user: the user requesting the operation. Can be nil. Got from the JWT.
// Note: if the failIfNoUser is true , this handler will return errors if the JWT
// is invalid or does not exist in DB. Otherwise, if false, the user will be nil.
// It returns the result from invoking the inner handler.
func NameHandler(nameArg string, failIfNoUser bool,
	handler nameFn) gz.HandlerWithResult {

	return func(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {
		// Extract the user associated with the JWT, if any.
		user, ok, errMsg := getUserFromJWT(tx, r)
		if !ok && ((errMsg.ErrCode != gz.ErrorAuthJWTInvalid &&
			errMsg.ErrCode != gz.ErrorAuthNoUser) || failIfNoUser) {
			return nil, &errMsg
		}

		// Get the resource name
		params := mux.Vars(r)
		name, valid := params[nameArg]
		// If the name does not exist
		if !valid {
			return nil, gz.NewErrorMessage(gz.ErrorNameWrongFormat)
		}

		result, em := handler(name, user, tx, w, r)
		if em != nil {
			return nil, em
		}
		return result, nil
	}
}

// readUser returns the owner name based on the URI requested.
// param[in] The params key to look for.
// deleted[in] Whether to include deleted users in the search query.
func readOwner(tx *gorm.DB, r *http.Request, param string, deleted bool) (*string, bool, *gz.ErrMsg) {

	// Extract the owner from the request.
	params := mux.Vars(r)
	// Get the owner
	name, present := params[param]
	// If the "owner" key does not exist
	if !present {
		return nil, false, gz.NewErrorMessage(gz.ErrorUserNotInRequest)
	}

	owner, em := users.OwnerByName(tx, name, deleted)
	if em != nil {
		return nil, false, em
	}

	errMsg := gz.ErrorMessageOK()
	return owner.Name, true, &errMsg
}

// ParseStruct reads the http request and decodes sent values
// into the given struct. It uses the isForm bool to know if the values comes
// as "request.Form" values or as "request.Body".
// It also calls validator to validate the struct fields.
func ParseStruct(s interface{}, r *http.Request, isForm bool) *gz.ErrMsg {
	// TODO: stop using globals. Move to own packages.
	if isForm {
		if errs := globals.FormDecoder.Decode(s, r.Form); errs != nil {
			return gz.NewErrorMessageWithArgs(gz.ErrorFormInvalidValue, errs,
				getDecodeErrorsExtraInfo(errs))
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(s); err != nil {
			return gz.NewErrorMessageWithBase(gz.ErrorUnmarshalJSON, err)
		}
	}
	// Validate struct values
	if em := ValidateStruct(s); em != nil {
		return em
	}
	return nil
}

// ValidateStruct Validate struct values using golang validator.v9
func ValidateStruct(s interface{}) *gz.ErrMsg {
	if errs := globals.Validate.Struct(s); errs != nil {
		return gz.NewErrorMessageWithArgs(gz.ErrorFormInvalidValue, errs,
			getValidationErrorsExtraInfo(errs))
	}
	return nil
}

// Builds the ErrMsg extra info from the given DecodeErrors
func getDecodeErrorsExtraInfo(err error) []string {
	errs := err.(form.DecodeErrors)
	extra := make([]string, 0, len(errs))
	for field, er := range errs {
		extra = append(extra, fmt.Sprintf("Field: %s. %v", field, er.Error()))
	}
	return extra
}

// Builds the ErrMsg extra info from the given ValidationErrors
func getValidationErrorsExtraInfo(err error) []string {
	validationErrors := err.(validator.ValidationErrors)
	extra := make([]string, 0, len(validationErrors))
	for _, fe := range validationErrors {
		extra = append(extra, fmt.Sprintf("%s:%v", fe.StructField(), fe.Value()))
	}
	return extra
}

// getUserFromJWT returns the User associated to the http request's JWT token.
// This function can return ErrorAuthJWTInvalid if the token cannot be
// read, or ErrorAuthNoUser no user with such identity exists in the DB.
func getUserFromJWT(tx *gorm.DB, r *http.Request) (*users.User, bool, gz.ErrMsg) {
	var user *users.User

	// Check if a Private-Token is used, which will supercede a JWT token.
	if token := r.Header.Get("Private-Token"); len(token) > 0 {
		var accessToken *gz.AccessToken
		var err *gz.ErrMsg
		if accessToken, err = gz.ValidateAccessToken(token, tx); err != nil {
			return nil, false, gz.ErrorMessage(gz.ErrorUnauthorized)
		}

		user = new(users.User)
		if err := tx.Where("id = ?", accessToken.UserID).First(user).Error; err != nil {
			return nil, false, *gz.NewErrorMessage(gz.ErrorUnauthorized)
		}
	} else {
		identity, valid := gz.GetUserIdentity(r)
		if !valid {
			return nil, false, gz.ErrorMessage(gz.ErrorAuthJWTInvalid)
		}

		var em *gz.ErrMsg
		user, em = users.ByIdentity(tx, identity, false)
		if em != nil {
			return nil, false, *em
		}
	}

	errMsg := gz.ErrorMessageOK()
	return user, true, errMsg
}

// getRequestFiles return the multipart form files from the request field "file"
// or "file[]"
func getRequestFiles(r *http.Request) []*multipart.FileHeader {
	// The "file" Form field contains the multiple files.
	var files []*multipart.FileHeader
	files = r.MultipartForm.File["file"]
	fLen := len(files)
	if fLen == 0 {
		files = r.MultipartForm.File["file[]"]
		fLen = len(files)
		if fLen == 0 {
			return nil
		}
	}
	return files
}

func pathIncludesAny(path string, slice []string) bool {
	for _, s := range slice {
		if strings.Contains(path, s) {
			return true
		}
	}
	return false
}

var invalidFileNames = []string{".git", ".gitconfig", ".gitignore", ".hg",
	".hgignore", ".hgrc", ".hgtags"}

// populateTmpDir takes the incoming multipart form request
// and populates a given dirpath with the POSTed files. If the request contains all files
// within a sigle root folder and rmDir argument is true, then that outer folder
// will be removed, leaving all children files at the root level.
// Returns the given dirpath, or an ErrMsg.
func populateTmpDir(r *http.Request, rmDir bool, dirpath string) (string, *gz.ErrMsg) {
	// The "file" Form field contains the multiple files.
	files := getRequestFiles(r)
	fLen := len(files)
	if fLen == 0 {
		return "", gz.NewErrorMessage(gz.ErrorFormMissingFiles)
	}

	// First check if all files are in the same root dir and if we should rm it.
	rmDir, outDir, em := getOuterDir(files, rmDir)
	if em != nil {
		return "", em
	}

	// Process files
	for _, fh := range files {
		fn, err := extractFilepath(fh)
		if err != nil {
			continue
		}
		if len(fn) == 0 {
			continue
		}
		file, err := fh.Open()
		defer func(file multipart.File) {
			err := file.Close()
			if err != nil {
				log.Println("Failed to close file:", err)
			}
		}(file)
		if err != nil {
			return "", gz.NewErrorMessageWithBase(gz.ErrorForm, err)
		}
		// If file path includes any of the items from the list of invalid names,
		// then error
		if pathIncludesAny(fn, invalidFileNames) {
			return "", gz.NewErrorMessageWithArgs(gz.ErrorFormInvalidValue, err, []string{fn})
		}
		// Need to remove outer dir?
		if rmDir {
			fn = fn[len(outDir):]
		}
		// Create the destination file in target dirpath folder
		// This assumes given file name can have slashes (subfolders)
		fileFullPath := filepath.Join(dirpath, fn)
		// Sanity check: check for duplicate file entries
		if _, err := os.Stat(fileFullPath); err == nil {
			return "", gz.NewErrorMessageWithArgs(gz.ErrorFormDuplicateFile, err, []string{fileFullPath})
		}
		if err := os.MkdirAll(filepath.Dir(fileFullPath), 0711); err != nil {
			return "", gz.NewErrorMessageWithBase(gz.ErrorCreatingDir, err)
		}
		dest, err := os.Create(fileFullPath)
		defer func(dest *os.File) {
			err := dest.Close()
			if err != nil {
				log.Println("Failed to close file:", err)
			}
		}(dest)
		if err != nil {
			return "", gz.NewErrorMessageWithBase(gz.ErrorForm, err)
		}
		// Now copy contents
		if _, err := io.Copy(dest, file); err != nil {
			return "", gz.NewErrorMessageWithBase(gz.ErrorForm, err)
		}
	}

	return dirpath, nil
}

// extractFilepath extracts the full filename from the Content-Disposition header.
// If it's not found, it returns the default Filename from the given multipart.FileHeader
func extractFilepath(fh *multipart.FileHeader) (string, error) {
	cd, ok := fh.Header["Content-Disposition"]
	if !ok || len(cd) == 0 {
		return fh.Filename, nil
	}
	_, params, err := mime.ParseMediaType(cd[0])
	if err != nil {
		return "", err
	}
	fn, ok := params["filename"]
	if !ok {
		return fh.Filename, nil
	}
	return fn, nil
}

// getOuterDir determines if the outer directory should be removed, if so, it returns the outer directory
// name.
func getOuterDir(files []*multipart.FileHeader, remove bool) (bool, string, *gz.ErrMsg) {
	var outDir string
	first, err := extractFilepath(files[0])
	if err != nil {
		return false, "", gz.NewErrorMessageWithBase(gz.ErrorForm, err)
	}
	if !strings.Contains(first, "/") {
		// No folder in first file, then there is no common folder
		remove = false
	} else {
		// Find out the folder name
		if strings.HasPrefix(first, "/") {
			outDir = "/" + strings.SplitAfter(filepath.Clean(first), "/")[1]
		} else {
			outDir = strings.SplitAfter(filepath.Clean(first), "/")[0]
		}
		for i := 0; i < len(files) && remove; i++ {
			fn, err := extractFilepath(files[i])
			if err != nil {
				continue
			}
			remove = strings.HasPrefix(fn, outDir)
		}
	}
	return remove, outDir, nil
}

// writeIgnResourceVersionHeader writes the ign resource version header into the given response.
func writeIgnResourceVersionHeader(w http.ResponseWriter, version int) {
	w.Header().Set("X-Ign-Resource-Version", strconv.Itoa(version))
}

// serveFileOrLink returns a link to download the provided zip file from if linkRequested is set to true.
//
//	If linkRequested is set to true:
//		- it will write the URL as a plain text.
//		- link must contain the URL where to download the resource
//	If linkRequested is set to false:
//		- it will stream the file from the host machine directly to the client
//		- link must contain the URL where to download the resource
func serveFileOrLink(w http.ResponseWriter, r *http.Request, linkRequested bool, link string, res res.Resource, version int) error {
	writeIgnResourceVersionHeader(w, version)

	if linkRequested {
		return serveLink(w, link)
	}
	return serveZipFile(w, r, res, version, link)
}

// serveZipFile serves a zip file located in path in the HTTP response.
// This function also writes the HTTP status code to 200 and sets the Content Type to application/zip since
// it's streaming the zip file directly to the client.
func serveZipFile(w http.ResponseWriter, r *http.Request, res res.Resource, version int, path string) error {
	// Set content type so clients can identify a zip file will be downloaded
	w.Header().Set("Content-Type", "application/zip")
	// Remove request header to always serve fresh
	r.Header.Del("If-Modified-Since")
	// Set zip response headers
	zipFileName := fmt.Sprintf("model-%sv%d.zip", *res.GetUUID(), version)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipFileName))
	http.ServeFile(w, r, path)
	return nil
}

// serveLink writes a link to a zip file into the HTTP response.
// This function also writes the HTTP status code to 200 and sets the Content Type to text/plain given that
// it's returning a link to the zip file.
func serveLink(w http.ResponseWriter, link string) error {
	// Set content type so clients can identify a link is being provided
	w.Header().Set("Content-Type", "text/plain")
	// Return the link
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(link))
	return err
}

// isLinkRequested returns true if a link was explicitly requested in the given HTTP request.
func isLinkRequested(r *http.Request) bool {
	return strings.ToLower(r.URL.Query().Get("link")) == "true"
}
