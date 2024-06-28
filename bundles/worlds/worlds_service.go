package worlds

import (
	"context"
	"encoding/xml"
	"fmt"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/gazebo-web/gz-go/v7/storage"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	res "github.com/gazebo-web/fuel-server/bundles/common_resources"
	"github.com/gazebo-web/fuel-server/bundles/generics"
	"github.com/gazebo-web/fuel-server/bundles/license"
	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/permissions"
	fuel "github.com/gazebo-web/fuel-server/proto"
	"github.com/gazebo-web/fuel-server/vcs"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// ParseWorldContentsEnvVar holds the name of the boolean env var to check if parsing
// world file contents to look for model references is enabled or not.
const ParseWorldContentsEnvVar = "IGN_FUEL_PARSE_WORLD_MODEL_INCLUDES"

// Service is the main struct exported by this Worlds Service.
type Service struct {
	Storage storage.Storage
}

// GetWorld returns a world by its name and owner's name.
func (ws *Service) GetWorld(tx *gorm.DB, owner, name string,
	user *users.User) (*World, *gz.ErrMsg) {

	w, err := GetWorldByName(tx, name, owner)
	if err != nil {
		em := gz.NewErrorMessageWithArgs(gz.ErrorNameNotFound, err, []string{name})
		return nil, em
	}

	// make sure the user has the correct permissions
	if ok, em := users.CheckPermissions(tx, *w.UUID, user, *w.Private, permissions.Read); !ok {
		return nil, em
	}

	return w, nil
}

// GetWorldProto returns a world proto struct, given a world name and owner.
// The user argument is the user requesting the operation.
func (ws *Service) GetWorldProto(ctx context.Context, tx *gorm.DB, owner,
	name string, user *users.User) (*fuel.World, *gz.ErrMsg) {

	world, em := ws.GetWorld(tx, owner, name, user)
	if em != nil {
		return nil, em
	}
	// get the world latest version number
	latestVersion, err := res.GetLatestVersion(ctx, world)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	// Load the metadata
	tx.Model(&world).Related(&world.Metadata)

	fuelWorld := ws.WorldToProto(world)
	fuelWorld.Version = proto.Int64(int64(latestVersion))

	if user != nil {
		if ml, _ := ws.getWorldLike(tx, world, user); ml != nil {
			fuelWorld.IsLiked = proto.Bool(true)
		}
	}

	return fuelWorld, nil
}

// WorldList returns a paginated list of worlds.
// If the likedBy argument is set, it will return the list of worlds liked by an user.
// TODO: find a way to MERGE this with the one from Worlds service.
func (ws *Service) WorldList(p *gz.PaginationRequest, tx *gorm.DB, owner *string,
	order, search string, likedBy *users.User, user *users.User) (*fuel.Worlds, *gz.PaginationResult, *gz.ErrMsg) {

	var worldList Worlds
	// Create query
	q := QueryForWorlds(tx)

	// Override default Order BY, unless the user explicitly requested ASC order
	if !(order != "" && strings.ToLower(order) == "asc") {
		q = q.Order("created_at desc, id", true)
	}

	// Check if we should return the list of liked worlds instead.
	if likedBy != nil {
		q = q.Joins("JOIN world_likes ON worlds.id = world_likes.world_id").Where("user_id = ?", &likedBy.ID)
	} else {

		// filter resources based on privacy setting
		q = res.QueryForResourceVisibility(tx, q, owner, user)

		// If a search criteria was defined, then also apply a fulltext search on "world's name + description + tags"
		if search != "" {
			// Trim leading and trailing whitespaces
			searchStr := strings.TrimSpace(search)
			if len(searchStr) > 0 {
				// Note: this is a fulltext search IN NATURAL LANGUAGE MODE.
				// See https://dev.mysql.com/doc/refman/5.7/en/fulltext-search.html for other
				// modes, eg BOOLEAN and WITH QUERY EXPANSION modes.

				// Probably this can be improved a lot. But to avoid fighting against making GORM with complex
				// queries work we are going to first execute a raw query to get the matching world IDs, and
				// then ask GORM to retrieve those worlds.
				sq := "(SELECT world_id FROM (SELECT * FROM tags WHERE MATCH (name) AGAINST (?)) AS a " +
					"INNER JOIN world_tags ON tag_id = id) UNION " +
					"(SELECT id FROM worlds WHERE MATCH (name, description) AGAINST (?));"
				var ids []int
				if err := tx.Raw(sq, searchStr, searchStr).Pluck("world_id", &ids).Error; err != nil {
					em := gz.NewErrorMessageWithBase(gz.ErrorNoDatabase, err)
					return nil, nil, em
				}
				// Now that we got the IDs , use them in the main query
				q = q.Where("id IN (?)", ids)
			}
		}
	}

	// Use pagination
	paginationResult, err := gz.PaginateQuery(q, &worldList, *p)
	if err != nil {
		em := gz.NewErrorMessageWithBase(gz.ErrorInvalidPaginationRequest, err)
		return nil, nil, em
	}
	if !paginationResult.PageFound {
		em := gz.NewErrorMessage(gz.ErrorPaginationPageNotFound)
		return nil, nil, em
	}

	var worldsProto fuel.Worlds
	// Encode worlds into a protobuf message
	for _, w := range worldList {
		fuelWorld := ws.WorldToProto(&w)
		worldsProto.Worlds = append(worldsProto.Worlds, fuelWorld)
	}

	return &worldsProto, paginationResult, nil
}

// RemoveWorld removes a world. The user argument is the requesting user. It
// is used to check if the user can perform the operation.
func (ws *Service) RemoveWorld(ctx context.Context, tx *gorm.DB, owner, name string, user *users.User) *gz.ErrMsg {

	world, em := ws.GetWorld(tx, owner, name, user)
	if em != nil {
		return em
	}

	// make sure the user requesting removal has the correct permissions
	ok, err := globals.Permissions.IsAuthorized(*user.Username, *world.UUID, permissions.Write)
	if !ok {
		return err
	}

	// remove resource from permission db
	ok, err = globals.Permissions.RemoveResource(*world.UUID)
	if !ok {
		return err
	}
	// NOTE: no need to remove the world's ModelIncludes.

	// Remove the model from ElasticSearch
	ElasticSearchRemoveWorld(ctx, world)

	return res.Remove(tx, world, *user.Username)
}

// WorldToProto creates a new 'fuel.World' from the given world.
// NOTE: returned "thumbnail urls" are pointing to the "tip" version.
func (ws *Service) WorldToProto(world *World) *fuel.World {
	fuelWorld := fuel.World{
		// Note: time.RFC3339 is the format expected by Go's JSON unmarshal
		CreatedAt:  proto.String(world.CreatedAt.UTC().Format(time.RFC3339)),
		UpdatedAt:  proto.String(world.UpdatedAt.UTC().Format(time.RFC3339)),
		Name:       proto.String(*world.Name),
		Owner:      proto.String(*world.Owner),
		Likes:      proto.Int64(int64(world.Likes)),
		Downloads:  proto.Int64(int64(world.Downloads)),
		Filesize:   proto.Int64(int64(world.Filesize)),
		Permission: proto.Int64(int64(world.Permission)),
		LicenseId:  proto.Uint64(uint64(world.LicenseID)),
	}

	// Optional fields
	if world.UploadDate != nil {
		fuelWorld.UploadDate =
			proto.String(world.UploadDate.UTC().Format(time.RFC3339))
	}
	if world.DeletedAt != nil {
		fuelWorld.DeletedAt =
			proto.String(world.DeletedAt.UTC().Format(time.RFC3339))
	}
	if world.ModifyDate != nil {
		fuelWorld.ModifyDate =
			proto.String(world.ModifyDate.UTC().Format(time.RFC3339))
	}
	if world.Description != nil {
		fuelWorld.Description = proto.String(*world.Description)
	}
	if world.License.Name != nil {
		fuelWorld.LicenseName = proto.String(*world.License.Name)
	}
	if world.License.ContentURL != nil {
		fuelWorld.LicenseUrl = proto.String(*world.License.ContentURL)
	}
	if world.License.ImageURL != nil {
		fuelWorld.LicenseImage = proto.String(*world.License.ImageURL)
	}
	if world.Private != nil {
		fuelWorld.Private = proto.Bool(*world.Private)
	}

	if len(world.Tags) > 0 {
		tags := []string{}
		for _, tag := range world.Tags {
			tags = append(tags, *tag.Name)
		}
		fuelWorld.Tags = tags
	}

	// Append metadata, if it exists
	if len(world.Metadata) > 0 {
		var metadata []*fuel.Metadatum

		// Convert DB representation to proto
		for _, datum := range world.Metadata {
			fuelDatum := fuel.Metadatum{
				Key:   proto.String(*datum.Key),
				Value: proto.String(*datum.Value),
			}
			metadata = append(metadata, &fuelDatum)
		}
		fuelWorld.Metadata = metadata
	}

	// Squash first thumbnail url
	if tbnPaths, err := res.GetThumbnails(world); err == nil {
		url := fmt.Sprintf("/%s/worlds/%s/tip/files/%s", *world.Owner,
			url.PathEscape(*world.Name), tbnPaths[0])
		fuelWorld.ThumbnailUrl = proto.String(url)
	}

	return &fuelWorld
}

// getWorldLike returns a world like.
func (ws *Service) getWorldLike(tx *gorm.DB, world *World, user *users.User) (*WorldLike, *gz.ErrMsg) {
	var worldLike WorldLike
	if err := tx.Where("user_id = ? AND world_id = ?", user.ID, world.ID).First(&worldLike).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorIDNotFound, err)
	}
	return &worldLike, nil
}

// CreateWorldLike creates a WorldLike.
// Returns the created worldLike, or a gz.errMsg.
func (ws *Service) CreateWorldLike(tx *gorm.DB, owner, worldName string, user *users.User) (*WorldLike, *gz.ErrMsg) {
	if user == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}

	world, em := ws.GetWorld(tx, owner, worldName, user)
	if em != nil {
		return nil, em
	}

	// Register the like.
	worldLike := WorldLike{UserID: &user.ID, WorldID: &world.ID}
	if err := tx.Create(&worldLike).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}
	// Update the number of likes of the world.
	errorMsg := ws.increaseLikeCounter(tx, world, 1)
	if errorMsg != nil {
		return nil, errorMsg
	}
	return &worldLike, nil
}

// CreateWorldReport creates a WorldReport
func (ws *Service) CreateWorldReport(tx *gorm.DB, owner, worldName, reason string) (*WorldReport, *gz.ErrMsg) {

	world, err := GetWorldByName(tx, worldName, owner)

	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorNameNotFound, err)
	}

	worldReport := WorldReport{
		Report: generics.Report{
			Reason: &reason,
		},
		WorldID: &world.ID,
	}

	if err := tx.Create(&worldReport).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	return &worldReport, nil
}

// RemoveWorldLike removes a worldLike.
// Returns the removed worldLike or a gz.errMsg.
func (ws *Service) RemoveWorldLike(tx *gorm.DB, owner, worldName string, user *users.User) (*WorldLike, *gz.ErrMsg) {

	if user == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}

	world, em := ws.GetWorld(tx, owner, worldName, user)
	if em != nil {
		return nil, em
	}

	// Unlike the world.
	var worldLike WorldLike
	q := tx.Where("user_id = ? AND world_id = ?", &user.ID, &world.ID).Delete(&worldLike)
	if q.Error != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbDelete, q.Error)
	}

	// Decrease the number of likes of the world if there was an existing like
	if q.RowsAffected > 0 {
		errorMsg := ws.decreaseLikeCounter(tx, world, uint(q.RowsAffected))
		if errorMsg != nil {
			return nil, errorMsg
		}
	}

	return &worldLike, nil
}

// applyExpression updates a world using SQL expression that can perform operations on referred values.
func (ws *Service) applyExpression(tx *gorm.DB, world *World, field string, expr *gorm.SqlExpr) *gz.ErrMsg {
	if err := tx.Model(world).Update(field, expr).Error; err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}
	return nil
}

// ComputeAllCounters is an initialization function that iterates
// all worlds and updates their likes and downloads counter, based on the number
// of records in corresponding tables world_likes and world_downloads.
func (ws *Service) ComputeAllCounters(tx *gorm.DB) *gz.ErrMsg {
	var worldList Worlds
	if err := tx.Model(&World{}).Unscoped().Find(&worldList).Error; err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorNoDatabase, err)
	}
	for _, w := range worldList {
		if em := ws.computeLikeCounter(tx, &w); em != nil {
			return em
		}
		if em := ws.computeDownloadCounter(tx, &w); em != nil {
			return em
		}
	}
	return nil
}

// computeLikeCounter counts the number of likes and updates the world accordingly.
// This query is VERY EXPENSIVE. Only use to set the state if it doesn't exist.
// For all other purposes, the use of increase/decreaseLikeCounter is recommended.
func (ws *Service) computeLikeCounter(tx *gorm.DB, world *World) *gz.ErrMsg {
	// Count the number of likes of the world.
	var counter int
	if err := tx.Model(&WorldLike{}).Where("world_id = ?", world.ID).Count(&counter).Error; err != nil {
		// Note: This is not currently covered by the tests.
		return gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}
	// Update the number of likes of the world.
	if err := tx.Model(world).Update("likes", counter).Error; err != nil {
		// Note: This is not currently covered by the tests.
		return gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}
	return nil
}

// increaseLikeCounter increases the current like count of a world.
func (ws *Service) increaseLikeCounter(tx *gorm.DB, world *World, delta uint) *gz.ErrMsg {
	return ws.applyExpression(tx, world, "likes", gorm.Expr("likes + ?", delta))
}

// decreaseLikeCounter decreases the current like count of a world.
func (ws *Service) decreaseLikeCounter(tx *gorm.DB, world *World, delta uint) *gz.ErrMsg {
	return ws.applyExpression(tx, world, "likes", gorm.Expr("likes - ?", delta))
}

// computeDownloadCounter counts the number of downloads and updates the world accordingly.
// This query is VERY EXPENSIVE. Only use to set the state if it doesn't exist.
// For all other purposes, the use of increaseDownloadCounter is recommended.
func (ws *Service) computeDownloadCounter(tx *gorm.DB, world *World) *gz.ErrMsg {
	// Count the number of downloads of the world.
	var count int
	if err := tx.Model(&WorldDownload{}).Where("world_id = ?", world.ID).Count(&count).Error; err != nil {
		// Note: This is not currently covered by the tests.
		return gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}
	// Update the number of downloads of the world.
	if err := tx.Model(world).Update("Downloads", count).Error; err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}
	return nil
}

// increaseDownloadCounter increases the current download count of a world by 1.
func (ws *Service) increaseDownloadCounter(tx *gorm.DB, world *World, delta uint) *gz.ErrMsg {
	return ws.applyExpression(tx, world, "downloads", gorm.Expr("downloads + ?", delta))
}

// GetFile returns the contents (bytes) of a world file. World version is considered.
// Returns the file's bytes and the resolved version of the world.
func (ws *Service) GetFile(ctx context.Context, tx *gorm.DB, owner, name, path,
	version string, user *users.User) (*[]byte, int, *gz.ErrMsg) {

	world, em := ws.GetWorld(tx, owner, name, user)
	if em != nil {
		return nil, -1, em
	}

	return res.GetFile(ctx, world, path, version)
}

// FileTree gets the world's FileTree
func (ws *Service) FileTree(ctx context.Context, tx *gorm.DB, owner, worldName,
	version string, user *users.User) (*fuel.FileTree, *gz.ErrMsg) {

	world, em := ws.GetWorld(tx, owner, worldName, user)
	if em != nil {
		return nil, em
	}

	return res.FileTree(ctx, world, version)
}

// DownloadZip returns the path to a zip file representing a world at the given
// version.
// This method increments the downloads counter of the world.
// Optional argument "user" represents the user (if any) requesting the operation.
// Returns the world, as well as a pointer to the zip's filepath and the
// resolved version.
func (ws *Service) DownloadZip(ctx context.Context, tx *gorm.DB, owner, worldName, version string,
	u *users.User, agent string, zipGetter res.GetZipResource) (*World, *string, int, *gz.ErrMsg) {

	world, em := ws.GetWorld(tx, owner, worldName, u)
	if em != nil {
		return nil, nil, 0, em
	}
	// increment downloads count
	worldDl := WorldDownload{WorldID: &world.ID, UserAgent: agent}
	if u != nil {
		worldDl.UserID = &u.ID
	}
	if err := tx.Create(&worldDl).Error; err != nil {
		return nil, nil, 0, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}
	// Update the number of downloads of the world.
	errorMsg := ws.increaseDownloadCounter(tx, world, 1)
	if errorMsg != nil {
		return nil, nil, 0, errorMsg
	}
	_, resolvedVersion, em := res.GetRevisionFromVersion(ctx, world, version)
	if em != nil {
		return nil, nil, 0, em
	}
	link, err := zipGetter(ctx, world, worlds, resolvedVersion)
	if err != nil {
		return nil, nil, 0, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}
	return world, &link, resolvedVersion, nil
}

// UpdateWorld updates a world. The user argument is the requesting user. It
// is used to check if the user can perform the operation.
// Fields that can be currently updated: desc, tags, and files.
// The filesPath argument points to a tmp folder from which to read the new files.
func (ws *Service) UpdateWorld(ctx context.Context, tx *gorm.DB, owner,
	worldName string, desc, tagstr, filesPath *string, private *bool,
	user *users.User, metadata *WorldMetadata) (*World, *gz.ErrMsg) {

	world, em := ws.GetWorld(tx, owner, worldName, user)
	if em != nil {
		return nil, em
	}

	// make sure the user requesting update has the correct permissions
	ok, err := globals.Permissions.IsAuthorized(*user.Username, *world.UUID, permissions.Write)
	if !ok {
		return nil, err
	}

	// Edit the description, if present.
	if desc != nil {
		tx.Model(&world).Update("Description", *desc)
	}
	// Edit the tags, if present.
	if tagstr != nil {
		tags, err := models.StrToTags(tx, *tagstr)
		if err != nil {
			return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
		}
		tx.Model(&world).Association("Tags").Replace(*tags)
	}

	// Update the metadata, if the data is present.
	if metadata != nil {
		// Handle the special case where the metadata consists of one Metadatum
		// element with empty Key and Value elements. This indicates that
		// the metadata should be cleared.
		if len(*metadata) == 1 && (*metadata)[0].IsEmpty() {
			tx.Model(&world).Association("Metadata").Clear()
		} else {
			tx.Model(&world).Association("Metadata").Replace(*metadata)
		}
	}

	// Update the modification date.
	tx.Model(&world).Update("ModifyDate", time.Now())

	// Update files, if present
	if filesPath != nil {
		// Replace ALL files with the new ones
		repo := globals.VCSRepoFactory(ctx, *world.Location)
		if err := repo.ReplaceFiles(ctx, *filesPath, *user.Username); err != nil {
			return nil, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
		}
		// update zip file and filesize
		if em := ws.updateZip(ctx, repo, world); em != nil {
			return nil, em
		}
		tx.Model(&world).Update("Filesize", world.Filesize)

		// parse the world file and find the model references
		if em := populateModelIncludes(ctx, tx, world, *filesPath); em != nil {
			return nil, em
		}
	}

	// Update privacy, if present.
	if private != nil {
		// check if JWT user has permission to update the privacy setting.
		// Only Owners and Admins can do that.
		if ok, em := users.CanPerformWithRole(tx, *world.Owner, *user.Username, permissions.Admin); !ok {
			return nil, em
		}
		tx.Model(&world).Update("Private", *private)
	}

	ElasticSearchUpdateWorld(ctx, *world)
	return world, nil
}

// updateZip creates a new zip file for the given world and also
// updates its Filesize field in DB.
func (ws *Service) updateZip(ctx context.Context, repo vcs.VCS, world *World) *gz.ErrMsg {
	zSize, path, em := res.ZipResourceTip(ctx, repo, world, worlds)
	if em != nil {
		return em
	}
	f, err := os.Open(path)
	if err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	v, err := res.GetLatestVersion(ctx, world)
	if err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	err = ws.Storage.UploadZip(ctx, res.CastResourceToStorageResource(world, uint64(v)), f)
	if err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	world.Filesize = int(zSize)
	return nil
}

// CreateWorld creates a new world.
// creator argument is the active user requesting the operation.
func (ws *Service) CreateWorld(ctx context.Context, tx *gorm.DB, cm CreateWorld,
	uuidStr, filesPath string, creator *users.User) (*World, *gz.ErrMsg) {

	// Sanity check: Ensure license exists
	license, err := license.ByID(tx, cm.License)
	if err != nil {
		return nil, gz.NewErrorMessageWithArgs(gz.ErrorFormInvalidValue, err, []string{"license"})
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

	// Sanity check: name should be unique for a user
	if _, err := GetWorldByName(tx, cm.Name, owner); err == nil {
		return nil, gz.NewErrorMessageWithArgs(gz.ErrorFormDuplicateWorldName, nil, []string{cm.Name})
	}
	// Process the optional tags
	pTags, err := models.StrToTags(tx, cm.Tags)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	private := false
	if cm.Private != nil {
		private = *cm.Private
	}

	world, err := NewWorld(&uuidStr, &cm.Name, &cm.Description, nil, &owner,
		creator.Username, *license, cm.Permission, *pTags, private, cm.Metadata)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorCreatingDir, err)
	}

	repo, em := res.CreateResourceRepo(ctx, &world, filesPath)
	if em != nil {
		return nil, em
	}

	// Zip the world and compute its size.
	if em := ws.updateZip(ctx, repo, &world); em != nil {
		return nil, em
	}

	// If everything went OK then create the world in DB.
	if err := tx.Create(&world).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	// add read and write permissions
	_, err = globals.Permissions.AddPermission(owner, *world.UUID, permissions.Read)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}
	_, err = globals.Permissions.AddPermission(owner, *world.UUID, permissions.Write)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	// parse the world file and find the model references
	em = populateModelIncludes(ctx, tx, &world, filesPath)
	if em != nil {
		return nil, em
	}

	ElasticSearchUpdateWorld(ctx, world)
	return &world, nil
}

// populateModelIncludes is an internal function that given a world and its location,
// computes and stores included model references.
func populateModelIncludes(ctx context.Context, tx *gorm.DB, world *World,
	worldDirPath string) *gz.ErrMsg {

	enabled, _ := gz.ReadEnvVar(ParseWorldContentsEnvVar)
	if flag, err := strconv.ParseBool(enabled); err != nil || !flag {
		return nil
	}

	worldVersion, err := res.GetLatestVersion(ctx, world)
	if err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	worldFilePath, err := getWorldMainFile(worldDirPath)
	if err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorFormInvalidValue, err)
	}

	incs, em := parseModelIncludes(tx, world, worldVersion, *worldFilePath)
	if em != nil {
		return em
	}
	for _, mi := range *incs {
		// Add Model Includes to DB
		if err := tx.Create(&mi).Error; err != nil {
			return gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
		}
	}
	return nil
}

// getWorldMainFile returns the first file path with extension '.world' on the
// given folder.
// Otherwise it returns an error.
func getWorldMainFile(worldDirPath string) (*string, error) {
	// TODO: an uploaded world folder can have multiple world files (with extension .world/.sdf)
	files, err := os.ReadDir(worldDirPath)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		ext := filepath.Ext(f.Name())
		if ext == ".world" {
			return gz.String(filepath.Join(worldDirPath, f.Name())), nil
		}
	}
	return nil, errors.New(".world file not found")
}

// Highlevel structure of a .world file, for xml parsing.
type worldFile struct {
	World worldNode `xml:"world"`
}
type worldNode struct {
	Includes []include `xml:"include"`
}
type include struct {
	URI string `xml:"uri"`
}

// parseModelIncludes is a helper function that given a world and its location
// on disk, finds the referenced external models. These references can be in
// the old form (model://) or new form (full url).
func parseModelIncludes(tx *gorm.DB, world *World,
	version int, worldFilePath string) (*ModelIncludes, *gz.ErrMsg) {

	// TODO: a world file can have multiple <world> elements. We assume only 1 for now
	xmlFile, err := os.Open(worldFilePath)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorFormInvalidValue, err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Println("Failed to close file:", err)
		}
	}(xmlFile)

	b, err := io.ReadAll(xmlFile)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorFormInvalidValue, err)
	}

	var w worldFile
	if err := xml.Unmarshal(b, &w); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	// Types of Model Includes:
	// 1) Full URI format: <server>/(owner)/models/(model_name)/(version_number)
	// .*/([^/]+)/models/([^/]+)/([0-9]+)
	// 2) Old Format - model://{model_name}
	fullModelIncludeRE := regexp.MustCompile(".*/([^/]+)/models/([^/]+)/([0-9]+)")
	fullModelIncludeRE.Longest()

	modelIncludes := ModelIncludes{}
	for _, inc := range w.World.Includes {
		var mOwner *string
		var incType string
		mVer := -1
		modelName := ""
		if strings.HasPrefix(inc.URI, "model://") {
			// Old format include
			modelName = inc.URI[8:]
			incType = "model_prefix"
		} else if m := fullModelIncludeRE.FindStringSubmatch(inc.URI); m != nil {
			mOwner = &m[1]
			modelName = m[2]
			mVer, _ = strconv.Atoi(m[3])
			incType = "full_url"
		} else {
			// no match . Fail
			err := errors.New("Model Include does not have valid format: " + inc.URI)
			return nil, gz.NewErrorMessageWithBase(gz.ErrorFormInvalidValue, err)
		}
		mi := ModelInclude{WorldID: world.ID, WorldVersion: &version,
			ModelOwner: mOwner, ModelName: &modelName, ModelVersion: &mVer,
			IncludeType: &incType,
		}
		modelIncludes = append(modelIncludes, mi)
	}

	return &modelIncludes, nil
}

// CloneWorld clones a world.
// creator argument is the active user requesting the operation.
func (ws *Service) CloneWorld(ctx context.Context, tx *gorm.DB, swOwner,
	swName string, cw CloneWorld, creator *users.User) (*World, *gz.ErrMsg) {

	world, em := ws.GetWorld(tx, swOwner, swName, creator)
	if em != nil {
		return nil, em
	}

	// Set the owner
	owner := cw.Owner
	if owner == "" {
		owner = *creator.Username
	} else {
		ok, em := users.VerifyOwner(tx, owner, *creator.Username, permissions.Read)
		if !ok {
			return nil, em
		}
	}

	private := false
	if world.Private != nil {
		private = *world.Private
	}

	if private {
		authorized, _ := globals.Permissions.IsAuthorized(
			*creator.Username, *world.UUID, permissions.Read)
		if !authorized {
			return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
		}
	}

	// Try to use the given name. Or find a new one
	var aName string
	if cw.Name != "" {
		aName = cw.Name
	} else {
		aName = *world.Name
	}
	worldName, err := ws.createUniqueName(tx, aName, owner)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorCreatingDir, err)
	}

	clonePrivate := false
	if cw.Private != nil {
		clonePrivate = *cw.Private
	}

	// Load the metadata
	tx.Model(&world).Related(&world.Metadata)

	// Create the new world (the clone) struct and folder
	clone, err := NewWorldAndUUID(&worldName, world.Description,
		nil, &owner, creator.Username, world.License, world.Permission, world.Tags,
		clonePrivate, &world.Metadata)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorCreatingDir, err)
	}

	repo, em := res.CloneResourceRepo(ctx, world, &clone)
	if em != nil {
		return nil, em
	}

	// Zip the world and compute its size.
	if em := ws.updateZip(ctx, repo, &clone); em != nil {
		os.RemoveAll(*clone.Location)
		return nil, em
	}

	// If everything went OK then create the new world in DB.
	if err := tx.Create(&clone).Error; err != nil {
		os.RemoveAll(*clone.Location)
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
	// parse the world file, find the model references and recreate them in DB
	if em := populateModelIncludes(ctx, tx, &clone, *clone.GetLocation()); em != nil {
		return nil, em
	}

	return &clone, nil
}

// createUniqueName is an internal helper to disambiguate among resource names
func (ws *Service) createUniqueName(tx *gorm.DB, name, owner string) (string, error) {
	// Find an unused name variation
	nameModifier := 1
	newName := name
	for {
		if _, err := GetWorldByName(tx, newName, owner); err == nil {
			newName = fmt.Sprintf("%s %d", newName, nameModifier)
			nameModifier++
		} else {
			// got the right new name. Exit loop
			break
		}
	}
	return newName, nil
}

// GetModelReferences returns the list of external "model includes" of a world.
// Argument @version is the world version. Can be "tip" too.
// Argument @user is the requesting user.
func (ws *Service) GetModelReferences(ctx context.Context, p *gz.PaginationRequest,
	tx *gorm.DB, owner, name, version string,
	user *users.User) (*ModelIncludes, *gz.PaginationResult, *gz.ErrMsg) {

	world, em := ws.GetWorld(tx, owner, name, user)
	if em != nil {
		return nil, nil, em
	}
	_, resolvedVersion, em := res.GetRevisionFromVersion(ctx, world, version)
	if em != nil {
		return nil, nil, em
	}

	q := tx.Model(&ModelInclude{}).Where("world_id = ? AND world_version = ?", world.ID, resolvedVersion)

	var includes ModelIncludes
	// Use pagination
	paginationResult, err := gz.PaginateQuery(q, &includes, *p)
	if err != nil {
		em := gz.NewErrorMessageWithBase(gz.ErrorInvalidPaginationRequest, err)
		return nil, nil, em
	}
	if !paginationResult.PageFound {
		em := gz.NewErrorMessage(gz.ErrorPaginationPageNotFound)
		return nil, nil, em
	}
	return &includes, paginationResult, nil
}
