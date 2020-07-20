package main

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/subt"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
)

// Leaderboard returns a paginated list of subt participant names
// sorted by their score.
// You can request this method with the following cURL request:
//   curl -k -X GET --url https://localhost:4430/1.0/subt/leaderboard
//     --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func Leaderboard(p *ign.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {
	// Parse parameters
	var competition string
	var circuit, owner *string
	params := r.URL.Query()

	// Competition
	param, ok := params["competition"]
	if ok {
		competition = param[0]
	} else {
		competition = subt.SubTPortalName
	}

	// Circuit
	param, ok = params["circuit"]
	if ok {
		circuit = &param[0]
	}

	// Owner
	param, ok = params["owner"]
	if ok {
		owner = &param[0]
	}

	return (&subt.Service{}).Leaderboard(p, tx, competition, circuit, owner)
}

// RegistrationsList returns a paginated list of subt registrations.
// You can request this method with the following cURL request:
//   curl -k -X GET --url https://localhost:4430/1.0/subt/registrations
//     --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func RegistrationsList(p *ign.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {
	// Get the parameters
	params := r.URL.Query()
	var status subt.RegStatus
	s, ok := params["status"]
	if !ok {
		status = subt.RegOpPending
	} else if s[0] == "pending" {
		status = subt.RegOpPending
	} else if s[0] == "done" {
		status = subt.RegOpDone
	} else if s[0] == "rejected" {
		status = subt.RegOpRejected
	} else {
		return nil, nil, ign.NewErrorMessage(ign.ErrorMissingField)
	}

	return (&subt.Service{}).RegistrationList(p, tx, subt.SubTPortalName, status, user)
}

// SubTRegistrationCreate creates a new pending registration.
// You can request this method with the following cURL request:
//    curl -k -X POST -d '{"partipant":<an organization>}' -H "Content-Type: application/json"
//			https://localhost:4430/1.0/subt/registrations
//      --header 'authorization: Bearer <your-jwt-token-here>'
func SubTRegistrationCreate(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	// Sanity check: Find the user associated to the given JWT. Fail if no user.
	user, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	var rc subt.RegistrationCreate
	if em := ParseStruct(&rc, r, false); em != nil {
		return nil, em
	}

	re, em := (&subt.Service{}).ApplyToSubT(r.Context(), tx, rc.Participant, user)
	return re, em
}

// SubTRegistrationUpdate accepts or rejects an existing pending registration.
// You can request this method with the following cURL request:
//    curl -k -X PATCH -d '{"resolution":<0,1, or 2>}' -H "Content-Type: application/json"
//			https://localhost:4430/1.0/subt/registrations/{competition}/{name}
//      --header 'authorization: Bearer <your-jwt-token-here>'
// It returns the updated registration.
func SubTRegistrationUpdate(orgName string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	// Sanity check: Find the user associated to the given JWT. Fail if no user.
	user, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	var ru subt.RegistrationUpdate
	if em := ParseStruct(&ru, r, false); em != nil {
		return nil, em
	}

	ru.Competition = subt.SubTPortalName
	ru.Participant = orgName
	return (&subt.Service{}).ResolveRegistration(r.Context(), tx, &ru, user)
}

// SubTRegistrationDelete deletes an existing pending registration. This can be
// invoked by the user that applied the original registration, or by admins of
// the competition (SubT).
// You can request this method with the following cURL request:
//    curl -k -X DELETE https://localhost:4430/1.0/subt/registrations/{competition}/{name}
//      --header 'authorization: Bearer <your-jwt-token-here>'
func SubTRegistrationDelete(orgName string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	// Sanity check: Find the user associated to the given JWT. Fail if no user.
	user, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	return (&subt.Service{}).DeleteRegistration(r.Context(), tx, subt.SubTPortalName,
		orgName, user)
}

// SubTParticipantDelete deletes an existing participant. This can be
// invoked by the user that applied the original registration, or by admins of
// the competition (SubT).
// You can request this method with the following cURL request:
//    curl -k -X DELETE https://localhost:4430/1.0/subt/participants/{competition}/{name}
//      --header 'authorization: Bearer <your-jwt-token-here>'
func SubTParticipantDelete(orgName string, jwtUser *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	// Sanity check: Find the user associated to the given JWT. Fail if no user.
	user, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	return (&subt.Service{}).DeleteParticipant(r.Context(), tx, subt.SubTPortalName,
		orgName, user)
}

// SubTParticipantsList returns a paginated list of subt participants (organizations for now).
// You can request this method with the following cURL request:
//   curl -k -X GET --url https://localhost:4430/1.0/subt/participants
//     --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func SubTParticipantsList(p *ign.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	return (&subt.Service{}).ParticipantsList(p, tx, subt.SubTPortalName, user)
}

// SubTSubmitLogFile submits a new log file for evaluation.
// You can request this method with the following cURL request:
//    curl -k -X POST -F owner=ownerName -F private=true
//      -F 'file=@<full-path-to-file;filename=theFileName>'
//      https://localhost:4430/1.0/subt/logfiles
//			--header 'authorization: Bearer <your-jwt-token-here>'
func SubTSubmitLogFile(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	// Parse form's values and files.
	if err := r.ParseMultipartForm(0); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorForm, err)
	}
	// Delete temporary files from r.ParseMultipartForm(0)
	defer r.MultipartForm.RemoveAll()

	var ls subt.LogSubmission
	if em := ParseStruct(&ls, r, true); em != nil {
		return nil, em
	}

	// Sanity check: Find the user associated to the given JWT. Fail if no user.
	user, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	f, fh, err := r.FormFile("file")
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorForm, err)
	}
	fName := fh.Filename
	log, em := (&subt.LogService{}).CreateLog(r.Context(), tx, f, fName,
		subt.SubTPortalName, &ls, user)
	return log, em
}

// SubTUpdateLogFile updates a log file.
//    curl -k -X PATCH -d '{"status":<0,1, or 2>, "score":<float32>}' -H "Content-Type: application/json"
//			https://localhost:4430/1.0/subt/logfiles/{id}
//      --header 'authorization: Bearer <your-jwt-token-here>'
// Returns the updated log file
func SubTUpdateLogFile(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	// Get the user and ID from request. Fail if any of those are missing
	user, id, em := getUserAndID(tx, r)
	if em != nil {
		return nil, em
	}

	var su subt.SubmissionUpdate
	if em := ParseStruct(&su, r, false); em != nil {
		return nil, em
	}

	s := &subt.LogService{}
	return s.UpdateLogFile(r.Context(), tx, subt.SubTPortalName, id, &su, user)
}

// SubTDeleteLogFile deletes a log file.
// You can request this method with the following curl request:
//   curl -k -X DELETE --url https://localhost:4430/1.0/subt/logfiles/{id}
func SubTDeleteLogFile(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	// Get the user and ID from request. Fail if any of those are missing
	user, id, em := getUserAndID(tx, r)
	if em != nil {
		return nil, em
	}

	s := &subt.LogService{}
	return s.RemoveLogFile(r.Context(), tx, subt.SubTPortalName, id, user)
}

// SubTLogFileDownload downloads an individual log file.
// If the url query includes link=true as parameter then this handler will
// return the download URL as a string result instead of doing a http redirect.
// You can request this method with the following curl request:
//   curl -k -X GET --url https://localhost:4430/1.0/subt/logfiles/{id}/file?link=true
func SubTLogFileDownload(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	// Sanity check: Find the user associated to the given JWT. Fail if no user.
	user, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, &errMsg
	}

	id, em := readID(r)
	if em != nil {
		return nil, em
	}

	// Get the parameters
	params := r.URL.Query()
	val, ok := params["link"]
	linkOnly := ok && val[0] == "true"

	s := &subt.LogService{}
	url, em := s.GetLogFileForDownload(r.Context(), tx, subt.SubTPortalName, id, user)
	if em != nil {
		return nil, em
	}

	if linkOnly {
		return *url, nil
	}
	http.Redirect(w, r, *url, http.StatusTemporaryRedirect)
	return nil, nil
}

// SubTGetLogFile returns info about a single log file.
// You can request this method with the following curl request:
//   curl -k -X GET --url https://localhost:4430/1.0/subt/logfiles/{id}
func SubTGetLogFile(tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.ErrMsg) {

	// Get the user and ID from request. Fail if any of those are missing
	user, id, em := getUserAndID(tx, r)
	if em != nil {
		return nil, em
	}

	s := &subt.LogService{}
	return s.GetLogFile(r.Context(), tx, subt.SubTPortalName, id, user)
}

// SubTLogFileList returns a paginated list of subt log files.
// You can request this method with the following cURL request:
//   curl -k -X GET --url https://localhost:4430/1.0/subt/logfiles
//     --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
// OR
//   curl -k -X GET --url https://localhost:4430/1.0/subt/participants/{name}/logfiles
//     --header 'authorization: Bearer <A_VALID_AUTH0_JWT_TOKEN>'
func SubTLogFileList(p *ign.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	// Get the parameters
	params := r.URL.Query()
	var status subt.SubmissionStatus
	s, ok := params["status"]
	if !ok {
		status = subt.StForReview
	} else if s[0] == "pending" {
		status = subt.StForReview
	} else if s[0] == "done" {
		status = subt.StDone
	} else if s[0] == "rejected" {
		status = subt.StRejected
	} else {
		return nil, nil, ign.NewErrorMessage(ign.ErrorMissingField)
	}

	owner, ok, em := readOwner(tx, r, "name", true)
	// If the owner does not exist
	if !ok && em.ErrCode != ign.ErrorUserNotInRequest {
		return nil, nil, em
	}

	svc := &subt.LogService{}
	return svc.LogFileList(p, tx, subt.SubTPortalName, owner, status, user)
}

// getUserAndID reads the user and ID from the http request.
// It fails if any of those cannot be found.
func getUserAndID(tx *gorm.DB, r *http.Request) (*users.User, uint, *ign.ErrMsg) {
	user, ok, errMsg := getUserFromJWT(tx, r)
	if !ok {
		return nil, 0, &errMsg
	}

	id, em := readID(r)
	if em != nil {
		return nil, 0, em
	}
	return user, id, nil
}

// readID extracts the id from the request.
func readID(r *http.Request) (uint, *ign.ErrMsg) {

	params := mux.Vars(r)
	idStr, ok := params["id"]
	if !ok {
		return 0, ign.NewErrorMessage(ign.ErrorIDNotInRequest)
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, ign.NewErrorMessage(ign.ErrorIDNotInRequest)
	}
	return uint(id), nil
}
