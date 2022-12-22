package collections

import (
	"context"
	"fmt"
	res "github.com/gazebo-web/fuel-server/bundles/common_resources"
	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/bundles/worlds"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/permissions"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
	"net/url"
	"os"
	"strings"
)

const noFullTimeSearch = ":noft:"

// Service is the main struct exported by this collections Service.
type Service struct{}

// ResourceWithID is used for resources that have a DB ID (eg. Model or World)
type ResourceWithID interface {
	GetID() uint
}

// GetCollection returns a single Collection by its name and owner's name.
// Optional: The user argument is the requesting user. It is used to check if
// the user can perform the operation.
func (s *Service) GetCollection(tx *gorm.DB, owner, name string,
	user *users.User) (*Collection, *gz.ErrMsg) {

	c, em := s.internalGetCollection(tx, owner, name, user)
	if em != nil {
		return nil, em
	}

	// Get the thumbnails
	// first , reset the query clauses
	blankQuery := tx.New()
	if em := populateCollectionThumbnails(blankQuery, c, user); em != nil {
		return nil, em
	}

	return c, nil
}

// internalGetCollection returns a single Collection by its name and owner's name.
// NOTE: This internal func does not populate thumbnails. This is done to avoid
// extra rounds to DB when not needed.
// Optional: The user argument is the requesting user. It is used to check if
// the user can perform the operation.
func (s *Service) internalGetCollection(tx *gorm.DB, owner, name string,
	user *users.User) (*Collection, *gz.ErrMsg) {

	// Create query
	q := QueryForCollections(tx)
	// filter resources based on privacy setting
	q = res.QueryForResourceVisibility(tx, q, &owner, user)
	// Find the collection
	c, err := ByName(q, name, owner)
	if err != nil {
		em := gz.NewErrorMessageWithArgs(gz.ErrorNameNotFound, err, []string{owner, name})
		return nil, em
	}
	return c, nil
}

// CollectionList returns a paginated list of Collections.
// Note: 'extend' argument is to only return collections that the user can
// add/remove assets (which is not the same as 'updating the collection details').
func (s *Service) CollectionList(p *gz.PaginationRequest, tx *gorm.DB,
	owner *string, order, search string, extend bool, user *users.User) (*Collections,
	*gz.PaginationResult, *gz.ErrMsg) {

	var list Collections
	// Create query
	q := QueryForCollections(tx)

	// Override default Order BY, unless the user explicitly requested ASC order
	if !(order != "" && strings.ToLower(order) == "asc") {
		q = q.Order("created_at desc, id", true)
	}

	if extend && user != nil {
		// only return collections that the user can extend (ie. associate assets)
		userGroups := globals.Permissions.GetGroupsForUser(*user.Username)
		userGroups = append(userGroups, *user.Username)
		q = q.Where("owner IN (?)", userGroups)
	} else {
		// filter resources based on privacy setting
		q = res.QueryForResourceVisibility(tx, q, owner, user)
	}

	// If a search criteria was defined, then also apply a fulltext search on "world's name + description"
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
				q = q.Where("name LIKE ?", expanded)
			} else {
				// Note: this is a fulltext search IN NATURAL LANGUAGE MODE.
				// See https://dev.mysql.com/doc/refman/5.7/en/fulltext-search.html for other
				// modes, eg BOOLEAN and WITH QUERY EXPANSION modes.
				q = q.Where("MATCH (name, description) AGAINST (?)", searchStr)
			}
		}
	}

	// Use pagination
	paginationResult, err := gz.PaginateQuery(q, &list, *p)
	if err != nil {
		em := gz.NewErrorMessageWithBase(gz.ErrorInvalidPaginationRequest, err)
		return nil, nil, em
	}
	if !paginationResult.PageFound {
		em := gz.NewErrorMessage(gz.ErrorPaginationPageNotFound)
		return nil, nil, em
	}

	// Get the thumbmails
	// first , reset the query clauses
	blankQuery := tx.New()
	result := Collections{}
	for _, col := range list {
		if em := populateCollectionThumbnails(blankQuery, &col, user); em != nil {
			return nil, nil, em
		}
		result = append(result, col)
	}
	return &result, paginationResult, nil
}

func populateCollectionThumbnails(tx *gorm.DB,
	col *Collection, user *users.User) *gz.ErrMsg {
	// first check if the collection has a Logo as thumbnail
	if tbnPaths, err := res.GetThumbnails(col); err == nil {
		url := fmt.Sprintf("/%s/%ss/%s/tip/files/%s", *col.GetOwner(), "collection",
			url.PathEscape(*col.GetName()), tbnPaths[0])
		col.ThumbnailUrls = []string{url}
		return nil
	}
	// otherwise return the asset thumbnails
	assocs, err := FindAssociations(tx, col, 4)
	if err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	for _, a := range *assocs {
		var r res.Resource
		var em *gz.ErrMsg
		if a.Type == TModel {
			s := &models.Service{}
			r, em = s.GetModel(tx, a.AssetOwner, a.AssetName, user)
		} else if a.Type == TWorld {
			s := &worlds.Service{}
			r, em = s.GetWorld(tx, a.AssetOwner, a.AssetName, user)
		}

		if em == nil {
			if tbnPaths, err := res.GetThumbnails(r); err == nil {
				url := fmt.Sprintf("/%s/%ss/%s/tip/files/%s", *r.GetOwner(), a.Type,
					url.PathEscape(*r.GetName()), tbnPaths[0])
				if col.ThumbnailUrls == nil {
					col.ThumbnailUrls = []string{url}
				} else {
					col.ThumbnailUrls = append(col.ThumbnailUrls, url)
				}
			}
		}
	}
	return nil
}

// RemoveCollection removes a Collection. The user argument is the requesting user. It
// is used to check if the user can perform the operation.
func (s *Service) RemoveCollection(tx *gorm.DB, owner, name string, user *users.User) *gz.ErrMsg {

	col, em := s.internalGetCollection(tx, owner, name, user)
	if em != nil {
		return em
	}

	// make sure the user requesting removal has the correct permissions
	ok, err := globals.Permissions.IsAuthorized(*user.Username, *col.UUID, permissions.Write)
	if !ok {
		return err
	}

	// remove resource from permission db
	ok, err = globals.Permissions.RemoveResource(*col.UUID)
	if !ok {
		return err
	}

	// Remove the resource from the database (soft-delete).
	if err := tx.Delete(col).Error; err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}

	return nil
}

// UpdateCollection updates a collection. The user argument is the requesting
// user. It is used to check if the user can perform the operation.
// Fields that can be currently updated: desc, private.
// The filesPath argument points to a tmp folder from which to read the new files.
// Returns the updated collection. Note: it will be the same instance as 'col' arg.
func (s *Service) UpdateCollection(ctx context.Context, tx *gorm.DB, colOwner,
	colName string, desc, filesPath *string, private *bool,
	user *users.User) (*Collection, *gz.ErrMsg) {

	col, em := s.internalGetCollection(tx, colOwner, colName, user)
	if em != nil {
		return nil, em
	}

	// make sure the user requesting update has the correct permissions
	ok, em := globals.Permissions.IsAuthorized(*user.Username, *col.UUID, permissions.Write)
	if !ok {
		return nil, em
	}

	// Edit the description, if present.
	if desc != nil {
		tx.Model(&col).Update("Description", *desc)
	}

	// Update privacy, if present.
	if private != nil {
		// check if JWT user has permission to update the privacy setting.
		// Only Owners and Admins can do that.
		if ok, em := users.CanPerformWithRole(tx, *col.Owner, *user.Username, permissions.Admin); !ok {
			return nil, em
		}
		tx.Model(&col).Update("Private", *private)
	}

	// Update files, if present
	if filesPath != nil {
		// Replace ALL files with the new ones
		repo := globals.VCSRepoFactory(ctx, *col.GetLocation())
		if err := repo.ReplaceFiles(ctx, *filesPath, *user.Username); err != nil {
			return nil, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
		}
	}

	// first , reset the query clauses
	blankQuery := tx.New()
	if em := populateCollectionThumbnails(blankQuery, col, user); em != nil {
		return nil, em
	}

	return col, nil
}

// CreateCollection creates a new collections.
// creator argument is the active user requesting the operation.
func (s *Service) CreateCollection(ctx context.Context, tx *gorm.DB, cc CreateCollection,
	creator *users.User) (*Collection, *gz.ErrMsg) {

	// Set the owner
	owner := cc.Owner
	if owner == "" {
		owner = *creator.Username
	} else {
		ok, em := users.VerifyOwner(tx, owner, *creator.Username, permissions.Read)
		if !ok {
			return nil, em
		}
	}

	// Sanity check: name should be unique for a user
	if _, err := ByName(tx, cc.Name, owner); err == nil {
		return nil, gz.NewErrorMessageWithArgs(gz.ErrorResourceExists,
			nil, []string{cc.Name})
	}

	private := false
	if cc.Private != nil {
		private = *cc.Private
	}

	col, err := NewCollection(&cc.Name, &cc.Description, &owner, creator.Username,
		private)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorCreatingDir, err)
	}

	if err := os.MkdirAll(*col.GetLocation(), 0711); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorCreatingDir, err)
	}
	_, em := res.CreateResourceRepo(ctx, &col, *col.GetLocation())
	if em != nil {
		return nil, em
	}

	// If everything went OK then create the collection in DB.
	if err := tx.Create(&col).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	// add read and write permissions
	_, err = globals.Permissions.AddPermission(owner, *col.UUID, permissions.Read)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}
	_, err = globals.Permissions.AddPermission(owner, *col.UUID, permissions.Write)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	return &col, nil
}

// AddAsset adds an asset to a collection.
// The user argument is the active user requesting the operation.
func (s *Service) AddAsset(ctx context.Context, tx *gorm.DB, owner, name string,
	no NameOwnerPair, assetType string, user *users.User) (*Collection, *gz.ErrMsg) {

	col, em := s.internalGetCollection(tx, owner, name, user)
	if em != nil {
		return nil, em
	}

	if em := validateAssetType(assetType); em != nil {
		return nil, em
	}

	// Sanity check: the underlying asset (model/world) should exist
	var res ResourceWithID
	ra, errmsg := findAssociatedAsset(tx, no.Owner, no.Name, assetType, user)
	if errmsg != nil {
		return nil, errmsg
	}
	res = ra.(ResourceWithID)

	// Sanity check: the association should NOT exist already
	if _, err := FindAssociation(tx, col.ID, no.Owner, no.Name, assetType); err == nil {
		return nil, gz.NewErrorMessageWithArgs(gz.ErrorResourceExists,
			nil, []string{no.Name, no.Owner, assetType})
	}

	// make sure the requesting user has the correct permissions
	ok, err := globals.Permissions.IsAuthorized(*user.Username, *col.UUID, permissions.Write)
	if !ok {
		return nil, err
	}

	ca := CollectionAsset{ColID: col.ID, AssetID: res.GetID(), AssetName: no.Name,
		AssetOwner: no.Owner, Type: assetType}
	if err := tx.Create(&ca).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	return col, nil
}

// findAssociatedAsset ensures the related asset (model/world) exists.
func findAssociatedAsset(tx *gorm.DB, owner, name,
	assetType string, user *users.User) (interface{}, *gz.ErrMsg) {

	if assetType == TModel {
		return (&models.Service{}).GetModel(tx, owner, name, user)
	}
	return (&worlds.Service{}).GetWorld(tx, owner, name, user)
}

// RemoveAssetFromAllCollections will remove an asset with the provided assetId from all collections. This function assumes that the caller has permissions to perform a Delete on the `collection_assets` table.
func (s *Service) RemoveAssetFromAllCollections(tx *gorm.DB, assetID uint) error {
	return tx.Where("asset_id = ?", assetID).Delete(&CollectionAsset{}).Error
}

// RemoveAsset removes an asset from a collection.
// user argument is the active user requesting the operation.
func (s *Service) RemoveAsset(ctx context.Context, tx *gorm.DB, owner, name string,
	no NameOwnerPair, assetType string, user *users.User) (*Collection, *gz.ErrMsg) {

	col, em := s.internalGetCollection(tx, owner, name, user)
	if em != nil {
		return nil, em
	}

	if em := validateAssetType(assetType); em != nil {
		return nil, em
	}

	// Sanity check: the association should exist
	assoc, err := FindAssociation(tx, col.ID, no.Owner, no.Name, assetType)
	if err != nil {
		return nil, gz.NewErrorMessage(gz.ErrorNonExistentResource)
	}

	// make sure the requesting user has the correct permissions
	ok, em := globals.Permissions.IsAuthorized(*user.Username, *col.UUID, permissions.Write)
	if !ok {
		return nil, em
	}

	// Remove the association from the database (hard-delete)
	if err := tx.Delete(assoc).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}

	return col, nil
}

// GetCollectionAssets returns a paginated list of assets from a collection.
// The optional "assetsType" argument can be used to filter which type of assets
// to return.
// The user argument is the user requesting the operation.
func (s *Service) GetCollectionAssets(p *gz.PaginationRequest, tx *gorm.DB,
	colOwner, colName string, assetsType string, user *users.User) (interface{},
	*gz.PaginationResult, *gz.ErrMsg) {

	col, em := s.internalGetCollection(tx, colOwner, colName, user)
	if em != nil {
		return nil, nil, em
	}

	// TODO(patricio): improve all this once we can return mixed content as part of the
	// same query. For now, we are going to return models OR worlds.

	if em := validateAssetType(assetsType); em != nil {
		return nil, nil, em
	}
	q := tx.Joins(fmt.Sprintf("JOIN collection_assets ON %ss.id = collection_assets.asset_id", assetsType))
	q = q.Where("col_id = ?", col.ID).Where("type = ?", assetsType)

	// Delegate to corresponding service based on type
	if assetsType == TModel {
		return (&models.Service{}).ModelList(p, q, nil, "", "", nil, user, nil)
	}
	return (&worlds.Service{}).WorldList(p, q, nil, "", "", nil, user)
}

// GetAssociatedCollections returns a paginated list of collections given the
// name and owner of an associated asset (eg. model or world).
// The "assetType" argument is used to identify if the name and owner correspond
// to a model or world.
// The user argument is the user requesting the operation.
func (s *Service) GetAssociatedCollections(p *gz.PaginationRequest, tx *gorm.DB,
	no NameOwnerPair, assetType string, user *users.User) (*Collections, *gz.PaginationResult, *gz.ErrMsg) {

	if em := validateAssetType(assetType); em != nil {
		return nil, nil, em
	}
	if assetType == TModel {
		if _, em := (&models.Service{}).GetModel(tx, no.Owner, no.Name, user); em != nil {
			return nil, nil, em
		}
	} else if assetType == TWorld {
		if _, em := (&worlds.Service{}).GetWorld(tx, no.Owner, no.Name, user); em != nil {
			return nil, nil, em
		}
	}

	q := tx.Joins("JOIN collection_assets ON collections.id = collection_assets.col_id")
	q = q.Where("asset_owner = ? AND asset_name = ? AND type = ?", no.Owner,
		no.Name, assetType)

	return s.CollectionList(p, q, nil, "", "", false, user)
}

// GetFile returns the contents (bytes) of a collection file. Version is considered.
// Returns the file's bytes and the resolved version.
// The user argument is the user requesting the operation.
func (s *Service) GetFile(ctx context.Context, tx *gorm.DB, owner, name, path,
	version string, user *users.User) (*[]byte, int, *gz.ErrMsg) {

	col, em := s.internalGetCollection(tx, owner, name, user)
	if em != nil {
		return nil, -1, em
	}
	return res.GetFile(ctx, col, path, version)
}

// CloneCollection clones a collection.
// creator argument is the active user requesting the operation.
func (s *Service) CloneCollection(ctx context.Context, tx *gorm.DB,
	sourceCollectionOwner, sourceCollectionName string,
	cloneData CloneCollection, creator *users.User) (*Collection, *gz.ErrMsg) {

	// Get source collection. This function will return an error if the `creator`
	// does not have the correct permissions to access the collection.
	sourceCollection, em := s.GetCollection(tx, sourceCollectionOwner, sourceCollectionName, creator)
	if em != nil {
		return nil, em
	}

	// Set the owner
	owner := cloneData.Owner
	if owner == "" {
		owner = *creator.Username
	} else {
		ok, em := users.VerifyOwner(tx, owner, *creator.Username, permissions.Read)
		if !ok {
			return nil, em
		}
	}

	private := false
	if sourceCollection.Private != nil {
		private = *sourceCollection.Private
	}

	if private {
		authorized, _ := globals.Permissions.IsAuthorized(
			*creator.Username, *sourceCollection.UUID, permissions.Read)
		if !authorized {
			return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
		}
	}

	// Try to use the given clone collection's or source collection's name. Or find a new one
	var cName string
	if cloneData.Name != "" {
		cName = cloneData.Name
	} else {
		cName = *sourceCollection.Name
	}
	collectionName, err := s.createUniqueCollectionName(tx, cName, owner)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	clonePrivate := false
	if cloneData.Private != nil {
		clonePrivate = *cloneData.Private
	}

	// Create the new Collection (the clone) struct and folder
	clone, err := NewCollection(&collectionName, sourceCollection.Description,
		&owner, creator.Username, clonePrivate)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	// If everything went OK then create the  new model in DB.
	if err := tx.Create(&clone).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	// Get the source collection's assets
	var sourceAssets CollectionAssets
	if err := tx.Where("col_id = ?", sourceCollection.ID).Find(&sourceAssets).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorIDNotFound, err)
	}

	// Insert the assets
	if err := insertAssets(tx, &sourceAssets, clone.ID); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	// add read and write permissions
	_, err = globals.Permissions.AddPermission(owner, *clone.UUID, permissions.Read)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}
	_, err = globals.Permissions.AddPermission(owner, *clone.UUID, permissions.Write)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	return &clone, nil
}

// createUniqueCollectionName is an internal helper that creates a new unique collection name.
func (s *Service) createUniqueCollectionName(tx *gorm.DB, name, owner string) (string, error) {
	// Find an unused name variation
	nameModifier := 1
	newName := name
	for {
		if _, err := ByName(tx, newName, owner); err == nil {
			newName = fmt.Sprintf("%s %d", name, nameModifier)
			nameModifier++
		} else {
			// got the right new name. Exit loop
			break
		}
	}
	return newName, nil
}

// insertAssets bulk inserts a set of assests into a collection.
func insertAssets(tx *gorm.DB, assets *CollectionAssets, collectionID uint) error {
	if assets == nil || len(*assets) <= 0 {
		return nil
	}

	valueStrings := []string{}
	valueArgs := []interface{}{}

	for _, asset := range *assets {
		valueStrings = append(valueStrings, "(?, ?, ?, ?, ?)")
		valueArgs = append(valueArgs, collectionID)
		valueArgs = append(valueArgs, asset.AssetID)
		valueArgs = append(valueArgs, asset.AssetName)
		valueArgs = append(valueArgs, asset.AssetOwner)
		valueArgs = append(valueArgs, asset.Type)
	}

	stmt := fmt.Sprintf("INSERT INTO collection_assets (col_id, asset_id, asset_name, asset_owner, type) VALUES %s", strings.Join(valueStrings, ","))
	if err := tx.Exec(stmt, valueArgs...).Error; err != nil {
		return err
	}

	return nil
}
