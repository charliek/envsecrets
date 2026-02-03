package project

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/charliek/envsecrets/internal/domain"
)

// validateEnvSecretPath validates a path from .envsecrets file
// Returns an error if the path is unsafe (absolute, contains .., or has control characters)
func validateEnvSecretPath(path string) error {
	// Reject empty paths
	if path == "" {
		return domain.Errorf(domain.ErrInvalidArgs, "empty path not allowed")
	}

	// Reject paths with null bytes or control characters
	for _, c := range path {
		if c == 0 || (c < 32 && c != '\t') {
			return domain.Errorf(domain.ErrInvalidArgs, "path contains invalid characters: %q", path)
		}
	}

	// Clean and validate
	cleaned := filepath.Clean(path)

	// Reject absolute paths
	if filepath.IsAbs(cleaned) {
		return domain.Errorf(domain.ErrInvalidArgs, "absolute path not allowed in .envsecrets: %q", path)
	}

	// Reject paths that start with or contain ".." which would escape the project
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) ||
		strings.Contains(cleaned, string(filepath.Separator)+".."+string(filepath.Separator)) ||
		strings.HasSuffix(cleaned, string(filepath.Separator)+"..") {
		return domain.Errorf(domain.ErrInvalidArgs, "path traversal not allowed in .envsecrets: %q", path)
	}

	return nil
}

// ParseGitignoreMarker extracts tracked files from # envsecrets section in .gitignore
func ParseGitignoreMarker(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil // Not an error if .gitignore doesn't exist
	}
	defer f.Close()

	var files []string
	inSection := false
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "# envsecrets" {
			inSection = true
			continue
		}

		if inSection {
			// Section ends on blank line or new comment (that isn't # envsecrets)
			if trimmed == "" || (strings.HasPrefix(trimmed, "#") && trimmed != "# envsecrets") {
				break
			}

			if err := validateEnvSecretPath(trimmed); err != nil {
				continue // Skip invalid paths in gitignore
			}
			files = append(files, trimmed)
		}
	}

	if len(files) == 0 {
		return nil, nil // No marker found
	}
	return files, nil
}

// ParseEnvSecretsFile reads and parses a .envsecrets file
func ParseEnvSecretsFile(path string) (*domain.EnvSecretsConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domain.ErrNoEnvFiles
		}
		return nil, domain.Errorf(domain.ErrGitError, "failed to read .envsecrets: %v", err)
	}
	defer f.Close()

	config := &domain.EnvSecretsConfig{}
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for repo: directive
		if strings.HasPrefix(line, "repo:") {
			repoStr := strings.TrimSpace(strings.TrimPrefix(line, "repo:"))
			// Validate format
			if _, err := ParseRepoString(repoStr); err != nil {
				return nil, domain.Errorf(domain.ErrInvalidArgs, "invalid repo directive at line %d: %v", lineNum, err)
			}
			config.RepoOverride = repoStr
			continue
		}

		// Validate path for security
		if err := validateEnvSecretPath(line); err != nil {
			return nil, domain.Errorf(domain.ErrInvalidArgs, "invalid path at line %d: %v", lineNum, err)
		}

		config.Files = append(config.Files, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to parse .envsecrets: %v", err)
	}

	return config, nil
}

// WriteEnvSecretsFile writes a .envsecrets file (simple file list, no config)
func WriteEnvSecretsFile(path string, files []string) error {
	return WriteEnvSecretsFileWithConfig(path, &domain.EnvSecretsConfig{Files: files})
}

// WriteEnvSecretsFileWithConfig writes a .envsecrets file preserving config directives
func WriteEnvSecretsFileWithConfig(path string, config *domain.EnvSecretsConfig) error {
	f, err := os.Create(path)
	if err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to create .envsecrets: %v", err)
	}
	defer f.Close()

	// Write repo directive if present
	if config.RepoOverride != "" {
		if _, err := f.WriteString("repo: " + config.RepoOverride + "\n"); err != nil {
			return domain.Errorf(domain.ErrGitError, "failed to write .envsecrets: %v", err)
		}
	}

	// Write file list
	for _, file := range config.Files {
		if _, err := f.WriteString(file + "\n"); err != nil {
			return domain.Errorf(domain.ErrGitError, "failed to write .envsecrets: %v", err)
		}
	}

	return nil
}

// IsTracked checks if a file is tracked in the .envsecrets file
func IsTracked(envSecretsPath, filePath string) (bool, error) {
	config, err := ParseEnvSecretsFile(envSecretsPath)
	if err != nil {
		return false, err
	}

	for _, f := range config.Files {
		if f == filePath {
			return true, nil
		}
	}

	return false, nil
}

// AddToTracked adds a file to the .envsecrets file if not already tracked
func AddToTracked(envSecretsPath, filePath string) error {
	config, err := ParseEnvSecretsFile(envSecretsPath)
	if err != nil {
		if err == domain.ErrNoEnvFiles {
			config = &domain.EnvSecretsConfig{}
		} else {
			return err
		}
	}

	// Check if already tracked
	for _, f := range config.Files {
		if f == filePath {
			return nil // Already tracked
		}
	}

	config.Files = append(config.Files, filePath)
	return WriteEnvSecretsFileWithConfig(envSecretsPath, config)
}

// RemoveFromTracked removes a file from the .envsecrets file
func RemoveFromTracked(envSecretsPath, filePath string) error {
	config, err := ParseEnvSecretsFile(envSecretsPath)
	if err != nil {
		return err
	}

	var newFiles []string
	found := false
	for _, f := range config.Files {
		if f == filePath {
			found = true
			continue
		}
		newFiles = append(newFiles, f)
	}

	if !found {
		return domain.Errorf(domain.ErrFileNotFound, "file not tracked: %s", filePath)
	}

	config.Files = newFiles
	return WriteEnvSecretsFileWithConfig(envSecretsPath, config)
}
