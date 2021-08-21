package vcs

import (
	"bytes"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// TODO: consider having a repo.Rollback() to checkout and return to
// last working version.

// VCS - Version Control System basic interface.
type VCS interface {
	CloneTo(ctx context.Context, target string) error
	GetFile(ctx context.Context, rev string, pathFromRoot string) (*[]byte, error)
	InitRepo(ctx context.Context) error
	ReplaceFiles(ctx context.Context, folder, owner string) error
	RevisionCount(ctx context.Context, rev string) (int, error)
	Tag(ctx context.Context, tag string) error
	Walk(ctx context.Context, rev string, includeFolders bool, fn WalkFn) error
	Zip(ctx context.Context, rev, output string) (*string, error)
}

// WalkFn allows to process a repository file entry when using the Walk func.
// WalkFn receives a file and its folder parent paths. isDir argument is true
// when the given path is a folder.
type WalkFn (func(path, parent string, isDir bool) error)

// NewRepo creates a new GitVCS repository object.
func (g GitVCS) NewRepo(dirpath string) VCS {
	r := GitVCS{Path: dirpath}
	return &r
}


// CommndCallback is a command function used in the Operation struct
type CommandCallback func() (OperationResult)

// Operation is a list of commands to be executed in series
type Operation struct {
	CommandCb CommandCallback
}

// OperationResult stores the standard out and error msg of a command execution
type OperationResult struct {
	output string
	err	error
	stderr bytes.Buffer
}

// OperationHandler is a struct that contains other structs needed to execute
// an operation, including an operation channel, results channel and the states
// of the operation.
type OperationHandler struct {
	Operations chan Operation
	OperationResults chan OperationResult
	WG sync.WaitGroup
	stopOperationLoop bool
	initialized bool
}

// Init creates the operation channels and initializes the operatation states
func (o *OperationHandler) Init() {
	o.Operations = make(chan Operation, 1)
	o.OperationResults = make(chan OperationResult, 1)
	o.stopOperationLoop = false
	o.initialized = true
}

// GitVCS represents a local Git repo.
type GitVCS struct {
	Path string
	opHandler OperationHandler
}

func ensureFolderExists(dir string) error {
	_, err := os.Stat(dir)
	return ign.WithStack(err)
}

// make sure the revision is valid. If "tip" or an empty revision
// is given as argument, then this func returns "master"
func ensureRev(rev string) string {
	rev = strings.TrimSpace(rev)
	if rev == "" || rev == "tip" {
		return "master"
	}
	return rev
}

// InitRepo - Inits the version control repository and commits all
// files found. Git Path must exist.
func (g *GitVCS) InitRepo(ctx context.Context) error {
	err := g.initAndCommitAll(ctx, "Created repository")
	if err != nil {
		g.opHandler.stopOperationLoop = true
	}
    return err
}

// initAndCommitAll - Inits the version control repository and commits all
// files found. Git Path must exist.
func (g *GitVCS) initAndCommitAll(ctx context.Context, msg string) error {
	if err := g.Init(ctx); err != nil {
		return err
	}
	if err := g.addAll(ctx); err != nil {
		return err
	}
	if err := g.Commit(ctx, msg); err != nil {
		return err
	}
	return nil
}

// Init - initializes the version control repository.
// Git Path must exist.
func (g *GitVCS) Init(ctx context.Context) error {
	if err := ensureFolderExists(g.Path); err != nil {
		return err
	}
	// Init the git repository
	// An init command does not need to be queued so use exec.Command directly
	// instead of creating an Operation
	cmd := exec.Command("git", "init", g.Path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err != nil {
		err = ign.WithStack(err)
		ign.LoggerFromContext(ctx).Info("Error while running process. Err: " +
			fmt.Sprint(err) + ". Stderr: " + stderr.String())
	}
	return err
}

// GetFile - Gets a single file with a given revision from the repo.
func (g *GitVCS) GetFile(ctx context.Context, rev string, pathFromRoot string) (*[]byte, error) {
	ign.LoggerFromContext(ctx).Info("WARNING: ideally, we should not use the plain GitVCS implementation. Try to use GoGitVCS")
	if err := ensureFolderExists(g.Path); err != nil {
		return nil, err
	}
	rev = ensureRev(rev)

	// Create an cmd callback and execute it
	var cb = func() (OperationResult) {
		cmd := exec.Command("git", "-C", g.Path, "show", rev+":"+pathFromRoot)
		var bs []byte
		var err error
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		bs, err = cmd.Output()
		var result OperationResult
		result.output = string(bs[:])
		result.err = err
		result.stderr = stderr
		return result
	}

	result := g.opHandler.ExecuteOperation(cb);
	
	bs := []byte(result.output)
	var err error
	if result.err != nil {
		err = ign.WithStack(result.err)
		ign.LoggerFromContext(ctx).Info("Error while running process. Err: " +
			fmt.Sprint(result.err) + ". Stderr: " + result.stderr.String())
	}
	return &bs, err
}

// Walk - given a revision, Walk func iterates over the repository and invokes the
// WalkFn on each leaf file. If includeFolders argument is true then WalkFn
// will be invoked with folder nodes too. The function returns error if any
// WalkFn invocation ended with error.
// Revision argument can be an empty string; in that case "master" will be used.
func (g *GitVCS) Walk(ctx context.Context, rev string, includeFolders bool, fn WalkFn) error {
	// TODO: implement
	return errors.New("GitVCS's Walk function is not implemented yet")
}

// addAll - 'git add' all files and folders found in the repo path.
func (g *GitVCS) addAll(ctx context.Context) error {
	if err := ensureFolderExists(g.Path); err != nil {
		return err
	}

	// Create an cmd callback and execute it
	var cb = func() (OperationResult) {
		// add all files in repo folder, recursively
		cmd := exec.Command("git", "-C", g.Path, "add", "-A")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()

		var result OperationResult
		result.output = ""
		result.err = err
		result.stderr = stderr
		return result
	}
	result := g.opHandler.ExecuteOperation(cb);

	var err error
	if result.err != nil {
		err = ign.WithStack(result.err)
		ign.LoggerFromContext(ctx).Info("Error while running process. Err: " +
			 fmt.Sprint(result.err) + ". Stderr: " + result.stderr.String())
	}
	return err
}

// Commit everything to a repository
func (g *GitVCS) Commit(ctx context.Context, message string) error {
	if err := ensureFolderExists(g.Path); err != nil {
		return err
	}

	// Create an cmd callback and execute it
	var cb = func() (OperationResult) {
		cmd := exec.Command("git", "-C", g.Path, "commit", "-m", message, "--allow-empty")
		err := cmd.Run()
		var result OperationResult
		result.err = err
		return result
	}

	result := g.opHandler.ExecuteOperation(cb);

	var err error
	if result.err != nil {
		err = ign.WithStack(err)
	}
	return err
}

// Zip - creates a zip with the repository files, at a given revision.
// If revision is empty or "tip", last commit from "master" branch will be used.
// If output is empty, then a zip file in the tmp folder will be created.
// Returns a string path pointing to the created zip file.
func (g *GitVCS) Zip(ctx context.Context, rev, output string) (*string, error) {
	return archive(ctx, g.Path, rev, output, &g.opHandler)
}

// archive creates a zip with the repository files, at a given revision.
// If revision (rev arg) is empty or "tip", then last commit from "HEAD"
// will be used. If output is empty, then a zip file in the tmp folder
// will be created.
// Returns a string path pointing to the created zip file.
func archive(ctx context.Context, repoPath, rev, output string, opHandler *OperationHandler) (*string, error) {

	if err := ensureFolderExists(repoPath); err != nil {
		return nil, err
	}
	rev = ensureRev(rev)

	var folder, zipPath string
	var err error

	if output == "" {
		folder, err = ioutil.TempDir("", "repo")
		zipPath = filepath.Join(folder, rev+".os.Filezip")
	} else {
		zipPath = output
	}
	if err != nil {
		return nil, ign.WithStack(err)
	}


	// Create an cmd callback and execute it
	var cb = func() (OperationResult) {
		cmd := exec.Command("git", "-C", repoPath, "archive",
			"--format=zip", "-o", zipPath, rev)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		_, err = cmd.Output()
		var result OperationResult
		result.err = err
		return result
	}

	result := opHandler.ExecuteOperation(cb);

	if result.err != nil {
		err = ign.WithStack(result.err)
		ign.LoggerFromContext(ctx).Info("Error while running git archive process. Err: " +
			fmt.Sprint(result.err) + ". Stderr: " + result.stderr.String())
		return nil, err
	}
	return &zipPath, nil
}

// ReplaceFiles - replaces all files from repo HEAD with the files from the given folder.
// owner is an optional argument used to set the git commit user. If empty, then the default
// git user will be used.
func (g *GitVCS) ReplaceFiles(ctx context.Context, folder, owner string) error {
	return errors.New("GitVCS's ReplaceFiles function is not implemented yet")
}

// CloneTo - makes a local clone of repo into given target
func (g *GitVCS) CloneTo(ctx context.Context, target string) error {
	if err := ensureFolderExists(g.Path); err != nil {
		return err
	}
	if err := doLocalClone(ctx, g.Path, target); err != nil {
		return err
	}
	return nil
}

// Tag the current repository.
func (g *GitVCS) Tag(ctx context.Context, tag string) error {
	if err := ensureFolderExists(g.Path); err != nil {
		return err
	}

	// Create an cmd callback and execute it
	var cb = func() (OperationResult) {
		cmd := exec.Command("git", "tag", "-a", tag, "-m", "ign-fuelserver created tag after cloning")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		var result OperationResult
		result.output = ""
		result.err = err
		result.stderr = stderr
		return result
	}

	result := g.opHandler.ExecuteOperation(cb)

	var err error
	if result.err != nil {
		err = ign.WithStack(result.err)
		ign.LoggerFromContext(ctx).Info("Error while Tagging repo: " + g.Path +
			 ". Err: " + fmt.Sprint(result.err) + ". Stderr: " + result.stderr.String())
	}
	return err
}

// doLocalClone - makes a local clone of source into target
func doLocalClone(ctx context.Context, source, target string) error {
	// A clone command does not need to be queued so use exec.Command directly
    // instead of creating an Operation
	cmd := exec.Command("git", "clone", "--local", source, target)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		err = ign.WithStack(err)
		ign.LoggerFromContext(ctx).Info("Error while cloning git repo: " +
			source + ". Err: " + fmt.Sprint(err) + ". Stderr: " + stderr.String())
	}
	return err
}

// RevisionCount - get the number of revisions up to a specific revision
// If revision is empty, last commit from "master" branch will be used.
// Returns the number of revisions.
func (g *GitVCS) RevisionCount(ctx context.Context, rev string) (int, error) {
	ign.LoggerFromContext(ctx).Info("WARNING: ideally, we should not use the plain GitVCS implementation. Try to use GoGitVCS")
	return getRevisionCount(ctx, g.Path, rev, &g.opHandler)
}

// getRevisionCount returns the number of revisions from given commit/revision
func getRevisionCount(ctx context.Context, repoPath, rev string, opHandler *OperationHandler) (int, error) {
	if err := ensureFolderExists(repoPath); err != nil {
		return 0, err
	}

	rev = ensureRev(rev)


	// Create an cmd callback and execute it
	var cb = func() (OperationResult) {
		cmd := exec.Command("git", "-C", repoPath, "rev-list", "--count", rev)
		var bs []byte
		var err error
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		bs, err = cmd.Output()
		var result OperationResult
		result.output = string(bs[:])
		result.err = err
		result.stderr = stderr
		return result
	}

	result := opHandler.ExecuteOperation(cb)

	var count int
	var err error
	if result.err != nil {
		err = ign.WithStack(result.err)
		ign.LoggerFromContext(ctx).Info("Error while running process. Err: " +
			fmt.Sprint(result.err) + ". Stderr: " + result.stderr.String())
		count = 0
	} else {
		parsed, parseErr := strconv.Atoi(strings.Fields(result.output)[0])
		if parseErr != nil {
			parseErr = ign.WithStack(parseErr)
			ign.LoggerFromContext(ctx).Info("Error while parsing revision count: " + 
				fmt.Sprint(parseErr))
		} else {
			count = parsed
		}
	}
	return count, err
}

/// RunOperationLoop is a go routine that monitors the channel for any new
/// operations that need to be run. It allows only one operation running
/// at any one time. If the operation consists of multiple commands, it
/// executes the commands in series and stops if there is an error. The last
/// ouput of the last successful command is returned
func (o *OperationHandler) RunOperationLoop() {
	for !o.stopOperationLoop {
		// get the operation from the queue
		op := <- o.Operations
		// add to wait group so that it blocks other incoming operations
		o.WG.Add(1)

		result := op.CommandCb()
		o.OperationResults <- result

		// make sure to call done so it unblocks other operations.
		// This allows a new operation to be put into the queue
		o.WG.Done()
	}
}

/// ExecuteOperation is a blocking call for executing a VCS operation,
/// i.e. a callback function containing a group of git commands. It first waits
/// for the existing operation (if any) to finish before queueing the new input
/// operation. When the operation is complete, it returns the output of the
/// execution.
func (o *OperationHandler) ExecuteOperation(cb CommandCallback) (OperationResult) {

	if !o.initialized {
		o.Init()
		go o.RunOperationLoop()
	}

	// queue a new operation
	var op Operation
	if cb != nil {
		op.CommandCb = cb
	} else {
		var result OperationResult
		result.output = "Empty operation"
		result.err = errors.New(result.output)
		return result
	}
	// wait for operation queue to be available
	o.WG.Wait()
	o.Operations <- op
	result := <- o.OperationResults
	return result
}
