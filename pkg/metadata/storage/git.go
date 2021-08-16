package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"text/template"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/sirupsen/logrus"
)

var (
	_ Backend   = &gitBackend{}
	_ Committer = &gitBackend{}
)

type gitBackend struct {
	// Since git repos are represented locally as directories,
	// use the local dir backend as the underlying Backend.
	*localDirBackend

	// Git repo.
	repo *git.Repository

	// Saved PullOptions for refresh during Commit().
	pullOpts *git.PullOptions
	// Saved CommitOptions for committing changes during Commit()
	cmtOpts *git.CommitOptions
}

// NewGitBackend returns a Backend that transacts with a remote git repo.
// This repo must already exist and be accessible using CloneOptions.{Auth,CABundle,InsecureSkipTLS}.
func NewGitBackend(ctx context.Context, dir string, clOpts git.CloneOptions, cmtOpts git.CommitOptions) (Backend, error) {
	b := &gitBackend{}

	// Default to a common branch name.
	if clOpts.ReferenceName == "" {
		clOpts.ReferenceName = plumbing.Master
	}
	// This backend only cares about the specified branch.
	clOpts.SingleBranch = true

	b.pullOpts = &git.PullOptions{
		// Auth info.
		Auth:            clOpts.Auth,
		CABundle:        clOpts.CABundle,
		InsecureSkipTLS: clOpts.InsecureSkipTLS,
		// Pull configuration.
		RemoteName:        clOpts.RemoteName,
		ReferenceName:     clOpts.ReferenceName,
		SingleBranch:      clOpts.SingleBranch,
		RecurseSubmodules: clOpts.RecurseSubmodules,
		Depth:             clOpts.Depth,
		Progress:          clOpts.Progress,
	}

	// Commit() will handle staging.
	cmtOpts.All = false
	b.cmtOpts = &cmtOpts

	if err := b.validate(); err != nil {
		return nil, fmt.Errorf("invalid git backend configuration: %v", err)
	}

	// Create the local dir backend for local r/w.
	lb, err := NewLocalBackend(dir)
	if err != nil {
		return nil, fmt.Errorf("error creating local backend for git repo: %w", err)
	}
	b.localDirBackend = lb.(*localDirBackend)

	// Try to clone the repo first using the provided clone options,
	// since it may not exist locally.
	logrus.Debugf("attempting to clone repo %q remote %q branch %q", clOpts.URL, clOpts.RemoteName, clOpts.ReferenceName)
	clonedRepo, err := git.PlainCloneContext(ctx, dir, false, &clOpts)
	if err == nil {
		b.repo = clonedRepo
		return b, b.init()
	}
	// Ignore ErrRepositoryAlreadyExists since the repo should be open-able
	// if not clonable.
	if !errors.Is(err, git.ErrRepositoryAlreadyExists) {
		return nil, fmt.Errorf("error cloning repo: %v", err)
	}

	// Try to open the repo. It should be open-able since dir exists.
	// If dir is not a git repo, this will fail and return a helpful error message.
	b.repo, err = git.PlainOpen(dir)
	if err != nil {
		return nil, fmt.Errorf("error opening repo: %v", err)
	}

	// Validate CommitOptions after the repo has been set.
	if err := b.cmtOpts.Validate(b.repo); err != nil {
		return nil, fmt.Errorf("invalid CommitOptions: %v", err)
	}

	// Pull the latest changes for the desired branch.
	logrus.Debugf("pulling branch %q", b.pullOpts.ReferenceName)
	wt, err := b.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("error getting git working tree: %v", err)
	}
	if err := wt.PullContext(ctx, b.pullOpts); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil, fmt.Errorf("error pulling branch %q state: %v", b.pullOpts.ReferenceName.Short(), err)
	}

	return b, b.init()
}

func (b *gitBackend) init() error {
	return nil
}

func (b *gitBackend) validate() error {
	// PullOptions validation.
	if b.pullOpts == nil {
		return fmt.Errorf("PullOptions must be non-nil")
	}
	// ReferenceName is used in Commit() to ensure the correct branch is checked out.
	if b.pullOpts.ReferenceName == "" {
		return fmt.Errorf("PullOptions.ReferenceName must be configured")
	}
	if err := b.pullOpts.Validate(); err != nil {
		return fmt.Errorf("invalid PullOptions: %v", err)
	}

	// CommitOptions validation.
	if b.cmtOpts == nil {
		return fmt.Errorf("CommitOptions must be non-nil")
	}

	return nil
}

// Commit writes all local changes to the git remote for the configured repo.
// The commit message has format:
//
//  [auto] chore(metadata): update metadata and associated files
//
//  Signed-off-by: Name <email@srv.com>
//
func (b *gitBackend) Commit(ctx context.Context) error {

	// NB(estroz): some form of locking may be necessary so a branch isn't being modified
	// by another actor when entering Commit(). No locking is probably fine for now
	// since the repo dir is likely a randomized temp dir.

	// Ensure the currently checked out branch is the one b was configured with.
	head, err := b.repo.Head()
	if err != nil {
		return fmt.Errorf("error getting git HEAD: %v", err)
	}
	if b.pullOpts.ReferenceName.String() != head.Name().String() {
		return fmt.Errorf("current HEAD %q of git repo is not at expected ref %q", head.Name().String(), b.pullOpts.ReferenceName.String())
	}

	wt, err := b.repo.Worktree()
	if err != nil {
		return fmt.Errorf("error getting git working tree: %v", err)
	}

	// Make sure git state is fresh.
	if err := wt.PullContext(ctx, b.pullOpts); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("error pulling branch %q state: %v", b.pullOpts.ReferenceName.Short(), err)
	}

	// Stage changes.
	// TODO(estroz): staging all changes might be bad if no .gitignore exists
	// for temporary mirror artifacts. Clean this up by saving changed files and
	// new files created by the backend.
	addOpts := &git.AddOptions{
		All: true,
	}
	if err := wt.AddWithOptions(addOpts); err != nil {
		return fmt.Errorf("error staging git state changes: %v", err)
	}

	// Commit staged changes.
	commitMsg, err := newCommitMsg(b.cmtOpts.Author)
	if err != nil {
		return fmt.Errorf("error creating commit message: %v", err)
	}
	cmtHash, err := wt.Commit(commitMsg, b.cmtOpts)
	if err != nil {
		return fmt.Errorf("error committing changes: %v", err)
	}

	// Push committed changes.
	pushOpts := &git.PushOptions{
		// Auth info.
		Auth:            b.pullOpts.Auth,
		CABundle:        b.pullOpts.CABundle,
		InsecureSkipTLS: b.pullOpts.InsecureSkipTLS,
		// Push configuration.
		RemoteName:        b.pullOpts.RemoteName,
		RefSpecs:          []gitconfig.RefSpec{gitconfig.RefSpec(b.pullOpts.ReferenceName)},
		RequireRemoteRefs: []gitconfig.RefSpec{gitconfig.RefSpec(b.pullOpts.ReferenceName)},
		Prune:             false,
		Progress:          b.pullOpts.Progress,
	}
	if err := b.repo.PushContext(ctx, pushOpts); err != nil {
		return fmt.Errorf("error pushing %q: %v", cmtHash, err)
	}

	return nil
}

// Commit message template. Includes DCO.
var commitMsgTmpl = template.Must(template.New("cm").Parse(`
[auto] chore(metadata): update metadata and associated files

Signed-off-by: {{ .Name }} <{{ .Email }}>
`))

// newCommitMsg crafts a commit message following the outlined format.
func newCommitMsg(author *object.Signature) (string, error) {
	if author.Name == "" {
		return "", fmt.Errorf("commit Author.Name must be set")
	}
	if author.Email == "" {
		return "", fmt.Errorf("commit Author.Email must be set")
	}

	buf := bytes.Buffer{}
	err := commitMsgTmpl.Execute(&buf, author)
	return buf.String(), err
}
