package domain

import "time"

// RepoInfo identifies a repository
type RepoInfo struct {
	// Owner is the repository owner (user or organization)
	Owner string `json:"owner"`
	// Name is the repository name
	Name string `json:"name"`
	// RemoteURL is the full remote URL
	RemoteURL string `json:"remote_url,omitempty"`
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
	Hash string `json:"hash"`
	// ShortHash is the abbreviated commit hash
	ShortHash string `json:"short_hash"`
	// Message is the commit message
	Message string `json:"message"`
	// Author is the commit author
	Author string `json:"author"`
	// Date is the commit timestamp
	Date time.Time `json:"date"`
	// Files is the list of files changed in this commit
	Files []string `json:"files,omitempty"`
}

// FileStatus represents the status of a tracked file
type FileStatus struct {
	// Path is the file path relative to project root
	Path string `json:"path"`
	// LocalExists indicates if the file exists locally
	LocalExists bool `json:"local_exists"`
	// CacheExists indicates if the file exists in cache
	CacheExists bool `json:"cache_exists"`
	// Modified indicates if local differs from cache
	Modified bool `json:"modified"`
}

// SyncStatus represents the sync status between local and remote
type SyncStatus struct {
	// LocalHead is the local HEAD commit hash
	LocalHead string `json:"local_head"`
	// RemoteHead is the remote HEAD commit hash
	RemoteHead string `json:"remote_head"`
	// InSync indicates if local and remote are in sync
	InSync bool `json:"in_sync"`
}

// PushResult contains the result of a push operation
type PushResult struct {
	// CommitHash is the new commit hash
	CommitHash string `json:"commit_hash,omitempty"`
	// FilesUpdated is the number of files updated
	FilesUpdated int `json:"files_updated"`
	// FilesAdded is the number of files added
	FilesAdded int `json:"files_added"`
	// FilesDeleted is the number of files deleted
	FilesDeleted int `json:"files_deleted"`
}

// PullResult contains the result of a pull operation
type PullResult struct {
	// FilesUpdated is the number of files updated
	FilesUpdated int `json:"files_updated"`
	// FilesCreated is the number of files created
	FilesCreated int `json:"files_created"`
	// FilesSkipped is the number of files skipped (e.g., up to date)
	FilesSkipped int `json:"files_skipped"`
	// FilesSkippedConflict is the number of conflicting files that were skipped
	FilesSkippedConflict int `json:"files_skipped_conflict,omitempty"`
	// Ref is the commit ref that was pulled
	Ref string `json:"ref,omitempty"`
	// FilesWithConflicts lists files that would be overwritten by pull
	// These are files that exist locally with different content than remote
	FilesWithConflicts []string `json:"files_with_conflicts,omitempty"`
}

// EnvSecretsConfig holds parsed .envsecrets file contents
type EnvSecretsConfig struct {
	// RepoOverride from "repo: owner/name" directive
	RepoOverride string `json:"repo_override,omitempty"`
	// Files is the list of tracked file paths
	Files []string `json:"files"`
}
