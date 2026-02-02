package project

import (
	"os"
	"path/filepath"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/domain"
	limitedio "github.com/charliek/envsecrets/internal/io"
	"github.com/charliek/envsecrets/internal/pathutil"
	"github.com/go-git/go-git/v5"
)

// Discovery handles project discovery operations
type Discovery struct {
	projectRoot string
}

// NewDiscovery creates a new project discovery starting from the given path
func NewDiscovery(startPath string) (*Discovery, error) {
	if startPath == "" {
		var err error
		startPath, err = os.Getwd()
		if err != nil {
			return nil, domain.Errorf(domain.ErrNotInRepo, "failed to get working directory: %v", err)
		}
	}

	root, err := findGitRoot(startPath)
	if err != nil {
		return nil, err
	}

	return &Discovery{projectRoot: root}, nil
}

// findGitRoot walks up the directory tree to find the git root
func findGitRoot(startPath string) (string, error) {
	path, err := filepath.Abs(startPath)
	if err != nil {
		return "", domain.Errorf(domain.ErrNotInRepo, "invalid path: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
			return path, nil
		}

		parent := filepath.Dir(path)
		if parent == path {
			return "", domain.ErrNotInRepo
		}
		path = parent
	}
}

// ProjectRoot returns the project root directory
func (d *Discovery) ProjectRoot() string {
	return d.projectRoot
}

// RepoInfo returns the repository information
func (d *Discovery) RepoInfo() (*domain.RepoInfo, error) {
	repo, err := git.PlainOpen(d.projectRoot)
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to open repository: %v", err)
	}

	remotes, err := repo.Remotes()
	if err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to get remotes: %v", err)
	}

	if len(remotes) == 0 {
		return nil, domain.Errorf(domain.ErrNotInRepo, "no remotes configured")
	}

	// Prefer origin, fall back to first remote
	var remoteURL string
	for _, remote := range remotes {
		if remote.Config().Name == "origin" {
			if len(remote.Config().URLs) > 0 {
				remoteURL = remote.Config().URLs[0]
				break
			}
		}
	}

	if remoteURL == "" && len(remotes) > 0 && len(remotes[0].Config().URLs) > 0 {
		remoteURL = remotes[0].Config().URLs[0]
	}

	if remoteURL == "" {
		return nil, domain.Errorf(domain.ErrNotInRepo, "no remote URL found")
	}

	info, err := ParseRemoteURL(remoteURL)
	if err != nil {
		return nil, err
	}

	return info, nil
}

// EnvSecretsFile returns the path to the .envsecrets file
func (d *Discovery) EnvSecretsFile() string {
	return filepath.Join(d.projectRoot, constants.EnvSecretsFile)
}

// EnvFiles returns the list of tracked environment files
func (d *Discovery) EnvFiles() ([]string, error) {
	envFile := d.EnvSecretsFile()

	files, err := ParseEnvSecretsFile(envFile)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, domain.ErrNoFilesTracked
	}

	return files, nil
}

// secureJoinPath safely joins the project root with a relative path,
// preventing path traversal attacks (e.g., ../../../etc/passwd)
func (d *Discovery) secureJoinPath(relativePath string) (string, error) {
	return pathutil.SecureJoin(d.projectRoot, relativePath)
}

// FileExists checks if a file exists in the project
func (d *Discovery) FileExists(relPath string) bool {
	fullPath, err := d.secureJoinPath(relPath)
	if err != nil {
		return false
	}
	_, err = os.Stat(fullPath)
	return err == nil
}

// ReadFile reads a file from the project with size limit protection
func (d *Discovery) ReadFile(relPath string) ([]byte, error) {
	fullPath, err := d.secureJoinPath(relPath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domain.Errorf(domain.ErrFileNotFound, "file not found: %s", relPath)
		}
		return nil, domain.Errorf(domain.ErrGitError, "failed to open file: %v", err)
	}
	defer f.Close()

	data, err := limitedio.LimitedReadAll(f, constants.MaxEnvFileSize, "env file")
	if err != nil {
		return nil, err
	}
	return data, nil
}

// WriteFile writes a file to the project
// Uses 0600 permissions for env files to prevent unauthorized access
func (d *Discovery) WriteFile(relPath string, content []byte) error {
	fullPath, err := d.secureJoinPath(relPath)
	if err != nil {
		return err
	}

	// Ensure directory exists with restrictive permissions (0700)
	// since it may contain sensitive env files
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to create directory: %v", err)
	}

	// Use restrictive permissions (0600) for decrypted env files
	if err := os.WriteFile(fullPath, content, 0600); err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to write file: %v", err)
	}
	return nil
}
