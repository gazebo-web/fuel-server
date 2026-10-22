package vcs

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

// newTestRepo creates a GoGitVCS repository in a temporary folder with the
// given files committed.
func newTestRepo(t *testing.T, files map[string]string) VCS {
	dir := filepath.Join(t.TempDir(), "repo")
	writeTestFiles(t, dir, files)
	repo := GoGitVCS{}.NewRepo(dir)
	require.NoError(t, repo.InitRepo(context.Background()))
	return repo
}

// writeTestFiles writes the given files, keyed by their relative path, into
// dir.
func writeTestFiles(t *testing.T, dir string, files map[string]string) {
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	}
}

func TestReplaceFilesWithoutChanges(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{"model.sdf": "<sdf/>", "meshes/mesh.dae": "mesh"}
	repo := newTestRepo(t, files)

	// Uploading the same files creates a new revision.
	upload := t.TempDir()
	writeTestFiles(t, upload, files)
	require.NoError(t, repo.ReplaceFiles(ctx, upload, "user"))

	count, err := repo.RevisionCount(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestReplaceFilesWithoutFiles(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t, map[string]string{"model.sdf": "<sdf/>"})

	// Uploading no files creates a new revision, which has no files.
	require.NoError(t, repo.ReplaceFiles(ctx, t.TempDir(), "user"))

	count, err := repo.RevisionCount(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}
