package cli

import (
	"context"
	"strings"

	"github.com/charliek/envsecrets/internal/cache"
	"github.com/charliek/envsecrets/internal/config"
	"github.com/charliek/envsecrets/internal/crypto"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/project"
	"github.com/charliek/envsecrets/internal/storage"
)

// ProjectContext holds all the components needed for project operations
type ProjectContext struct {
	Config    *config.Config
	Discovery *project.Discovery
	RepoInfo  *domain.RepoInfo
	Storage   storage.Storage
	Encrypter crypto.Encrypter
	Cache     *cache.Cache
}

// NewProjectContext creates a new project context with all required components
func NewProjectContext(ctx context.Context, cfg *config.Config) (*ProjectContext, error) {
	repoOverride := GetRepo()

	var repoInfo *domain.RepoInfo
	var discovery *project.Discovery

	if repoOverride != "" {
		// Use override - parse the provided repo string
		var err error
		repoInfo, err = project.ParseRepoString(repoOverride)
		if err != nil {
			return nil, err
		}
		// Still try to get discovery for EnvFiles(), but it may fail if not in git repo
		discovery, err = project.NewDiscovery("")
		if err != nil {
			// Log at verbose level - this is expected when not in a git repo
			out := GetOutput()
			out.Verbose("Project discovery unavailable: %v", err)
		}
	} else {
		// Normal discovery
		var err error
		discovery, err = project.NewDiscovery("")
		if err != nil {
			return nil, err
		}
		repoInfo, err = discovery.RepoInfo()
		if err != nil {
			return nil, err
		}
	}

	// Create storage client with retry wrapper
	gcsStore, err := storage.NewGCSStorage(ctx, cfg.Bucket, cfg.GCSCredentials)
	if err != nil {
		return nil, err
	}

	// Ensure cleanup on error after this point
	var returnErr error
	defer func() {
		if returnErr != nil {
			gcsStore.Close()
		}
	}()

	store := storage.NewRetryingStorage(gcsStore, storage.DefaultRetryConfig())

	// Resolve passphrase and create encrypter
	resolver := config.NewPassphraseResolver(cfg)
	passphrase, err := resolver.Resolve()
	if err != nil {
		returnErr = err
		return nil, err
	}

	encrypter, err := crypto.NewAgeEncrypter(passphrase)
	if err != nil {
		returnErr = err
		return nil, err
	}

	// Create cache
	cacheRepo, err := cache.NewCache(repoInfo, store)
	if err != nil {
		returnErr = err
		return nil, err
	}

	return &ProjectContext{
		Config:    cfg,
		Discovery: discovery,
		RepoInfo:  repoInfo,
		Storage:   store,
		Encrypter: encrypter,
		Cache:     cacheRepo,
	}, nil
}

// EnvFiles returns the list of environment files to track
func (pc *ProjectContext) EnvFiles() ([]string, error) {
	return pc.Discovery.EnvFiles()
}

// ReadProjectFile reads a file from the project directory
func (pc *ProjectContext) ReadProjectFile(path string) ([]byte, error) {
	return pc.Discovery.ReadFile(path)
}

// WriteProjectFile writes a file to the project directory
func (pc *ProjectContext) WriteProjectFile(path string, content []byte) error {
	return pc.Discovery.WriteFile(path, content)
}

// FileExists checks if a file exists in the project
func (pc *ProjectContext) FileExists(path string) bool {
	return pc.Discovery.FileExists(path)
}

// Close releases resources held by the ProjectContext
func (pc *ProjectContext) Close() error {
	return pc.Storage.Close()
}

// GetFileStatuses returns the status of all tracked files
func (pc *ProjectContext) GetFileStatuses() ([]domain.FileStatus, error) {
	files, err := pc.EnvFiles()
	if err != nil {
		return nil, err
	}

	var statuses []domain.FileStatus
	for _, file := range files {
		status := domain.FileStatus{
			Path:        file,
			LocalExists: pc.FileExists(file),
		}

		// Check if file exists in cache
		_, err := pc.Cache.ReadEncrypted(file)
		status.CacheExists = err == nil

		// Check if modified (compare encrypted content)
		if status.LocalExists && status.CacheExists {
			localContent, err := pc.ReadProjectFile(file)
			if err == nil {
				encrypted, err := pc.Cache.ReadEncrypted(file)
				if err == nil {
					// Decrypt cache content to compare
					decrypted, err := pc.Encrypter.Decrypt(encrypted)
					if err == nil {
						status.Modified = string(localContent) != string(decrypted)
					}
				}
			}
		} else if status.LocalExists != status.CacheExists {
			status.Modified = true
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

// extractReposFromObjects extracts unique owner/repo combinations from storage paths
func extractReposFromObjects(objects []string) map[string]bool {
	repos := make(map[string]bool)
	for _, obj := range objects {
		parts := strings.Split(obj, "/")
		if len(parts) >= 2 {
			repo := parts[0] + "/" + parts[1]
			repos[repo] = true
		}
	}
	return repos
}
