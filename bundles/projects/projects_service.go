package projects

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	res "github.com/gazebo-web/fuel-server/bundles/common_resources"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

// AutoSaveMessage is the canonical commit message used by
// ai-builder-service when the chat engine auto-snapshots a project.
// The server-side squash path keys on this exact string to find the
// run of auto-saves it should collapse.
const AutoSaveMessage = "auto-save"

const (
	gitAuthor = "fuel-server-projects"
	gitEmail  = "projects@fuel-server.local"
)

// Service exposes the project operations.
type Service struct{}

// Create inserts a new project row, creates the on-disk folder, initializes
// a git repo, and records the initial empty commit. Returns the populated
// Project on success. The caller must hold a valid transaction; the service
// does not manage commit/rollback semantics.
func (s *Service) Create(ctx context.Context, tx *gorm.DB, jwtUser *users.User,
	name, desc, gazebo, sdf, ros2 *string) (*Project, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	if name == nil || *name == "" {
		return nil, gz.NewErrorMessage(gz.ErrorNameWrongFormat)
	}

	owner := *jwtUser.Username

	// Reject duplicate names for this owner.
	if existing, _ := GetProjectByName(tx, *name, owner); existing != nil {
		return nil, gz.NewErrorMessage(gz.ErrorResourceExists)
	}

	proj, err := NewProjectAndUUID(name, desc, &owner, jwtUser.Username, gazebo, sdf, ros2)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	// Ensure the owner's projects/ subfolder exists, then the project's own
	// folder. CreateOwnerFolder only seeds models/worlds/collections, so we
	// handle the projects/ tree ourselves.
	if err := os.MkdirAll(filepath.Dir(*proj.Location), 0711); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorCreatingDir, err)
	}
	if err := os.MkdirAll(*proj.Location, 0711); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorCreatingDir, err)
	}

	// Seed the repo with an empty .gitignore so the initial commit has
	// something to record, then tag it with the UUID so the existing
	// GetLatestVersion helper can count revisions correctly.
	placeholder := filepath.Join(*proj.Location, ".gitignore")
	if err := os.WriteFile(placeholder, []byte{}, 0644); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorCreatingFile, err)
	}
	repo := globals.VCSRepoFactory(ctx, *proj.Location)
	if err := repo.InitRepo(ctx); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	// Newer git installs default `init.defaultBranch` to "main"; the
	// shared common_resources.GetLatestVersion helper hardcodes "master"
	// as the branch to count commits on, so we'd 500 on every read of a
	// project file. `git branch -M master` force-renames whatever the
	// current branch is to "master" — a no-op when we're already there,
	// and a rename when the host defaulted to "main".
	if err := exec.CommandContext(ctx, "git",
		"-C", *proj.Location, "branch", "-M", "master").Run(); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorRepo,
			errors.New("git branch -M master: "+err.Error()))
	}
	if err := repo.Tag(ctx, *proj.UUID); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}

	if err := tx.Create(&proj).Error; err != nil {
		// Best-effort cleanup on DB failure so a retried Create can succeed.
		_ = os.RemoveAll(*proj.Location)
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	return &proj, nil
}

// Get loads a project by (owner, name) and enforces owner-only visibility.
// Projects are never public, so any caller other than the owner is
// rejected with 403. A system admin can read any project.
func (s *Service) Get(tx *gorm.DB, owner, name string,
	jwtUser *users.User) (*Project, *gz.ErrMsg) {

	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	p, err := GetProjectByName(tx, name, owner)
	if err != nil {
		return nil, gz.NewErrorMessageWithArgs(gz.ErrorNameNotFound, err, []string{name})
	}
	if !s.canRead(p, jwtUser) {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}
	// Repos created before the master-branch fix may have HEAD pointing
	// at "main"; common_resources.GetLatestVersion hardcodes "master"
	// and 500s in that case. Repair lazily on first read. The hot-path
	// reference check is a cheap go-git lookup; the rename is only
	// shelled out when we actually find drift.
	ensureMasterBranch(p)
	return p, nil
}

// ensureRepoInitialized makes p.Location a valid git repo even if it was
// never initialized (older fuel-server builds had a Create path where
// InitRepo silently no-op'd). Idempotent: if the repo already opens
// cleanly we return early. Otherwise we shell out to `git init` + force
// the branch to "master" + stage and commit any pre-existing files.
// Returns a typed gz.ErrMsg so callers fold it into the regular error
// handling path.
func ensureRepoInitialized(ctx context.Context, p *Project) *gz.ErrMsg {
	if p == nil || p.Location == nil || *p.Location == "" {
		return gz.NewErrorMessage(gz.ErrorUnexpected)
	}
	if _, err := git.PlainOpen(*p.Location); err == nil {
		return nil
	}
	// Bring the folder up to a usable state. Each step is fatal —
	// returning an error here surfaces the underlying git stderr to the
	// frontend via gzMcpError.
	if err := exec.CommandContext(ctx, "git",
		"init", *p.Location).Run(); err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorRepo,
			errors.New("git init: "+err.Error()))
	}
	if err := exec.CommandContext(ctx, "git",
		"-C", *p.Location, "branch", "-M", "master").Run(); err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorRepo,
			errors.New("git branch -M master: "+err.Error()))
	}
	if err := exec.CommandContext(ctx, "git",
		"-C", *p.Location, "add", "-A").Run(); err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorRepo,
			errors.New("git add -A: "+err.Error()))
	}
	commit := exec.CommandContext(ctx, "git",
		"-C", *p.Location, "-c", "user.name="+gitAuthor,
		"-c", "user.email="+gitEmail,
		"commit", "--allow-empty", "-m", "self-heal: initialize repo")
	if err := commit.Run(); err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorRepo,
			errors.New("git commit: "+err.Error()))
	}
	if p.UUID != nil && *p.UUID != "" {
		// Tag with the UUID so res.GetLatestVersion's revision-counting
		// helper finds the same anchor it expects from a regular Create.
		_ = exec.CommandContext(ctx, "git",
			"-C", *p.Location, "tag", *p.UUID).Run()
	}
	return nil
}

// ensureMasterBranch makes the repo at p.Location have a "master"
// branch even if it was originally initialized with "main" (the default
// on git ≥ 2.28). Idempotent and best-effort — failures are logged but
// don't block the read.
func ensureMasterBranch(p *Project) {
	if p == nil || p.Location == nil || *p.Location == "" {
		return
	}
	repo, err := git.PlainOpen(*p.Location)
	if err != nil {
		return
	}
	if _, err := repo.Reference(plumbing.NewBranchReferenceName("master"), true); err == nil {
		return // master already exists; nothing to do
	}
	// Slow path: shell out to rename. Runs at most once per repo lifetime
	// (subsequent calls hit the early-return above).
	_ = exec.Command("git", "-C", *p.Location, "branch", "-M", "master").Run()
}

// GetForCaller is a convenience wrapper that infers the owner from the
// authenticated user. Used by routes where the URL doesn't carry an owner.
func (s *Service) GetForCaller(tx *gorm.DB, name string,
	jwtUser *users.User) (*Project, *gz.ErrMsg) {
	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	return s.Get(tx, *jwtUser.Username, name, jwtUser)
}

// List returns the caller's projects. Pagination is applied at the DB level.
func (s *Service) List(tx *gorm.DB, jwtUser *users.User, limit, offset int) (Projects, *gz.ErrMsg) {
	if jwtUser == nil {
		return nil, gz.NewErrorMessage(gz.ErrorAuthNoUser)
	}
	list, err := ListForOwner(tx, *jwtUser.Username, limit, offset)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}
	return list, nil
}

// ProjectUpdate collects the patchable fields of a Project. nil entries
// mean "don't touch"; non-nil entries (including empty strings) are
// written through. Kept as an explicit struct rather than a long
// positional arg list so new fields land without cascading signature
// changes.
type ProjectUpdate struct {
	Description   *string
	GazeboVersion *string
	SDFVersion    *string
	ROS2Distro    *string
	GitHubRepo    *string
	GazeboWSURL   *string
	GazeboWSKey   *string
}

// Update patches a project's metadata. Only fields passed as non-nil are
// updated. The caller must be the owner (or a system admin).
func (s *Service) Update(tx *gorm.DB, owner, name string, jwtUser *users.User,
	u ProjectUpdate) (*Project, *gz.ErrMsg) {

	p, em := s.Get(tx, owner, name, jwtUser)
	if em != nil {
		return nil, em
	}
	if !s.canWrite(p, jwtUser) {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}

	updates := map[string]interface{}{}
	if u.Description != nil {
		updates["description"] = *u.Description
	}
	if u.GazeboVersion != nil {
		updates["gazebo_version"] = *u.GazeboVersion
	}
	if u.SDFVersion != nil {
		updates["sdf_version"] = *u.SDFVersion
	}
	if u.ROS2Distro != nil {
		updates["ros2_distro"] = *u.ROS2Distro
	}
	if u.GitHubRepo != nil {
		updates["git_hub_repo"] = *u.GitHubRepo
	}
	if u.GazeboWSURL != nil {
		updates["gazebo_ws_url"] = *u.GazeboWSURL
	}
	if u.GazeboWSKey != nil {
		updates["gazebo_ws_key"] = *u.GazeboWSKey
	}
	if len(updates) == 0 {
		return p, nil
	}
	updates["updated_at"] = time.Now()
	if err := tx.Model(p).Updates(updates).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}
	return p, nil
}

// Delete soft-deletes the row. The on-disk repo is left in place (mirrors
// Model / World behavior) so history can be recovered operationally.
func (s *Service) Delete(tx *gorm.DB, owner, name string, jwtUser *users.User) *gz.ErrMsg {
	p, em := s.Get(tx, owner, name, jwtUser)
	if em != nil {
		return em
	}
	if !s.canWrite(p, jwtUser) {
		return gz.NewErrorMessage(gz.ErrorUnauthorized)
	}
	if err := tx.Delete(p).Error; err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}
	return nil
}

// VersionInfo summarizes one commit in the project's git log.
type VersionInfo struct {
	Version       int       `json:"version"`
	CommitHash    string    `json:"commit_hash"`
	CommitMessage string    `json:"commit_message"`
	Author        string    `json:"author"`
	CreatedAt     time.Time `json:"created_at"`
}

// ListVersions walks the project's git log and returns one VersionInfo per
// commit, newest first. Version 1 is the initial commit.
func (s *Service) ListVersions(ctx context.Context, tx *gorm.DB, owner, name string,
	jwtUser *users.User) ([]VersionInfo, *gz.ErrMsg) {

	p, em := s.Get(tx, owner, name, jwtUser)
	if em != nil {
		return nil, em
	}
	repo, err := git.PlainOpen(*p.Location)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	head, err := repo.Head()
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}

	// Walk in natural (newest-first) order; count total commits to assign
	// monotonic version numbers where the oldest = 1.
	commits := []*object.Commit{}
	_ = iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, c)
		return nil
	})
	total := len(commits)
	out := make([]VersionInfo, 0, total)
	for i, c := range commits {
		out = append(out, VersionInfo{
			Version:       total - i,
			CommitHash:    c.Hash.String(),
			CommitMessage: c.Message,
			Author:        c.Author.Name,
			CreatedAt:     c.Author.When.UTC(),
		})
	}
	return out, nil
}

// GetFile reads a single file out of the project's repo at the given
// version string ("tip"/""/"1"/"2"/...). Returns bytes + resolved version.
func (s *Service) GetFile(ctx context.Context, tx *gorm.DB, owner, name, path, version string,
	jwtUser *users.User) (*[]byte, int, *gz.ErrMsg) {

	p, em := s.Get(tx, owner, name, jwtUser)
	if em != nil {
		return nil, 0, em
	}
	return res.GetFile(ctx, p, path, version)
}

// WriteFileOptions controls the behavior of WriteFile. Carries a struct
// rather than positional args so future flags (squash, amend, ...) land
// without cascading signature changes.
type WriteFileOptions struct {
	// CommitMessage is the commit message to record for the save. For
	// auto-saves callers should pass projects.AutoSaveMessage so the
	// squash path can key off it.
	CommitMessage string
	// SquashAutoSaves, when true, rewrites history before committing.
	// Any run of consecutive AutoSaveMessage commits at HEAD is
	// collapsed into this one save. Useful for the manual "Save with
	// label" flow that follows a burst of chat-engine auto-saves.
	SquashAutoSaves bool
}

// WriteFile writes a single file into the project's repo at HEAD, stages it,
// and commits. The commit message is used as-is (callers supply either an
// auto-save tag or a user-provided label). Returns the new version number.
//
// Thin backward-compatible wrapper over WriteFileWithOptions.
func (s *Service) WriteFile(ctx context.Context, tx *gorm.DB, owner, name, relPath string,
	content []byte, commitMessage string, jwtUser *users.User) (int, *gz.ErrMsg) {
	return s.WriteFileWithOptions(ctx, tx, owner, name, relPath, content, jwtUser,
		WriteFileOptions{CommitMessage: commitMessage})
}

// WriteFileWithOptions is the full form. See WriteFileOptions.
func (s *Service) WriteFileWithOptions(ctx context.Context, tx *gorm.DB, owner, name, relPath string,
	content []byte, jwtUser *users.User, opts WriteFileOptions) (int, *gz.ErrMsg) {

	p, em := s.Get(tx, owner, name, jwtUser)
	if em != nil {
		return 0, em
	}
	if !s.canWrite(p, jwtUser) {
		return 0, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}
	if relPath == "" {
		return 0, gz.NewErrorMessageWithBase(gz.ErrorNameWrongFormat,
			errors.New("path is required"))
	}
	// Disallow absolute paths and "..". Only POSIX-style separators inside
	// the repo root are accepted.
	clean := filepath.ToSlash(filepath.Clean(relPath))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." ||
		clean[0] == '/' || bytes.Contains([]byte(clean), []byte("..")) {
		return 0, gz.NewErrorMessageWithBase(gz.ErrorNameWrongFormat,
			errors.New("invalid path: "+relPath))
	}

	// Serialize every git mutation below against other writers of this
	// project, in this process or the ai-builder agent.
	unlock, err := lockRepo(*p.Location)
	if err != nil {
		return 0, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	defer unlock()

	// If the caller asked us to squash, first rewind HEAD past the run of
	// auto-save commits at the tip. The reset is destructive (those
	// commits disappear from history) — intentional; they only existed
	// because we were auto-checkpointing a build turn and the user is now
	// labeling the current state as the canonical save.
	//
	// Squash is best-effort: if the git reset fails for any reason
	// (permission, working-tree dirty, missing branch, ...) we log and
	// fall through to the regular write+commit path. Better to land a
	// labeled commit on top of the auto-save run than to abort the whole
	// Save and leave the user's work uncommitted.
	if opts.SquashAutoSaves {
		if em := s.rewindAutoSaves(ctx, p); em != nil {
			cause := em.Msg
			if em.BaseError != nil {
				cause = cause + ": " + em.BaseError.Error()
			}
			gz.LoggerFromContext(ctx).Warning(
				"projects: squash failed for " + *p.Name + "; falling back to plain commit: " + cause,
			)
		}
	}

	fullPath := filepath.Join(*p.Location, clean)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return 0, gz.NewErrorMessageWithBase(gz.ErrorCreatingDir, err)
	}
	if err := os.WriteFile(fullPath, content, 0644); err != nil {
		return 0, gz.NewErrorMessageWithBase(gz.ErrorCreatingFile, err)
	}

	// Self-heal: a project row pointing at a folder with no .git has been
	// observed for projects created by older fuel-server builds where
	// InitRepo silently no-op'd. Rather than blocking every Save with
	// "repository does not exist", lazily initialize the repo here. Any
	// pre-existing files in the folder (including the bytes we just
	// wrote) are picked up by the initial commit.
	if em := ensureRepoInitialized(ctx, p); em != nil {
		return 0, em
	}

	// Stage + commit via go-git.
	repo, err := git.PlainOpen(*p.Location)
	if err != nil {
		return 0, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	w, err := repo.Worktree()
	if err != nil {
		return 0, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	if _, err := w.Add(clean); err != nil {
		return 0, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	authorName := gitAuthor
	if jwtUser != nil && jwtUser.Username != nil && *jwtUser.Username != "" {
		authorName = *jwtUser.Username
	}
	commitMessage := opts.CommitMessage
	// Skip the commit if staging produced no changes (writing the same
	// content that's already in HEAD). Checking status before commit is
	// simpler and cross-version than catching an empty-commit error from
	// go-git, which varies across library versions.
	status, err := w.Status()
	if err != nil {
		return 0, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	if !status.IsClean() {
		if _, err := w.Commit(commitMessage, &git.CommitOptions{
			Author: &object.Signature{
				Name:  authorName,
				Email: gitEmail,
				When:  time.Now(),
			},
		}); err != nil {
			return 0, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
		}
	}

	// NOTE: we deliberately do not bump Project.UpdatedAt on file saves.
	// Git is the authoritative source of "last changed" for file content
	// (HEAD commit time). Touching the DB here would churn the row on
	// every chat tool call without adding signal. List sort order uses
	// the DB's updated_at which only moves on metadata patches; that's
	// intentional. See journal Q-MCP-9.
	latest, err := res.GetLatestVersion(ctx, p)
	if err != nil {
		return 0, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}
	return latest, nil
}

// rewindAutoSaves inspects the project's repo, counts the run of
// consecutive AutoSaveMessage commits anchored at HEAD, and hard-resets
// the working branch to the parent of the earliest one. A no-op if HEAD
// is not an auto-save commit or if resetting would orphan the repo
// (there's only one commit in history).
//
// Implementation note: we shell out to `git reset` because go-git's
// storer-level branch-ref rewrites across worktree state are fiddly and
// the plain-git binary is already a runtime dep of fuel-server (see
// vcs/vcs.go for the same fallback pattern).
func (s *Service) rewindAutoSaves(ctx context.Context, p *Project) *gz.ErrMsg {
	repo, err := git.PlainOpen(*p.Location)
	if err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	head, err := repo.Head()
	if err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	defer iter.Close()

	// Walk the log from HEAD; remember the first commit whose message is
	// NOT an auto-save. That's where we'll reset to. If every commit
	// back to the initial one is an auto-save we leave history alone —
	// there's nothing to anchor to.
	var target *object.Commit
	count := 0
	for {
		c, err := iter.Next()
		if err != nil {
			break
		}
		msg := strings.TrimSpace(c.Message)
		if msg == AutoSaveMessage {
			count++
			continue
		}
		target = c
		break
	}
	if count == 0 || target == nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "reset", "--mixed", target.Hash.String())
	cmd.Dir = *p.Location
	if out, err := cmd.CombinedOutput(); err != nil {
		return gz.NewErrorMessageWithBase(gz.ErrorRepo,
			errors.New("git reset: "+err.Error()+": "+string(out)))
	}
	return nil
}

// -- helpers ----------------------------------------------------------------

// canRead encodes the "owner-only, except system admins" rule.
func (s *Service) canRead(p *Project, jwtUser *users.User) bool {
	return s.isOwnerOrAdmin(p, jwtUser)
}

// canWrite is the same policy as canRead for Phase 2; kept separate so we
// can narrow it in Phase 3 (e.g. read-only PATs).
func (s *Service) canWrite(p *Project, jwtUser *users.User) bool {
	return s.isOwnerOrAdmin(p, jwtUser)
}

func (s *Service) isOwnerOrAdmin(p *Project, jwtUser *users.User) bool {
	if jwtUser == nil || jwtUser.Username == nil || p.Owner == nil {
		return false
	}
	if *p.Owner == *jwtUser.Username {
		return true
	}
	return globals.Permissions.IsSystemAdmin(*jwtUser.Username)
}

// revisionPath returns the parsed version number from a string. "tip" or ""
// mean the latest commit. Used by callers that need integer version ids.
func revisionPath(rev string) (int, error) {
	if rev == "" || rev == "tip" {
		return 0, nil
	}
	n, err := strconv.Atoi(rev)
	if err != nil || n <= 0 {
		return 0, errors.New("invalid version: " + rev)
	}
	return n, nil
}

// unused today, kept so other helpers can reference it without stutter.
var _ = plumbing.Revision("")
var _ = path.Join
