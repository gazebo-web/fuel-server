package projects

import (
	"context"
	"errors"
	"net/http"
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
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/jinzhu/gorm"
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

	// The release decides the SDF version and ROS 2 distro; any values the
	// caller sent for those are replaced.
	var release string
	if gazebo != nil {
		release = *gazebo
	}
	release, versions, err := LookupRelease(release)
	if err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorFormInvalidValue, err)
	}
	gazebo, sdf, ros2 = &release, &versions.SDF, &versions.ROS2

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
	if rev, err := headRev(*p.Location); err == nil {
		p.HeadRev = rev
	}
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
	if u.SDFVersion != nil {
		updates["sdf_version"] = *u.SDFVersion
	}
	if u.ROS2Distro != nil {
		updates["ros2_distro"] = *u.ROS2Distro
	}
	if u.GazeboVersion != nil {
		release, versions, err := LookupRelease(*u.GazeboVersion)
		if err != nil {
			return nil, gz.NewErrorMessageWithBase(gz.ErrorFormInvalidValue, err)
		}
		updates["gazebo_version"] = release
		updates["sdf_version"] = versions.SDF
		updates["ros2_distro"] = versions.ROS2
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

// WriteFileOptions controls WriteFileWithOptions.
type WriteFileOptions struct {
	// CommitMessage labels the commit. Agent and chat edits pass an
	// auto-save message (see IsAutoSave) so labeled saves and the idle
	// squash can fold them.
	CommitMessage string
	// SquashAutoSaves folds the run of auto-save commits at HEAD, plus
	// this write, into one commit labeled CommitMessage (a user's Save).
	SquashAutoSaves bool
	// BaseRev, when set, rejects the write if the file changed since that
	// revision (optimistic concurrency, design §3.2.4).
	BaseRev string
	// Amend folds the write into HEAD instead of adding a version. Used for
	// thumbnails so they don't create history entries.
	Amend bool
}

// WriteResult describes the commit a write produced.
type WriteResult struct {
	Version int    `json:"version"`
	Rev     string `json:"rev"`
	// Squashed is how many auto-save commits a labeled save folded in.
	Squashed int `json:"squashed,omitempty"`
}

// writeAccess loads the project, checks the caller may write it, and
// takes the repo lock. The returned unlock must be called.
func (s *Service) writeAccess(ctx context.Context, tx *gorm.DB, owner, name string,
	jwtUser *users.User) (*Project, func(), *gz.ErrMsg) {
	p, em := s.Get(tx, owner, name, jwtUser)
	if em != nil {
		return nil, nil, em
	}
	if !s.canWrite(p, jwtUser) {
		return nil, nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}
	unlock, err := lockRepo(*p.Location)
	if err != nil {
		return nil, nil, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	// Self-heal repos left uninitialized by older fuel-server builds.
	if em := ensureRepoInitialized(ctx, p); em != nil {
		unlock()
		return nil, nil, em
	}
	return p, unlock, nil
}

// finishWrite fills in the version number and, for auto-saves, restarts
// the project's idle-squash timer.
func finishWrite(ctx context.Context, p *Project, rev, message string, squashed int) (WriteResult, *gz.ErrMsg) {
	if IsAutoSave(message) {
		scheduleIdleSquash(*p.Location)
	}
	// NOTE: Project.UpdatedAt is deliberately not bumped on file saves;
	// git is the source of truth for content changes (journal Q-MCP-9).
	v, err := res.GetLatestVersion(ctx, p)
	if err != nil {
		return WriteResult{}, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}
	return WriteResult{Version: v, Rev: rev, Squashed: squashed}, nil
}

func authorOf(jwtUser *users.User) string {
	if jwtUser != nil && jwtUser.Username != nil && *jwtUser.Username != "" {
		return *jwtUser.Username
	}
	return gitAuthor
}

// WriteFileWithOptions writes one file into the project repo and commits.
func (s *Service) WriteFileWithOptions(ctx context.Context, tx *gorm.DB, owner, name, relPath string,
	content []byte, jwtUser *users.User, opts WriteFileOptions) (WriteResult, *gz.ErrMsg) {
	return s.WriteFiles(ctx, tx, owner, name, map[string][]byte{relPath: content}, nil, jwtUser, opts)
}

// WriteFiles writes several files as one commit, after emptying each of
// replaceDirs (e.g. a regenerated model's folder, so files it no longer
// has don't linger). opts.BaseRev is checked for every written file.
func (s *Service) WriteFiles(ctx context.Context, tx *gorm.DB, owner, name string, files map[string][]byte,
	replaceDirs []string, jwtUser *users.User, opts WriteFileOptions) (WriteResult, *gz.ErrMsg) {

	if len(files) == 0 && len(replaceDirs) == 0 {
		return WriteResult{}, repoErr(errors.New("path is required"))
	}
	for rel := range files {
		if _, err := cleanRelPath(rel); err != nil {
			return WriteResult{}, repoErr(err)
		}
	}
	p, unlock, em := s.writeAccess(ctx, tx, owner, name, jwtUser)
	if em != nil {
		return WriteResult{}, em
	}
	defer unlock()
	for rel := range files {
		clean, _ := cleanRelPath(rel)
		if err := checkBaseRev(*p.Location, clean, opts.BaseRev); err != nil {
			return WriteResult{}, repoErr(err)
		}
	}

	author := authorOf(jwtUser)
	var rev string
	var err error
	squashed := 0
	if opts.SquashAutoSaves {
		if err := applyFilesLocked(*p.Location, files, replaceDirs); err != nil {
			return WriteResult{}, repoErr(err)
		}
		rev, squashed, err = squashAutoSavesLocked(ctx, *p.Location,
			func(int) string { return opts.CommitMessage }, author, true)
	} else {
		rev, err = writeFilesLocked(ctx, *p.Location, files, replaceDirs, opts.CommitMessage, author, opts.Amend)
	}
	if err != nil {
		return WriteResult{}, repoErr(err)
	}
	return finishWrite(ctx, p, rev, opts.CommitMessage, squashed)
}

// EditFile replaces the one occurrence of oldStr with newStr in a project
// file and commits, rejecting the edit if the file changed since baseRev.
func (s *Service) EditFile(ctx context.Context, tx *gorm.DB, owner, name, relPath,
	oldStr, newStr, baseRev, message string, jwtUser *users.User) (WriteResult, *gz.ErrMsg) {

	p, unlock, em := s.writeAccess(ctx, tx, owner, name, jwtUser)
	if em != nil {
		return WriteResult{}, em
	}
	defer unlock()
	rev, err := editFileLocked(ctx, *p.Location, relPath, oldStr, newStr, baseRev, message, authorOf(jwtUser))
	if err != nil {
		return WriteResult{}, repoErr(err)
	}
	return finishWrite(ctx, p, rev, message, 0)
}

// Checkpoint records a labeled version, folding any trailing auto-saves
// into it. With nothing to fold it still records the label.
func (s *Service) Checkpoint(ctx context.Context, tx *gorm.DB, owner, name, message string,
	jwtUser *users.User) (WriteResult, *gz.ErrMsg) {

	if strings.TrimSpace(message) == "" {
		return WriteResult{}, gz.NewErrorMessageWithBase(gz.ErrorFormInvalidValue,
			errors.New("message is required"))
	}
	p, unlock, em := s.writeAccess(ctx, tx, owner, name, jwtUser)
	if em != nil {
		return WriteResult{}, em
	}
	defer unlock()
	rev, n, err := squashAutoSavesLocked(ctx, *p.Location,
		func(int) string { return message }, authorOf(jwtUser), true)
	if err != nil {
		return WriteResult{}, repoErr(err)
	}
	return finishWrite(ctx, p, rev, message, n)
}

// ReadFile returns a project file at rev ("" or "tip" = HEAD) and the
// resolved commit hash.
func (s *Service) ReadFile(ctx context.Context, tx *gorm.DB, owner, name, relPath, rev string,
	jwtUser *users.User) ([]byte, string, *gz.ErrMsg) {

	p, em := s.Get(tx, owner, name, jwtUser)
	if em != nil {
		return nil, "", em
	}
	b, resolved, err := readFileAt(*p.Location, relPath, rev)
	if err != nil {
		return nil, "", repoErr(err)
	}
	return b, resolved, nil
}

// ListFiles returns the project's file paths at rev and the resolved hash.
func (s *Service) ListFiles(ctx context.Context, tx *gorm.DB, owner, name, rev string,
	jwtUser *users.User) ([]string, string, *gz.ErrMsg) {

	p, em := s.Get(tx, owner, name, jwtUser)
	if em != nil {
		return nil, "", em
	}
	files, resolved, err := listFilesAt(*p.Location, rev)
	if err != nil {
		return nil, "", repoErr(err)
	}
	return files, resolved, nil
}

// repoErr maps repo-level errors to API errors with useful status codes.
func repoErr(err error) *gz.ErrMsg {
	var em *gz.ErrMsg
	switch {
	case errors.Is(err, ErrStaleRev), errors.Is(err, ErrUnknownRev):
		em = gz.NewErrorMessageWithBase(gz.ErrorFormInvalidValue, err)
		em.StatusCode = http.StatusConflict
	case errors.Is(err, ErrTooLarge):
		em = gz.NewErrorMessageWithBase(gz.ErrorFormInvalidValue, err)
		em.StatusCode = http.StatusRequestEntityTooLarge
	case errors.Is(err, ErrFileNotFound):
		em = gz.NewErrorMessageWithBase(gz.ErrorNameNotFound, err)
		em.StatusCode = http.StatusNotFound
	case errors.Is(err, ErrNoMatch), errors.Is(err, ErrAmbiguousMatch), errors.Is(err, ErrBinary),
		strings.HasPrefix(err.Error(), "invalid path"), err.Error() == "path is required":
		em = gz.NewErrorMessageWithBase(gz.ErrorFormInvalidValue, err)
		em.StatusCode = http.StatusBadRequest
	default:
		em = gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	return em
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
