package project

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/charliek/envsecrets/internal/domain"
)

var (
	// SSH URL pattern: git@host:owner/repo.git
	sshPattern = regexp.MustCompile(`^git@([^:]+):([^/]+)/(.+?)(?:\.git)?$`)

	// HTTPS URL pattern: https://host/owner/repo.git
	httpsPattern = regexp.MustCompile(`^https?://([^/]+)/([^/]+)/(.+?)(?:\.git)?$`)

	// Valid owner pattern: alphanumeric, hyphens, underscores, dots (no slashes)
	validOwnerPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

	// Valid name pattern: alphanumeric, hyphens, underscores, dots, slashes (for nested paths)
	validNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`)
)

// ParseRepoString parses "owner/name" format into RepoInfo
func ParseRepoString(repo string) (*domain.RepoInfo, error) {
	repo = strings.TrimSpace(repo)
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, domain.Errorf(domain.ErrInvalidArgs, "invalid repo format: expected owner/name, got %q", repo)
	}

	// Validate characters (alphanumeric, hyphens, underscores, dots; slashes allowed in name for nested paths)
	if !validOwnerPattern.MatchString(parts[0]) {
		return nil, domain.Errorf(domain.ErrInvalidArgs, "invalid owner name: only alphanumeric, hyphens, underscores, and dots allowed")
	}
	if !validNamePattern.MatchString(parts[1]) {
		return nil, domain.Errorf(domain.ErrInvalidArgs, "invalid repo name: only alphanumeric, hyphens, underscores, dots, and slashes allowed")
	}

	return &domain.RepoInfo{Owner: parts[0], Name: parts[1]}, nil
}

// ParseRemoteURL parses a git remote URL and extracts owner/repo info
func ParseRemoteURL(remoteURL string) (*domain.RepoInfo, error) {
	remoteURL = strings.TrimSpace(remoteURL)

	// Try SSH pattern first
	if matches := sshPattern.FindStringSubmatch(remoteURL); matches != nil {
		return &domain.RepoInfo{
			Owner:     matches[2],
			Name:      matches[3],
			RemoteURL: remoteURL,
		}, nil
	}

	// Try HTTPS pattern
	if matches := httpsPattern.FindStringSubmatch(remoteURL); matches != nil {
		return &domain.RepoInfo{
			Owner:     matches[2],
			Name:      matches[3],
			RemoteURL: remoteURL,
		}, nil
	}

	// Try parsing as URL
	u, err := url.Parse(remoteURL)
	if err == nil && u.Host != "" {
		path := strings.TrimPrefix(u.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 2 {
			return &domain.RepoInfo{
				Owner:     parts[0],
				Name:      parts[1],
				RemoteURL: remoteURL,
			}, nil
		}
	}

	return nil, domain.Errorf(domain.ErrNotInRepo, "failed to parse remote URL: %s", remoteURL)
}
