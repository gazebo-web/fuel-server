package main

// Tests for upload and stored file handling.

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/gz-go/v7"
	gztest "github.com/gazebo-web/gz-go/v7/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateUploadPath checks which uploaded file paths are accepted.
func TestValidateUploadPath(t *testing.T) {
	valid := []string{"model.config", "meshes/base.dae", "/outer/model.sdf",
		"thumbnails/1.png", "a..b/c..d", "./model.sdf", "...", "dir\\..\\file"}
	for _, fn := range valid {
		assert.NoError(t, validateUploadPath(fn), "Path should be valid: %s", fn)
	}
	invalid := []string{"..", "../model.sdf", "meshes/../../model.sdf", "/../model.sdf",
		"a/b/..", "model\x00.sdf"}
	for _, fn := range invalid {
		assert.Error(t, validateUploadPath(fn), "Path should be invalid: %s", fn)
	}
}

// TestModelUploadPathTraversal checks that uploaded file names cannot write
// files outside of the folder that holds the uploaded model.
func TestModelUploadPathTraversal(t *testing.T) {
	// General test setup
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")
	testUser := createUser(t)
	defer removeUser(testUser, t)

	// The escaped files would land inside the resource dir (create) or the
	// system temp dir (update), where the test looks for them afterwards.
	escapedName := "escaped-" + testUser + ".txt"
	assertNotEscaped := func(t *testing.T) {
		_, err := os.Stat(filepath.Join(os.TempDir(), escapedName))
		assert.True(t, os.IsNotExist(err), "File escaped into the temp dir")
		err = filepath.Walk(globals.ResourceDir, func(p string, info os.FileInfo, err error) error {
			if err == nil && info.Name() == escapedName {
				return fmt.Errorf("file escaped to %s", p)
			}
			return nil
		})
		assert.NoError(t, err)
	}

	params := map[string]string{
		"name":       "traversal",
		"tags":       "",
		"license":    "1",
		"permission": "0",
	}
	badNames := []string{
		"../" + escapedName,
		"meshes/../../" + escapedName,
		"/../" + escapedName,
		"../../../" + escapedName,
	}
	for _, name := range badNames {
		t.Run("create "+name, func(t *testing.T) {
			files := []gztest.FileDesc{
				{Path: "model.config", Contents: constModelConfigFileContents},
				{Path: name, Contents: "escaped"},
			}
			code, bslice := postWithArgs(t, "/1.0/models", &jwt, params, files)
			require.Equal(t, http.StatusBadRequest, code, string(*bslice))
			gztest.AssertBackendErrorCode(t.Name(), bslice, gz.ErrorFormInvalidValue, t)
			assertNotEscaped(t)
		})
	}

	// Files within a single outer folder have that folder removed.
	t.Run("create with outer folder", func(t *testing.T) {
		files := []gztest.FileDesc{
			{Path: "outer/model.config", Contents: constModelConfigFileContents},
			{Path: "outer/../../" + escapedName, Contents: "escaped"},
		}
		code, bslice := postWithArgs(t, "/1.0/models", &jwt, params, files)
		require.Equal(t, http.StatusBadRequest, code, string(*bslice))
		assertNotEscaped(t)
	})

	// Updating a model's files goes through a temporary folder.
	createTestModelWithOwner(t, &jwt, "traversal", testUser, false)
	uri := "/1.0/" + testUser + "/models/traversal"
	for _, name := range badNames {
		t.Run("update "+name, func(t *testing.T) {
			files := []gztest.FileDesc{
				{Path: "model.config", Contents: constModelConfigFileContents},
				{Path: name, Contents: "escaped"},
			}
			code, bslice, _ := gztest.SendMultipartMethod(t.Name(), t, "PATCH", uri, &jwt, map[string]string{}, files)
			require.Equal(t, http.StatusBadRequest, code, string(*bslice))
			gztest.AssertBackendErrorCode(t.Name(), bslice, gz.ErrorFormInvalidValue, t)
			assertNotEscaped(t)
		})
	}
}
