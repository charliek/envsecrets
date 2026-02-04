package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/git"
	limitedio "github.com/charliek/envsecrets/internal/io"
	"github.com/charliek/envsecrets/internal/pathutil"
	"github.com/charliek/envsecrets/internal/storage"
)

// Cache manages the local cache of encrypted environment files
type Cache struct {
	baseDir  string
	storage  storage.Storage
	repoInfo *domain.RepoInfo
	repo     git.Repository
}

// NewCache creates a new cache for the given repository
func NewCache(repoInfo *domain.RepoInfo, store storage.Storage) (*Cache, error) {
	baseDir := constants.DefaultCacheDir()
	cachePath := filepath.Join(baseDir, repoInfo.Owner, repoInfo.Name)

	gitRepo, err := git.NewGoGitRepository(cachePath)
	if err != nil {
		return nil, err
	}

	return &Cache{
		baseDir:  cachePath,
		storage:  store,
		repoInfo: repoInfo,
		repo:     gitRepo,
	}, nil
}

// NewCacheWithRepo creates a cache with a custom repository implementation (for testing)
func NewCacheWithRepo(repoInfo *domain.RepoInfo, store storage.Storage, repo git.Repository, basePath string) *Cache {
	return &Cache{
		baseDir:  basePath,
		storage:  store,
		repoInfo: repoInfo,
		repo:     repo,
	}
}

// secureJoinPath safely joins the cache base directory with a relative path,
// preventing path traversal attacks (e.g., ../../../etc/passwd)
func (c *Cache) secureJoinPath(relativePath string) (string, error) {
	return pathutil.SecureJoin(c.baseDir, relativePath)
}

// Init initializes the cache repository
func (c *Cache) Init() error {
	return c.repo.Init()
}

// Path returns the cache directory path
func (c *Cache) Path() string {
	return c.baseDir
}

// WriteEncrypted writes encrypted content to the cache
func (c *Cache) WriteEncrypted(filename string, content []byte) error {
	agePath := filename + constants.AgeExtension
	return c.repo.WriteFile(agePath, content)
}

// ReadEncrypted reads encrypted content from the cache
func (c *Cache) ReadEncrypted(filename string) ([]byte, error) {
	agePath := filename + constants.AgeExtension
	return c.repo.ReadFile(agePath, "")
}

// ReadEncryptedAtRef reads encrypted content from the cache at a specific ref
func (c *Cache) ReadEncryptedAtRef(filename, ref string) ([]byte, error) {
	agePath := filename + constants.AgeExtension
	return c.repo.ReadFile(agePath, ref)
}

// RemoveEncrypted removes an encrypted file from the cache
func (c *Cache) RemoveEncrypted(filename string) error {
	agePath := filename + constants.AgeExtension
	return c.repo.RemoveFile(agePath)
}

// StageAll stages all changes for commit
func (c *Cache) StageAll() error {
	files, err := c.ListLocalFiles()
	if err != nil {
		return err
	}

	for _, f := range files {
		if err := c.repo.Add(f); err != nil {
			return err
		}
	}

	return nil
}

// Commit creates a new commit with the given message
func (c *Cache) Commit(message string) (string, error) {
	return c.repo.Commit(message)
}

// Head returns the current HEAD commit hash
func (c *Cache) Head() (string, error) {
	return c.repo.Head()
}

// HasChanges returns true if there are uncommitted changes
func (c *Cache) HasChanges() (bool, error) {
	return c.repo.HasChanges()
}

// Log returns the commit history
func (c *Cache) Log(n int) ([]domain.Commit, error) {
	return c.repo.Log(n)
}

// Checkout checks out a specific ref
func (c *Cache) Checkout(ref string) error {
	return c.repo.Checkout(ref)
}

// CheckoutBranch checks out a branch by name (attaches HEAD)
func (c *Cache) CheckoutBranch(branch string) error {
	return c.repo.CheckoutBranch(branch)
}

// GetDefaultBranch returns the repository's default branch name
func (c *Cache) GetDefaultBranch() (string, error) {
	return c.repo.GetDefaultBranch()
}

// ListLocalFiles lists all files in the cache (including .age extension)
func (c *Cache) ListLocalFiles() ([]string, error) {
	var files []string

	err := filepath.Walk(c.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directory
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only include .age files
		if strings.HasSuffix(path, constants.AgeExtension) {
			relPath, err := filepath.Rel(c.baseDir, path)
			if err != nil {
				return err
			}
			files = append(files, relPath)
		}

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, domain.Errorf(domain.ErrGitError, "failed to list files: %v", err)
	}

	return files, nil
}

// ListTrackedFiles returns original filenames (without .age extension)
func (c *Cache) ListTrackedFiles() ([]string, error) {
	ageFiles, err := c.ListLocalFiles()
	if err != nil {
		return nil, err
	}

	var files []string
	for _, f := range ageFiles {
		files = append(files, strings.TrimSuffix(f, constants.AgeExtension))
	}

	return files, nil
}

// SyncToStorage uploads the cache to cloud storage
func (c *Cache) SyncToStorage(ctx context.Context) error {
	files, err := c.ListLocalFiles()
	if err != nil {
		return err
	}

	// Upload each file
	for _, file := range files {
		if err := c.uploadFile(ctx, file); err != nil {
			return err
		}
	}

	// Upload HEAD ref
	head, err := c.Head()
	if err != nil {
		return err
	}

	headPath := c.repoInfo.CachePath() + "/HEAD"
	err = c.storage.Upload(ctx, headPath, strings.NewReader(head))
	if err != nil {
		return err
	}

	return nil
}

// uploadFile uploads a single file to storage with proper resource cleanup
func (c *Cache) uploadFile(ctx context.Context, file string) (err error) {
	// Validate path to prevent traversal attacks from corrupted git index
	localPath, err := c.secureJoinPath(file)
	if err != nil {
		return domain.Errorf(domain.ErrUploadFailed, "invalid path: %v", err)
	}
	remotePath := c.repoInfo.CachePath() + "/" + file

	f, err := os.Open(localPath)
	if err != nil {
		return domain.Errorf(domain.ErrUploadFailed, "failed to open %s: %v", file, err)
	}
	defer func() {
		closeErr := f.Close()
		if err == nil && closeErr != nil {
			err = domain.Errorf(domain.ErrUploadFailed, "failed to close %s: %v", file, closeErr)
		}
	}()

	if err := c.storage.Upload(ctx, remotePath, f); err != nil {
		return err
	}

	return nil
}

// SyncFromStorage downloads the cache from cloud storage
func (c *Cache) SyncFromStorage(ctx context.Context) error {
	// Ensure cache directory exists with restrictive permissions
	if err := os.MkdirAll(c.baseDir, 0700); err != nil {
		return domain.Errorf(domain.ErrDownloadFailed, "failed to create cache directory: %v", err)
	}

	// Initialize git repo if needed
	if err := c.Init(); err != nil {
		return err
	}

	// List remote files
	prefix := c.repoInfo.CachePath() + "/"
	remoteFiles, err := c.storage.List(ctx, prefix)
	if err != nil {
		return err
	}

	// Download each .age file
	for _, remotePath := range remoteFiles {
		// Skip HEAD file
		if strings.HasSuffix(remotePath, "/HEAD") {
			continue
		}

		localFile := strings.TrimPrefix(remotePath, prefix)

		// Validate path to prevent traversal attacks from malicious GCS paths
		localPath, err := c.secureJoinPath(localFile)
		if err != nil {
			return domain.Errorf(domain.ErrDownloadFailed, "path traversal attempt detected: %v", err)
		}

		// Ensure directory exists with restrictive permissions
		if err := os.MkdirAll(filepath.Dir(localPath), 0700); err != nil {
			return domain.Errorf(domain.ErrDownloadFailed, "failed to create directory: %v", err)
		}

		r, err := c.storage.Download(ctx, remotePath)
		if err != nil {
			return err
		}

		// Use size-limited read to prevent memory exhaustion from malicious content
		data, err := limitedio.LimitedReadAll(r, constants.MaxEncryptedFileSize, fmt.Sprintf("file %s", remotePath))
		closeErr := r.Close()
		if err != nil {
			return domain.Errorf(domain.ErrDownloadFailed, "failed to read %s: %v", remotePath, err)
		}
		if closeErr != nil {
			return domain.Errorf(domain.ErrDownloadFailed, "failed to close reader for %s: %v", remotePath, closeErr)
		}

		if err := os.WriteFile(localPath, data, 0600); err != nil {
			return domain.Errorf(domain.ErrDownloadFailed, "failed to write %s: %v", localPath, err)
		}
	}

	return nil
}

// isValidGitHash checks if a string is a valid 40-character hex git hash
func isValidGitHash(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// GetRemoteHead gets the HEAD ref from cloud storage
func (c *Cache) GetRemoteHead(ctx context.Context) (string, error) {
	headPath := c.repoInfo.CachePath() + "/HEAD"
	r, err := c.storage.Download(ctx, headPath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	// HEAD file should be small (just a commit hash), but limit for safety
	const maxHeadSize = 1024
	data, err := limitedio.LimitedReadAll(r, maxHeadSize, "HEAD file")
	if err != nil {
		return "", domain.Errorf(domain.ErrDownloadFailed, "failed to read HEAD: %v", err)
	}

	head := strings.TrimSpace(string(data))

	// Validate HEAD is a proper git hash
	if !isValidGitHash(head) {
		return "", domain.Errorf(domain.ErrDownloadFailed, "invalid HEAD format: expected 40-character hex hash")
	}

	return head, nil
}

// Exists checks if the cache exists locally
func (c *Cache) Exists() bool {
	_, err := os.Stat(filepath.Join(c.baseDir, ".git"))
	return err == nil
}

// ExistsRemote checks if the cache exists in cloud storage
func (c *Cache) ExistsRemote(ctx context.Context) (bool, error) {
	headPath := c.repoInfo.CachePath() + "/HEAD"
	return c.storage.Exists(ctx, headPath)
}

// DeleteRemote deletes all files for this repo from cloud storage
func (c *Cache) DeleteRemote(ctx context.Context) error {
	prefix := c.repoInfo.CachePath() + "/"
	files, err := c.storage.List(ctx, prefix)
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := c.storage.Delete(ctx, file); err != nil {
			return err
		}
	}

	return nil
}

// CacheHealth represents the health status of the cache
type CacheHealth struct {
	// Exists indicates if the cache directory exists
	Exists bool
	// GitValid indicates if the git repository is valid
	GitValid bool
	// HeadValid indicates if HEAD can be resolved
	HeadValid bool
	// FileCount is the number of .age files in the cache
	FileCount int
	// Error contains any error encountered during validation
	Error error
}

// Validate checks the health of the local cache
func (c *Cache) Validate() CacheHealth {
	health := CacheHealth{}

	// Check if cache directory exists
	if _, err := os.Stat(c.baseDir); err != nil {
		if os.IsNotExist(err) {
			return health // Cache doesn't exist yet, which is OK
		}
		health.Error = fmt.Errorf("failed to stat cache directory: %w", err)
		return health
	}
	health.Exists = true

	// Check if .git directory exists
	gitDir := filepath.Join(c.baseDir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		health.Error = fmt.Errorf("cache exists but .git directory is missing or invalid")
		return health
	}

	// Try to get HEAD
	head, err := c.repo.Head()
	if err != nil {
		// Empty repo is OK
		if err != domain.ErrNotInitialized {
			health.Error = fmt.Errorf("failed to read HEAD: %w", err)
			return health
		}
	} else {
		health.HeadValid = head != ""
	}
	health.GitValid = true

	// Count .age files
	files, err := c.ListLocalFiles()
	if err != nil {
		health.Error = fmt.Errorf("failed to list files: %w", err)
		return health
	}
	health.FileCount = len(files)

	return health
}

// Reset removes the local cache and re-downloads from storage
func (c *Cache) Reset(ctx context.Context) error {
	// Remove local cache directory
	if err := os.RemoveAll(c.baseDir); err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to remove cache directory: %v", err)
	}

	// Check if remote exists
	exists, err := c.ExistsRemote(ctx)
	if err != nil {
		return err
	}

	if exists {
		// Re-download from remote
		return c.SyncFromStorage(ctx)
	}

	// Initialize fresh cache
	return c.Init()
}
