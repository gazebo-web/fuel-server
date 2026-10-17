package projects

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

// Repo-level operations on a project's git working tree. They take the
// repo location directly (no DB, no auth) so they can be tested against a
// temporary repo; Service methods do the permission checks and call these.
// Every mutating function here expects the caller to hold lockRepo.

var (
	// ErrStaleRev means the file changed since the caller's base_rev.
	ErrStaleRev = errors.New("file changed since base_rev")
	// ErrUnknownRev means base_rev is not in history (e.g. squashed).
	ErrUnknownRev = errors.New("base_rev not found in project history")
	// ErrFileNotFound means the path doesn't exist at the revision.
	ErrFileNotFound = errors.New("file not found")
	// ErrNoMatch means old_str does not occur in the file.
	ErrNoMatch = errors.New("old_str not found")
	// ErrAmbiguousMatch means old_str occurs more than once.
	ErrAmbiguousMatch = errors.New("old_str is not unique")
	// ErrBinary means a text edit was attempted on a binary file.
	ErrBinary = errors.New("binary files can't be edited as text")
	// ErrTooLarge means a write would exceed a size limit.
	ErrTooLarge = errors.New("too large")
)

// Size limits on a project's working tree (design §8.5). Variables so
// tests can lower them.
var (
	MaxFileSize    int64 = 10 << 20
	MaxModelSize   int64 = 50 << 20 // everything under models/<name>/
	MaxProjectSize int64 = 300 << 20
)

// binaryExts are file types the text tools never read or write.
var binaryExts = map[string]bool{
	".glb": true, ".bin": true, ".png": true, ".jpg": true, ".jpeg": true, ".ktx2": true,
	".stl": true, ".zip": true, ".gz": true, ".tar": true, ".blend": true,
}

// IsBinary reports whether a file is binary, by extension or, when content
// is given, by a NUL byte in its first 8 KB.
func IsBinary(path string, content []byte) bool {
	if binaryExts[strings.ToLower(filepath.Ext(path))] {
		return true
	}
	if len(content) > 8<<10 {
		content = content[:8<<10]
	}
	return bytes.IndexByte(content, 0) >= 0
}

// IsAutoSave reports whether a commit message marks an automatic save
// (agent edits, chat auto-saves). Runs of these at HEAD are what labeled
// saves and the idle squash collapse.
func IsAutoSave(msg string) bool {
	msg = strings.TrimSpace(msg)
	return msg == AutoSaveMessage || strings.HasPrefix(msg, AutoSaveMessage+":")
}

// cleanRelPath validates a repo-relative path: no absolute paths, no "..",
// POSIX separators. Returns the cleaned path.
func cleanRelPath(rel string) (string, error) {
	if rel == "" {
		return "", errors.New("path is required")
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	if filepath.IsAbs(clean) || clean == "." || strings.HasPrefix(clean, "/") ||
		clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") ||
		clean == ".git" || strings.HasPrefix(clean, ".git/") {
		return "", errors.New("invalid path: " + rel)
	}
	return clean, nil
}

// resolveCommit returns the commit for rev ("" or "tip" = HEAD).
func resolveCommit(repo *git.Repository, rev string) (*object.Commit, error) {
	if rev == "" || rev == "tip" {
		head, err := repo.Head()
		if err != nil {
			return nil, err
		}
		return repo.CommitObject(head.Hash())
	}
	h, err := repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return nil, ErrUnknownRev
	}
	c, err := repo.CommitObject(*h)
	if err != nil {
		return nil, ErrUnknownRev
	}
	return c, nil
}

// headRev returns the full hash of HEAD.
func headRev(location string) (string, error) {
	repo, err := git.PlainOpen(location)
	if err != nil {
		return "", err
	}
	head, err := repo.Head()
	if err != nil {
		return "", err
	}
	return head.Hash().String(), nil
}

// readFileAt returns a file's content at rev and the resolved commit hash.
func readFileAt(location, rel, rev string) ([]byte, string, error) {
	clean, err := cleanRelPath(rel)
	if err != nil {
		return nil, "", err
	}
	repo, err := git.PlainOpen(location)
	if err != nil {
		return nil, "", err
	}
	c, err := resolveCommit(repo, rev)
	if err != nil {
		return nil, "", err
	}
	f, err := c.File(clean)
	if err != nil {
		return nil, c.Hash.String(), ErrFileNotFound
	}
	s, err := f.Contents()
	if err != nil {
		return nil, "", err
	}
	return []byte(s), c.Hash.String(), nil
}

// listFilesAt returns the sorted file paths at rev and the resolved hash.
func listFilesAt(location, rev string) ([]string, string, error) {
	repo, err := git.PlainOpen(location)
	if err != nil {
		return nil, "", err
	}
	c, err := resolveCommit(repo, rev)
	if err != nil {
		return nil, "", err
	}
	iter, err := c.Files()
	if err != nil {
		return nil, "", err
	}
	var out []string
	_ = iter.ForEach(func(f *object.File) error {
		out = append(out, f.Name)
		return nil
	})
	sort.Strings(out)
	return out, c.Hash.String(), nil
}

// checkBaseRev returns ErrStaleRev if rel differs between baseRev and
// HEAD. Only the target file matters: commits that touch other files
// (thumbnails, other models) don't make an edit stale. Empty baseRev
// skips the check.
func checkBaseRev(location, rel, baseRev string) error {
	if baseRev == "" {
		return nil
	}
	repo, err := git.PlainOpen(location)
	if err != nil {
		return err
	}
	base, err := resolveCommit(repo, baseRev)
	if err != nil {
		return err
	}
	head, err := resolveCommit(repo, "")
	if err != nil {
		return err
	}
	blob := func(c *object.Commit) string {
		f, err := c.File(rel)
		if err != nil {
			return ""
		}
		return f.Hash.String()
	}
	if blob(base) != blob(head) {
		return ErrStaleRev
	}
	return nil
}

// commitAll stages every change and commits it with the git CLI (go-git v4
// can't amend or allow empty commits). Returns the new HEAD hash.
func commitAll(ctx context.Context, location, message, author string, amend, allowEmpty bool) (string, error) {
	if author == "" {
		author = gitAuthor
	}
	if out, err := exec.CommandContext(ctx, "git", "-C", location, "add", "-A").CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add: %v: %s", err, out)
	}
	args := []string{"-C", location, "-c", "user.name=" + author, "-c", "user.email=" + gitEmail, "commit", "-q"}
	if amend {
		args = append(args, "--amend", "--no-edit")
	} else {
		args = append(args, "-m", message)
	}
	if allowEmpty || amend {
		args = append(args, "--allow-empty")
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_NAME="+author, "GIT_COMMITTER_EMAIL="+gitEmail)
	if out, err := cmd.CombinedOutput(); err != nil {
		if !allowEmpty && bytes.Contains(out, []byte("nothing to commit")) {
			return headRev(location)
		}
		return "", fmt.Errorf("git commit: %v: %s", err, out)
	}
	return headRev(location)
}

// writeFileLocked writes content to rel and commits it. With amend, the
// change is folded into HEAD instead of creating a new version.
func writeFileLocked(ctx context.Context, location, rel string, content []byte,
	message, author string, amend bool) (string, error) {
	return writeFilesLocked(ctx, location, map[string][]byte{rel: content}, nil, message, author, amend)
}

// writeFilesLocked applies a file set (see applyFilesLocked) and commits
// it as one commit.
func writeFilesLocked(ctx context.Context, location string, files map[string][]byte, replaceDirs []string,
	message, author string, amend bool) (string, error) {
	if err := applyFilesLocked(location, files, replaceDirs); err != nil {
		return "", err
	}
	return commitAll(ctx, location, message, author, amend, false)
}

// applyFilesLocked empties each of replaceDirs, then writes files, after
// checking the result stays within the size limits. Nothing is changed if
// a path is invalid or a limit would be exceeded.
func applyFilesLocked(location string, files map[string][]byte, replaceDirs []string) error {
	cleanFiles := make(map[string][]byte, len(files))
	for rel, content := range files {
		clean, err := cleanRelPath(rel)
		if err != nil {
			return err
		}
		cleanFiles[clean] = content
	}
	cleanDirs := make([]string, 0, len(replaceDirs))
	for _, d := range replaceDirs {
		clean, err := cleanRelPath(d)
		if err != nil {
			return err
		}
		cleanDirs = append(cleanDirs, clean)
	}
	if err := checkSizes(location, cleanFiles, cleanDirs); err != nil {
		return err
	}
	for _, d := range cleanDirs {
		if err := os.RemoveAll(filepath.Join(location, d)); err != nil {
			return err
		}
	}
	for clean, content := range cleanFiles {
		full := filepath.Join(location, clean)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(full, content, 0644); err != nil {
			return err
		}
	}
	return nil
}

// checkSizes checks the working tree that applying files and replaceDirs
// would produce against MaxFileSize, MaxModelSize and MaxProjectSize.
func checkSizes(location string, files map[string][]byte, replaceDirs []string) error {
	sizes := map[string]int64{}
	err := filepath.Walk(location, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(location, p)
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			if rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		sizes[rel] = info.Size()
		return nil
	})
	if err != nil {
		return err
	}
	for rel := range sizes {
		for _, d := range replaceDirs {
			if rel == d || strings.HasPrefix(rel, d+"/") {
				delete(sizes, rel)
			}
		}
	}
	for rel, content := range files {
		if n := int64(len(content)); n > MaxFileSize {
			return fmt.Errorf("%w: %s is %s; the limit per file is %s", ErrTooLarge, rel, mb(n), mb(MaxFileSize))
		}
		sizes[rel] = int64(len(content))
	}
	var total int64
	perModel := map[string]int64{}
	for rel, n := range sizes {
		total += n
		if parts := strings.SplitN(rel, "/", 3); len(parts) == 3 && parts[0] == "models" {
			perModel[parts[1]] += n
		}
	}
	for name, n := range perModel {
		if n > MaxModelSize {
			return fmt.Errorf("%w: model %s would be %s; the limit per model is %s", ErrTooLarge, name, mb(n), mb(MaxModelSize))
		}
	}
	if total > MaxProjectSize {
		return fmt.Errorf("%w: the project would be %s; the limit is %s", ErrTooLarge, mb(total), mb(MaxProjectSize))
	}
	return nil
}

func mb(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }

// pruneAge is how long objects no commit reaches are kept. Squashed
// auto-save revisions stay readable for a while, since the viewer and
// agents may still hold them; after that they are pruned.
var pruneAge = "1.hour.ago"

// gcLocked drops old objects no commit reaches any more, e.g. meshes
// replaced in auto-saves that a squash folded away, so binary churn
// doesn't grow the repo forever.
func gcLocked(ctx context.Context, location string) error {
	for _, args := range [][]string{
		{"reflog", "expire", "--expire=" + pruneAge, "--expire-unreachable=" + pruneAge, "--all"},
		{"gc", "--prune=" + pruneAge, "--quiet"},
	} {
		if out, err := exec.CommandContext(ctx, "git", append([]string{"-C", location}, args...)...).CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %v: %s", args[0], err, out)
		}
	}
	return nil
}

// editFileLocked replaces the single occurrence of oldStr with newStr in
// rel and commits. The file is read from HEAD's working tree after the
// staleness check, under the caller's lock, so the read-modify-write is
// atomic with respect to other writers.
func editFileLocked(ctx context.Context, location, rel, oldStr, newStr, baseRev,
	message, author string) (string, error) {
	clean, err := cleanRelPath(rel)
	if err != nil {
		return "", err
	}
	if err := checkBaseRev(location, clean, baseRev); err != nil {
		return "", err
	}
	current, _, err := readFileAt(location, clean, "")
	if err != nil {
		return "", err
	}
	if IsBinary(clean, current) {
		return "", ErrBinary
	}
	switch n := bytes.Count(current, []byte(oldStr)); {
	case oldStr == "" || n == 0:
		return "", ErrNoMatch
	case n > 1:
		return "", fmt.Errorf("%w: %d occurrences; include more surrounding text", ErrAmbiguousMatch, n)
	}
	updated := bytes.Replace(current, []byte(oldStr), []byte(newStr), 1)
	return writeFileLocked(ctx, location, clean, updated, message, author, false)
}

// squashAutoSavesLocked collapses the run of auto-save commits at HEAD
// into one commit labeled label(n), keeping the resulting tree. When HEAD
// is not an auto-save it does nothing, unless force is set, in which case
// it records a labeled commit anyway (a user's Save with nothing new).
// Returns the new HEAD and how many commits were folded.
func squashAutoSavesLocked(ctx context.Context, location string, label func(n int) string,
	author string, force bool) (string, int, error) {
	repo, err := git.PlainOpen(location)
	if err != nil {
		return "", 0, err
	}
	head, err := repo.Head()
	if err != nil {
		return "", 0, err
	}
	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return "", 0, err
	}
	var target *object.Commit
	count := 0
	for {
		c, err := iter.Next()
		if err != nil {
			break
		}
		if IsAutoSave(c.Message) {
			count++
			continue
		}
		target = c
		break
	}
	iter.Close()
	if count == 0 || target == nil {
		if !force {
			return head.Hash().String(), 0, nil
		}
		rev, err := commitAll(ctx, location, label(0), author, false, true)
		return rev, 0, err
	}
	if out, err := exec.CommandContext(ctx, "git", "-C", location,
		"reset", "-q", "--soft", target.Hash.String()).CombinedOutput(); err != nil {
		return "", 0, fmt.Errorf("git reset: %v: %s", err, out)
	}
	rev, err := commitAll(ctx, location, label(count), author, false, true)
	if err != nil {
		return "", 0, err
	}
	if err := gcLocked(ctx, location); err != nil {
		log.Printf("projects: gc %s: %v", location, err) // the squash itself succeeded
	}
	return rev, count, nil
}

// -- idle squash -------------------------------------------------------------

// IdleSquashAfter is how long a project must go without writes before its
// trailing auto-save commits are folded into one version (design Q-3).
var IdleSquashAfter = 10 * time.Minute

var idleTimers = struct {
	sync.Mutex
	m map[string]*time.Timer
}{m: map[string]*time.Timer{}}

// scheduleIdleSquash (re)starts the idle timer for a project after an
// auto-save. Timers are in-memory and best effort: a restart drops them,
// and the next auto-save or labeled save picks the run up again.
func scheduleIdleSquash(location string) {
	idleTimers.Lock()
	defer idleTimers.Unlock()
	if t, ok := idleTimers.m[location]; ok {
		t.Stop()
	}
	idleTimers.m[location] = time.AfterFunc(IdleSquashAfter, func() {
		idleTimers.Lock()
		delete(idleTimers.m, location)
		idleTimers.Unlock()
		_, _ = idleSquash(context.Background(), location)
	})
}

// idleSquash folds trailing auto-saves into one "agent session" version.
func idleSquash(ctx context.Context, location string) (int, error) {
	unlock, err := lockRepo(location)
	if err != nil {
		return 0, err
	}
	defer unlock()
	repo, err := git.PlainOpen(location)
	if err != nil {
		return 0, err
	}
	head, err := repo.Head()
	if err != nil {
		return 0, err
	}
	c, err := repo.CommitObject(head.Hash())
	if err != nil || !IsAutoSave(c.Message) {
		return 0, err
	}
	// A write raced in after the timer fired; let its own timer handle it.
	if time.Since(c.Committer.When) < IdleSquashAfter-time.Second {
		return 0, nil
	}
	_, n, err := squashAutoSavesLocked(ctx, location, func(n int) string {
		if n == 1 {
			return "Agent session (1 edit)"
		}
		return fmt.Sprintf("Agent session (%d edits)", n)
	}, gitAuthor, false)
	return n, err
}
