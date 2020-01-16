package users

import (
	"bitbucket.org/ignitionrobotics/ign-fuelserver/globals"
	"bitbucket.org/ignitionrobotics/ign-go"
	"context"
	"github.com/satori/go.uuid"
	"os"
	"path"
)

// NewUUID creates a new valid UUID for for a resource type (eg. "models").
// Returns the generated UUID and a resource path using that UUID. The resource
// path will exist within the user folder.
func NewUUID(owner string, resType string) (uuidStr, resPath string, err error) {
	// This loop should execute once
	for {
		// Create a uuid for the model
		uuidStr, err = newUUID()
		if err != nil {
			return "", "", err
		}
		// Test the tentative path to the new resource
		resPath = GetResourcePath(owner, uuidStr, resType)
		// Break when the directory doesn't exist.
		if _, err = os.Stat(resPath); err != nil {
			break
		}
	}
	return uuidStr, resPath, nil
}

// GetResourcePath returns a os path to a resource (eg. user/models/uuid)
func GetResourcePath(owner, uuidStr, resType string) string {
	return path.Join(globals.ResourceDir, owner, resType, uuidStr)
}

// newUUID returns a new UUID
func newUUID() (uuidStr string, err error) {

	// Create a uuid for the model
	uuidStr = uuid.NewV4().String()

	return uuidStr, nil
}

// CreateOwnerFolder creates a folder for the given owner. The folder will
// have models and worls subfolders.
// Fails if already exists.
// Returns the path pointing to the created owner's folder (eg. /fuel/owner)
func CreateOwnerFolder(ctx context.Context, owner string, failIfDirExist bool) (*string, *ign.ErrMsg) {
	dirpath := path.Join(globals.ResourceDir, owner)
	ign.LoggerFromContext(ctx).Info("Request for creating owner folder [" + dirpath + "]")

	// Sanity check: The directory shouldn't exist
	var userDirExist bool
	if _, err := os.Stat(dirpath); err == nil {
		userDirExist = true
		if failIfDirExist {
			return nil, ign.NewErrorMessage(ign.ErrorResourceExists)
		}
	}

	if !userDirExist {
		// Create the directory to store the user
		if err := os.MkdirAll(dirpath, 0711); err != nil {
			return nil, ign.NewErrorMessage(ign.ErrorCreatingDir)
		}

		// Create the directory to store the models
		dirModels := path.Join(dirpath, "models")
		if err := os.Mkdir(dirModels, 0711); err != nil {
			return nil, ign.NewErrorMessageWithBase(ign.ErrorCreatingDir, err)
		}

		// Create the directory to store the worlds
		dirWorlds := path.Join(dirpath, "worlds")
		if err := os.Mkdir(dirWorlds, 0711); err != nil {
			return nil, ign.NewErrorMessageWithBase(ign.ErrorCreatingDir, err)
		}

		// Create the directory to store collections
		dirCols := path.Join(dirpath, "collections")
		if err := os.Mkdir(dirCols, 0711); err != nil {
			return nil, ign.NewErrorMessageWithBase(ign.ErrorCreatingDir, err)
		}
	}
	return &dirpath, nil
}
