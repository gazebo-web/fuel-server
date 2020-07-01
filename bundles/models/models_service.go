package models

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/category"
	res "gitlab.com/ignitionrobotics/web/fuelserver/bundles/common_resources"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/generics"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/license"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	"gitlab.com/ignitionrobotics/web/fuelserver/permissions"
	"gitlab.com/ignitionrobotics/web/fuelserver/proto"
	"gitlab.com/ignitionrobotics/web/fuelserver/vcs"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"net/url"
	"os"
	"strings"
	"time"
)

// Service is the main struct exported by this Models Service.
// It was meant as a way to structure code and help future extensions.
type Service struct{}

// GetModel returns a model by its name and owner's name.
func (ms *Service) GetModel(tx *gorm.DB, owner, name string,
	user *users.User) (*Model, *ign.ErrMsg) {

	// Get the model
	model, err := GetModelByName(tx, name, owner)
	if err != nil {
		em := ign.NewErrorMessageWithArgs(ign.ErrorNameNotFound, err, []string{name})
		return nil, em
	}

	// make sure the user has the correct permissions
	if ok, em := users.CheckPermissions(tx, *model.UUID, user, *model.Private, permissions.Read); !ok {
		return nil, em
	}

	return model, nil
}

// GetModelProto returns a model proto struct, given a model name and owner.
// The user argument is the user requesting the operation.
func (ms *Service) GetModelProto(ctx context.Context, tx *gorm.DB, owner,
	name string, user *users.User) (*fuel.Model, *ign.ErrMsg) {

	model, em := ms.GetModel(tx, owner, name, user)
	if em != nil {
		return nil, em
	}

	// get model latest version number
	latestVersion, err := res.GetLatestVersion(ctx, model)
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}

	// Load the metadata
	tx.Model(&model).Related(&model.Metadata)

	fuelModel := ms.ModelToProto(model)
	fuelModel.Version = proto.Int64(int64(latestVersion))

	if user != nil {
		if ml, _ := ms.getModelLike(tx, model, user); ml != nil {
			fuelModel.IsLiked = proto.Bool(true)
		}
	}

	return fuelModel, nil
}

// ModelList returns a paginated list of models.
// If the likedBy argument is set, it will return the list of models liked by a user.
// This function returns a list of fuel.Model that can then be mashalled into json or protobuf.
// TODO: find a way to MERGE this with the one from Worlds service.
func (ms *Service) ModelList(p *ign.PaginationRequest, tx *gorm.DB, owner *string,
	order, search string, likedBy *users.User, user *users.User, categories *category.Categories) (*fuel.Models, *ign.PaginationResult, *ign.ErrMsg) {

	var modelList Models
	// Create query
	q := QueryForModels(tx)
	var categoryIds []uint
	if categories != nil && len(*categories) > 0 {
		for _, c := range *categories {
			categoryIds = append(categoryIds, c.ID)
		}
		subquery := tx.Table("model_categories").Select("DISTINCT(model_id)").Where("category_id IN (?)", categoryIds).QueryExpr()
		q = q.Where("id IN (?)", subquery)
	}

	var cat category.Category
	if categories != nil {
		for _, cat = range *categories {
			q = q.Joins("JOIN model_categories ON models.id = model_categories.model_id").Where("category_id = ?", &cat.ID)
		}
	}

	// Override default Order BY, unless the user explicitly requested ASC order
	if !(order != "" && strings.ToLower(order) == "asc") {
		// Important: you need to reassign 'q' to keep the updated query
		q = q.Order("created_at desc, id", true)
	}

	// Check if we should return the list of liked models instead.
	if likedBy != nil {
		q = q.Joins("JOIN model_likes ON models.id = model_likes.model_id").Where("user_id = ?", &likedBy.ID)
	} else {

		// filter resources based on privacy setting
		q = res.QueryForResourceVisibility(tx, q, owner, user)

		// If a search criteria was defined, then also apply a fulltext search on "models + tags"
		if search != "" {
			// Trim leading and trailing whitespaces
			searchStr := strings.TrimSpace(search)
			if len(searchStr) > 0 {
				// Note: this is a fulltext search IN NATURAL LANGUAGE MODE.
				// See https://dev.mysql.com/doc/refman/5.7/en/fulltext-search.html for other
				// modes, eg BOOLEAN and WITH QUERY EXPANSION modes.

				// Probably this can be improved a lot. But to avoid fighting against making GORM with complex
				// queries work we are going to first execute a raw query to get the matching model IDs, and
				// then ask GORM to retrieve those models.
				sq := "(SELECT model_id FROM (SELECT * FROM tags WHERE MATCH (name) AGAINST (?)) AS a " +
					"INNER JOIN model_tags ON tag_id = id) UNION " +
					"(SELECT id FROM models WHERE MATCH (name, description) AGAINST (?));"
				var ids []int
				if err := tx.Raw(sq, searchStr, searchStr).Pluck("model_id", &ids).Error; err != nil {
					em := ign.NewErrorMessageWithBase(ign.ErrorNoDatabase, err)
					return nil, nil, em
				}
				// Now that we got the IDs , use them in the main query
				q = q.Where("id IN (?)", ids)
			}
		}
	}

	// Use pagination
	paginationResult, err := ign.PaginateQuery(q, &modelList, *p)
	if err != nil {
		em := ign.NewErrorMessageWithBase(ign.ErrorInvalidPaginationRequest, err)
		return nil, nil, em
	}
	if !paginationResult.PageFound {
		em := ign.NewErrorMessage(ign.ErrorPaginationPageNotFound)
		return nil, nil, em
	}

	var modelsProto fuel.Models
	// Encode models into a protobuf message
	for _, model := range modelList {
		fuelModel := ms.ModelToProto(&model)
		modelsProto.Models = append(modelsProto.Models, fuelModel)
	}

	return &modelsProto, paginationResult, nil
}

// RemoveModel removes a model. The user argument is the requesting user. It
// is used to check if the user can perform the operation.
func (ms *Service) RemoveModel(tx *gorm.DB, owner, modelName string,
	user *users.User) *ign.ErrMsg {

	model, em := ms.GetModel(tx, owner, modelName, user)
	if em != nil {
		return em
	}

	// make sure the user requesting removal has the correct permissions
	ok, err := globals.Permissions.IsAuthorized(*user.Username, *model.UUID, permissions.Write)
	if !ok {
		return err
	}

	// remove resource from permission db
	ok, err = globals.Permissions.RemoveResource(*model.UUID)
	if !ok {
		return err
	}

	return res.Remove(tx, model, *user.Username)
}

// ModelToProto creates a new 'fuel.Model' from the given model.
// NOTE: returned "thumbnail urls" are pointing to the "tip" version.
func (ms *Service) ModelToProto(model *Model) *fuel.Model {
	fuelModel := fuel.Model{
		// Note: time.RFC3339 is the format expected by Go's JSON unmarshal
		CreatedAt:  proto.String(model.CreatedAt.UTC().Format(time.RFC3339)),
		UpdatedAt:  proto.String(model.UpdatedAt.UTC().Format(time.RFC3339)),
		Name:       proto.String(*model.Name),
		Owner:      proto.String(*model.Owner),
		Likes:      proto.Int64(int64(model.Likes)),
		Downloads:  proto.Int64(int64(model.Downloads)),
		Filesize:   proto.Int64(int64(model.Filesize)),
		Permission: proto.Int64(int64(model.Permission)),
		LicenseId:  proto.Uint64(uint64(model.LicenseID)),
	}

	// Optional fields
	if model.UploadDate != nil {
		fuelModel.UploadDate =
			proto.String(model.UploadDate.UTC().Format(time.RFC3339))
	}
	if model.DeletedAt != nil {
		fuelModel.DeletedAt =
			proto.String(model.DeletedAt.UTC().Format(time.RFC3339))
	}
	if model.ModifyDate != nil {
		fuelModel.ModifyDate =
			proto.String(model.ModifyDate.UTC().Format(time.RFC3339))
	}
	if model.Description != nil {
		fuelModel.Description = proto.String(*model.Description)
	}
	if model.URLName != nil {
		fuelModel.UrlName = proto.String(*model.URLName)
	}
	if model.License.Name != nil {
		fuelModel.LicenseName = proto.String(*model.License.Name)
	}
	if model.License.ContentURL != nil {
		fuelModel.LicenseUrl = proto.String(*model.License.ContentURL)
	}
	if model.License.ImageURL != nil {
		fuelModel.LicenseImage = proto.String(*model.License.ImageURL)
	}
	if model.Private != nil {
		fuelModel.Private = proto.Bool(*model.Private)
	}

	if len(model.Tags) > 0 {
		tags := []string{}
		for _, tag := range model.Tags {
			tags = append(tags, *tag.Name)
		}
		fuelModel.Tags = tags
	}

	if model.Categories != nil && len(model.Categories) > 0 {
		categories := []string{}
		for _, category := range model.Categories {
			categories = append(categories, *category.Name)
		}
		fuelModel.Categories = categories
	}

	// Append metadata, if it exists
	if len(model.Metadata) > 0 {
		var metadata []*fuel.ModelMetadatum

		// Convert DB representation to proto
		for _, datum := range model.Metadata {
			fuelDatum := fuel.ModelMetadatum{
				Key:   proto.String(*datum.Key),
				Value: proto.String(*datum.Value),
			}
			metadata = append(metadata, &fuelDatum)
		}
		fuelModel.Metadata = metadata
	}

	// Squash first thumbnail url into model.
	if tbnPaths, err := res.GetThumbnails(model); err == nil {
		url := fmt.Sprintf("/%s/models/%s/tip/files/%s", *model.Owner,
			url.PathEscape(*model.Name), tbnPaths[0])
		fuelModel.ThumbnailUrl = proto.String(url)
	}

	return &fuelModel
}

// ModelFileTree gets the model's FileTree
func (ms *Service) ModelFileTree(ctx context.Context, tx *gorm.DB, owner, modelName,
	version string, user *users.User) (*fuel.FileTree, *ign.ErrMsg) {

	model, em := ms.GetModel(tx, owner, modelName, user)
	if em != nil {
		return nil, em
	}

	return res.FileTree(ctx, model, version)
}

// getModelLike returns a model like.
func (ms *Service) getModelLike(tx *gorm.DB, model *Model, user *users.User) (*ModelLike, *ign.ErrMsg) {
	var modelLike ModelLike
	if err := tx.Where("user_id = ? AND model_id = ?", user.ID, model.ID).First(&modelLike).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorIDNotFound, err)
	}
	return &modelLike, nil
}

// CreateModelLike creates a ModelLike.
// Returns the created modelLike, the current count of likes, or an ign.errMsg.
func (ms *Service) CreateModelLike(tx *gorm.DB, owner, modelName string,
	user *users.User) (*ModelLike, int, *ign.ErrMsg) {

	if user == nil {
		return nil, 0, ign.NewErrorMessage(ign.ErrorAuthNoUser)
	}

	model, em := ms.GetModel(tx, owner, modelName, user)
	if em != nil {
		return nil, 0, em
	}

	// Register the like.
	modelLike := ModelLike{UserID: &user.ID, ModelID: &model.ID}
	if err := tx.Create(&modelLike).Error; err != nil {
		return nil, 0, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}
	// Update the number of likes of the model.
	count, errorMsg := ms.updateLikesCounter(tx, model)
	if errorMsg != nil {
		return nil, 0, errorMsg
	}
	return &modelLike, count, nil
}

// CreateModelReport creates a ModelReport
func (ms *Service) CreateModelReport(tx *gorm.DB, owner, modelName, reason string) (*ModelReport, *ign.ErrMsg) {

	model, err := GetModelByName(tx, modelName, owner)

	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorNameNotFound, err)
	}

	modelReport := ModelReport{
		Report: generics.Report{
			Reason: &reason,
		},
		ModelID: &model.ID,
	}

	if err = tx.Create(&modelReport).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	return &modelReport, nil
}

// RemoveModelLike removes a ModelLike.
// Returns the removed modelLike, the current count of likes, or an ign.errMsg.
func (ms *Service) RemoveModelLike(tx *gorm.DB, owner, modelName string,
	user *users.User) (*ModelLike, int, *ign.ErrMsg) {

	if user == nil {
		return nil, 0, ign.NewErrorMessage(ign.ErrorAuthNoUser)
	}

	model, em := ms.GetModel(tx, owner, modelName, user)
	if em != nil {
		return nil, 0, em
	}

	// Unlike the model.
	var modelLike ModelLike
	if err := tx.Where("user_id = ? AND model_id = ?", &user.ID, &model.ID).Delete(&modelLike).Error; err != nil {
		return nil, 0, ign.NewErrorMessageWithBase(ign.ErrorDbDelete, err)
	}
	// Update the number of likes of the model.
	count, errorMsg := ms.updateLikesCounter(tx, model)
	if errorMsg != nil {
		return nil, 0, errorMsg
	}
	return &modelLike, count, nil
}

// ComputeAllCounters is an initialization function that iterates
// all models and updates their likes and downloads counter, based on the number
// of records in corresponding tables model_likes and model_downloads.
func (ms *Service) ComputeAllCounters(tx *gorm.DB) *ign.ErrMsg {
	var modelList Models
	if err := tx.Model(&Model{}).Unscoped().Find(&modelList).Error; err != nil {
		return ign.NewErrorMessageWithBase(ign.ErrorNoDatabase, err)
	}
	for _, model := range modelList {
		if _, em := ms.updateLikesCounter(tx, &model); em != nil {
			return em
		}
		if _, em := ms.updateDownloadsCounter(tx, &model); em != nil {
			return em
		}
	}
	return nil
}

// updateLikesCounter counts the number of likes and updates the model accordingly.
func (ms *Service) updateLikesCounter(tx *gorm.DB, model *Model) (int, *ign.ErrMsg) {
	var counter int
	// Count the number of likes of the model.
	if err := tx.Model(&ModelLike{}).Where("model_id = ?", model.ID).Count(&counter).Error; err != nil {
		// Note: This is not currently covered by the tests.
		return 0, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}
	// Update the number of likes of the model.
	if err := tx.Model(model).Update("likes", counter).Error; err != nil {
		// Note: This is not currently covered by the tests.
		return 0, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}
	return counter, nil
}

// updateDownloadsCounter counts the number of downloads and updates the
// model accordingly.
func (ms *Service) updateDownloadsCounter(tx *gorm.DB, model *Model) (int, *ign.ErrMsg) {
	// Count the number of downloads of the model.
	var count int
	if err := tx.Model(&ModelDownload{}).Where("model_id = ?", model.ID).Count(&count).Error; err != nil {
		return 0, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}
	if err := tx.Model(model).Update("Downloads", count).Error; err != nil {
		return 0, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}
	return count, nil
}

// GetFile returns the contents (bytes) of a model file. Model version is considered.
// Returns the file's bytes and the resolved version of the model.
func (ms *Service) GetFile(ctx context.Context, tx *gorm.DB, owner, name, path,
	version string, user *users.User) (*[]byte, int, *ign.ErrMsg) {

	model, em := ms.GetModel(tx, owner, name, user)
	if em != nil {
		return nil, -1, em
	}

	return res.GetFile(ctx, model, path, version)
}

// DownloadZip returns the path to a zip file representing a model at the given
// version.
// This method increments the downloads counter.
// Optional argument "user" represents the user (if any) requesting the operation.
// Returns the model, as well as a pointer to the zip's filepath and the
// resolved version.
func (ms *Service) DownloadZip(ctx context.Context, tx *gorm.DB, owner, modelName,
	version string, u *users.User, agent string) (*Model, *string, int, *ign.ErrMsg) {

	model, em := ms.GetModel(tx, owner, modelName, u)
	if em != nil {
		return nil, nil, 0, em
	}
	// increment downloads count
	modelDl := ModelDownload{ModelID: &model.ID, UserAgent: agent}
	if u != nil {
		modelDl.UserID = &u.ID
	}
	if err := tx.Create(&modelDl).Error; err != nil {
		return nil, nil, 0, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}
	// Update the number of downloads of the model.
	_, errorMsg := ms.updateDownloadsCounter(tx, model)
	if errorMsg != nil {
		return nil, nil, 0, errorMsg
	}
	path, resolvedVersion, em := res.GetZip(ctx, model, "models", version)
	return model, path, resolvedVersion, em
}

// UpdateModel updates a model. The user argument is the requesting user. It
// is used to check if the user can perform the operation.
// Fields that can be currently updated: desc, tags, and the model files.
// The filesPath argument points to a tmp folder from which to read the model's files.
// Returns the updated model
func (ms *Service) UpdateModel(ctx context.Context, tx *gorm.DB, owner,
	modelName string, desc, tagstr, filesPath *string, private *bool,
	user *users.User, metadata *ModelMetadata, categories *string) (*Model, *ign.ErrMsg) {

	model, em := ms.GetModel(tx, owner, modelName, user)
	if em != nil {
		return nil, em
	}
	// Check user permissions
	ok, err := globals.Permissions.IsAuthorized(*user.Username, *model.UUID, permissions.Write)
	if !ok {
		return nil, err
	}

	// Edit the model description, if present.
	if desc != nil {
		tx.Model(&model).Update("Description", *desc)
	}
	// Edit the model tags, if present.
	if tagstr != nil {
		tags, err := StrToTags(tx, *tagstr)
		if err != nil {
			return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
		}
		tx.Model(&model).Association("Tags").Replace(*tags)
	}

	if categories != nil {

		sl := ign.StrToSlice(*categories)

		cats, err := category.StrSliceToCategories(tx, sl)
		if err != nil {
			return nil, ign.NewErrorMessageWithBase(ign.ErrorFormInvalidValue, err)
		}

		if cats != nil {
			length := len(*cats)

			if length > globals.MaxCategoriesPerModel {
				return nil, ign.NewErrorMessage(ign.ErrorFormInvalidValue)
			}

			if length == 0 {
				tx.Model(&model).Association("Categories").Clear()
			} else {
				tx.Model(&model).Association("Categories").Replace(cats)
			}
		}
	}

	// Update the metadata, if the data is present.
	if metadata != nil {
		// Handle the special case where the metadata consists of one Metadatum
		// element with empty Key and Value elements. This indicates that
		// the metadata should be cleared.
		if len(*metadata) == 1 && (*metadata)[0].IsEmpty() {
			tx.Model(&model).Association("Metadata").Clear()
		} else {
			tx.Model(&model).Association("Metadata").Replace(*metadata)
		}
	}

	// Update the modification date.
	tx.Model(&model).Update("ModifyDate", time.Now())

	// Update files, if present
	if filesPath != nil {
		// Replace ALL model files with the new ones
		repo := globals.VCSRepoFactory(ctx, *model.Location)
		if err := repo.ReplaceFiles(ctx, *filesPath, *user.Username); err != nil {
			return nil, ign.NewErrorMessageWithBase(ign.ErrorRepo, err)
		}
		// update model's zip and model's filesize
		if em := ms.updateModelZip(ctx, repo, model); em != nil {
			return nil, em
		}
		tx.Model(&model).Update("Filesize", model.Filesize)
	}

	// update model privacy if present
	if private != nil {
		// check if JWT user has permission to update the privacy setting.
		// Only Owners and Admins can do that.
		if ok, em := users.CanPerformWithRole(tx, *model.Owner, *user.Username, permissions.Admin); !ok {
			return nil, em
		}
		tx.Model(&model).Update("Private", *private)
	}

	return model, nil
}

// updateModelZip creates a new zip file for the given model and also
// updates its Filesize field in DB.
func (ms *Service) updateModelZip(ctx context.Context, repo vcs.VCS,
	model *Model) *ign.ErrMsg {
	zSize, em := res.ZipResourceTip(ctx, repo, model, "models")
	if em != nil {
		return em
	}
	model.Filesize = int(zSize)
	return nil
}

// CreateModel creates a new model.
// creator argument is the active user requesting the operation.
func (ms *Service) CreateModel(ctx context.Context, tx *gorm.DB, cm CreateModel,
	uuidStr, filesPath string, creator *users.User) (*Model, *ign.ErrMsg) {

	// Sanity check: Ensure license exists
	license, err := license.ByID(tx, cm.License)
	if err != nil {
		return nil, ign.NewErrorMessageWithArgs(ign.ErrorFormInvalidValue, err,
			[]string{"license"})
	}

	// Set categories
	var categories *category.Categories
	if len(cm.Categories) > 0 {

		sl := ign.StrToSlice(cm.Categories)
		length := len(sl)

		if length > globals.MaxCategoriesPerModel || length == 0 {
			return nil, ign.NewErrorMessage(ign.ErrorFormInvalidValue)
		}

		categories, err = category.StrSliceToCategories(tx, sl)
		if err != nil {
			return nil, ign.NewErrorMessageWithBase(ign.ErrorFormInvalidValue, err)
		}
	}

	// Set the owner
	owner := cm.Owner
	if owner == "" {
		owner = *creator.Username
	} else {
		ok, em := users.VerifyOwner(tx, owner, *creator.Username, permissions.Read)
		if !ok {
			return nil, em
		}
	}

	// Sanity check: model name should be unique for an owner
	if _, err := GetModelByName(tx, cm.Name, owner); err == nil {
		return nil, ign.NewErrorMessageWithArgs(ign.ErrorFormDuplicateModelName, nil, []string{cm.Name})
	}
	// Process the optional tags
	pTags, err := StrToTags(tx, cm.Tags)
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	private := false
	if cm.Private != nil {
		private = *cm.Private
	}

	// Create the Model struct
	model, err := NewModel(&uuidStr, &cm.Name, &cm.URLName, &cm.Description,
		&filesPath, &owner, creator.Username, *license, cm.Permission, *pTags,
		private, categories)

	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorCreatingDir, err)
	}

	repo, em := res.CreateResourceRepo(ctx, &model, filesPath)
	if em != nil {
		return nil, em
	}

	// Zip the model and compute its size.
	if em := ms.updateModelZip(ctx, repo, &model); em != nil {
		return nil, em
	}

	// If everything went OK then create the model in DB.
	if err := tx.Create(&model).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	// add read and write permissions
	ok, em := globals.Permissions.AddPermission(owner, *model.UUID, permissions.Read)
	if !ok {
		return nil, em
	}
	ok, em = globals.Permissions.AddPermission(owner, *model.UUID, permissions.Write)
	if !ok {
		return nil, em
	}

	return &model, nil
}

// CloneModel clones a model.
// creator argument is the active user requesting the operation.
func (ms *Service) CloneModel(ctx context.Context, tx *gorm.DB, smOwner,
	smName string, cm CloneModel, creator *users.User) (*Model, *ign.ErrMsg) {

	// Get source model (sm)
	model, em := ms.GetModel(tx, smOwner, smName, creator)
	if em != nil {
		return nil, em
	}

	// Set the owner
	owner := cm.Owner
	if owner == "" {
		owner = *creator.Username
	} else {
		ok, em := users.VerifyOwner(tx, owner, *creator.Username, permissions.Read)
		if !ok {
			return nil, em
		}
	}

	private := false
	if model.Private != nil {
		private = *model.Private
	}

	if private {
		authorized, _ := globals.Permissions.IsAuthorized(
			*creator.Username, *model.UUID, permissions.Read)
		if !authorized {
			return nil, ign.NewErrorMessage(ign.ErrorUnauthorized)
		}
	}

	// Try to use the given model name. Or find a new one
	var mName string
	if cm.Name != "" {
		mName = cm.Name
	} else {
		mName = *model.Name
	}
	modelName, err := ms.createUniqueModelName(tx, mName, owner)
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorCreatingDir, err)
	}

	clonePrivate := false
	if cm.Private != nil {
		clonePrivate = *cm.Private
	}

	// Create the new Model (the clone) struct and folder
	clone, err := NewModelAndUUID(&modelName, model.URLName, model.Description,
		nil, &owner, creator.Username, model.License, model.Permission, model.Tags,
		clonePrivate, &model.Categories)
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorCreatingDir, err)
	}

	repo, em := res.CloneResourceRepo(ctx, model, &clone)
	if em != nil {
		return nil, em
	}

	// Zip the model and compute its size.
	if em := ms.updateModelZip(ctx, repo, &clone); em != nil {
		os.Remove(*clone.Location)
		return nil, em
	}

	// If everything went OK then create the  new model in DB.
	if err := tx.Create(&clone).Error; err != nil {
		os.Remove(*clone.Location)
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	// add read and write permissions
	ok, em := globals.Permissions.AddPermission(owner, *clone.UUID, permissions.Read)
	if !ok {
		return nil, em
	}
	ok, em = globals.Permissions.AddPermission(owner, *clone.UUID, permissions.Write)
	if !ok {
		return nil, em
	}

	return &clone, nil
}

// createUniqueModelName is an internal helper to disambiguate among model names
func (ms *Service) createUniqueModelName(tx *gorm.DB, name, owner string) (string, error) {
	// Find an unused name variation
	nameModifier := 1
	newName := name
	for {
		if _, err := GetModelByName(tx, newName, owner); err == nil {
			newName = fmt.Sprintf("%s %d", newName, nameModifier)
			nameModifier++
		} else {
			// got the right new name. Exit loop
			break
		}
	}
	return newName, nil
}
