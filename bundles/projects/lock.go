package projects

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockRepo takes an exclusive advisory lock for the project repo at
// location and returns a function that releases it. All writes to a
// project's repo (file writes, the auto-save squash, lazy repo init) must
// hold it: fuel-server and the ai-builder agent can write the same repo
// concurrently, and git operations interleaved across writers corrupt
// history.
//
// The lock is flock(2) on a sibling file, "<location>.lock", so it works
// across processes as well as goroutines (each call opens its own file
// description), is released automatically if the process dies, and never
// appears inside the repo's working tree.
func lockRepo(location string) (func(), error) {
	path := filepath.Clean(location) + ".lock"
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
