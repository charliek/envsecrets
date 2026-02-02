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

// ParseEnvSecretsFile reads and parses a .envsecrets file
func ParseEnvSecretsFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domain.ErrNoEnvFiles
		}
		return nil, domain.Errorf(domain.ErrGitError, "failed to read .envsecrets: %v", err)
	}
	defer f.Close()

	var files []string
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Validate path for security
		if err := validateEnvSecretPath(line); err != nil {
			return nil, domain.Errorf(domain.ErrInvalidArgs, "invalid path at line %d: %v", lineNum, err)
		}

		files = append(files, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, domain.Errorf(domain.ErrGitError, "failed to parse .envsecrets: %v", err)
	}

	return files, nil
}

// WriteEnvSecretsFile writes a .envsecrets file
func WriteEnvSecretsFile(path string, files []string) error {
	f, err := os.Create(path)
	if err != nil {
		return domain.Errorf(domain.ErrGitError, "failed to create .envsecrets: %v", err)
	}
	defer f.Close()

	for _, file := range files {
		if _, err := f.WriteString(file + "\n"); err != nil {
			return domain.Errorf(domain.ErrGitError, "failed to write .envsecrets: %v", err)
		}
	}

	return nil
}

// IsTracked checks if a file is tracked in the .envsecrets file
func IsTracked(envSecretsPath, filePath string) (bool, error) {
	files, err := ParseEnvSecretsFile(envSecretsPath)
	if err != nil {
		return false, err
	}

	for _, f := range files {
		if f == filePath {
			return true, nil
		}
	}

	return false, nil
}

// AddToTracked adds a file to the .envsecrets file if not already tracked
func AddToTracked(envSecretsPath, filePath string) error {
	files, err := ParseEnvSecretsFile(envSecretsPath)
	if err != nil {
		if err == domain.ErrNoEnvFiles {
			files = []string{}
		} else {
			return err
		}
	}

	// Check if already tracked
	for _, f := range files {
		if f == filePath {
			return nil // Already tracked
		}
	}

	files = append(files, filePath)
	return WriteEnvSecretsFile(envSecretsPath, files)
}

// RemoveFromTracked removes a file from the .envsecrets file
func RemoveFromTracked(envSecretsPath, filePath string) error {
	files, err := ParseEnvSecretsFile(envSecretsPath)
	if err != nil {
		return err
	}

	var newFiles []string
	found := false
	for _, f := range files {
		if f == filePath {
			found = true
			continue
		}
		newFiles = append(newFiles, f)
	}

	if !found {
		return domain.Errorf(domain.ErrFileNotFound, "file not tracked: %s", filePath)
	}

	return WriteEnvSecretsFile(envSecretsPath, newFiles)
}
