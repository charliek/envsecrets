package git

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Compile-time assertion that GoGitRepository implements Repository
var _ Repository = (*GoGitRepository)(nil)

// Repository defines the interface for git operations
type Repository interface {
	// Init initializes a new git repository
	Init() error

	// Add stages files for commit
	Add(paths ...string) error

	// Commit creates a new commit with the given message
	Commit(message string) (string, error)

	// Log returns the last n commits
	Log(n int) ([]domain.Commit, error)

	// Checkout checks out the given ref
	Checkout(ref string) error

	// CheckoutBranch checks out a branch by name (attaches HEAD)
	CheckoutBranch(branch string) error

	// GetDefaultBranch returns the repository's default branch name (main or master)
	GetDefaultBranch() (string, error)

	// ListFiles returns all files in the repository
	ListFiles() ([]string, error)

	// ReadFile reads a file at the given ref (empty string for working tree)
	ReadFile(path, ref string) ([]byte, error)

	// WriteFile writes content to a file in the working tree
	WriteFile(path string, content []byte) error

	// RemoveFile removes a file from the working tree and stages the removal
	RemoveFile(path string) error

	// Head returns the current HEAD commit hash
	Head() (string, error)

	// HasChanges returns true if there are uncommitted changes
	HasChanges() (bool, error)
}

// GoGitRepository implements Repository using go-git
type GoGitRepository struct {
	path string
	repo *git.Repository
}

// NewGoGitRepository opens or creates a git repository at the given path
func NewGoGitRepository(path string) (*GoGitRepository, error) {
	repo, err := git.PlainOpen(path)
	if err == git.ErrRepositoryNotExists {
		return &GoGitRepository{path: path, repo: nil}, nil
	}
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to open repository: %v", err)
	}

	return &GoGitRepository{path: path, repo: repo}, nil
}

// Init implements Repository.Init
func (r *GoGitRepository) Init() error {
	if r.repo != nil {
		return nil // Already initialized
	}

	// Create directory if it doesn't exist with restrictive permissions
	if err := os.MkdirAll(r.path, 0700); err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to create directory: %v", err)
	}

	repo, err := git.PlainInit(r.path, false)
	if err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to init repository: %v", err)
	}

	r.repo = repo
	return nil
}

// Add implements Repository.Add
func (r *GoGitRepository) Add(paths ...string) error {
	if r.repo == nil {
		return domain.ErrNotInitialized
	}

	wt, err := r.repo.Worktree()
	if err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to get worktree: %v", err)
	}

	for _, path := range paths {
		if _, err := wt.Add(path); err != nil {
			return domain.Errorf(domain.ErrGitError, "failed to add %s: %v", path, err)
		}
	}

	return nil
}

// Commit implements Repository.Commit
func (r *GoGitRepository) Commit(message string) (string, error) {
	if r.repo == nil {
		return "", domain.ErrNotInitialized
	}

	wt, err := r.repo.Worktree()
	if err != nil {
		return "", domain.Errorf(domain.ErrGitError, "failed to get worktree: %v", err)
	}

	commit, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "envsecrets",
			Email: "envsecrets@local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", domain.Errorf(domain.ErrGitError, "failed to commit: %v", err)
	}

	return commit.String(), nil
}

// Log implements Repository.Log
func (r *GoGitRepository) Log(n int) ([]domain.Commit, error) {
	if r.repo == nil {
		return nil, domain.ErrNotInitialized
	}

	iter, err := r.repo.Log(&git.LogOptions{})
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to get log: %v", err)
	}
	defer iter.Close()

	var commits []domain.Commit
	count := 0
	err = iter.ForEach(func(c *object.Commit) error {
		if count >= n {
			return nil
		}

		hash := c.Hash.String()
		commits = append(commits, domain.Commit{
			Hash:      hash,
			ShortHash: hash[:constants.ShortHashLength],
			Message:   c.Message,
			Author:    c.Author.Name,
			Date:      c.Author.When,
		})
		count++
		return nil
	})
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to iterate log: %v", err)
	}

	return commits, nil
}

// Checkout implements Repository.Checkout
func (r *GoGitRepository) Checkout(ref string) error {
	if r.repo == nil {
		return domain.ErrNotInitialized
	}

	wt, err := r.repo.Worktree()
	if err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to get worktree: %v", err)
	}

	hash, err := r.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return domain.Errorf(domain.ErrRefNotFound, "failed to resolve ref %s: %v", ref, err)
	}

	err = wt.Checkout(&git.CheckoutOptions{
		Hash:  *hash,
		Force: true,
	})
	if err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to checkout %s: %v", ref, err)
	}

	return nil
}

// CheckoutBranch implements Repository.CheckoutBranch
func (r *GoGitRepository) CheckoutBranch(branch string) error {
	if r.repo == nil {
		return domain.ErrNotInitialized
	}

	wt, err := r.repo.Worktree()
	if err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to get worktree: %v", err)
	}

	branchRef := plumbing.NewBranchReferenceName(branch)
	err = wt.Checkout(&git.CheckoutOptions{
		Branch: branchRef,
		Keep:   true, // Keep working tree changes
	})
	if err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to checkout branch %s: %v", branch, err)
	}

	return nil
}

// GetDefaultBranch implements Repository.GetDefaultBranch
func (r *GoGitRepository) GetDefaultBranch() (string, error) {
	if r.repo == nil {
		return "", domain.ErrNotInitialized
	}

	// Check for common default branch names
	for _, branch := range []string{"main", "master"} {
		ref := plumbing.NewBranchReferenceName(branch)
		if _, err := r.repo.Reference(ref, true); err == nil {
			return branch, nil
		}
	}

	return "", domain.Errorf(domain.ErrRefNotFound, "no default branch found (checked main, master)")
}

// ListFiles implements Repository.ListFiles
func (r *GoGitRepository) ListFiles() ([]string, error) {
	if r.repo == nil {
		return nil, domain.ErrNotInitialized
	}

	ref, err := r.repo.Head()
	if err != nil {
		// No commits yet, return empty list
		if err == plumbing.ErrReferenceNotFound {
			return nil, nil
		}
		return nil, domain.Errorf(domain.ErrGitError, "failed to get HEAD: %v", err)
	}

	commit, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to get commit: %v", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to get tree: %v", err)
	}

	var files []string
	err = tree.Files().ForEach(func(f *object.File) error {
		files = append(files, f.Name)
		return nil
	})
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to list files: %v", err)
	}

	sort.Strings(files)
	return files, nil
}

// ReadFile implements Repository.ReadFile
func (r *GoGitRepository) ReadFile(path, ref string) ([]byte, error) {
	if r.repo == nil {
		return nil, domain.ErrNotInitialized
	}

	// Read from working tree if ref is empty
	if ref == "" {
		fullPath := filepath.Join(r.path, path)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, domain.Errorf(domain.ErrFileNotFound, "file not found: %s", path)
			}
			return nil, domain.Errorf(domain.ErrGitError, "failed to read file: %v", err)
		}
		return data, nil
	}

	// Read from commit
	hash, err := r.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, domain.Errorf(domain.ErrRefNotFound, "failed to resolve ref %s: %v", ref, err)
	}

	commit, err := r.repo.CommitObject(*hash)
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to get commit: %v", err)
	}

	file, err := commit.File(path)
	if err != nil {
		return nil, domain.Errorf(domain.ErrFileNotFound, "file not found: %s at %s", path, ref)
	}

	content, err := file.Contents()
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to read file contents: %v", err)
	}

	return []byte(content), nil
}

// WriteFile implements Repository.WriteFile
func (r *GoGitRepository) WriteFile(path string, content []byte) error {
	fullPath := filepath.Join(r.path, path)

	// Ensure directory exists with restrictive permissions
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to create directory: %v", err)
	}

	if err := os.WriteFile(fullPath, content, 0600); err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to write file: %v", err)
	}

	return nil
}

// RemoveFile implements Repository.RemoveFile
func (r *GoGitRepository) RemoveFile(path string) error {
	if r.repo == nil {
		return domain.ErrNotInitialized
	}

	wt, err := r.repo.Worktree()
	if err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to get worktree: %v", err)
	}

	if _, err := wt.Remove(path); err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to remove %s: %v", path, err)
	}

	return nil
}

// Head implements Repository.Head
func (r *GoGitRepository) Head() (string, error) {
	if r.repo == nil {
		return "", domain.ErrNotInitialized
	}

	ref, err := r.repo.Head()
	if err != nil {
		if err == plumbing.ErrReferenceNotFound {
			return "", nil // No commits yet
		}
		return "", domain.Errorf(domain.ErrGitError, "failed to get HEAD: %v", err)
	}

	return ref.Hash().String(), nil
}

// HasChanges implements Repository.HasChanges
func (r *GoGitRepository) HasChanges() (bool, error) {
	if r.repo == nil {
		return false, domain.ErrNotInitialized
	}

	wt, err := r.repo.Worktree()
	if err != nil {
		return false, domain.Errorf(domain.ErrGitError, "failed to get worktree: %v", err)
	}

	status, err := wt.Status()
	if err != nil {
		return false, domain.Errorf(domain.ErrGitError, "failed to get status: %v", err)
	}

	return !status.IsClean(), nil
}

// Path returns the repository path
func (r *GoGitRepository) Path() string {
	return r.path
}

// IsInitialized returns true if the repository is initialized
func (r *GoGitRepository) IsInitialized() bool {
	return r.repo != nil
}
