package reviews

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/fuelserver/proto"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"reflect"
	"strings"
	"time"
)

const noFullTextSearch = ":noft:"

// Service is the main struct exported by this Reviews Service.
// It was meant as a way to structure code and help future extensions.
type Service struct{
  ResourceType reflect.Type
}

// GetResourceInstance returns an instance of the type contained in ResourceType.
func (s *Service) GetResourceInstance() interface{} {
  return reflect.New(s.ResourceType).Elem().Interface()
}

// GetResourceSlice returns a slice of the type contained in ResourceType.
func (ms *Service) GetResourceSlice(len int, cap int) interface{} {
	resourceSlice := reflect.MakeSlice(reflect.SliceOf(ms.ResourceType), 0, 0)
	rs := reflect.New(resourceSlice.Type())
	rs.Elem().Set(resourceSlice)
	return rs.Interface()
}

// ReviewList returns a paginated list of reviews.
// This function returns a list of Reviews that can then be mashalled into json or protobuf.
func (ms *Service) ReviewList(p *ign.PaginationRequest, tx *gorm.DB, owner *string,
	order, search string, user *users.User) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	resourceInstance := ms.GetResourceInstance()
	reviewList := ms.GetResourceSlice(0, 0)

	// Create query
	q := tx.Model(&resourceInstance)

	q.Preload("Review")

	// Override default Order BY, unless the user explicitly requested ASC order
	if !(order != "" && strings.ToLower(order) == "asc") {
		// Important: you need to reassign 'q' to keep the updated query
		q = q.Order("created_at desc, id", true)
	}

	// filter resources based on privacy setting
	// todo(anyone) reviews do not have a "private" field so this does not work
	// We need filter resource based on model privacy setting
	// q = res.QueryForResourceVisibility(tx, q, owner, user)

	// todo(anyone) check if search works
	// If a search criteria was defined, then also apply a fulltext search on "review's description"
	if search != "" {
		// Trim leading and trailing whitespaces
		searchStr := strings.TrimSpace(search)
		if len(searchStr) > 0 {
			// Check if the user wants a full-text search or a simple one. The simple
			// search allows searching for "partial words" (eg. UI filtering while the
			// user types in).
			if strings.HasPrefix(searchStr, noFullTextSearch) {
				searchStr = strings.TrimPrefix(searchStr, noFullTextSearch)
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
	paginationResult, err := ign.PaginateQuery(q, reviewList, *p)
	if err != nil {
		em := ign.NewErrorMessageWithBase(ign.ErrorInvalidPaginationRequest, err)
		return nil, nil, em
	}
	if !paginationResult.PageFound {
		em := ign.NewErrorMessage(ign.ErrorPaginationPageNotFound)
		return nil, nil, em
	}

	return reviewList, paginationResult, nil

	// todo(anyone) convert and return ResourceReview to proto
	// var reviewsProto fuel.Reviews
	// switch t := reviewList.(type) {
	//   case []ModelReview:
	//     for _, mr := range t {
	//       fuelReview := ms.ReviewToProto(mr.Review)
	//       reviewsProto.Reviews = append(reviewsProto.Reviews, fuelReview)
	//     }
	// }
	// return &reviewsProto, paginationResult, nil
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
