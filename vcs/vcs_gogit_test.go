package vcs

import (
	"context"
	"crypto/rand"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRepo creates a git repository with the given files.
func newTestRepo(t *testing.T, files map[string][]byte) *GoGitVCS {
	dir := t.TempDir()
	writeFiles(t, dir, files)
	repo := GoGitVCS{}.NewRepo(dir).(*GoGitVCS)
	require.NoError(t, repo.InitRepo(context.Background()))
	return repo
}

func writeFiles(t *testing.T, dir string, files map[string][]byte) {
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, content, 0644))
	}
}

func randomBytes(t *testing.T, size int) []byte {
	bs := make([]byte, size)
	_, err := rand.Read(bs)
	require.NoError(t, err)
	return bs
}

func readFile(t *testing.T, repo VCS, rev, path string) []byte {
	f, err := repo.OpenFile(context.Background(), rev, path)
	require.NoError(t, err)
	c, err := f.Content()
	require.NoError(t, err)
	defer c.Close()
	bs, err := io.ReadAll(c)
	require.NoError(t, err)
	assert.Equal(t, f.Size, int64(len(bs)))
	return bs
}

func TestOpenFile(t *testing.T) {
	small := []byte("<?xml version=\"1.0\"?>\n<model/>\n")
	large := randomBytes(t, maxInMemoryFileSize+1)
	repo := newTestRepo(t, map[string][]byte{
		"model.config":     small,
		"meshes/large.dae": large,
	})
	ctx := context.Background()

	check := func(t *testing.T) {
		for _, r := range []VCS{repo, GitVCS{}.NewRepo(repo.Path)} {
			assert.Equal(t, small, readFile(t, r, "", "model.config"))
			assert.Equal(t, large, readFile(t, r, "tip", "meshes/large.dae"))

			// The hash is the blob hash.
			f, err := r.OpenFile(ctx, "", "meshes/large.dae")
			require.NoError(t, err)
			out, err := exec.Command("git", "-C", repo.Path, "rev-parse", "HEAD:meshes/large.dae").Output()
			require.NoError(t, err)
			assert.Equal(t, string(out[:len(out)-1]), f.Hash)

			for _, path := range []string{"meshes", ".", "missing.sdf", "meshes/missing.dae"} {
				_, err = r.OpenFile(ctx, "", path)
				assert.Error(t, err, path)
			}
		}
	}

	t.Run("loose objects", check)

	out, err := exec.Command("git", "-C", repo.Path, "repack", "-a", "-d").CombinedOutput()
	require.NoError(t, err, string(out))
	// Reopen the repo so that go-git sees the new packfile.
	repo = GoGitVCS{}.NewRepo(repo.Path).(*GoGitVCS)
	t.Run("packed objects", check)
}

func TestOpenFileRevisions(t *testing.T) {
	v1 := randomBytes(t, maxInMemoryFileSize+1)
	repo := newTestRepo(t, map[string][]byte{"large.dae": v1, "model.sdf": []byte("v1")})
	ctx := context.Background()

	// ReplaceFiles commits with go-git, which writes loose objects.
	v2 := randomBytes(t, maxInMemoryFileSize+2)
	folder := t.TempDir()
	writeFiles(t, folder, map[string][]byte{"large.dae": v2, "model.sdf": []byte("v2")})
	require.NoError(t, repo.ReplaceFiles(ctx, folder, "owner"))

	assert.Equal(t, v2, readFile(t, repo, "", "large.dae"))
	assert.Equal(t, []byte("v2"), readFile(t, repo, "", "model.sdf"))
	assert.Equal(t, v1, readFile(t, repo, "HEAD~1", "large.dae"))
	assert.Equal(t, []byte("v1"), readFile(t, repo, "HEAD~1", "model.sdf"))
}

func TestOpenFileCanceled(t *testing.T) {
	repo := newTestRepo(t, map[string][]byte{"large.dae": randomBytes(t, 8*maxInMemoryFileSize)})
	ctx, cancel := context.WithCancel(context.Background())
	f, err := repo.OpenFile(ctx, "", "large.dae")
	require.NoError(t, err)
	c, err := f.Content()
	require.NoError(t, err)
	defer c.Close()

	_, err = io.ReadFull(c, make([]byte, 1024))
	require.NoError(t, err)
	// The request went away: git is killed, and reading fails.
	cancel()
	_, err = io.Copy(io.Discard, c)
	assert.Error(t, err)
}

func TestOpenFileWaitsForGit(t *testing.T) {
	data := randomBytes(t, 2*maxInMemoryFileSize)
	repo := newTestRepo(t, map[string][]byte{"large.dae": data})
	f, err := repo.OpenFile(context.Background(), "", "large.dae")
	require.NoError(t, err)
	c, err := f.Content()
	require.NoError(t, err)
	git, ok := c.r.(*gitBlobReader)
	require.True(t, ok, "the file is not read with git")

	bs, err := io.ReadAll(c)
	require.NoError(t, err)
	assert.Equal(t, data, bs)
	// Reading the content checked the git exit status.
	require.NotNil(t, git.cmd.ProcessState)
	assert.True(t, git.cmd.ProcessState.Success(), git.cmd.ProcessState.String())
	require.NoError(t, c.Close())
}

func TestOpenFileGitFailsAtEnd(t *testing.T) {
	data := randomBytes(t, 2*maxInMemoryFileSize)
	repo := newTestRepo(t, map[string][]byte{"large.dae": data})
	f, err := repo.OpenFile(context.Background(), "", "large.dae")
	require.NoError(t, err)

	// A git that writes the whole content, and then fails.
	bin := t.TempDir()
	content := filepath.Join(bin, "content")
	require.NoError(t, os.WriteFile(content, data, 0644))
	script := "#!/bin/sh\ncat " + content + "\necho failed >&2\nexit 1\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	c, err := f.Content()
	require.NoError(t, err)
	defer c.Close()
	bs, err := io.ReadAll(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 1")
	assert.Contains(t, err.Error(), "failed")
	assert.Less(t, len(bs), len(data))
}

func TestOpenFileReadError(t *testing.T) {
	repo := newTestRepo(t, map[string][]byte{"large.dae": randomBytes(t, 2*maxInMemoryFileSize)})
	f, err := repo.OpenFile(context.Background(), "", "large.dae")
	require.NoError(t, err)
	// The object goes away after the file was opened.
	require.NoError(t, os.Remove(filepath.Join(repo.Path, ".git", "objects", f.Hash[:2], f.Hash[2:])))
	_, err = f.Content()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status")
}

func TestOpenFileClosesObjectFiles(t *testing.T) {
	if _, err := os.ReadDir("/proc/self/fd"); err != nil {
		t.Skip("the open files cannot be counted")
	}
	countFDs := func() int {
		fds, err := os.ReadDir("/proc/self/fd")
		require.NoError(t, err)
		return len(fds)
	}
	repo := newTestRepo(t, map[string][]byte{"model.sdf": []byte("<sdf/>")})
	// Files that are not closed are only closed by their finalizer.
	defer debug.SetGCPercent(debug.SetGCPercent(-1))

	check := func(t *testing.T) {
		before := countFDs()
		for i := 0; i < 100; i++ {
			_, err := repo.OpenFile(context.Background(), "", "model.sdf")
			require.NoError(t, err)
		}
		assert.Less(t, countFDs()-before, 10)
	}

	t.Run("loose objects", check)

	out, err := exec.Command("git", "-C", repo.Path, "repack", "-a", "-d").CombinedOutput()
	require.NoError(t, err, string(out))
	repo = GoGitVCS{}.NewRepo(repo.Path).(*GoGitVCS)
	t.Run("packed objects", check)
}

// totalAlloc returns the bytes allocated while running fn.
func totalAlloc(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func TestLargeFileIsNotLoadedInMemory(t *testing.T) {
	const size = 32 << 20
	repo := newTestRepo(t, map[string][]byte{"large.dae": randomBytes(t, size)})
	ctx := context.Background()

	alloc := totalAlloc(func() {
		f, err := repo.OpenFile(ctx, "", "large.dae")
		require.NoError(t, err)
		c, err := f.Content()
		require.NoError(t, err)
		defer c.Close()
		n, err := io.Copy(io.Discard, c)
		require.NoError(t, err)
		require.Equal(t, int64(size), n)
	})
	assert.Less(t, alloc, uint64(size/8), "OpenFile allocated %d bytes", alloc)

	alloc = totalAlloc(func() {
		require.NoError(t, repo.Walk(ctx, "", true, func(path, parent string, isDir bool) error {
			return nil
		}))
	})
	assert.Less(t, alloc, uint64(size/8), "Walk allocated %d bytes", alloc)
}

func TestWalk(t *testing.T) {
	repo := newTestRepo(t, map[string][]byte{
		"model.config":       []byte("config"),
		"meshes/a.dae":       []byte("a"),
		"meshes/sub/b.dae":   []byte("b"),
		"materials/c.png":    []byte("c"),
		"thumbnails/1.png":   []byte("1"),
		"meshes/sub/c/d.stl": []byte("d"),
	})
	var files, folders []string
	err := repo.Walk(context.Background(), "", true, func(path, parent string, isDir bool) error {
		assert.Equal(t, filepath.Dir(path), parent)
		if isDir {
			folders = append(folders, path)
		} else {
			files = append(files, path)
		}
		return nil
	})
	require.NoError(t, err)
	sort.Strings(files)
	sort.Strings(folders)
	assert.Equal(t, []string{"/materials/c.png", "/meshes/a.dae", "/meshes/sub/b.dae",
		"/meshes/sub/c/d.stl", "/model.config", "/thumbnails/1.png"}, files)
	assert.Equal(t, []string{"/materials", "/meshes", "/meshes/sub", "/meshes/sub/c",
		"/thumbnails"}, folders)
}
