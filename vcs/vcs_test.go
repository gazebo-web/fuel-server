package vcs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitRepoUsesMainBranch checks that new repositories use the "main"
// branch even when git is configured with a different default branch.
func TestInitRepoUsesMainBranch(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "init.defaultBranch")
	t.Setenv("GIT_CONFIG_VALUE_0", "master")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.config"), []byte("<model/>"), 0644))

	ctx := context.Background()
	repo := GoGitVCS{}.NewRepo(dir)
	require.NoError(t, repo.InitRepo(ctx))

	branch, err := exec.Command("git", "-C", dir, "symbolic-ref", "--short", "HEAD").Output()
	require.NoError(t, err)
	assert.Equal(t, "main", strings.TrimSpace(string(branch)))

	count, err := repo.RevisionCount(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	zipPath := filepath.Join(t.TempDir(), "model.zip")
	_, err = repo.Zip(ctx, "", zipPath)
	assert.NoError(t, err)
}

// TestMasterBranchRepository checks that repositories created on the
// "master" branch, before "main" was the default, can still be read.
func TestMasterBranchRepository(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.config"), []byte("<model/>"), 0644))
	for _, args := range [][]string{
		{"-c", "init.defaultBranch=master", "init"},
		{"add", "-A"},
		{"commit", "-m", "Created repository"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	ctx := context.Background()
	repo := GoGitVCS{}.NewRepo(dir)

	count, err := repo.RevisionCount(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	contents, err := repo.GetFile(ctx, "tip", "model.config")
	require.NoError(t, err)
	assert.Equal(t, "<model/>", string(*contents))

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

// TestGitTagError checks that a failed tag is reported to the caller.
func TestGitTagError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.config"), []byte("<model/>"), 0644))

	ctx := context.Background()
	repo := GitVCS{}.NewRepo(dir)
	require.NoError(t, repo.InitRepo(ctx))
	require.NoError(t, repo.Tag(ctx, "my-uuid"))
	assert.Error(t, repo.Tag(ctx, "my-uuid"))
}
