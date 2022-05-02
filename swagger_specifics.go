package main

import (
	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/proto"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"os"
)

// This module contains swagger specifics related to doc generation.
// The are defined as private to avoid issues with linter and swagger
// requesting conflicting comments on types.

/////////////////////////////////////////////////
///////  swagger responses
/////////////////////////////////////////////////

// File response
// swagger:response fileResponse
type fileResponse struct {
	// In: body
	File os.File
}

// FileResponse is used to represent a File response (any file) type
// in swagger documentation.
// See: https://goswagger.io/faq/faq_spec.html#how-to-define-a-swagger-response-that-produces-a-binary-file

// Array of Models
// swagger:response jsonModels
type jsonModels struct {
	// In: body
	Models []*fuel.Model
}

/////////////////////////////////////////////////
///////  swagger Parameters
/////////////////////////////////////////////////

// swagger:parameters listOwnerModels singleOwnerModel singleUser deleteUser
type userInPath struct {
	// in: path
	Username string `json:"username"`
}

// swagger:parameters singleOwnerModel
type modelInPath struct {
	// Model name
	// in: path
	Model string `json:"model"`
}

// swagger:parameters downloadModelFile
type fileInPath struct {
	// File path within model
	// in: path
	Path string `json:"path"`
}

// swagger:parameters downloadModelFile singleModel deleteModel modelFileTree
type uuidInPath struct {
	// in: path
	UUID string `json:"uuid"`
}

// swagger:parameters listModels listOwnerModels
type listModelsParams struct {
	// Search query
	// in: query
	SearchQuery string `json:"q"`

	// in: query
	// enum: asc, desc
	// default: desc
	Order string `json:"order"`
}

// swagger:parameters listUsers listModels listOwnerModels listLicenses
type paginationParams struct {
	// The page to return
	// Minimum: 1
	// default: 1
	// in: query
	Page int `json:"page"`

	// Size of the pages
	// Minimum: 1
	// Maximum: 100
	// default: 20
	// in: query
	PageSize int `json:"per_page"`
}

// CreateUser is used to represent user input in swagger documentation.
// TODO: use this struct to parse and validate input parameters hadler_users.go
type createUserPayload struct {
	// Username
	//
	// Required: true
	Username *string `json:"username,omitempty"`

	// email
	// Required: true
	Email *string `json:"email,omitempty"`

	// Name
	Name *string `json:"name,omitempty"`

	// Organization
	Organization *string `json:"org,omitempty"`
}

// swagger:parameters createUser
// See: https://goswagger.io/generate/spec/params.html
type createUserParam struct {
	// The user data
	//
	// required: true
	// in:body
	User createUserPayload `json:"user"`
}

// swagger:parameters createModel
type createModelParam struct {
	// Model data
	//
	// required: true
	// in:body
	Model models.CreateModel `json:"model"`
}

/////////////////////////////////////////////////
///////  swagger Errors
/////////////////////////////////////////////////

// Ign Fuel error serialized as JSON
// swagger:response fuelError
type fuelError struct {
	// In: body
	ErrMsg ign.ErrMsg
}
