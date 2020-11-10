package main

import (
//	"fmt"
//	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
//	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/category"
//	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/collections"
//	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/generics"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/reviews"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"net/http"
//	"strconv"
)

// ReviewList returns the list of reviews from a team/user. The returned value
// will be of type "fuel.Reviews"
// It follows the func signature defined by type "searchHandler".
// You can request this method with the following curl request:
//     curl -k -X GET --url https://localhost:4430/1.0/reviews
// or  curl -k -X GET --url https://localhost:4430/1.0/reviews.proto
// or  curl -k -X GET --url https://localhost:4430/1.0/reviews.json
// or  curl -k -X GET --url https://localhost:4430/1.0/{username}/reviews with all the
// above format variants.
func ReviewList(p *ign.PaginationRequest, owner *string, order, search string,
	user *users.User, tx *gorm.DB, w http.ResponseWriter,
	r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	ms := &reviews.Service{}

	return ms.ReviewList(p, tx, owner, order, search, user)
}
