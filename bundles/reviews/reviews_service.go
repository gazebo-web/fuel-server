package reviews

import (
//	"context"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/jinzhu/gorm"
//	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/category"
//	res "gitlab.com/ignitionrobotics/web/fuelserver/bundles/common_resources"
//	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/generics"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
//	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
//	"gitlab.com/ignitionrobotics/web/fuelserver/permissions"
	"gitlab.com/ignitionrobotics/web/fuelserver/proto"
//	"gitlab.com/ignitionrobotics/web/fuelserver/vcs"
	"gitlab.com/ignitionrobotics/web/ign-go"
//	"net/url"
//	"os"
	"strings"
	"time"
)

const noFullTimeSearch = ":noft:"

// Service is the main struct exported by this Reviews Service.
// It was meant as a way to structure code and help future extensions.
type Service struct{}

// ReviewList returns a paginated list of reviews.
// This function returns a list of Reviews that can then be mashalled into json or protobuf.
func (ms *Service) ReviewList(p *ign.PaginationRequest, tx *gorm.DB, owner *string,
	order, search string, user *users.User) (*fuel.Reviews, *ign.PaginationResult, *ign.ErrMsg) {

	var reviewList Reviews
	// Create query
	q := QueryForReviews(tx)

	// Override default Order BY, unless the user explicitly requested ASC order
	if !(order != "" && strings.ToLower(order) == "asc") {
		// Important: you need to reassign 'q' to keep the updated query
		q = q.Order("created_at desc, id", true)
	}

  // filter resources based on privacy setting
  // todo(anyone) reviews do not have a "private" field so this does not work
  // We need filter resource based on model privacy setting
  // q = res.QueryForResourceVisibility(tx, q, owner, user)

	// If a search criteria was defined, then also apply a fulltext search on "review's description"
	if search != "" {
		// Trim leading and trailing whitespaces
		searchStr := strings.TrimSpace(search)
		if len(searchStr) > 0 {
			// Check if the user wants a full-text search or a simple one. The simple
			// search allows searching for "partial words" (eg. UI filtering while the
			// user types in).
			if strings.HasPrefix(searchStr, noFullTimeSearch) {
				searchStr = strings.TrimPrefix(searchStr, noFullTimeSearch)
				expanded := fmt.Sprintf("%%%s%%", searchStr)
				q = q.Where("title LIKE ?", expanded)
			} else {
				// Note: this is a fulltext search IN NATURAL LANGUAGE MODE.
				// See https://dev.mysql.com/doc/refman/5.7/en/fulltext-search.html for other
				// modes, eg BOOLEAN and WITH QUERY EXPANSION modes.
				q = q.Where("MATCH (title, description) AGAINST (?)", searchStr)
			}
		}
	}

	// Use pagination
	paginationResult, err := ign.PaginateQuery(q, &reviewList, *p)
	if err != nil {
		em := ign.NewErrorMessageWithBase(ign.ErrorInvalidPaginationRequest, err)
		return nil, nil, em
	}
	if !paginationResult.PageFound {
		em := ign.NewErrorMessage(ign.ErrorPaginationPageNotFound)
		return nil, nil, em
	}

//	return &reviewList, paginationResult, nil

	var reviewsProto fuel.Reviews
	// Encode reviews into a protobuf message
	for _, review := range reviewList {
		fuelReview := ms.ReviewToProto(&review)
		reviewsProto.Reviews = append(reviewsProto.Reviews, fuelReview)
	}
	return &reviewsProto, paginationResult, nil
}

// ReviewToProto creates a new 'fuel.Review' from the given review.
func (ms *Service) ReviewToProto(review *Review) *fuel.Review {
	fuelReview := fuel.Review{
		// Note: time.RFC3339 is the format expected by Go's JSON unmarshal
		CreatedAt:    proto.String(review.CreatedAt.UTC().Format(time.RFC3339)),
		UpdatedAt:    proto.String(review.UpdatedAt.UTC().Format(time.RFC3339)),
		Creator:      proto.String(*review.Creator),
		Owner:        proto.String(*review.Owner),
		Title:        proto.String(*review.Title),
		Description:  proto.String(*review.Description),
		Branch:       proto.String(*review.Branch),
		Status:       proto.String(*review.Status),
    Reviewers:    review.Reviewers,
    Approvals:    review.Approvals,
	}

	return &fuelReview
}

// CreateReview creates a model review for a new model.
func (ms *Service) CreateReview() (*Review, *ign.ErrMsg) {



}
