package reviews

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/comments"
	res "gitlab.com/ignitionrobotics/web/fuelserver/bundles/common_resources"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/models"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	"gitlab.com/ignitionrobotics/web/fuelserver/permissions"
	"gitlab.com/ignitionrobotics/web/ign-go"
)

const noFullTextSearch = ":noft:"

// Service is the main struct exported by this Reviews Service.
// It was meant as a way to structure code and help future extensions.
type Service struct {
	ResourceType reflect.Type
}

// GetResourceInstance returns an instance of the type contained in ResourceType.
func (s *Service) GetResourceInstance() interface{} {
	return reflect.New(s.ResourceType).Elem().Interface()
}

// GetResourceSlice returns a slice of the type contained in ResourceType.
func (s *Service) GetResourceSlicePtr(len int, cap int) interface{} {
	resourceSlice := reflect.MakeSlice(reflect.SliceOf(s.ResourceType), len, cap)
	rs := reflect.New(resourceSlice.Type())
	rs.Elem().Set(resourceSlice)
	return rs.Interface()
}

// ReviewList returns a paginated list of reviews.
// This function returns a list of Reviews that can then be mashalled into json or protobuf.
func (s *Service) ReviewList(p *ign.PaginationRequest, tx *gorm.DB, owner *string,
	order, search string, modelID *uint, user *users.User) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	resourceInstance := s.GetResourceInstance()
	reviewList := s.GetResourceSlicePtr(0, 0)

	// Create query
	q := tx.Model(&resourceInstance)

	// Override default Order BY, unless the user explicitly requested ASC order
	if !(order != "" && strings.ToLower(order) == "asc") {
		// Important: you need to reassign 'q' to keep the updated query
		q = q.Order("created_at desc, id", true)
	}

	// filter resources based on privacy setting
	// We need filter resource based on review privacy setting
	q = res.QueryForResourceVisibility(tx, q, owner, user)

	// filter resources based on modelID, if exist
	if modelID != nil {
		q = res.QueryForModelReview(q, user, *modelID)
	}

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

	reviewListValue := reflect.ValueOf(reviewList)
	reviewListValueLen := reflect.Indirect(reviewListValue).Len()
	reviewsProto := make([]interface{}, reviewListValueLen)
	for i := 0; i < reviewListValueLen; i++ {
		// Get the item from the slice
		review := reflect.Indirect(reviewListValue).Index(i).Addr()
		// Attempt to cast it
		protoReview, ok := review.Interface().(Protobuffer)
		// If the review cannot be cast to the interface, just fail
		if !ok {
			em := ign.NewErrorMessage(ign.ErrorMarshalProto)
			return nil, nil, em
		}
		// Store the element's protobuf representation
		reviewsProto[i] = protoReview.ToProto()
	}
	return reviewsProto, paginationResult, nil
}

// Protobuffer should be implemented by resources that have a protobuf
// representation. It provides methods to convert to a protobuf representation.
type Protobuffer interface {
	// This method returns a protobuf representation of the object
	// Note: consider using proto.Message interface instead of just an empty
	// interface as ToProto return data type.
	// https://godoc.org/github.com/golang/protobuf/proto#Message
	ToProto() interface{}
}

func newModelReviewID(tx *gorm.DB, modelID *uint) (*uint, error) {
	var modelReview ModelReview
	var modelReviewID uint
	if err := tx.Order("model_review_id desc").First(&modelReview, "model_id = ?", *modelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			modelReviewID = 1
			return &modelReviewID, nil
		}
		return nil, err
	}
	modelReviewID = *modelReview.ModelReviewID + 1
	return &modelReviewID, nil
}

// CreateModelReview creates a new model review
func (s *Service) CreateModelReview(cmr CreateModelReview, tx *gorm.DB, creator *users.User) (*ModelReview, *ign.ErrMsg) {
	// set the owner
	owner := cmr.CreateReview.Owner
	if owner == "" {
		owner = *creator.Username
	} else {
		ok, em := users.VerifyOwner(tx, owner, *creator.Username, permissions.Read)
		if !ok {
			return nil, em
		}
	}

	modelReviewID, err := newModelReviewID(tx, cmr.ModelID)
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorCreatingDir, err)
	}

	// create the ModelReview struct
	modelReview, err := NewModelReview(&cmr.CreateReview.Title, &cmr.CreateReview.Description,
		&owner, &cmr.CreateReview.Branch, &cmr.CreateReview.Status,
		cmr.CreateReview.Reviewers, cmr.CreateReview.Approvals, cmr.ModelID, modelReviewID)
	modelReview.Creator = creator.Username
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorCreatingDir, err)
	}

	// create model review in the DB
	if err := tx.Create(&modelReview).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	// read and write permissions
	// convert ID to string
	_, err = globals.Permissions.AddPermission(owner, *modelReview.UUID, permissions.Read)
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}
	_, err = globals.Permissions.AddPermission(owner, *modelReview.UUID, permissions.Write)
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}

	return &modelReview, nil
}

func (s *Service) GetReview(
	tx *gorm.DB,
	user *users.User,
	modelOwner string,
	modelName string,
	modelReviewID uint,
) (*ModelReview, *ign.ErrMsg) {
	ms := models.Service{}
	model, ignErr := ms.GetModel(tx, modelOwner, modelName, user)
	if ignErr != nil {
		return nil, ignErr
	}

	review := ModelReview{ModelID: &model.ID, ModelReviewID: &modelReviewID}
	// var review ModelReview
	result := tx.Model(&review).First(&review)
	if result.Error != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorIDNotFound, result.Error)
	}
	return &review, nil
}

// user: the user making the request
func (s *Service) UpdateReview(
	tx *gorm.DB,
	user *users.User,
	review *ModelReview,
	updateReview *UpdateReview,
) (*ModelReview, *ign.ErrMsg) {
	ok, ignErr := globals.Permissions.IsAuthorized(*user.Username, *review.UUID, permissions.Write)
	if ignErr != nil {
		return nil, ignErr
	}
	if !ok {
		return nil, ign.NewErrorMessage(ign.ErrorUnauthorized)
	}

	if updateReview.Branch != nil {
		review.Branch = updateReview.Branch
	}
	if updateReview.Description != nil {
		review.Description = updateReview.Description
	}
	if updateReview.Private != nil {
		review.Private = updateReview.Private
	}
	if updateReview.Status != nil {
		review.Status = updateReview.Status
	}
	if updateReview.Title != nil {
		review.Title = updateReview.Title
	}

	review.UpdatedAt = time.Now()

	err := tx.Save(review).Error
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	return review, nil
}

func AddComment(tx *gorm.DB, owner *string, pc *comments.PostComment, reviewID uint) (*ModelReviewComment, *ign.ErrMsg) {
	likes := 0
	rc := ModelReviewComment{
		ModelReviewID: reviewID,
		Comment: comments.Comment{
			Body:      &pc.Body,
			UpdatedAt: time.Now(),
			CreatedAt: time.Now(),
			Owner:     owner,
			Likes:     &likes,
		},
	}
	if result := tx.Create(&rc); result.Error != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, result.Error)
	}
	return &rc, nil
}
