package git

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
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

	// Log returns the last n commits. When includeFiles is true, each commit
	// includes the list of files changed relative to its parent.
	Log(n int, includeFiles bool) ([]domain.Commit, error)

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

	// PackAll encodes all objects in the repository into a packfile written to w
	PackAll(w io.Writer) error

	// UnpackAll restores objects from a packfile read from r
	UnpackAll(r io.Reader) error

	// GetAllRefs returns all references as a map of ref name to commit hash
	GetAllRefs() (map[string]string, error)

	// SetRef creates or updates a reference to point at the given hash
	SetRef(name, hash string) error

	// DeleteRef removes a reference
	DeleteRef(name string) error
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
		// Verify the repo still exists on disk (may have been deleted by Reset)
		if _, err := os.Stat(filepath.Join(r.path, ".git")); err == nil {
			return nil // Already initialized and exists
		}
		r.repo = nil // Stale handle — reinitialize
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

	name, email := commitAuthorIdentity()
	commit, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  name,
			Email: email,
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", domain.Errorf(domain.ErrGitError, "failed to commit: %v", err)
	}

	return commit.String(), nil
}

// commitAuthorIdentity returns the (name, email) pair stamped on commits.
// Order of precedence:
//  1. ENVSECRETS_MACHINE_ID (if set) — used as both the human label and the
//     email's host part, e.g. machine_id="laptop-A" → "user <user@laptop-A>".
//  2. $USER + os.Hostname() — e.g. "charliek <charliek@MacBookPro.local>".
//  3. Fallback: "envsecrets <envsecrets@local>" (matches the historical value
//     so older commits remain visually distinct, not impersonated).
func commitAuthorIdentity() (string, string) {
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME") // Windows
	}

	if machineID := os.Getenv("ENVSECRETS_MACHINE_ID"); machineID != "" {
		// Keep the email's host part as the machine identifier in both
		// branches so cross-machine attribution stays consistent. When the
		// OS user is unknown, fall back to the project name as the
		// local-part rather than using machineID twice (which would read
		// like a person, not a machine).
		if user == "" {
			return "envsecrets", "envsecrets@" + machineID
		}
		return user, user + "@" + machineID
	}

	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "local"
	}

	if user == "" {
		return "envsecrets", "envsecrets@" + host
	}
	return user, user + "@" + host
}

// Log implements Repository.Log
func (r *GoGitRepository) Log(n int, includeFiles bool) ([]domain.Commit, error) {
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
			return storer.ErrStop
		}

		hash := c.Hash.String()
		commit := domain.Commit{
			Hash:        hash,
			ShortHash:   hash[:constants.ShortHashLength],
			Message:     c.Message,
			Author:      c.Author.Name,
			AuthorEmail: c.Author.Email,
			Date:        c.Author.When,
		}

		if includeFiles {
			files, err := commitFiles(c)
			if err != nil {
				return err
			}
			commit.Files = files
		}

		commits = append(commits, commit)
		count++
		return nil
	})
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to iterate log: %v", err)
	}

	return commits, nil
}

// commitFiles returns the list of files changed in a commit by diffing
// against its parent tree (or an empty tree for root commits).
func commitFiles(c *object.Commit) ([]string, error) {
	commitTree, err := c.Tree()
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to get commit tree: %v", err)
	}

	var parentTree *object.Tree
	if c.NumParents() > 0 {
		parent, err := c.Parents().Next()
		if err != nil {
			return nil, domain.Errorf(domain.ErrGitError, "failed to get parent commit: %v", err)
		}
		parentTree, err = parent.Tree()
		if err != nil {
			return nil, domain.Errorf(domain.ErrGitError, "failed to get parent tree: %v", err)
		}
	}

	changes, err := object.DiffTree(parentTree, commitTree)
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to diff trees: %v", err)
	}

	var files []string
	for _, change := range changes {
		name := change.To.Name
		if name == "" {
			name = change.From.Name // deleted file
		}
		files = append(files, name)
	}
	sort.Strings(files)
	return files, nil
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

// PackAll implements Repository.PackAll
func (r *GoGitRepository) PackAll(w io.Writer) error {
	if r.repo == nil {
		return domain.ErrNotInitialized
	}

	store := r.repo.Storer

	// Collect all object hashes
	var hashes []plumbing.Hash
	iter, err := store.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to iterate objects: %v", err)
	}

	err = iter.ForEach(func(obj plumbing.EncodedObject) error {
		hashes = append(hashes, obj.Hash())
		return nil
	})
	if err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to collect objects: %v", err)
	}

	if len(hashes) == 0 {
		return nil
	}

	enc := packfile.NewEncoder(w, store, false)
	if _, err := enc.Encode(hashes, 10); err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to encode packfile: %v", err)
	}

	return nil
}

// UnpackAll implements Repository.UnpackAll
func (r *GoGitRepository) UnpackAll(rd io.Reader) error {
	if r.repo == nil {
		return domain.ErrNotInitialized
	}

	store := r.repo.Storer

	scanner := packfile.NewScanner(rd)
	parser, err := packfile.NewParserWithStorage(scanner, store)
	if err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to create packfile parser: %v", err)
	}

	if _, err := parser.Parse(); err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to parse packfile: %v", err)
	}

	return nil
}

// GetAllRefs implements Repository.GetAllRefs
func (r *GoGitRepository) GetAllRefs() (map[string]string, error) {
	if r.repo == nil {
		return nil, domain.ErrNotInitialized
	}

	refs := make(map[string]string)

	iter, err := r.repo.Storer.IterReferences()
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to iterate references: %v", err)
	}

	err = iter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() == plumbing.HashReference {
			refs[ref.Name().String()] = ref.Hash().String()
		}
		return nil
	})
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to collect references: %v", err)
	}

	// Include HEAD
	head, err := r.repo.Head()
	if err == nil {
		refs["HEAD"] = head.Hash().String()
	}

	return refs, nil
}

// SetRef implements Repository.SetRef
func (r *GoGitRepository) SetRef(name, hash string) error {
	if r.repo == nil {
		return domain.ErrNotInitialized
	}

	h := plumbing.NewHash(hash)
	refName := plumbing.ReferenceName(name)

	ref := plumbing.NewHashReference(refName, h)
	if err := r.repo.Storer.SetReference(ref); err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to set reference %s: %v", name, err)
	}

	return nil
}

// DeleteRef implements Repository.DeleteRef
func (r *GoGitRepository) DeleteRef(name string) error {
	if r.repo == nil {
		return domain.ErrNotInitialized
	}

	refName := plumbing.ReferenceName(name)
	if err := r.repo.Storer.RemoveReference(refName); err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to delete reference %s: %v", name, err)
	}

	return nil
}

// Path returns the repository path
func (r *GoGitRepository) Path() string {
	return r.path
}

// IsInitialized returns true if the repository is initialized
func (r *GoGitRepository) IsInitialized() bool {
	return r.repo != nil
}
