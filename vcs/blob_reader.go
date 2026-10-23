package vcs

import (
	"errors"
	"fmt"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/go-git/go-git/v5/plumbing/object"
	"io"
	"os"
)

// blobReader reads the content of a git blob as an io.ReadSeekCloser, which
// is what http.ServeContent needs. go-git reads the blobs larger than
// largeObjectThreshold from disk as they are read, but these readers cannot
// seek: a seek forward skips the bytes in between, and a seek backward opens
// the blob again.
// A blobReader is not safe for concurrent use.
type blobReader struct {
	blob   *object.Blob
	logger gz.Logger
	// r reads the blob from position rpos. It is nil after Close.
	r    io.ReadCloser
	rpos int64
	// offset is the position of the next Read, set by Seek.
	offset int64
}

// newBlobReader opens the given blob, so that a blob that cannot be read is
// reported before its content is used. Read errors are logged with the given
// logger, because http.ServeContent ignores them.
func newBlobReader(blob *object.Blob, logger gz.Logger) (*blobReader, error) {
	r, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	return &blobReader{blob: blob, logger: logger, r: r}, nil
}

// Read implements io.Reader.
func (b *blobReader) Read(p []byte) (int, error) {
	if b.r == nil {
		return 0, os.ErrClosed
	}
	if b.offset >= b.blob.Size {
		return 0, io.EOF
	}
	n, err := b.read(p)
	if err != nil && err != io.EOF {
		b.logger.Error(fmt.Sprintf("Error while reading blob %s at offset %d. Err: %v",
			b.blob.Hash, b.offset, err))
	}
	return n, err
}

func (b *blobReader) read(p []byte) (int, error) {
	if b.rpos > b.offset {
		r, err := b.blob.Reader()
		if err != nil {
			return 0, err
		}
		_ = b.r.Close()
		b.r, b.rpos = r, 0
	}
	if b.rpos < b.offset {
		n, err := io.CopyN(io.Discard, b.r, b.offset-b.rpos)
		b.rpos += n
		if err != nil {
			return 0, unexpectedEOF(err)
		}
	}
	n, err := b.r.Read(p)
	b.rpos += int64(n)
	b.offset = b.rpos
	if err == io.EOF && b.rpos < b.blob.Size {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

// unexpectedEOF converts io.EOF to io.ErrUnexpectedEOF.
func unexpectedEOF(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

// Seek implements io.Seeker. It only sets the position of the next Read.
func (b *blobReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += b.offset
	case io.SeekEnd:
		offset += b.blob.Size
	default:
		return 0, errors.New("blobReader.Seek: invalid whence")
	}
	if offset < 0 {
		return 0, errors.New("blobReader.Seek: negative position")
	}
	b.offset = offset
	return offset, nil
}

// Close implements io.Closer.
func (b *blobReader) Close() error {
	if b.r == nil {
		return os.ErrClosed
	}
	err := b.r.Close()
	b.r = nil
	return err
}
