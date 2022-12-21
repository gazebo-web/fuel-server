package main

import (
	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/bundles/reviews"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"net/http"
	"reflect"
)

// ModelReviewList returns the list of reviews for models from a team/user
// The returned value will be of type "fuel.ModelReviews"
// It follows the func signature defined by type "searchHandler".
// You can request this method with the following curl request:
//
//	curl -k -X GET --url https://localhost:4430/1.0/models/reviews
//
// or  curl -k -X GET --url https://localhost:4430/1.0/models/reviews.proto
// or  curl -k -X GET --url https://localhost:4430/1.0/models/reviews.json
// or  curl -k -X GET --url https://localhost:4430/1.0/{username}/reviews with all the
// above format variants.
func ModelReviewList(p *gz.PaginationRequest, owner *string, order, search string,
	user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *gz.PaginationResult, *gz.ErrMsg) {

	// Note that the `Service`'s `ResourceType` field is being configured with a specific review type.
	// The `review.Service` methods will have to make use of the `ResourceType` field to generically create return values.
	ms := &reviews.Service{ResourceType: reflect.TypeOf(reviews.ModelReview{})}

	// assume no negative ID value
	return ms.ReviewList(p, tx, owner, order, search, nil, user)
}

// UserModelReview returns a list of reviews for a selected model from a team/user.
// The returned value will be of type "fuel.ModelReviews"
// It follows the func signature defined by type "searchHandler".
// You can request this method with the following curl request:
//
//	curl -k -X GET --url https://localhost:4430/1.0/{username}/models/{model}
func UserModelReview(p *gz.PaginationRequest, owner *string, order, search string,
	user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *gz.PaginationResult, *gz.ErrMsg) {

	// Note that the `Service`'s `ResourceType` field is being configured with a specific review type.
	// The `review.Service` methods will have to make use of the `ResourceType` field to generically create return values.
	ms := &reviews.Service{ResourceType: reflect.TypeOf(reviews.ModelReview{})}

	// parse request for modelID
	query := mux.Vars(r)

	// Get the model name from the path
	modelName := query["model"]

	// Get the model record
	model, err := models.GetModelByName(tx, modelName, *owner)
	if err != nil {
		em := gz.NewErrorMessage(gz.ErrorNameNotFound)
		return nil, nil, em
	}

	return ms.ReviewList(p, tx, owner, order, search, &model.ID, user)
}
