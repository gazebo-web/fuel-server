package main

import (
	"gitlab.com/ignitionrobotics/web/ign-go"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	"encoding/json"
	"fmt"
	"github.com/go-playground/form"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"gopkg.in/go-playground/validator.v9"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// NoResult is a middleware that adapts a ign.HandlerWithResult into a ign.Handler.
func NoResult(handler ign.HandlerWithResult) ign.Handler {
	return func(tx *gorm.DB, w http.ResponseWriter, r *http.Request) *ign.ErrMsg {
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
type searchFnHandler func(p *ign.PaginationRequest, owner *string, order,
	search string, user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg)

// SearchHandler is a middleware handler that wraps a searchFnHandler and
// invokes it with the following extra arguments:
// - p: a configured pagination request
// - owner: got from the route, if any.
// - order and search: got from the URL Query parameters.
// - user: the user requesting the operation. Got from the JWT.
// It returns the list of resources from an owner, and also writes the pagination
// headers into the HTTP response.
func SearchHandler(handler searchFnHandler) ign.HandlerWithResult {
	return func(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
		// Prepare pagination
		pr, em := ign.NewPaginationRequest(r)
		if em != nil {
			return nil, em
		}

		// Get JWT user
		// it is ok for user to be nil
		user, ok, errMsg := getUserFromJWT(tx, r)
		if !ok && (errMsg.ErrCode != ign.ErrorAuthJWTInvalid &&
			errMsg.ErrCode != ign.ErrorAuthNoUser) {
			return nil, &errMsg
		}

		owner, order, search, valid, em := readListParams(r, tx)
		if !valid {
			return nil, em
		}

		list, pagination, em := handler(pr, owner, order, search, user, tx, w, r)
		if em != nil {
			return nil, em
		}

		ign.WritePaginationHeaders(*pagination, w, r)

		return list, nil
	}
}

type pagHandler func(p *ign.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg)

// PaginationHandlerWithUser is a middleware handler that wraps a pageHandler
// function and invokes it with the following extra arguments:
// - p: a configured pagination request
// - user: the user requesting the operation. Got from the JWT.
// If failIfNoUser is true the the middleware will fail if the JWT does not
// represent a valid user. Otherwise will pass 'nil' to the inner handler.
// It returns the list of resources from an owner, and also writes the pagination
// headers into the HTTP response.
func PaginationHandlerWithUser(handler pagHandler, failIfNoUser bool) ign.HandlerWithResult {
	return func(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

		// Prepare pagination
		pr, em := ign.NewPaginationRequest(r)
		if em != nil {
			return nil, em
		}

		// Get JWT user
		user, ok, errMsg := getUserFromJWT(tx, r)
		if !ok && (failIfNoUser || (errMsg.ErrCode != ign.ErrorAuthJWTInvalid &&
			errMsg.ErrCode != ign.ErrorAuthNoUser)) {
			return nil, &errMsg
		}

		list, pagination, em := handler(pr, user, tx, w, r)
		if em != nil {
			return nil, em
		}

		ign.WritePaginationHeaders(*pagination, w, r)
		return list, nil
	}
}

// PaginationHandler is a middleware handler that wraps a pageHandler function and
// invokes it with the following extra arguments:
// - p: a configured pagination request
// - user: the user requesting the operation. Got from the JWT.
// It returns the list of resources from an owner, and also writes the pagination
// headers into the HTTP response.
func PaginationHandler(handler pagHandler) ign.HandlerWithResult {
	return PaginationHandlerWithUser(handler, false)
}

type nameAndOwner func(owner, name string, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg)

// NameOwnerHandler is a middleware handler that wraps a nameAndOwner function and
// invokes it with the following extra arguments:
// - owner: the owner name got from the route.
// - name: a resource name.
// - user: the user requesting the operation. Can be nil. Got from the JWT.
// Note: if the failIfNoUser is true , this handler will return errors if the JWT
// is invalid or does not exist in DB. Otherwise, if false, the user will be nil.
// It returns the result from invoking the inner handler.
func NameOwnerHandler(nameArg string, failIfNoUser bool,
	handler nameAndOwner) ign.HandlerWithResult {

	return func(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

		// Extract the user associated with the JWT, if any.
		user, ok, errMsg := getUserFromJWT(tx, r)
		if !ok && ((errMsg.ErrCode != ign.ErrorAuthJWTInvalid &&
			errMsg.ErrCode != ign.ErrorAuthNoUser) || failIfNoUser) {
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
	r *http.Request) (name, owner string, em *ign.ErrMsg) {

	// Get the parameters
	params := mux.Vars(r)
	// Get the owner
	var valid bool
	var uniqueOwner *string
	uniqueOwner, valid, em = readOwner(tx, r, "username", true)
	// If the owner does not exist
	if !valid {
		if em.ErrCode == ign.ErrorUserNotInRequest {
			// override the error if user not present in request
			em = ign.NewErrorMessage(ign.ErrorOwnerNotInRequest)
		}
		return
	}

	// Get the resource name
	name, valid = params[nameArg]
	// If the name does not exist
	if !valid {
		em = ign.NewErrorMessage(ign.ErrorNameWrongFormat)
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
func readListParams(r *http.Request, tx *gorm.DB) (owner *string, order, search string, valid bool, em *ign.ErrMsg) {
	// Get the requested owner, if any
	owner, valid, em = readOwner(tx, r, "username", true)
	if !valid && em.ErrCode != ign.ErrorUserNotInRequest {
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
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg)

// NameHandler is a middleware handler that wraps a nameFn function and
// invokes it with the following extra arguments:
// - name: the name got from the route.
// - user: the user requesting the operation. Can be nil. Got from the JWT.
// Note: if the failIfNoUser is true , this handler will return errors if the JWT
// is invalid or does not exist in DB. Otherwise, if false, the user will be nil.
// It returns the result from invoking the inner handler.
func NameHandler(nameArg string, failIfNoUser bool,
	handler nameFn) ign.HandlerWithResult {

	return func(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {
		// Extract the user associated with the JWT, if any.
		user, ok, errMsg := getUserFromJWT(tx, r)
		if !ok && ((errMsg.ErrCode != ign.ErrorAuthJWTInvalid &&
			errMsg.ErrCode != ign.ErrorAuthNoUser) || failIfNoUser) {
			return nil, &errMsg
		}

		// Get the resource name
		params := mux.Vars(r)
		name, valid := params[nameArg]
		// If the name does not exist
		if !valid {
			return nil, ign.NewErrorMessage(ign.ErrorNameWrongFormat)
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
func readOwner(tx *gorm.DB, r *http.Request, param string, deleted bool) (*string, bool, *ign.ErrMsg) {

	// Extract the owner from the request.
	params := mux.Vars(r)
	// Get the owner
	name, present := params[param]
	// If the "owner" key does not exist
	if !present {
		return nil, false, ign.NewErrorMessage(ign.ErrorUserNotInRequest)
	}

	owner, em := users.OwnerByName(tx, name, deleted)
	if em != nil {
		return nil, false, em
	}

	errMsg := ign.ErrorMessageOK()
	return owner.Name, true, &errMsg
}

// ParseStruct reads the http request and decodes sent values
// into the given struct. It uses the isForm bool to know if the values comes
// as "request.Form" values or as "request.Body".
// It also calls validator to validate the struct fields.
func ParseStruct(s interface{}, r *http.Request, isForm bool) *ign.ErrMsg {
	// TODO: stop using globals. Move to own packages.
	if isForm {
		if errs := globals.FormDecoder.Decode(s, r.Form); errs != nil {
			return ign.NewErrorMessageWithArgs(ign.ErrorFormInvalidValue, errs,
				getDecodeErrorsExtraInfo(errs))
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(s); err != nil {
			return ign.NewErrorMessageWithBase(ign.ErrorUnmarshalJSON, err)
		}
	}
	// Validate struct values
	if em := ValidateStruct(s); em != nil {
		return em
	}
	return nil
}

// ValidateStruct Validate struct values using golang validator.v9
func ValidateStruct(s interface{}) *ign.ErrMsg {
	if errs := globals.Validate.Struct(s); errs != nil {
		return ign.NewErrorMessageWithArgs(ign.ErrorFormInvalidValue, errs,
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
func getUserFromJWT(tx *gorm.DB, r *http.Request) (*users.User, bool, ign.ErrMsg) {
	var user *users.User

	// Check if a Private-Token is used, which will supercede a JWT token.
	if token := r.Header.Get("Private-Token"); len(token) > 0 {
		var accessToken *ign.AccessToken
		var err *ign.ErrMsg
		if accessToken, err = ign.ValidateAccessToken(token, tx); err != nil {
			return nil, false, ign.ErrorMessage(ign.ErrorUnauthorized)
		}

		user = new(users.User)
		if err := tx.Where("id = ?", accessToken.UserID).First(user).Error; err != nil {
			return nil, false, *ign.NewErrorMessage(ign.ErrorUnauthorized)
		}
	} else {
		identity, valid := ign.GetUserIdentity(r)
		if !valid {
			return nil, false, ign.ErrorMessage(ign.ErrorAuthJWTInvalid)
		}

		var em *ign.ErrMsg
		user, em = users.ByIdentity(tx, identity, false)
		if em != nil {
			return nil, false, *em
		}
	}

	errMsg := ign.ErrorMessageOK()
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
func populateTmpDir(r *http.Request, rmDir bool, dirpath string) (string, *ign.ErrMsg) {
	// The "file" Form field contains the multiple files.
	files := getRequestFiles(r)
	fLen := len(files)
	if fLen == 0 {
		return "", ign.NewErrorMessage(ign.ErrorFormMissingFiles)
	}

	// First check if all files are in the same root dir and if we should rm it.
	var outDir string
	if strings.Index(files[0].Filename, "/") < 0 {
		// No folder in first file, then there is no common folder
		rmDir = false
	} else {
		// Find out the folder name
		if strings.HasPrefix(files[0].Filename, "/") {
			outDir = "/" + strings.SplitAfter(filepath.Clean(files[0].Filename), "/")[1]
		} else {
			outDir = strings.SplitAfter(filepath.Clean(files[0].Filename), "/")[0]
		}
		for i := 0; i < fLen && rmDir; i++ {
			rmDir = strings.HasPrefix(files[i].Filename, outDir)
		}
	}

	// Process files
	for _, fh := range files {
		file, err := fh.Open()
		defer file.Close()
		if err != nil {
			return "", ign.NewErrorMessageWithBase(ign.ErrorForm, err)
		}
		fn := fh.Filename
		// If file path includes any of the items from the list of invalid names,
		// then error
		if pathIncludesAny(fn, invalidFileNames) {
			return "", ign.NewErrorMessageWithArgs(ign.ErrorFormInvalidValue, err, []string{fn})
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
			return "", ign.NewErrorMessageWithArgs(ign.ErrorFormDuplicateFile, err, []string{fileFullPath})
		}
		if err := os.MkdirAll(filepath.Dir(fileFullPath), 0711); err != nil {
			return "", ign.NewErrorMessageWithBase(ign.ErrorCreatingDir, err)
		}
		dest, err := os.Create(fileFullPath)
		defer dest.Close()
		if err != nil {
			return "", ign.NewErrorMessageWithBase(ign.ErrorForm, err)
		}
		// Now copy contents
		if _, err := io.Copy(dest, file); err != nil {
			return "", ign.NewErrorMessageWithBase(ign.ErrorForm, err)
		}
	}

	return dirpath, nil
}

// internal function that computes and sets the header X-Ign-Resource-Version.
// TODO: this is a strong candidate to move to a models-related middleware.
func writeIgnResourceVersionHeader(versionStr string,
	w http.ResponseWriter, r *http.Request) (version string, err error) {
	version = versionStr
	w.Header().Set("X-Ign-Resource-Version", versionStr)
	return
}
