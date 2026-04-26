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
	// Author is the commit author name (git Author.Name)
	Author string `json:"author"`
	// AuthorEmail is the commit author email (git Author.Email). For
	// envsecrets-stamped commits this is "<user>@<machine-id-or-hostname>",
	// so the host part identifies the machine that pushed the commit.
	AuthorEmail string `json:"author_email,omitempty"`
	// Date is the commit timestamp
	Date time.Time `json:"date"`
	// Files is the list of files changed in this commit
	Files []string `json:"files,omitempty"`
}

// AuthorDisplay returns a human-facing attribution string that combines the
// commit's Name with the host part of its email so cross-machine identity
// is always visible. Falls back to just Name when no email is available
// (older commits or non-envsecrets-stamped history).
func (c Commit) AuthorDisplay() string {
	if c.AuthorEmail == "" {
		return c.Author
	}
	at := -1
	for i := len(c.AuthorEmail) - 1; i >= 0; i-- {
		if c.AuthorEmail[i] == '@' {
			at = i
			break
		}
	}
	if at < 0 || at == len(c.AuthorEmail)-1 {
		return c.Author
	}
	host := c.AuthorEmail[at+1:]
	if c.Author == "" {
		return host
	}
	if c.Author == host {
		return c.Author
	}
	return c.Author + "@" + host
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

// SyncAction represents the recommended next action for the user
type SyncAction string

const (
	// SyncActionInSync means local and remote agree; nothing to do
	SyncActionInSync SyncAction = "in_sync"
	// SyncActionPush means the user has local-only changes
	SyncActionPush SyncAction = "push"
	// SyncActionPull means the user has remote-only changes
	SyncActionPull SyncAction = "pull"
	// SyncActionPullThenPush means both sides changed but no overlap
	SyncActionPullThenPush SyncAction = "pull_then_push"
	// SyncActionReconcile means both sides changed the same files differently
	SyncActionReconcile SyncAction = "reconcile"
	// SyncActionFirstPushInit means remote is empty; push to initialize
	SyncActionFirstPushInit SyncAction = "first_push_init"
	// SyncActionFirstPull means we have no local baseline yet but remote has commits
	SyncActionFirstPull SyncAction = "first_pull"
	// SyncActionNothingTracked means .envsecrets has no files listed
	SyncActionNothingTracked SyncAction = "nothing_tracked"
)

// SyncStatus represents the sync status between local and remote
type SyncStatus struct {
	// LocalHead is the local HEAD commit hash
	LocalHead string `json:"local_head"`
	// RemoteHead is the remote HEAD commit hash
	RemoteHead string `json:"remote_head"`
	// LastSynced is the commit this machine last successfully pushed or pulled to.
	// Empty if this machine has never synced (fresh clone or post-Reset).
	LastSynced string `json:"last_synced,omitempty"`
	// LastSyncedAt is the wall-clock time this machine last synced (file mtime).
	// Zero if LastSynced is empty.
	LastSyncedAt time.Time `json:"last_synced_at,omitempty"`
	// InSync indicates if local and remote are at the same commit
	InSync bool `json:"in_sync"`
	// LocalChanges lists files where the working tree differs from LastSynced
	LocalChanges []string `json:"local_changes,omitempty"`
	// RemoteChanges lists files where remote HEAD differs from LastSynced
	RemoteChanges []string `json:"remote_changes,omitempty"`
	// Conflicts lists files that changed locally AND remotely to different content
	Conflicts []string `json:"conflicts,omitempty"`
	// Action is the recommended next action for the user
	Action SyncAction `json:"action"`
	// RemoteAuthor is the author of the remote HEAD commit, formatted as
	// "name@host" when the email is available so cross-machine attribution
	// is visible at a glance. Empty if no remote.
	RemoteAuthor string `json:"remote_author,omitempty"`
	// RemoteAuthorEmail is the raw author email, retained for JSON consumers
	// that want to filter or match on it programmatically.
	RemoteAuthorEmail string `json:"remote_author_email,omitempty"`
	// RemoteCommittedAt is when the remote HEAD commit was authored
	RemoteCommittedAt time.Time `json:"remote_committed_at,omitempty"`
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
	// Warning is a non-fatal advisory the caller should surface to the user.
	// Currently used to flag when the post-push baseline marker write failed
	// (push succeeded remotely but this machine kept a stale LAST_SYNCED).
	Warning string `json:"warning,omitempty"`
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
	// Warning is a non-fatal advisory the caller should surface to the user.
	// Currently used to flag when the post-pull baseline marker write failed
	// (pull succeeded locally but this machine kept a stale LAST_SYNCED).
	Warning string `json:"warning,omitempty"`
}

// StorageFormatInfo describes the storage format version found in a remote repo
type StorageFormatInfo struct {
	// Version is the format version number (0 means no FORMAT file found)
	Version int `json:"version"`
	// Detected is true if the version was read from a FORMAT file
	Detected bool `json:"detected"`
}

// EnvSecretsConfig holds parsed .envsecrets file contents
type EnvSecretsConfig struct {
	// RepoOverride from "repo: owner/name" directive
	RepoOverride string `json:"repo_override,omitempty"`
	// Files is the list of tracked file paths
	Files []string `json:"files"`
}
