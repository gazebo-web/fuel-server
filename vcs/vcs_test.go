package vcs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitRepoUsesMasterBranch checks that new repositories use the "master"
// branch even when git is configured with a different default branch.
func TestInitRepoUsesMasterBranch(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "init.defaultBranch")
	t.Setenv("GIT_CONFIG_VALUE_0", "main")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.config"), []byte("<model/>"), 0644))

	ctx := context.Background()
	repo := GoGitVCS{}.NewRepo(dir)
	require.NoError(t, repo.InitRepo(ctx))

	count, err := repo.RevisionCount(ctx, "master")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	zipPath := filepath.Join(t.TempDir(), "model.zip")
	_, err = repo.Zip(ctx, "", zipPath)
	assert.NoError(t, err)
}

// TestGitTag checks that tags are created in the repository itself.
func TestGitTag(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.config"), []byte("<model/>"), 0644))

	ctx := context.Background()
	repo := GitVCS{}.NewRepo(dir)
	require.NoError(t, repo.InitRepo(ctx))
	require.NoError(t, repo.Tag(ctx, "my-uuid"))

	count, err := repo.RevisionCount(ctx, "my-uuid")
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
