package vcs

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"gopkg.in/src-d/go-billy.v4/osfs"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/storage/filesystem"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	gitName  = "ign-fuelserver"
	gitEmail = "ign-fuelserver@test.org"
)

// GoGitVCS represents a local Git repo.
type GoGitVCS struct {
	Path string
	r    *git.Repository
}

// NewRepo creates a new GoGitVCS repository object.
func (g GoGitVCS) NewRepo(dirpath string) VCS {
	repo := GoGitVCS{Path: dirpath}
	r, err := git.PlainOpen(dirpath)
	if err == nil {
		repo.r = r
	}
	return &repo
}

// InitRepo - Inits the version control repository and commits all
// files found. Git Path must exist.
func (g *GoGitVCS) InitRepo(ctx context.Context) error {

	// TODO, FIXME: For some reason that I quite don't understand,
	// when the Git repo "init" is done using go-git, the newly created git
	// repository ends in a state in which "git archive" does not work. The
	// error is "error: Invalid path '.git/HEAD'". In addition, the ".git"
	// folder has fewer elements VS. when created with plain "git init" command.
	// For now, I'm disabling the repo initialization using go-git and delegating
	// to plain git init command impl.

	//return g.goGitInitAndCommitAll("Created repository")

	// fallback to git command implementation
	r := GitVCS{}.NewRepo(g.Path)
	if err := r.InitRepo(ctx); err != nil {
		return err
	}
	// Go-Git'Open the repo
	gogitRepo, err := git.PlainOpen(g.Path)
	if err == nil {
		g.r = gogitRepo
	}
	return nil
}

func (g *GoGitVCS) isRepoOpen() bool {
	return g.r != nil
}

func (g *GoGitVCS) assertValidRepo() error {
	if err := ensureFolderExists(g.Path); err != nil {
		return err
	}
	// ensure Repo is go-git "Open"
	if !g.isRepoOpen() {
		return errors.New("Repo does not exist. Repo: " + g.Path)
	}
	return nil
}

func (g *GoGitVCS) goGitInitAndCommitAll(ctx context.Context, msg string) error {
	if err := ensureFolderExists(g.Path); err != nil {
		return err
	}
	if g.isRepoOpen() {
		return errors.New("Repo already exists. Repo: " + g.Path)
	}
	fs := osfs.New(g.Path)
	dot, _ := fs.Chroot(".git")
	storage, err := filesystem.NewStorage(dot)
	if err != nil {
		return ign.WithStack(err)
	}
	r, err := git.Init(storage, fs)
	if err != nil {
		err = ign.WithStack(err)
		ign.LoggerFromContext(ctx).Info("Error while doing git Init. Err: " + fmt.Sprint(err) + ". Repo: " + g.Path)
		return err
	}
	g.r = r

	// Add all files to git index
	w, _ := r.Worktree()
	if _, err := w.Add("."); err != nil {
		return ign.WithStack(err)
	}

	// Commit
	_, err = w.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  gitName,
			Email: gitEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return ign.WithStack(err)
	}
	return nil
}

// getCommit gets a commit object from the given revision. If revision is empty
// or "tip" then master is used.
func (g *GoGitVCS) getCommit(ctx context.Context, rev string) (*object.Commit, error) {
	rev = ensureRev(rev)
	// Get revision's hash
	pr := plumbing.Revision(rev)
	hash, err := g.r.ResolveRevision(pr)
	if err != nil {
		err = ign.WithStack(err)
		ign.LoggerFromContext(ctx).Info("Error while getting hash. Err: " + fmt.Sprint(err))
		return nil, err
	}
	// retrieve the commit object
	commit, err := g.r.CommitObject(*hash)
	if err != nil {
		err = ign.WithStack(err)
		ign.LoggerFromContext(ctx).Info("Error while getting commit. Err: " + fmt.Sprint(err) + ". Repo: " + g.Path)
		return nil, err
	}
	return commit, nil
}

// GetFile - Gets a single file with a given revision from the repo.
func (g *GoGitVCS) GetFile(ctx context.Context, rev string, pathFromRoot string) (*[]byte, error) {
	if err := g.assertValidRepo(); err != nil {
		return nil, err
	}
	rev = ensureRev(rev)
	commit, err := g.getCommit(ctx, rev)
	if err != nil {
		return nil, err
	}
	// retrieve the tree from the commit
	f, err := commit.File(pathFromRoot)
	if err != nil {
		err = ign.WithStack(err)
		ign.LoggerFromContext(ctx).Info("Error while getting File. Err: " + fmt.Sprint(err) + ". Repo: " + g.Path)
		return nil, err
	}
	reader, err := f.Reader()
	if err != nil {
		err = ign.WithStack(err)
		ign.LoggerFromContext(ctx).Info("Error while getting file Reader. Err: " + fmt.Sprint(err) + ". Repo: " + g.Path)
		return nil, err
	}
	var bs []byte
	bs, err = ioutil.ReadAll(reader)
	if err != nil {
		err = ign.WithStack(err)
	}
	return &bs, err
}

// Walk - given a revision, Walk func iterates over the repository and invokes the
// WalkFn on each leaf file. If includeFolders argument is true then WalkFn
// will be invoked with folder nodes too. The function returns error if any
// WalkFn invocation ended with error.
// Revision argument can be an empty string; in that case the master branch
// will be used.
func (g *GoGitVCS) Walk(ctx context.Context, rev string, includeFolders bool, fn WalkFn) error {
	if err := g.assertValidRepo(); err != nil {
		return err
	}
	rev = ensureRev(rev)
	commit, err := g.getCommit(ctx, rev)
	if err != nil {
		return err
	}
	// Get the files iterator
	iter, err := commit.Files()
	if err != nil {
		return ign.WithStack(err)
	}

	visitedFolders := make(map[string]bool)
	iter.ForEach(func(f *object.File) error {
		// Skip ".git" folder and its contents
		if strings.HasPrefix(f.Name, ".git") {
			return nil
		}
		// backward compatibility: exclude ".hg" folder too.
		if strings.HasPrefix(f.Name, ".hg") {
			return nil
		}
		parentPath := filepath.Clean(filepath.Dir(f.Name))
		// Visit folders?
		if includeFolders {
			folders := strings.Split(parentPath, "/")
			foPath := ""
			for _, foName := range folders {
				foParent := foPath
				if foParent == "" {
					foParent = "."
				}
				foPath = filepath.Join(foPath, foName)
				// Skip repo root and visited folders
				if foPath != "." && !visitedFolders[foPath] {
					visitedFolders[foPath] = true
					// Need to prefix all paths with "/" as client code is expecting that.
					if err := fn(filepath.Join("/", foPath), filepath.Join("/", foParent), true); err != nil {
						return ign.WithStack(err)
					}
				}
			}
		}
		// Need to prefix all paths with "/" as client code is expecting that.
		err := fn(filepath.Join("/", f.Name), filepath.Join("/", parentPath), false)
		if err != nil {
			return ign.WithStack(err)
		}
		return nil
	})
	return nil
}

// Zip - creates a zip with the repository files, at a given revision.
// If revision is empty or "tip" , last commit from "master" branch will
// be used. If output is empty, then a zip file in the tmp folder will be
// created.
// Returns a string path pointing to the created zip file.
func (g *GoGitVCS) Zip(ctx context.Context, rev, output string) (*string, error) {
	return archive(ctx, g.Path, rev, output)
}

// ReplaceFiles - replaces all files from repo HEAD with the files from the given folder.
// owner is an optional argument used to set the git commit user. If empty, then the default
// git user will be used.
func (g *GoGitVCS) ReplaceFiles(ctx context.Context, folder, owner string) error {
	if err := g.assertValidRepo(); err != nil {
		return err
	}
	// First, remove all files from master.
	w, err := g.r.Worktree()
	if err != nil {
		return ign.WithStack(err)
	}
	removeFn := func(path, parentPath string, isDir bool) error {
		if path == "/" || path == "/.gitignore" {
			// We don't remove the model root folder
			return nil
		}
		// remove "/" (root) prefix from path
		path = path[1:]
		fullpath := filepath.Join(g.Path, path)
		if _, err := os.Stat(fullpath); err == nil {
			if _, err := w.Remove(path); err != nil {
				return ign.WithStack(err)
			}
		}
		return nil
	}
	if err := g.Walk(ctx, "", true, removeFn); err != nil {
		return err
	}

	// Now, replace with files from given folder
	if err := CopyDir(folder, g.Path); err != nil {
		return err
	}

	// Then, Add all files to git index
	addFn := func(path string, f os.FileInfo, err error) error {
		// remove repo base path
		path = path[len(folder):]
		if path == "" || strings.Contains(path, ".hg") ||
			strings.Contains(path, ".git") || strings.Contains(path, ".gitignore") {
			return nil
		}
		// and trim "/" prefix
		path = path[1:]
		if _, err := w.Add(path); err != nil {
			return ign.WithStack(err)
		}
		return nil
	}
	if err := filepath.Walk(folder, addFn); err != nil {
		return ign.WithStack(err)
	}

	// Commit
	gitUser := owner
	if gitUser == "" {
		gitUser = gitName
	}

	_, err = w.Commit("ReplaceFiles - new version", &git.CommitOptions{
		Author: &object.Signature{
			Name:  gitUser,
			Email: gitEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return ign.WithStack(err)
	}
	return nil
}

// CloneTo - makes a local clone of repo into given target.
func (g *GoGitVCS) CloneTo(ctx context.Context, target string) error {
	// fallback to git command implementation
	r := GitVCS{}.NewRepo(g.Path)
	return r.CloneTo(ctx, target)
}

// Tag the current repository.
func (g *GoGitVCS) Tag(ctx context.Context, tag string) error {
	if err := g.assertValidRepo(); err != nil {
		return err
	}
	// create new tag from head
	headRef, err := g.r.Head()
	if err != nil {
		err = ign.WithStack(err)
		ign.LoggerFromContext(ctx).Info("Error while tagging repo. Err: " + fmt.Sprint(err) + ". Repo: " + g.Path)
		return err
	}
	completeTag := "refs/tags/" + tag
	ref := plumbing.NewHashReference(plumbing.ReferenceName(completeTag), headRef.Hash())
	err = g.r.Storer.SetReference(ref)
	if err != nil {
		err = ign.WithStack(err)
		ign.LoggerFromContext(ctx).Info("Error while tagging repo. Err: " + fmt.Sprint(err) + ". Repo: " + g.Path)
		return err
	}
	return nil
}

// HasTag - Checks for the existence of the given tag.
// Return bool, and an error if something unexpected happened.
func (g *GoGitVCS) HasTag(tag string) (bool, error) {
	if err := g.assertValidRepo(); err != nil {
		return false, err
	}
	completeTag := "refs/tags/" + tag
	_, err := g.r.Storer.Reference(plumbing.ReferenceName(completeTag))
	if err != nil {
		return false, nil
	}
	return true, nil
}

// RevisionCount - get the number of revisions up to a specific revision
// If revision is empty, last commit from "master" branch will be used.
// Returns the number of revisions.
func (g *GoGitVCS) RevisionCount(ctx context.Context, rev string) (int, error) {
	return getRevisionCount(ctx, g.Path, rev)
}
