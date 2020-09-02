package commonres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	"gitlab.com/ignitionrobotics/web/fuelserver/permissions"
	"gitlab.com/ignitionrobotics/web/fuelserver/proto"
	"gitlab.com/ignitionrobotics/web/fuelserver/vcs"
	"gitlab.com/ignitionrobotics/web/ign-go"
)

// This package contains common functions for file based resources. Eg: model,
// worlds, etc.
// Functions and types in this package are commonly used by services.

// Resource represents a resource with files (eg. model, world)
type Resource interface {
	GetName() *string
	GetOwner() *string
	SetOwner(owner string)
	GetLocation() *string
	SetLocation(location string)
	GetUUID() *string
}

// GetFile returns the contents (bytes) of a resource file. Given version is considered.
// Returns the file's bytes and the resolved version of the resource.
func GetFile(ctx context.Context, res Resource, path, version string) (*[]byte, int, *ign.ErrMsg) {
	rev, resolvedVersion, em := GetRevisionFromVersion(ctx, res, version)
	if em != nil {
		return nil, 0, em
	}
	repo := globals.VCSRepoFactory(ctx, *res.GetLocation())
	bs, err := repo.GetFile(ctx, rev, path)
	if err != nil {
		return nil, 0, ign.NewErrorMessageWithBase(ign.ErrorFileNotFound, err)
	}
	return bs, resolvedVersion, nil
}

// GetRevisionFromVersion finds the revision hash from a given resource version.
// Version 1 is the initial version of the resource when the
// repo was created or cloned.
// Returns the found revision, the resolved version or an error.
func GetRevisionFromVersion(ctx context.Context, res Resource,
	version string) (string, int, *ign.ErrMsg) {

	// get latest version number
	latestVersion, err := GetLatestVersion(ctx, res)
	if err != nil {
		return "", 0, ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}

	var resRev string
	var resolvedVersion int
	if version == "tip" || version == "" {
		resRev = ""
		resolvedVersion = latestVersion
	} else {
		// parse the version given in route
		resVersionParsed, parseErr := strconv.Atoi(version)
		if parseErr != nil {
			return "", 0, ign.NewErrorMessageWithArgs(ign.ErrorFormInvalidValue, parseErr, []string{"version"})
		}

		if resVersionParsed <= 0 {
			return "", 0, ign.NewErrorMessageWithArgs(ign.ErrorFormInvalidValue,
				errors.New("Invalid version: "+version), []string{"version"})
		}

		// get revision of specified version by computing a ref name from HEAD
		revNumberFromHEAD := latestVersion - resVersionParsed
		resRev = "HEAD~" + strconv.Itoa(revNumberFromHEAD)
		if revNumberFromHEAD < 0 {
			return "", 0, ign.NewErrorMessageWithBase(ign.ErrorVersionNotFound,
				errors.New("Unkown revision: "+resRev))
		}
		resolvedVersion = resVersionParsed
	}

	return resRev, resolvedVersion, nil
}

// GetLatestVersion gets the latest version number of a file based resource.
func GetLatestVersion(ctx context.Context, res Resource) (int, error) {

	repo := globals.VCSRepoFactory(ctx, *res.GetLocation())

	// get the total number of revisions of this file
	totalRevCount, err := repo.RevisionCount(ctx, "master")
	if err != nil {
		return -1, err
	}

	// get the number of revisions at the initial version
	// this is indicated by a tag based on the resource's UUID, created when the
	// resource repo is first created or cloned.
	initialRevCount, err := repo.RevisionCount(ctx, *res.GetUUID())
	if err != nil {
		return -1, err
	}

	versionNumber := totalRevCount - initialRevCount + 1
	return versionNumber, nil
}

// GetThumbnails returns a slice of urls pointing to the thumbnails.
func GetThumbnails(res Resource) (tbns []string, err error) {
	var files []string
	files, err = filepath.Glob(filepath.Join(*res.GetLocation(), "thumbnails/*"))
	if len(files) == 0 {
		err = errors.New("No thumbnails found")
		return
	}

	tbns = make([]string, 0)
	for _, fullpath := range files {
		tbns = append(tbns, fullpath[len(*res.GetLocation())+1:])
	}
	return
}

// FileTree gets a the file tree of a versioned resource.
func FileTree(ctx context.Context, res Resource, version string) (*fuel.FileTree, *ign.ErrMsg) {

	rev, resolvedVersion, errMsg := GetRevisionFromVersion(ctx, res, version)
	if errMsg != nil {
		return nil, errMsg
	}

	ft := fuel.FileTree{
		Name:    proto.String(*res.GetName()),
		Owner:   proto.String(*res.GetOwner()),
		Version: proto.Int64(int64(resolvedVersion)),
	}

	// Get the file tree
	dirPath := filepath.Clean(*res.GetLocation())
	if _, err := os.Stat(dirPath); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorFileTree, err)
	}

	// Use a map to be independent from the order followed by Walk
	var folderNodes = make(map[string]*fuel.FileTree_FileNode)

	// Get the world repository
	repo := globals.VCSRepoFactory(ctx, dirPath)
	walkFn := func(path, parentPath string, isDir bool) error {
		if path == "/" {
			// We don't create a tree node for the resource's root folder
			return nil
		}
		// Process current node
		name := filepath.Base(path)
		node := fuel.FileTree_FileNode{Name: &name, Path: &path}
		if parentPath == "/" {
			// The parent folder is the world root folder
			ft.FileTree = append(ft.FileTree, &node)
		} else {
			parent := folderNodes[parentPath]
			parent.Children = append(parent.Children, &node)
		}
		if isDir {
			folderNodes[path] = &node
		}
		// Return OK value
		return nil
	}
	if err := repo.Walk(ctx, rev, true, walkFn); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorFileTree, err)
	}

	// All OK
	return &ft, nil
}

// Remove removes a resource. The user argument is the requesting user. It
// is used to check if the user can perform the operation.
func Remove(tx *gorm.DB, res Resource, user string) *ign.ErrMsg {

	// Sanity check: Make sure the file exists.
	if res.GetLocation() != nil {
		dirPath := filepath.Dir(*res.GetLocation())
		if _, err := os.Stat(dirPath); err != nil {
			return ign.NewErrorMessageWithBase(ign.ErrorNonExistentResource, err)
		}
	} else if globals.ResourceDir != "" {
		return ign.NewErrorMessage(ign.ErrorNonExistentResource)
	}

	// NOTE: we are not removing the files.

	// Remove the resource from the database (soft-delete).
	if err := tx.Delete(res).Error; err != nil {
		return ign.NewErrorMessageWithBase(ign.ErrorDbDelete, err)
	}

	return nil
}

// ZipResourceTip creates a new zip file for the given resource. Returns the zip
// Filesize or an error.
// subfolder arg is the resource type folder for the user (eg. models, worlds)
func ZipResourceTip(ctx context.Context, repo vcs.VCS, res Resource, subfolder string) (int64, *ign.ErrMsg) {
	zipPath := getOrCreateZipLocation(res, subfolder, "")

	// If the zippath doesn't exist, then this is the first version. Recompute
	// the zippath.
	if _, err := os.Stat(zipPath); err != nil {
		zipPath = getOrCreateZipLocation(res, subfolder, "1")
	}

	// Zip the model and compute its size
	_, err := repo.Zip(ctx, "", zipPath)
	if err != nil {
		ign.LoggerFromContext(ctx).Info("Error trying to zip resource", err)
		return -1, ign.NewErrorMessageWithBase(ign.ErrorCreatingFile, err)
	}
	fInfo, err := os.Stat(zipPath)
	if err != nil {
		ign.LoggerFromContext(ctx).Info("Error getting zip file info / stat", err)
		return -1, ign.NewErrorMessageWithBase(ign.ErrorCreatingFile, err)
	}
	return fInfo.Size(), nil
}

// getOrCreateZipLocation either returns the path to an existing resource's zip
// file or creates a new '.zips' folder for the user and return the zip path.
// subfolder arg is the resource type folder for the user (eg. models, worlds)
func getOrCreateZipLocation(res Resource, subfolder, version string) string {
	zipsFolder := filepath.Join(globals.ResourceDir, *res.GetOwner(), subfolder, ".zips")
	os.Mkdir(zipsFolder, 0711)

	if version == "" || version == "tip" {
		version = ""
	} else {
		version = "v" + version
	}

	// path to this model's zip
	zipPath := filepath.Join(zipsFolder, strings.Replace(*res.GetUUID(), " ", "_", -1)+version+".zip")
	return zipPath
}

// GetZip returns a path to the existing resource zip for the given version.
// It creates the zip if it does not exist.
// subfolder arg is the resource type folder for the user (eg. models, worlds)
func GetZip(ctx context.Context, res Resource, subfolder string, version string) (*string, int, *ign.ErrMsg) {

	rev, resolvedVersion, em := GetRevisionFromVersion(ctx, res, version)
	if em != nil {
		return nil, 0, em
	}

	path := getOrCreateZipLocation(res, subfolder, version)
	zipPath := &path

	if _, err := os.Stat(path); err != nil {
		repo := globals.VCSRepoFactory(ctx, *res.GetLocation())
		var err error
		zipPath, err = repo.Zip(ctx, rev, path)
		if err != nil {
			return nil, 0, ign.NewErrorMessageWithBase(ign.ErrorZipNotAvailable, err)
		}
	}

	return zipPath, resolvedVersion, nil
}

// CreateResourceRepo creates the VCS repository for a given resource
// Returns the created VCS repository.
func CreateResourceRepo(ctx context.Context, res Resource, filesPath string) (vcs.VCS, *ign.ErrMsg) {
	// Create the world repository
	repo := globals.VCSRepoFactory(ctx, filesPath)
	if err := repo.InitRepo(ctx); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorRepo, err)
	}
	// Tag the repo with the world's UUID
	if err := repo.Tag(ctx, *res.GetUUID()); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorRepo, err)
	}
	return repo, nil
}

// CloneResourceRepo clones the VCS repository of a given resource.
// Returns the VCS respository of the clone.
func CloneResourceRepo(ctx context.Context, res, clone Resource) (vcs.VCS, *ign.ErrMsg) {
	// Open the VCS repo of the source world and clone it
	repo := globals.VCSRepoFactory(ctx, *res.GetLocation())
	if err := repo.CloneTo(ctx, *clone.GetLocation()); err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorCreatingDir, err)
	}

	// Now get the VCS repository of the clone (it is a different repo)
	repo = globals.VCSRepoFactory(ctx, *clone.GetLocation())
	// and tag it with the clone's UUID
	if err := repo.Tag(ctx, *clone.GetUUID()); err != nil {
		os.Remove(*clone.GetLocation())
		return nil, ign.NewErrorMessageWithBase(ign.ErrorCreatingDir, err)
	}
	return repo, nil
}

// QueryForResourceVisibility checks the relationship between requestor (user)
// and the resource owner to formulate a database query to determine whether a
// resource is visible to the user
func QueryForResourceVisibility(tx, q *gorm.DB, owner *string, user *users.User) *gorm.DB {
	// Check resource visibility
	publicOnly := false
	// if owner is specified
	if owner != nil {
		if user == nil {
			// if no user is specified, only public resources are visible
			publicOnly = true
		} else {
			// check if owner is an org
			org, _ := users.ByOrganizationName(tx, *owner, false)
			if org != nil {
				// if owner is an org, check if requestor is part of that org
				ok, _ := globals.Permissions.IsAuthorized(*user.Username, *org.Name,
					permissions.Read)
				if !ok {
					// if requestor is not part of that org, only public resources will
					// be returned
					publicOnly = true
				}
			} else if *user.Username != *owner {
				// if owner is not an org then this is another user's resource
				// TODO check permissions when resource sharing is implemented
				// but for now assume user can only acccess other user's public
				// resources
				publicOnly = true
			}
		}
		if !publicOnly {
			q = q.Where("owner = ?", *owner)
		} else {
			q = q.Where("owner = ? AND private = ?", *owner, 0)
		}
	} else {
		// if owner is not specified, the query should only return resources that
		// are either 1) public or 2) private resources that requestor has read
		// permissions
		if user == nil {
			q = q.Where("private = ?", 0)
		} else {
			userGroups := globals.Permissions.GetGroupsForUser(*user.Username)
			userGroups = append(userGroups, *user.Username)
			q = q.Where("private = ? OR owner IN (?)", 0, userGroups)
		}
	}
	return q
}

// MoveResource will move a resource's on-disk location from sourceOwner to destOwner.
func MoveResource(resource Resource, destOwner string) *ign.ErrMsg {
	searchStr := "/" + *resource.GetOwner() + "/"
	replaceStr := "/" + destOwner + "/"
	newLocation := strings.Replace(*resource.GetLocation(), searchStr, replaceStr, 1)

	if newLocation == *resource.GetLocation() {
		extra := fmt.Sprintf("Source and destination owners are identical")
		return ign.NewErrorMessageWithArgs(ign.ErrorUnauthorized, nil, []string{extra})
	}

	// Move resource on disk
	if err := os.Rename(*resource.GetLocation(), newLocation); err != nil {
		return ign.NewErrorMessageWithBase(ign.ErrorCreatingDir, err)
	}

	// Set the new location and owner
	resource.SetLocation(newLocation)
	resource.SetOwner(destOwner)

	return nil
}
