package vcs

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// largeFileRepo creates a repository with a random file larger than
// largeObjectThreshold, which go-git reads from disk as it is used.
func largeFileRepo(t *testing.T) (*GoGitVCS, string) {
	mesh := randomContent(t, 2*largeObjectThreshold+123)
	return newTestRepo(t, map[string]string{"meshes/mesh.dae": mesh}), mesh
}

// TestBlobReaderSeek checks that a blobReader reads like a bytes.Reader of
// the same content, whatever the seeks.
func TestBlobReaderSeek(t *testing.T) {
	repo, mesh := largeFileRepo(t)
	size := int64(len(mesh))
	content, err := repo.GetFile(context.Background(), "", "meshes/mesh.dae")
	require.NoError(t, err)
	defer content.Close()
	want := bytes.NewReader([]byte(mesh))

	steps := []struct {
		offset int64
		whence int
		read   int
	}{
		{0, io.SeekCurrent, 512},
		{0, io.SeekStart, 1000},
		{0, io.SeekEnd, 0},
		{-16, io.SeekEnd, 16},
		{size / 2, io.SeekStart, 100},
		{10, io.SeekStart, 10},
		{5, io.SeekCurrent, 70000},
		{-1000, io.SeekCurrent, 100},
		{size - 50, io.SeekStart, 100},
		{size + 10, io.SeekStart, 10},
	}
	for _, s := range steps {
		desc := fmt.Sprintf("Seek(%d, %d) and read %d bytes", s.offset, s.whence, s.read)
		wantPos, _ := want.Seek(s.offset, s.whence)
		pos, err := content.Seek(s.offset, s.whence)
		require.NoError(t, err, desc)
		require.Equal(t, wantPos, pos, desc)

		wantBuf := make([]byte, s.read)
		wantN, wantErr := io.ReadFull(want, wantBuf)
		buf := make([]byte, s.read)
		n, err := io.ReadFull(content, buf)
		require.Equal(t, wantErr, err, desc)
		require.Equal(t, wantN, n, desc)
		require.True(t, bytes.Equal(wantBuf[:wantN], buf[:n]), "%s: content differs", desc)
	}

	_, err = content.Seek(-1, io.SeekStart)
	assert.Error(t, err)
	_, err = content.Seek(0, 42)
	assert.Error(t, err)

	require.NoError(t, content.Close())
	_, err = content.Read(make([]byte, 1))
	assert.ErrorIs(t, err, os.ErrClosed)
	assert.ErrorIs(t, content.Close(), os.ErrClosed)
}

// TestBlobReaderServeContent checks the content of a large file sent with
// http.ServeContent, which seeks to detect the content type and the size,
// and to send ranges.
func TestBlobReaderServeContent(t *testing.T) {
	repo, mesh := largeFileRepo(t)

	serve := func(rangeHeader string) *httptest.ResponseRecorder {
		content, err := repo.GetFile(context.Background(), "", "meshes/mesh.dae")
		require.NoError(t, err)
		defer content.Close()
		r := httptest.NewRequest("GET", "/mesh", nil)
		if rangeHeader != "" {
			r.Header.Set("Range", rangeHeader)
		}
		w := httptest.NewRecorder()
		// Without an extension, ServeContent reads the beginning of the file
		// to detect its content type.
		http.ServeContent(w, r, "mesh", time.Time{}, content)
		return w
	}

	w := serve("")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, strconv.Itoa(len(mesh)), w.Header().Get("Content-Length"))
	assert.True(t, w.Body.String() == mesh, "content differs")

	w = serve("bytes=1048576-1048675")
	assert.Equal(t, http.StatusPartialContent, w.Code)
	assert.Equal(t, mesh[1048576:1048676], w.Body.String())

	w = serve("bytes=-100")
	assert.Equal(t, http.StatusPartialContent, w.Code)
	assert.Equal(t, mesh[len(mesh)-100:], w.Body.String())
}

// TestBlobReaderTruncatedObject checks that reading a file that is shorter
// than its git object header says fails, instead of returning a short file.
func TestBlobReaderTruncatedObject(t *testing.T) {
	repo, mesh := largeFileRepo(t)
	hash := runGit(t, repo, "rev-parse", "HEAD:meshes/mesh.dae")
	var object bytes.Buffer
	zw := zlib.NewWriter(&object)
	_, err := fmt.Fprintf(zw, "blob %d\x00%s", len(mesh), mesh[:100])
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	objectPath := filepath.Join(repo.Path, ".git", "objects", hash[:2], hash[2:])
	require.NoError(t, os.Remove(objectPath))
	require.NoError(t, os.WriteFile(objectPath, object.Bytes(), 0444))

	content, err := repo.GetFile(context.Background(), "", "meshes/mesh.dae")
	require.NoError(t, err)
	defer content.Close()
	bs, err := io.ReadAll(content)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.Equal(t, mesh[:100], string(bs))
}
