package vcs

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBytesFile returns a File with the given content, and a counter of the
// times it was opened.
func newBytesFile(content []byte, size int64) (*File, *int) {
	opens := 0
	f := &File{Hash: "hash", Size: size, open: func() (io.ReadCloser, error) {
		opens++
		return io.NopCloser(bytes.NewReader(content)), nil
	}}
	return f, &opens
}

func TestFileContentReadSeek(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789"), 100)
	f, opens := newBytesFile(data, int64(len(data)))
	c, err := f.Content()
	require.NoError(t, err)
	defer c.Close()
	assert.Equal(t, 1, *opens)

	// Seeking does not open the file.
	size, err := c.Seek(0, io.SeekEnd)
	require.NoError(t, err)
	assert.Equal(t, int64(len(data)), size)
	_, err = c.Seek(0, io.SeekStart)
	require.NoError(t, err)
	assert.Equal(t, 1, *opens)

	bs, err := io.ReadAll(c)
	require.NoError(t, err)
	assert.Equal(t, data, bs)
	assert.Equal(t, 1, *opens)

	// Seeking backwards opens the file again.
	pos, err := c.Seek(-10, io.SeekEnd)
	require.NoError(t, err)
	assert.Equal(t, int64(990), pos)
	buf := make([]byte, 5)
	_, err = io.ReadFull(c, buf)
	require.NoError(t, err)
	assert.Equal(t, "01234", string(buf))
	assert.Equal(t, 2, *opens)

	// Seeking forward skips content.
	pos, err = c.Seek(3, io.SeekCurrent)
	require.NoError(t, err)
	assert.Equal(t, int64(998), pos)
	bs, err = io.ReadAll(c)
	require.NoError(t, err)
	assert.Equal(t, "89", string(bs))
	assert.Equal(t, 2, *opens)

	// The beginning of the content is kept in memory.
	_, err = c.Seek(headSize-10, io.SeekStart)
	require.NoError(t, err)
	_, err = io.ReadFull(c, buf)
	require.NoError(t, err)
	assert.Equal(t, data[headSize-10:headSize-5], buf)
	assert.Equal(t, 2, *opens)

	// Reading past the end.
	_, err = c.Seek(2000, io.SeekStart)
	require.NoError(t, err)
	n, err := c.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)

	_, err = c.Seek(-1, io.SeekStart)
	assert.Error(t, err)
	_, err = c.Seek(0, 42)
	assert.Error(t, err)

	assert.NoError(t, c.Close())
	assert.NoError(t, c.Close())
}

func TestFileContentReadAfterClose(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789"), 100)
	f, opens := newBytesFile(data, int64(len(data)))
	c, err := f.Content()
	require.NoError(t, err)
	_, err = c.Seek(headSize+10, io.SeekStart)
	require.NoError(t, err)
	_, err = c.Read(make([]byte, 10))
	require.NoError(t, err)
	require.NoError(t, c.Close())

	// Reading does not open the file again, including after the beginning
	// of the content.
	for _, off := range []int64{0, headSize + 20} {
		_, err = c.Seek(off, io.SeekStart)
		require.NoError(t, err)
		n, err := c.Read(make([]byte, 10))
		assert.Equal(t, 0, n)
		assert.ErrorIs(t, err, os.ErrClosed)
	}
	assert.Equal(t, 1, *opens)
	assert.NoError(t, c.Close())
}

func TestFileContentEmpty(t *testing.T) {
	f, opens := newBytesFile(nil, 0)
	c, err := f.Content()
	require.NoError(t, err)
	defer c.Close()
	bs, err := io.ReadAll(c)
	require.NoError(t, err)
	assert.Empty(t, bs)
	assert.Equal(t, 0, *opens)
}

func TestFileContentOpenError(t *testing.T) {
	errOpen := errors.New("open failed")
	f := &File{Hash: "hash", Size: 10, open: func() (io.ReadCloser, error) {
		return nil, errOpen
	}}
	_, err := f.Content()
	assert.Equal(t, errOpen, err)
}

func TestFileContentReadError(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789"), 100)
	errRead := errors.New("read failed")
	opens := 0
	f := &File{Hash: "hash", Size: int64(len(data)), open: func() (io.ReadCloser, error) {
		opens++
		return io.NopCloser(io.MultiReader(bytes.NewReader(data[:600]), iotest.ErrReader(errRead))), nil
	}}
	c, err := f.Content()
	require.NoError(t, err)
	defer c.Close()
	assert.NoError(t, c.Err())

	bs, err := io.ReadAll(c)
	assert.Equal(t, errRead, err)
	assert.Equal(t, data[:600], bs)
	assert.Equal(t, errRead, c.Err())

	// The error is kept, and the file is not opened again.
	_, err = c.Seek(0, io.SeekStart)
	require.NoError(t, err)
	_, err = c.Read(make([]byte, 10))
	assert.Equal(t, errRead, err)
	assert.Equal(t, 1, opens)
}

func TestFileContentShortStream(t *testing.T) {
	data := []byte("0123456789")
	// The content is shorter than the file size.
	f, _ := newBytesFile(data, int64(len(data))+5)
	_, err := f.Content()
	assert.Equal(t, io.ErrUnexpectedEOF, err)

	// Same, after the beginning of the content.
	data = bytes.Repeat(data, 100)
	f, _ = newBytesFile(data, int64(len(data))+5)
	c, err := f.Content()
	require.NoError(t, err)
	defer c.Close()
	_, err = io.ReadAll(c)
	assert.Equal(t, io.ErrUnexpectedEOF, err)
	assert.Equal(t, io.ErrUnexpectedEOF, c.Err())

	// Same, when skipping content.
	c, err = f.Content()
	require.NoError(t, err)
	defer c.Close()
	_, err = c.Seek(int64(len(data))+2, io.SeekStart)
	require.NoError(t, err)
	_, err = c.Read(make([]byte, 1))
	assert.Equal(t, io.ErrUnexpectedEOF, err)
}

func TestFileContentLongStream(t *testing.T) {
	data := []byte("0123456789")
	// The content is longer than the file size.
	f, _ := newBytesFile(data, int64(len(data))-1)
	_, err := f.Content()
	assert.Error(t, err)

	// Same, after the beginning of the content.
	data = bytes.Repeat(data, 100)
	f, _ = newBytesFile(data, int64(len(data))-1)
	c, err := f.Content()
	require.NoError(t, err)
	defer c.Close()
	bs, err := io.ReadAll(c)
	assert.Error(t, err)
	assert.Less(t, len(bs), len(data)-1)
}

func TestFileContentErrorAtEnd(t *testing.T) {
	// The reader fails after returning the whole content, eg. git exits with
	// an error status.
	data := bytes.Repeat([]byte("0123456789"), 100)
	errRead := errors.New("read failed")
	f := &File{Hash: "hash", Size: int64(len(data)), open: func() (io.ReadCloser, error) {
		return io.NopCloser(io.MultiReader(bytes.NewReader(data), iotest.ErrReader(errRead))), nil
	}}
	c, err := f.Content()
	require.NoError(t, err)
	defer c.Close()
	bs, err := io.ReadAll(c)
	assert.Equal(t, errRead, err)
	assert.Less(t, len(bs), len(data))
}

func TestFileContentServeContent(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789"), 100)
	f, opens := newBytesFile(data, int64(len(data)))

	// serve serves the file, and returns the response and the times the file
	// was opened. The file name has no extension, so that ServeContent reads
	// the content to detect its type.
	serve := func(rangeHeader string) (*httptest.ResponseRecorder, int) {
		r := httptest.NewRequest("GET", "/file", nil)
		if rangeHeader != "" {
			r.Header.Set("Range", rangeHeader)
		}
		w := httptest.NewRecorder()
		before := *opens
		c, err := f.Content()
		require.NoError(t, err)
		defer c.Close()
		http.ServeContent(w, r, "file", time.Now(), c)
		return w, *opens - before
	}

	w, n := serve("")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "1000", w.Header().Get("Content-Length"))
	assert.Equal(t, data, w.Body.Bytes())
	assert.Equal(t, 1, n)

	w, n = serve("bytes=995-")
	assert.Equal(t, http.StatusPartialContent, w.Code)
	assert.Equal(t, "56789", w.Body.String())
	assert.Equal(t, 1, n)

	w, _ = serve("bytes=10-14,2-3")
	assert.Equal(t, http.StatusPartialContent, w.Code)
	assert.Contains(t, w.Body.String(), "\r\n\r\n01234\r\n")
	assert.Contains(t, w.Body.String(), "\r\n\r\n23\r\n")
}
