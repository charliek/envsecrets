package pathutil

import (
	"path/filepath"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"

	"github.com/charliek/envsecrets/internal/domain"
)

// SecureJoin safely joins a base directory with a relative path,
// preventing path traversal attacks (e.g., ../../../etc/passwd).
// It returns an error if the path attempts to escape the base directory.
func SecureJoin(baseDir, relativePath string) (string, error) {
	// Clean the path first to normalize it
	cleaned := filepath.Clean(relativePath)

	// Reject absolute paths
	if filepath.IsAbs(cleaned) {
		return "", domain.Errorf(domain.ErrInvalidArgs, "absolute path not allowed: %q", relativePath)
	}

	// Check for any ".." components that would escape the base directory
	// SecureJoin handles this, but we also check explicitly for clearer error messages
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, string(filepath.Separator)+"..") {
		return "", domain.Errorf(domain.ErrInvalidArgs, "path traversal not allowed: %q", relativePath)
	}

	// Use securejoin to prevent path traversal (extra safety layer)
	safePath, err := securejoin.SecureJoin(baseDir, relativePath)
	if err != nil {
		return "", domain.Errorf(domain.ErrInvalidArgs, "invalid path %q: %v", relativePath, err)
	}

	// Verify the result is actually within the base directory
	// Use path separator to prevent /home/user matching /home/user2
	if safePath != baseDir && !strings.HasPrefix(safePath, baseDir+string(filepath.Separator)) {
		return "", domain.Errorf(domain.ErrInvalidArgs, "path traversal attempt detected: %q", relativePath)
	}

	return safePath, nil
}
