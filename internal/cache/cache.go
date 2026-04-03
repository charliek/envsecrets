package cache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// Log returns the commit history. When includeFiles is true, each commit
// includes the list of changed files with .age extensions stripped.
func (c *Cache) Log(n int, includeFiles bool) ([]domain.Commit, error) {
	commits, err := c.repo.Log(n, includeFiles)
	if err != nil {
		return nil, err
	}
	if includeFiles {
		for i := range commits {
			for j, f := range commits[i].Files {
				commits[i].Files[j] = strings.TrimSuffix(f, constants.AgeExtension)
			}
		}
	}
	return commits, nil
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

// MaxFormatFileSize is the maximum size of a FORMAT file (64 bytes)
const MaxFormatFileSize = 64

// DetectRemoteVersion reads the FORMAT marker from cloud storage and returns
// the storage format version info. If no FORMAT file exists, returns Version=0.
func (c *Cache) DetectRemoteVersion(ctx context.Context) (*domain.StorageFormatInfo, error) {
	formatPath := c.repoInfo.CachePath() + "/" + constants.StorageFormatFile
	r, err := c.storage.Download(ctx, formatPath)
	if err != nil {
		if errors.Is(err, domain.ErrFileNotFound) {
			return &domain.StorageFormatInfo{Version: 0, Detected: false}, nil
		}
		return nil, domain.Errorf(domain.ErrDownloadFailed, "failed to download FORMAT: %v", err)
	}

	data, readErr := limitedio.LimitedReadAll(r, MaxFormatFileSize, "FORMAT file")
	closeErr := r.Close()
	if readErr != nil {
		return nil, domain.Errorf(domain.ErrDownloadFailed, "failed to read FORMAT: %v", readErr)
	}
	if closeErr != nil {
		return nil, domain.Errorf(domain.ErrDownloadFailed, "failed to close FORMAT reader: %v", closeErr)
	}

	versionStr := strings.TrimSpace(string(data))
	if versionStr == "" {
		return nil, domain.Errorf(domain.ErrDownloadFailed, "FORMAT file is empty")
	}

	version, err := strconv.Atoi(versionStr)
	if err != nil {
		return nil, domain.Errorf(domain.ErrDownloadFailed, "FORMAT file contains non-numeric content: %q", versionStr)
	}
	if version <= 0 {
		return nil, domain.Errorf(domain.ErrDownloadFailed, "FORMAT file contains invalid version: %d", version)
	}

	return &domain.StorageFormatInfo{Version: version, Detected: true}, nil
}

// CheckVersionCompatibility validates that the detected storage format version
// is compatible with this client.
func CheckVersionCompatibility(info *domain.StorageFormatInfo) error {
	if !info.Detected {
		return domain.Errorf(domain.ErrVersionUnknown,
			"no FORMAT marker found; this repository may use an incompatible storage format — run 'envsecrets delete <repo>' and re-push with the current version")
	}
	if info.Version > constants.CurrentFormatVersion {
		return domain.Errorf(domain.ErrVersionTooNew,
			"remote storage format v%d is not supported by this client (supports v%d); upgrade envsecrets",
			info.Version, constants.CurrentFormatVersion)
	}
	return nil
}

// WriteFormatMarker uploads the format version marker to cloud storage.
func (c *Cache) WriteFormatMarker(ctx context.Context, version int) error {
	formatPath := c.repoInfo.CachePath() + "/" + constants.StorageFormatFile
	if err := c.storage.Upload(ctx, formatPath, strings.NewReader(strconv.Itoa(version))); err != nil {
		return domain.Errorf(domain.ErrUploadFailed, "failed to upload FORMAT: %v", err)
	}
	return nil
}

// SyncToStorage uploads the cache to cloud storage using packfile format.
// Uploads objects.pack (all git objects), refs (branch/tag info), and HEAD.
func (c *Cache) SyncToStorage(ctx context.Context) error {
	prefix := c.repoInfo.CachePath()

	// Create packfile from all git objects
	var packBuf bytes.Buffer
	if err := c.repo.PackAll(&packBuf); err != nil {
		return domain.Errorf(domain.ErrUploadFailed, "failed to create packfile: %v", err)
	}

	// Upload packfile (only if there are objects)
	if packBuf.Len() > 0 {
		if err := c.storage.Upload(ctx, prefix+"/objects.pack", &packBuf); err != nil {
			return domain.Errorf(domain.ErrUploadFailed, "failed to upload packfile: %v", err)
		}
	}

	// Serialize and upload refs
	allRefs, err := c.repo.GetAllRefs()
	if err != nil {
		return domain.Errorf(domain.ErrUploadFailed, "failed to get refs: %v", err)
	}

	var refsBuf bytes.Buffer
	for name, hash := range allRefs {
		if name == "HEAD" {
			continue // HEAD is stored separately
		}
		fmt.Fprintf(&refsBuf, "%s %s\n", name, hash)
	}

	if err := c.storage.Upload(ctx, prefix+"/refs", &refsBuf); err != nil {
		return domain.Errorf(domain.ErrUploadFailed, "failed to upload refs: %v", err)
	}

	// Upload FORMAT marker before HEAD (HEAD is the existence marker, so it must be last)
	if err := c.WriteFormatMarker(ctx, constants.CurrentFormatVersion); err != nil {
		return err
	}

	// Upload HEAD
	head, err := c.Head()
	if err != nil {
		return err
	}

	if err := c.storage.Upload(ctx, prefix+"/HEAD", strings.NewReader(head)); err != nil {
		return domain.Errorf(domain.ErrUploadFailed, "failed to upload HEAD: %v", err)
	}

	return nil
}

// MaxPackfileSize is the maximum size of a packfile download (50 MB)
const MaxPackfileSize = 50 * 1024 * 1024

// MaxRefsFileSize is the maximum size of a refs file (1 MB)
const MaxRefsFileSize = 1 * 1024 * 1024

// SyncFromStorage downloads the cache from cloud storage using packfile format.
// Downloads objects.pack and refs, restores git objects and references, then
// checks out HEAD to populate the working tree.
func (c *Cache) SyncFromStorage(ctx context.Context) error {
	// Ensure cache directory exists with restrictive permissions
	if err := os.MkdirAll(c.baseDir, 0700); err != nil {
		return domain.Errorf(domain.ErrDownloadFailed, "failed to create cache directory: %v", err)
	}

	// Initialize git repo if needed
	if err := c.Init(); err != nil {
		return err
	}

	prefix := c.repoInfo.CachePath()

	// Download and restore packfile
	packReader, err := c.storage.Download(ctx, prefix+"/objects.pack")
	if err != nil {
		// Only ignore "not found" errors (empty repo case)
		if !errors.Is(err, domain.ErrFileNotFound) {
			return domain.Errorf(domain.ErrDownloadFailed, "failed to download packfile: %v", err)
		}
	} else {
		packData, readErr := limitedio.LimitedReadAll(packReader, MaxPackfileSize, "packfile")
		closeErr := packReader.Close()
		if readErr != nil {
			return domain.Errorf(domain.ErrDownloadFailed, "failed to read packfile: %v", readErr)
		}
		if closeErr != nil {
			return domain.Errorf(domain.ErrDownloadFailed, "failed to close packfile reader: %v", closeErr)
		}

		if len(packData) > 0 {
			if err := c.repo.UnpackAll(bytes.NewReader(packData)); err != nil {
				return domain.Errorf(domain.ErrDownloadFailed, "failed to unpack objects: %v", err)
			}
		}
	}

	// Download and restore refs
	refsReader, err := c.storage.Download(ctx, prefix+"/refs")
	if err != nil {
		// Only ignore "not found" errors (empty repo case)
		if !errors.Is(err, domain.ErrFileNotFound) {
			return domain.Errorf(domain.ErrDownloadFailed, "failed to download refs: %v", err)
		}
	} else {
		refsData, readErr := limitedio.LimitedReadAll(refsReader, MaxRefsFileSize, "refs file")
		closeErr := refsReader.Close()
		if readErr != nil {
			return domain.Errorf(domain.ErrDownloadFailed, "failed to read refs: %v", readErr)
		}
		if closeErr != nil {
			return domain.Errorf(domain.ErrDownloadFailed, "failed to close refs reader: %v", closeErr)
		}

		// Parse and set each ref
		for _, line := range strings.Split(string(refsData), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, " ", 2)
			if len(parts) != 2 {
				continue
			}
			refName, refHash := parts[0], parts[1]
			if !isValidGitHash(refHash) {
				continue // Skip malformed refs
			}
			if err := c.repo.SetRef(refName, refHash); err != nil {
				return domain.Errorf(domain.ErrDownloadFailed, "failed to set ref %s: %v", refName, err)
			}
		}
	}

	// Download HEAD to check if remote has data and get the commit hash
	headReader, err := c.storage.Download(ctx, prefix+"/HEAD")
	if err != nil {
		if errors.Is(err, domain.ErrFileNotFound) {
			return nil // No HEAD means empty repo — skip version check
		}
		return domain.Errorf(domain.ErrDownloadFailed, "failed to download HEAD: %v", err)
	}

	headData, readErr := limitedio.LimitedReadAll(headReader, 1024, "HEAD file")
	closeErr := headReader.Close()
	if readErr != nil {
		return domain.Errorf(domain.ErrDownloadFailed, "failed to read HEAD: %v", readErr)
	}
	if closeErr != nil {
		return domain.Errorf(domain.ErrDownloadFailed, "failed to close HEAD reader: %v", closeErr)
	}

	// Remote has data (HEAD exists) — validate format version
	info, err := c.DetectRemoteVersion(ctx)
	if err != nil {
		return err
	}
	if err := CheckVersionCompatibility(info); err != nil {
		return err
	}

	head := strings.TrimSpace(string(headData))
	if head != "" && isValidGitHash(head) {
		// Checkout HEAD to update working tree
		if err := c.repo.Checkout(head); err != nil {
			return domain.Errorf(domain.ErrDownloadFailed, "failed to checkout HEAD: %v", err)
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
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
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
	prefix := c.repoInfo.CachePath()

	// Delete known packfile-format files
	for _, name := range []string{"/objects.pack", "/refs", "/" + constants.StorageFormatFile, "/HEAD"} {
		if err := c.storage.Delete(ctx, prefix+name); err != nil {
			// Ignore not-found errors for individual files
			if !errors.Is(err, domain.ErrFileNotFound) {
				return err
			}
		}
	}

	// Also clean up any legacy flat files that might remain
	files, err := c.storage.List(ctx, prefix+"/")
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
