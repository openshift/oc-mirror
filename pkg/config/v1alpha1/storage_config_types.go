package v1alpha1

import (
	"github.com/go-git/go-git/v5"
)

// StorageConfig configures how metadata is stored.
type StorageConfig struct {
	Git *GitConfig `json:"git,omitempty"`
}

// GitConfig configures a git-based storage.
type GitConfig struct {
	// Dir to store the local git repo clone.
	Dir string `json:"dir"`
	// RepoURL at which the remote repo can be pulled.
	RepoURL string `json:"repoURL"`
}

// GetCloneOptions returns config data as CloneOptions.
func (gc GitConfig) GetCloneOptions() (*git.CloneOptions, error) {

	// TODO(estroz): complete this.

	opts := &git.CloneOptions{
		URL: gc.RepoURL,
	}

	return opts, nil
}

// GetCommitOptions returns config data as CommitOptions.
func (gc GitConfig) GetCommitOptions() (*git.CommitOptions, error) {

	// TODO(estroz): complete this.

	opts := &git.CommitOptions{
		All: false,
	}

	return opts, nil
}
