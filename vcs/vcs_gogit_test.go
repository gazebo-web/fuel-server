package vcs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// newTestRepo creates a GoGitVCS repository in a temporary folder with the
// given files committed.
func newTestRepo(t *testing.T, files map[string]string) *GoGitVCS {
	dir := filepath.Join(t.TempDir(), "repo")
	writeTestFiles(t, dir, files)
	repo := GoGitVCS{}.NewRepo(dir).(*GoGitVCS)
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

// replaceTestFiles commits the given files as a new revision of repo.
func replaceTestFiles(t *testing.T, repo *GoGitVCS, files map[string]string) {
	upload := t.TempDir()
	writeTestFiles(t, upload, files)
	require.NoError(t, repo.ReplaceFiles(context.Background(), upload, "user"))
}

// runGit runs a git command in the given repository and returns its output.
func runGit(t *testing.T, repo *GoGitVCS, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", repo.Path}, args...)...).Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

// repack moves all the objects of repo to a single pack, computing deltas
// again, and opens the repository again.
func repack(t *testing.T, repo *GoGitVCS) *GoGitVCS {
	runGit(t, repo, "repack", "-a", "-d", "-f")
	require.True(t, strings.HasPrefix(runGit(t, repo, "count-objects"), "0 objects"),
		"loose objects left after repack")
	return GoGitVCS{}.NewRepo(repo.Path).(*GoGitVCS)
}

// randomContent returns size random bytes, which git cannot compress.
func randomContent(t *testing.T, size int) string {
	bs := make([]byte, size)
	_, err := rand.Read(bs)
	require.NoError(t, err)
	return string(bs)
}

// readFile reads a file of repo with GetFile. It returns the SHA-256 of its
// content, its size and the bytes allocated while reading it.
func readFile(t *testing.T, repo VCS, rev, path string) ([sha256.Size]byte, int64, uint64) {
	var sum [sha256.Size]byte
	h := sha256.New()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	content, err := repo.GetFile(context.Background(), rev, path)
	require.NoError(t, err)
	n, err := io.Copy(h, content)
	require.NoError(t, err)
	require.NoError(t, content.Close())

	runtime.ReadMemStats(&after)
	h.Sum(sum[:0])
	return sum, n, after.TotalAlloc - before.TotalAlloc
}

// assertFile checks that the file path of repo, at revision rev, has the
// given content.
func assertFile(t *testing.T, repo VCS, rev, path, want string) {
	sum, n, _ := readFile(t, repo, rev, path)
	assert.Equal(t, int64(len(want)), n)
	assert.Equal(t, sha256.Sum256([]byte(want)), sum, "content differs")
}

func TestReplaceFilesWithoutChanges(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{"model.sdf": "<sdf/>", "meshes/mesh.dae": "mesh"}
	repo := newTestRepo(t, files)

	// Uploading the same files creates a new revision.
	replaceTestFiles(t, repo, files)

	count, err := repo.RevisionCount(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestReplaceFilesWithoutFiles(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t, map[string]string{"model.sdf": "<sdf/>"})

	// Uploading no files creates a new revision, which has no files.
	replaceTestFiles(t, repo, map[string]string{})

	count, err := repo.RevisionCount(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestGetFile(t *testing.T) {
	repo := newTestRepo(t, map[string]string{"model.sdf": "<sdf/>"})
	replaceTestFiles(t, repo, map[string]string{"model.sdf": "<sdf version='1.9'/>"})

	assertFile(t, repo, "", "model.sdf", "<sdf version='1.9'/>")
	assertFile(t, repo, "HEAD~1", "model.sdf", "<sdf/>")
	_, err := repo.GetFile(context.Background(), "", "missing.sdf")
	assert.Error(t, err)

	repo = repack(t, repo)
	assertFile(t, repo, "", "model.sdf", "<sdf version='1.9'/>")
	assertFile(t, repo, "HEAD~1", "model.sdf", "<sdf/>")
}

// TestGetFileLargeFile checks that the files larger than
// largeObjectThreshold are not loaded in memory, whether git stores them as
// loose objects or in a pack.
func TestGetFileLargeFile(t *testing.T) {
	const size = 8 << 20
	const path = "meshes/mesh.dae"
	old := randomContent(t, size)
	mesh := old[:size/2] + "changed" + old[size/2+len("changed"):]
	repo := newTestRepo(t, map[string]string{path: old})
	replaceTestFiles(t, repo, map[string]string{path: mesh})
	revisions := map[string]string{"HEAD": mesh, "HEAD~1": old}

	assertNotInMemory := func(t *testing.T, repo *GoGitVCS, rev string) {
		// Open the repository again, so that go-git's object cache is empty.
		_, _, alloc := readFile(t, GoGitVCS{}.NewRepo(repo.Path), rev, path)
		assert.Less(t, alloc, uint64(size/16), "bytes allocated to read revision %s", rev)
	}
	t.Run("loose", func(t *testing.T) {
		for rev, want := range revisions {
			assertFile(t, repo, rev, path, want)
			assertNotInMemory(t, repo, rev)
		}
	})
	t.Run("packed", func(t *testing.T) {
		repo := repack(t, repo)
		deltas := 0
		for rev, want := range revisions {
			assertFile(t, repo, rev, path, want)
			hash := runGit(t, repo, "rev-parse", rev+":"+path)
			if runGitInput(t, repo, hash, "cat-file", "--batch-check=%(deltabase)") != plumbing.ZeroHash.String() {
				// go-git v5 loads the base of a delta in memory, because
				// FSObject.Reader reads the pack without a filesystem.
				deltas++
				continue
			}
			assertNotInMemory(t, repo, rev)
		}
		require.Equal(t, 1, deltas, "git did not store one revision as a delta")
	})
}

// runGitInput runs a git command in the given repository with the given
// input, and returns its output.
func runGitInput(t *testing.T, repo *GoGitVCS, input string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", repo.Path}, args...)...)
	cmd.Stdin = strings.NewReader(input + "\n")
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}
