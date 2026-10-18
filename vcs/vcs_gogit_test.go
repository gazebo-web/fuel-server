package vcs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func headFiles(t *testing.T, repo VCS) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := repo.Walk(context.Background(), "", false, func(path, parent string, isDir bool) error {
		b, err := repo.GetFile(context.Background(), "", strings.TrimPrefix(path, "/"))
		if err != nil {
			return err
		}
		out[path] = string(*b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// ReplaceFiles makes the given folder's files the repo's new HEAD: changed
// files change, removed ones go, new ones (also in new folders) appear.
func TestReplaceFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"model.config": "v1", "model.sdf": "<sdf/>", "meshes/a.dae": "a"})
	repo := GoGitVCS{}.NewRepo(dir)
	if err := repo.InitRepo(ctx); err != nil {
		t.Fatal(err)
	}

	next := t.TempDir()
	writeFiles(t, next, map[string]string{"model.config": "v2", "materials/t.png": "png"})
	if err := repo.ReplaceFiles(ctx, next, "someone"); err != nil {
		t.Fatalf("ReplaceFiles: %+v", err)
	}
	got := headFiles(t, GoGitVCS{}.NewRepo(dir))
	var names []string
	for n := range got {
		names = append(names, n)
	}
	sort.Strings(names)
	want := []string{"/materials/t.png", "/model.config"}
	if len(names) != len(want) || names[0] != want[0] || names[1] != want[1] || got["/model.config"] != "v2" {
		t.Fatalf("HEAD files %v (%v), want %v", names, got, want)
	}
	n, err := repo.RevisionCount(ctx, "")
	if err != nil || n != 2 {
		t.Fatalf("revisions %d, %v", n, err)
	}

	// The same files again still make a new version.
	if err := repo.ReplaceFiles(ctx, next, "someone"); err != nil {
		t.Fatalf("ReplaceFiles with unchanged files: %+v", err)
	}
	if n, err = repo.RevisionCount(ctx, ""); err != nil || n != 3 {
		t.Fatalf("revisions %d, %v", n, err)
	}
}
