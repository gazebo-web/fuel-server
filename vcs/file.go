package vcs

import (
	"bytes"
	"context"
	"fmt"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/pkg/errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// File is a file stored in a repository at a given revision. Its content is
// not read until requested, so that large files (eg. meshes) do not need to be
// loaded in memory.
type File struct {
	// Hash is the git object hash of the file content.
	Hash string
	// Size is the size of the file content, in bytes.
	Size int64
	// open returns a new reader of the content, from its beginning.
	open func() (io.ReadCloser, error)
}

// Open returns a reader of the file content. The caller must close it.
func (f *File) Open() (io.ReadCloser, error) {
	return f.open()
}

// headSize is the size of the beginning of the content that FileContent keeps
// in memory. It is the size that http.ServeContent reads to detect the content
// type, so that detecting it does not open the file again.
const headSize = 512

// Content opens the file content, as needed by http.ServeContent. Its
// beginning is read before returning, so that a file that cannot be read
// fails before a response is sent. The rest is read on demand: seeking forward
// skips content and seeking backwards opens the file again. The caller must
// close it.
func (f *File) Content() (*FileContent, error) {
	c := &FileContent{file: f}
	if err := c.readHead(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// FileContent implements io.ReadSeekCloser on top of File.open.
type FileContent struct {
	file *File
	// mu guards the fields below. http.ServeContent reads multi-range requests
	// from a separate goroutine, which can still run when the caller closes.
	mu sync.Mutex
	// head is the beginning of the content.
	head []byte
	// off is the offset of the next Read.
	off int64
	// r is the open reader, if any, and rOff is its offset.
	r    io.ReadCloser
	rOff int64
	// err is the error that made Read fail. Read keeps returning it.
	err error
	// closed is set by Close, after which Read fails.
	closed bool
}

// readHead reads the beginning of the content into head.
func (c *FileContent) readHead() error {
	size := c.file.Size
	if size > headSize {
		size = headSize
	}
	head := make([]byte, size)
	for c.off < size {
		if _, err := c.read(head[c.off:]); err != nil && err != io.EOF {
			return err
		}
	}
	c.head, c.off = head, 0
	return nil
}

// Err returns the error that made Read fail, if any. Note that
// http.ServeContent ignores read errors: the response is cut short.
func (c *FileContent) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Read implements io.Reader.
func (c *FileContent) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, os.ErrClosed
	}
	if c.err != nil {
		return 0, c.err
	}
	if c.off < int64(len(c.head)) {
		n := copy(p, c.head[c.off:])
		c.off += int64(n)
		return n, nil
	}
	n, err := c.read(p)
	if err != nil && err != io.EOF {
		c.err = err
	}
	return n, err
}

// read reads the content at off from the open reader, opening the file again
// or skipping content as needed.
func (c *FileContent) read(p []byte) (int, error) {
	if c.off >= c.file.Size {
		return 0, io.EOF
	}
	if c.r != nil && c.rOff > c.off {
		_ = c.r.Close()
		c.r = nil
	}
	if c.r == nil {
		r, err := c.file.open()
		if err != nil {
			return 0, err
		}
		c.r, c.rOff = r, 0
	}
	if c.rOff < c.off {
		n, err := io.CopyN(io.Discard, c.r, c.off-c.rOff)
		c.rOff += n
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		if err != nil {
			return 0, err
		}
	}
	if remaining := c.file.Size - c.off; int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := c.r.Read(p)
	c.rOff += int64(n)
	end := c.off+int64(n) == c.file.Size
	if end && err == nil {
		// Make sure that the content ends here. Reaching the end of the git
		// output also checks git's exit status.
		err = expectEOF(c.r)
	}
	if end && err != nil && err != io.EOF {
		// Do not return the last bytes, so that the content is not complete.
		return 0, err
	}
	c.off += int64(n)
	if err == io.EOF && c.off < c.file.Size {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

// expectEOF returns io.EOF if r is at its end, or an error otherwise.
func expectEOF(r io.Reader) error {
	var b [1]byte
	n, err := io.ReadFull(r, b[:])
	if n > 0 {
		return errors.New("vcs: content longer than its size")
	}
	return err
}

// Seek implements io.Seeker.
func (c *FileContent) Seek(offset int64, whence int) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += c.off
	case io.SeekEnd:
		offset += c.file.Size
	default:
		return 0, errors.New("vcs: invalid whence")
	}
	if offset < 0 {
		return 0, errors.New("vcs: negative position")
	}
	c.off = offset
	return offset, nil
}

// Close implements io.Closer.
func (c *FileContent) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.r == nil {
		return nil
	}
	err := c.r.Close()
	c.r = nil
	return err
}

// gitBlobReader reads a blob from a "git cat-file" process.
type gitBlobReader struct {
	ctx    context.Context
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr bytes.Buffer
	done   bool
	err    error
}

// catBlob streams the content of the blob with the given hash using the git
// command, which, unlike go-git, does not load the blob in memory. The process
// is killed if ctx is done before the content is read.
func catBlob(ctx context.Context, repoPath, hash string) (io.ReadCloser, error) {
	// git only streams packed blobs bigger than core.bigFileThreshold (512 MiB
	// by default), smaller ones are loaded in memory. Note that blobs stored as
	// deltas are always loaded in memory.
	threshold := fmt.Sprintf("core.bigFileThreshold=%d", maxInMemoryFileSize)
	r := &gitBlobReader{
		ctx: ctx,
		cmd: exec.CommandContext(ctx, "git", "-C", repoPath, "-c", threshold, "cat-file", "blob", hash),
	}
	r.cmd.Stderr = &r.stderr
	stdout, err := r.cmd.StdoutPipe()
	if err != nil {
		return nil, gz.WithStack(err)
	}
	r.stdout = stdout
	if err := r.cmd.Start(); err != nil {
		err = gz.WithStack(err)
		gz.LoggerFromContext(ctx).Info("Error while starting git cat-file process. Err: " + fmt.Sprint(err))
		return nil, err
	}
	return r, nil
}

// Read implements io.Reader.
func (r *gitBlobReader) Read(p []byte) (int, error) {
	n, err := r.stdout.Read(p)
	if err == io.EOF {
		// Make sure that git did not fail half way.
		if werr := r.wait(); werr != nil {
			err = werr
		}
	}
	return n, err
}

// wait waits for git to exit, and returns an error if it failed.
func (r *gitBlobReader) wait() error {
	if r.done {
		return r.err
	}
	r.done = true
	if err := r.cmd.Wait(); err != nil {
		if r.ctx.Err() != nil {
			// git was killed because the request went away.
			r.err = gz.WithStack(r.ctx.Err())
			return r.err
		}
		r.err = errors.Wrap(err, "git cat-file failed. Stderr: "+strings.TrimSpace(r.stderr.String()))
		gz.LoggerFromContext(r.ctx).Info("Error while running git cat-file process. Err: " + fmt.Sprint(r.err))
	}
	return r.err
}

// Close implements io.Closer. It kills git if the content was not read to the
// end, eg. because the client went away.
func (r *gitBlobReader) Close() error {
	if r.done {
		return nil
	}
	r.done = true
	_ = r.cmd.Process.Kill()
	_ = r.cmd.Wait()
	return nil
}
