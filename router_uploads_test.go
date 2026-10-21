package main

// Tests for upload and stored file handling.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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

// zipFileNames downloads a zip file and returns the names of its entries.
func zipFileNames(t *testing.T, route string, jwt *string) map[string]bool {
	reqArgs := gztest.RequestArgs{Method: "GET", Route: route, SignedToken: jwt}
	resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, http.StatusOK, ctZip, t)
	bslice := resp.BodyAsBytes
	zipReader, err := zip.NewReader(bytes.NewReader(*bslice), int64(len(*bslice)))
	require.NoError(t, err)
	names := map[string]bool{}
	for _, f := range zipReader.File {
		names[f.Name] = true
	}
	return names
}

// TestResourceVersionZipsAfterUpdate checks that downloading an older
// version of a model or world after an update returns that version's files.
func TestResourceVersionZipsAfterUpdate(t *testing.T) {
	// General test setup
	setup()
	myJWT := os.Getenv("IGN_TEST_JWT")
	testUser := createUser(t)
	defer removeUser(testUser, t)

	modelV1Files := map[string]bool{"thumbnails/": true, "thumbnails/model.sdf": true, "model.config": true}
	modelV2Files := []gztest.FileDesc{{Path: "model.config", Contents: constModelConfigFileContents}, {Path: "model2.sdf", Contents: constModelSDFFileContents}}
	worldV1Files := map[string]bool{"thumbnails/": true, "thumbnails/world.sdf": true, "world.world": true}
	worldV2Files := []gztest.FileDesc{{Path: "world2.world", Contents: constWorldMainFileContents}}

	tests := []struct {
		desc string
		kind string
		// whether tip is downloaded before the update, which caches its zip.
		downloadTip bool
		url         func(owner, name, version string) string
		v1Files     map[string]bool
		v2Files     []gztest.FileDesc
	}{
		{"model", "models", false, modelURL, modelV1Files, modelV2Files},
		{"model with tip downloaded", "models", true, modelURL, modelV1Files, modelV2Files},
		{"world", "worlds", false, worldURL, worldV1Files, worldV2Files},
		{"world with tip downloaded", "worlds", true, worldURL, worldV1Files, worldV2Files},
	}
	for i, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			name := "versioned" + strconv.Itoa(i)
			if test.kind == "models" {
				createTestModelWithOwner(t, &myJWT, name, testUser, false)
			} else {
				createTestWorldWithOwner(t, &myJWT, name, testUser, false)
			}
			assert.Equal(t, test.v1Files, zipFileNames(t, test.url(testUser, name, "1")+".zip", &myJWT))
			if test.downloadTip {
				assert.Equal(t, test.v1Files, zipFileNames(t, test.url(testUser, name, "tip")+".zip", &myJWT))
			}

			// Update the files, which creates version 2.
			updateURI := "/1.0/" + testUser + "/" + test.kind + "/" + name
			code, bslice, _ := gztest.SendMultipartMethod(t.Name(), t, "PATCH", updateURI, &myJWT, map[string]string{}, test.v2Files)
			require.Equal(t, http.StatusOK, code, string(*bslice))

			v2Files := map[string]bool{}
			for _, f := range test.v2Files {
				v2Files[f.Path] = true
			}
			assert.Equal(t, test.v1Files, zipFileNames(t, test.url(testUser, name, "1")+".zip", &myJWT), "version 1 changed")
			assert.Equal(t, v2Files, zipFileNames(t, test.url(testUser, name, "2")+".zip", &myJWT))
			assert.Equal(t, v2Files, zipFileNames(t, test.url(testUser, name, "tip")+".zip", &myJWT), "tip is not version 2")
		})
	}
}

// TestUploadSizeLimit checks that request bodies larger than the configured
// limit are rejected.
func TestUploadSizeLimit(t *testing.T) {
	// General test setup
	setup()

	jwt := os.Getenv("IGN_TEST_JWT")
	testUser := createUser(t)
	defer removeUser(testUser, t)

	defer func(limit int64) { globals.MaxUploadBytes = limit }(globals.MaxUploadBytes)
	globals.MaxUploadBytes = 16 * 1024

	params := map[string]string{"name": "sizeLimit", "tags": "", "license": "1", "permission": "0"}
	largeFiles := []gztest.FileDesc{
		{Path: "model.config", Contents: constModelConfigFileContents},
		{Path: "large.dae", Contents: string(bytes.Repeat([]byte("a"), 32*1024))},
	}
	code, bslice := postWithArgs(t, "/1.0/models", &jwt, params, largeFiles)
	assert.Equal(t, http.StatusRequestEntityTooLarge, code, string(*bslice))

	smallFiles := []gztest.FileDesc{
		{Path: "model.config", Contents: constModelConfigFileContents},
	}
	code, bslice = postWithArgs(t, "/1.0/models", &jwt, params, smallFiles)
	assert.Equal(t, http.StatusOK, code, string(*bslice))
}

// TestDeniedPrivacyUpdateKeepsFiles checks that an update that is denied
// because the user cannot change the privacy setting does not change the
// files of a model or world.
func TestDeniedPrivacyUpdateKeepsFiles(t *testing.T) {
	// General test setup
	setup()

	myJWT := os.Getenv("IGN_TEST_JWT")
	testUser := createUser(t)
	defer removeUser(testUser, t)
	testOrg := createOrganization(t)
	defer removeOrganization(testOrg, t)

	// An org member can edit org resources, but not their privacy.
	memberJWT := createValidJWTForIdentity("another-user-2", t)
	member := createUserWithJWT(memberJWT, t)
	defer removeUserWithJWT(member, memberJWT, t)
	addUserToOrg(member, "member", testOrg, t)

	createTestModelWithOwner(t, &myJWT, "privacy", testOrg, true)
	createTestWorldWithOwner(t, &myJWT, "privacy", testOrg, true)

	tests := []struct {
		kind  string
		url   func(owner, name, version string) string
		files map[string]bool
	}{
		{"models", modelURL, map[string]bool{"thumbnails/": true, "thumbnails/model.sdf": true, "model.config": true}},
		{"worlds", worldURL, map[string]bool{"thumbnails/": true, "thumbnails/world.sdf": true, "world.world": true}},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			uri := "/1.0/" + testOrg + "/" + test.kind + "/privacy"
			params := map[string]string{"private": "false"}
			files := []gztest.FileDesc{{Path: "other.config", Contents: constModelConfigFileContents}}
			code, bslice, _ := gztest.SendMultipartMethod(t.Name(), t, "PATCH", uri, &memberJWT, params, files)
			require.Equal(t, http.StatusUnauthorized, code, string(*bslice))

			// The files and the version did not change.
			assert.Equal(t, test.files, zipFileNames(t, test.url(testOrg, "privacy", "tip")+".zip", &myJWT))
			reqArgs := gztest.RequestArgs{Method: "GET", Route: uri, SignedToken: &myJWT}
			resp := gztest.AssertRouteMultipleArgsStruct(reqArgs, http.StatusOK, ctJSON, t)
			ensureIgnResourceVersionHeader(resp.RespRecorder, 1, t)
		})
	}
}
