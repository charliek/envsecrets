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
)

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
