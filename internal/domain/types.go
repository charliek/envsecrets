package domain

import "time"

// RepoInfo identifies a repository
type RepoInfo struct {
	// Owner is the repository owner (user or organization)
	Owner string
	// Name is the repository name
	Name string
	// RemoteURL is the full remote URL
	RemoteURL string
}

// String returns the owner/name format
func (r RepoInfo) String() string {
	return r.Owner + "/" + r.Name
}

// CachePath returns the relative cache path for this repo
func (r RepoInfo) CachePath() string {
	return r.Owner + "/" + r.Name
}

// Commit represents a git commit
type Commit struct {
	// Hash is the full commit hash
	Hash string
	// ShortHash is the abbreviated commit hash
	ShortHash string
	// Message is the commit message
	Message string
	// Author is the commit author
	Author string
	// Date is the commit timestamp
	Date time.Time
	// Files is the list of files changed in this commit
	Files []string
}

// FileStatus represents the status of a tracked file
type FileStatus struct {
	// Path is the file path relative to project root
	Path string
	// LocalExists indicates if the file exists locally
	LocalExists bool
	// CacheExists indicates if the file exists in cache
	CacheExists bool
	// Modified indicates if local differs from cache
	Modified bool
}

// SyncStatus represents the sync status between local and remote
type SyncStatus struct {
	// LocalHead is the local HEAD commit hash
	LocalHead string
	// RemoteHead is the remote HEAD commit hash
	RemoteHead string
	// InSync indicates if local and remote are in sync
	InSync bool
}

// PushResult contains the result of a push operation
type PushResult struct {
	// CommitHash is the new commit hash
	CommitHash string
	// FilesUpdated is the number of files updated
	FilesUpdated int
	// FilesAdded is the number of files added
	FilesAdded int
	// FilesDeleted is the number of files deleted
	FilesDeleted int
}

// PullResult contains the result of a pull operation
type PullResult struct {
	// FilesUpdated is the number of files updated
	FilesUpdated int
	// FilesCreated is the number of files created
	FilesCreated int
	// FilesSkipped is the number of files skipped (e.g., up to date)
	FilesSkipped int
	// Ref is the commit ref that was pulled
	Ref string
	// FilesWithConflicts lists files that would be overwritten by pull
	// These are files that exist locally with different content than remote
	FilesWithConflicts []string
}
