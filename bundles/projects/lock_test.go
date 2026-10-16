package projects

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLockRepoIsExclusive runs several goroutines that each take the lock
// and briefly hold it, and checks no two ever hold it at once. Each call
// opens its own file description, so this also models two processes.
func TestLockRepoIsExclusive(t *testing.T) {
	location := filepath.Join(t.TempDir(), "owner", "projects", "uuid")

	var holders, maxHolders int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := lockRepo(location)
			if err != nil {
				t.Error(err)
				return
			}
			n := atomic.AddInt32(&holders, 1)
			for {
				m := atomic.LoadInt32(&maxHolders)
				if n <= m || atomic.CompareAndSwapInt32(&maxHolders, m, n) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&holders, -1)
			unlock()
		}()
	}
	wg.Wait()
	if maxHolders != 1 {
		t.Fatalf("lock held by %d goroutines at once, want 1", maxHolders)
	}
}

// TestLockRepoCanBeRetaken checks the lock is released by unlock.
func TestLockRepoCanBeRetaken(t *testing.T) {
	location := filepath.Join(t.TempDir(), "repo")
	for i := 0; i < 2; i++ {
		unlock, err := lockRepo(location)
		if err != nil {
			t.Fatal(err)
		}
		unlock()
	}
}
