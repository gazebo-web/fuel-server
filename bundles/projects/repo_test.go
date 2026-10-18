package projects

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
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

func TestWriteFilesReplacesDirsInOneCommit(t *testing.T) {
	dir := newTestRepo(t)
	ctx := context.Background()
	before := len(history(t, dir))
	_, err := writeFilesLocked(ctx, dir, map[string][]byte{
		"models/crate/model.sdf":              []byte("<sdf/>"),
		"models/crate/meshes/collision_1.glb": {0, 1},
		"models/crate/meshes/collision_2.glb": {0, 2},
	}, nil, "auto-save: model_generate crate", "", false)
	if err != nil {
		t.Fatal(err)
	}
	// Regenerating with fewer hulls must not leave collision_2 behind.
	_, err = writeFilesLocked(ctx, dir, map[string][]byte{
		"models/crate/model.sdf":              []byte("<sdf version='1.9'/>"),
		"models/crate/meshes/collision_1.glb": {0, 3},
	}, []string{"models/crate"}, "auto-save: model_generate crate", "", false)
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := listFilesAt(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(files, ","); got != ".gitignore,models/crate/meshes/collision_1.glb,models/crate/model.sdf" {
		t.Errorf("files %s", got)
	}
	if n := len(history(t, dir)) - before; n != 2 {
		t.Errorf("want one commit per write, got %d", n)
	}
	if _, err := writeFilesLocked(ctx, dir, map[string][]byte{"../escape": nil}, nil, "x", "", false); err == nil {
		t.Error("path outside the repo accepted")
	}
}

func TestSizeLimits(t *testing.T) {
	dir := newTestRepo(t)
	ctx := context.Background()
	defer func(f, m, p int64) { MaxFileSize, MaxModelSize, MaxProjectSize = f, m, p }(MaxFileSize, MaxModelSize, MaxProjectSize)
	MaxFileSize, MaxModelSize, MaxProjectSize = 100, 150, 250

	big := make([]byte, 101)
	if _, err := writeFileLocked(ctx, dir, "a.bin", big, "x", "", false); !errors.Is(err, ErrTooLarge) {
		t.Errorf("file over limit: %v", err)
	}
	if _, err := writeFilesLocked(ctx, dir, map[string][]byte{
		"models/a/1.glb": make([]byte, 80), "models/a/2.glb": make([]byte, 80),
	}, nil, "x", "", false); !errors.Is(err, ErrTooLarge) || !strings.Contains(err.Error(), "model a") {
		t.Errorf("model over limit: %v", err)
	}
	if _, err := writeFilesLocked(ctx, dir, map[string][]byte{
		"models/a/1.glb": make([]byte, 80), "models/b/1.glb": make([]byte, 80),
	}, nil, "x", "", false); err != nil {
		t.Fatalf("within limits: %v", err)
	}
	if _, err := writeFileLocked(ctx, dir, "world.sdf", make([]byte, 100), "x", "", false); !errors.Is(err, ErrTooLarge) {
		t.Errorf("project over limit: %v", err)
	}
	// Replacing a model's folder frees its space first.
	if _, err := writeFilesLocked(ctx, dir, map[string][]byte{"models/a/1.glb": make([]byte, 90)},
		[]string{"models/a"}, "x", "", false); err != nil {
		t.Errorf("replace within limits: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "world.sdf")); err == nil {
		t.Error("a rejected write must change nothing")
	}
}

// Squashing auto-saves prunes the blobs only they referenced, once they
// are older than pruneAge.
func TestSquashPrunesReplacedBlobs(t *testing.T) {
	defer func(a string) { pruneAge = a }(pruneAge)
	pruneAge = "now"
	dir := newTestRepo(t)
	ctx := context.Background()
	if _, err := writeFileLocked(ctx, dir, "models/m/mesh.glb", []byte("first mesh\x00"), AutoSaveMessage+": a", "", false); err != nil {
		t.Fatal(err)
	}
	old := blobHash(t, dir, "models/m/mesh.glb")
	if _, err := writeFileLocked(ctx, dir, "models/m/mesh.glb", []byte("second mesh\x00"), AutoSaveMessage+": b", "", false); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "cat-file", "-e", old).CombinedOutput(); err != nil {
		t.Fatalf("old blob should exist before the squash: %s", out)
	}
	if _, n, err := squashAutoSavesLocked(ctx, dir, func(int) string { return "turn: mesh" }, "", false); err != nil || n != 2 {
		t.Fatalf("squash: n=%d %v", n, err)
	}
	if err := exec.Command("git", "-C", dir, "cat-file", "-e", old).Run(); err == nil {
		t.Error("blob only an auto-save referenced survived the squash")
	}
}

func blobHash(t *testing.T, dir, path string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD:"+path).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestIsBinary(t *testing.T) {
	for _, c := range []struct {
		path    string
		content string
		want    bool
	}{
		{"models/m/meshes/visual.glb", "", true},
		{"thumbnail.PNG", "", true},
		{"world.sdf", "<sdf/>", false},
		{"notes.txt", "a\x00b", true},
		{"models/m/meshes/visual.gltf", `{"asset":{}}`, false},
	} {
		if got := IsBinary(c.path, []byte(c.content)); got != c.want {
			t.Errorf("%s: %v", c.path, got)
		}
	}
}

func TestSquashKeepsRecentRevisionsReadable(t *testing.T) {
	dir := newTestRepo(t)
	ctx := context.Background()
	rev := write(t, dir, "world.sdf", "v1", AutoSaveMessage+": a")
	write(t, dir, "world.sdf", "v2", AutoSaveMessage+": b")
	if _, _, err := squashAutoSavesLocked(ctx, dir, func(int) string { return "turn" }, "", false); err != nil {
		t.Fatal(err)
	}
	if got, _, err := readFileAt(dir, "world.sdf", rev); err != nil || string(got) != "v1" {
		t.Fatalf("a just-squashed revision must stay readable: %q %v", got, err)
	}
}
