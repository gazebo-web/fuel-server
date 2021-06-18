package main

import (
	"strconv"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/reviews"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"reflect"
	"net/http"
)

// ModelReviewList returns the list of reviews for models from a team/user
// The returned value will be of type "fuel.ModelReviews"
// It follows the func signature defined by type "searchHandler".
// You can request this method with the following curl request:
//     curl -k -X GET --url https://localhost:4430/1.0/reviews
// or  curl -k -X GET --url https://localhost:4430/1.0/reviews.proto
// or  curl -k -X GET --url https://localhost:4430/1.0/reviews.json
// or  curl -k -X GET --url https://localhost:4430/1.0/{username}/reviews with all the
// above format variants.
func ModelReviewList(p *ign.PaginationRequest, owner *string, order, search string,
	user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

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
//     curl -k -X GET --url https://localhost:4430/1.0/{model}/reviews
// or  curl -k -X GET --url https://localhost:4430/1.0/{model}/reviews.proto
// or  curl -k -X GET --url https://localhost:4430/1.0/{model}/reviews.json
// or  curl -k -X GET --url https://localhost:4430/1.0/{username}/models/{model}/reviews with all the
// above format variants.
func UserModelReview(p *ign.PaginationRequest, owner *string, order, search string,
	user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	// Note that the `Service`'s `ResourceType` field is being configured with a specific review type.
	// The `review.Service` methods will have to make use of the `ResourceType` field to generically create return values.
	ms := &reviews.Service{ResourceType: reflect.TypeOf(reviews.ModelReview{})}

	// parse request for modelID
	query := mux.Vars(r)

	// assuming one id value
	idStr := query["model"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		em := ign.NewErrorMessage(ign.ErrorPaginationPageNotFound)
		return nil, nil, em
	}
	// convert from uint64 to uint
	modelID := uint(id)
	return ms.ReviewList(p, tx, owner, order, search, &modelID, user)
}
