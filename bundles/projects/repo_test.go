package projects

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

// newTestRepo creates a repo shaped like a project repo: master branch and
// an initial commit.
func newTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", dir},
		{"-C", dir, "branch", "-M", "master"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if _, err := writeFileLocked(context.Background(), dir, ".gitignore", nil, "initial", "", false); err != nil {
		t.Fatal(err)
	}
	return dir
}

func history(t *testing.T, dir string) []string {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	head, _ := repo.Head()
	iter, _ := repo.Log(&git.LogOptions{From: head.Hash()})
	var msgs []string
	_ = iter.ForEach(func(c *object.Commit) error {
		msgs = append(msgs, strings.TrimSpace(c.Message))
		return nil
	})
	return msgs
}

func write(t *testing.T, dir, path, content, msg string) string {
	t.Helper()
	rev, err := writeFileLocked(context.Background(), dir, path, []byte(content), msg, "tester", false)
	if err != nil {
		t.Fatal(err)
	}
	return rev
}

func TestReadAndListAtRevisions(t *testing.T) {
	dir := newTestRepo(t)
	r1 := write(t, dir, "world.sdf", "v1", "auto-save: one")
	r2 := write(t, dir, "models/a/model.sdf", "m", "auto-save: two")

	got, rev, err := readFileAt(dir, "world.sdf", r1)
	if err != nil || string(got) != "v1" || rev != r1 {
		t.Fatalf("read at r1: %q %s %v", got, rev, err)
	}
	if _, _, err := readFileAt(dir, "models/a/model.sdf", r1); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("file should not exist at r1: %v", err)
	}
	files, rev, err := listFilesAt(dir, "")
	if err != nil || rev != r2 || strings.Join(files, ",") != ".gitignore,models/a/model.sdf,world.sdf" {
		t.Fatalf("list at HEAD: %v %s %v", files, rev, err)
	}
	if _, _, err := readFileAt(dir, "world.sdf", "0123456789abcdef0123456789abcdef01234567"); !errors.Is(err, ErrUnknownRev) {
		t.Fatalf("unknown rev: %v", err)
	}
}

func TestEditFile(t *testing.T) {
	ctx := context.Background()
	dir := newTestRepo(t)
	base := write(t, dir, "world.sdf", "<a/><b/><b/>", "auto-save: w")

	if _, err := editFileLocked(ctx, dir, "world.sdf", "<c/>", "x", base, "auto-save: e", ""); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("want ErrNoMatch, got %v", err)
	}
	if _, err := editFileLocked(ctx, dir, "world.sdf", "<b/>", "x", base, "auto-save: e", ""); !errors.Is(err, ErrAmbiguousMatch) {
		t.Fatalf("want ErrAmbiguousMatch, got %v", err)
	}
	rev, err := editFileLocked(ctx, dir, "world.sdf", "<a/>", "<A/>", base, "auto-save: e", "")
	if err != nil {
		t.Fatal(err)
	}
	got, _, _ := readFileAt(dir, "world.sdf", rev)
	if string(got) != "<A/><b/><b/>" {
		t.Fatalf("content %q", got)
	}
}

func TestBaseRevIsPerFile(t *testing.T) {
	ctx := context.Background()
	dir := newTestRepo(t)
	base := write(t, dir, "world.sdf", "<a/>", "auto-save: w")

	// A thumbnail commit in between must not make a world edit stale.
	write(t, dir, "thumbnail.png", "png", "thumbnail update")
	if _, err := editFileLocked(ctx, dir, "world.sdf", "<a/>", "<b/>", base, "auto-save: e", ""); err != nil {
		t.Fatalf("edit after unrelated commit: %v", err)
	}
	// Now world.sdf has changed since base: a second edit from base is stale.
	if _, err := editFileLocked(ctx, dir, "world.sdf", "<b/>", "<c/>", base, "auto-save: e", ""); !errors.Is(err, ErrStaleRev) {
		t.Fatalf("want ErrStaleRev, got %v", err)
	}
}

func TestSquashAutoSaves(t *testing.T) {
	ctx := context.Background()
	dir := newTestRepo(t)
	write(t, dir, "world.sdf", "v1", "Labeled one")
	write(t, dir, "world.sdf", "v2", "auto-save: a")
	write(t, dir, "world.sdf", "v3", AutoSaveMessage)
	write(t, dir, "world.sdf", "v4", "auto-save: c")

	rev, n, err := squashAutoSavesLocked(ctx, dir, func(n int) string { return "Saved" }, "tester", true)
	if err != nil || n != 3 {
		t.Fatalf("squash: n=%d err=%v", n, err)
	}
	if got := strings.Join(history(t, dir), "|"); got != "Saved|Labeled one|initial" {
		t.Fatalf("history %q", got)
	}
	if got, _, _ := readFileAt(dir, "world.sdf", rev); string(got) != "v4" {
		t.Fatalf("squash must keep the latest tree, got %q", got)
	}

	// Nothing to squash: force still records the label.
	if _, n, err := squashAutoSavesLocked(ctx, dir, func(int) string { return "Saved again" }, "tester", true); err != nil || n != 0 {
		t.Fatalf("forced label: n=%d err=%v", n, err)
	}
	if h := history(t, dir); h[0] != "Saved again" || len(h) != 4 {
		t.Fatalf("history %v", h)
	}
}

func TestAmendKeepsVersionCount(t *testing.T) {
	dir := newTestRepo(t)
	write(t, dir, "world.sdf", "v1", "Labeled")
	before := len(history(t, dir))
	if _, err := writeFileLocked(context.Background(), dir, "thumbnail.png", []byte("png"), "", "", true); err != nil {
		t.Fatal(err)
	}
	h := history(t, dir)
	if len(h) != before || h[0] != "Labeled" {
		t.Fatalf("amend changed history: %v", h)
	}
	if got, _, _ := readFileAt(dir, "thumbnail.png", ""); string(got) != "png" {
		t.Fatalf("thumbnail not in HEAD: %q", got)
	}
}

func TestIdleSquash(t *testing.T) {
	orig := IdleSquashAfter
	IdleSquashAfter = 0
	t.Cleanup(func() { IdleSquashAfter = orig })

	dir := newTestRepo(t)
	write(t, dir, "world.sdf", "v1", "auto-save: a")
	write(t, dir, "world.sdf", "v2", "auto-save: b")
	n, err := idleSquash(context.Background(), dir)
	if err != nil || n != 2 {
		t.Fatalf("idle squash n=%d err=%v", n, err)
	}
	if got := strings.Join(history(t, dir), "|"); got != "Agent session (2 edits)|initial" {
		t.Fatalf("history %q", got)
	}
	// Idempotent: HEAD is no longer an auto-save.
	if n, err := idleSquash(context.Background(), dir); err != nil || n != 0 {
		t.Fatalf("second idle squash n=%d err=%v", n, err)
	}
}

func TestCleanRelPath(t *testing.T) {
	for _, bad := range []string{"", "/etc/passwd", "../x", "a/../../x", ".git/config", ".git", "."} {
		if _, err := cleanRelPath(bad); err == nil {
			t.Errorf("cleanRelPath(%q) should fail", bad)
		}
	}
	for in, want := range map[string]string{"world.sdf": "world.sdf", "models/a/../b/model.sdf": "models/b/model.sdf", "a..b.sdf": "a..b.sdf"} {
		if got, err := cleanRelPath(in); err != nil || got != want {
			t.Errorf("cleanRelPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
