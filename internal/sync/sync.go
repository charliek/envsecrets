package sync

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/charliek/envsecrets/internal/cache"
	"github.com/charliek/envsecrets/internal/crypto"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/project"
	"github.com/charliek/envsecrets/internal/storage"
)

// Syncer orchestrates push/pull operations
type Syncer struct {
	discovery *project.Discovery
	repoInfo  *domain.RepoInfo
	storage   storage.Storage
	encrypter crypto.Encrypter
	cache     *cache.Cache
}

// NewSyncer creates a new syncer
func NewSyncer(
	discovery *project.Discovery,
	repoInfo *domain.RepoInfo,
	store storage.Storage,
	enc crypto.Encrypter,
	c *cache.Cache,
) *Syncer {
	return &Syncer{
		discovery: discovery,
		repoInfo:  repoInfo,
		storage:   store,
		encrypter: enc,
		cache:     c,
	}
}

// PushOptions configures a push operation.
type PushOptions struct {
	// Message is the commit message for the push
	Message string
	// DryRun shows what would be pushed without actually pushing
	DryRun bool
	// Force pushes even if there are conflicts with remote
	Force bool
	// AllowDeletes lets the push delete any files from remote: tracked files
	// missing from the working tree, and files no longer tracked. Without
	// it, every deletion must be listed in ConfirmedDeletions or Push
	// returns a *domain.PendingDeletionsError instead.
	AllowDeletes bool
	// ConfirmedDeletions approves deleting exactly these files. A deletion
	// discovered after confirmation (e.g. a file another machine published
	// meanwhile) is not covered, so Push refuses rather than delete it unseen.
	ConfirmedDeletions []string
}

// ConflictAction represents how to handle a file conflict
type ConflictAction int

const (
	// ConflictOverwrite replaces local file with remote version
	ConflictOverwrite ConflictAction = iota
	// ConflictSkip keeps the local file unchanged
	ConflictSkip
	// ConflictAbort cancels the entire pull operation
	ConflictAbort
)

// ConflictResolver is called for each conflicting file to determine the action
type ConflictResolver func(filename string) (ConflictAction, error)

// PullOptions configures a pull operation.
type PullOptions struct {
	// Ref specifies a specific version (commit hash) to pull
	Ref string
	// Force overwrites local files that have different content without prompting.
	// When false, pull will abort with ErrConflict if local files would be overwritten.
	Force bool
	// DryRun shows what would be pulled without actually pulling
	DryRun bool
	// ConflictResolver is called for each conflicting file when Force is false.
	// If nil and conflicts exist, the pull will abort with ErrConflict.
	ConflictResolver ConflictResolver
}

// GetSyncStatus computes a complete sync status: heads, last-synced marker,
// per-file 3-way classification, and a recommended action. Performs a remote
// fetch (SyncFromStorage) so the cache reflects current remote state.
func (s *Syncer) GetSyncStatus(ctx context.Context) (*domain.SyncStatus, error) {
	status := &domain.SyncStatus{}

	// Read last-synced marker first (works even with no remote)
	lastSynced, lastSyncedAt := s.readBaseline()
	status.LastSynced = lastSynced
	status.LastSyncedAt = lastSyncedAt

	// Check remote
	remoteExists, err := s.cache.ExistsRemote(ctx)
	if err != nil {
		return nil, err
	}

	if remoteExists {
		// Sync cache so cache@HEAD reflects remote
		if err := s.cache.SyncFromStorage(ctx); err != nil {
			return nil, err
		}
		rh, err := s.cache.GetRemoteHead(ctx)
		if err == nil {
			status.RemoteHead = rh
		}
		// Pull commit metadata for the remote HEAD (author + when). Use
		// Commit.AuthorDisplay so cross-machine attribution is visible —
		// git stores the per-machine label in Email's host part
		// ("user@machine-id"), and a Name-only display would hide it
		// whenever the OS user is the same on every machine.
		if commits, err := s.cache.Log(1, false); err == nil && len(commits) > 0 {
			status.RemoteAuthor = commits[0].AuthorDisplay()
			status.RemoteAuthorEmail = commits[0].AuthorEmail
			status.RemoteCommittedAt = commits[0].Date
		}
	}

	// Local head (after sync, this equals remote when remote exists)
	if lh, err := s.cache.Head(); err == nil {
		status.LocalHead = lh
	}
	status.InSync = status.LocalHead != "" && status.LocalHead == status.RemoteHead

	// Determine baseline cases that don't need a 3-way diff
	files, fileErr := s.discoveryEnvFiles()
	switch {
	case errors.Is(fileErr, domain.ErrNoFilesTracked):
		status.Action = domain.SyncActionNothingTracked
		return status, nil
	case fileErr != nil:
		return nil, fileErr
	case len(files) == 0:
		status.Action = domain.SyncActionNothingTracked
		return status, nil
	}

	if !remoteExists {
		status.Action = domain.SyncActionFirstPushInit
		return status, nil
	}

	if lastSynced == "" {
		// Cache reflects remote, but this machine has no recorded baseline
		// (fresh clone, post-Reset, upgraded from old client, or corrupted
		// marker). Even when the working tree happens to match remote, the
		// recommendation must be FirstPull — push refuses without a baseline
		// (see checkPushDivergence in push.go), so claiming InSync would lie
		// to the user about whether their next push will succeed. The
		// informational LocalHead/RemoteHead fields above still tell them
		// the heads agree.
		status.Action = domain.SyncActionFirstPull
		return status, nil
	}

	// No tracked file exists locally (e.g. `git clean -fdX`, or a checkout
	// that never pulled). Against the baseline every file looks deleted, but
	// publishing that would wipe remote, so recommend restoring instead.
	if missing, err := s.missingLocallyOnly(files); err != nil {
		return nil, err
	} else if len(missing) > 0 {
		status.Action = domain.SyncActionRestore
		status.Missing = missing
		return status, nil
	}

	// Three-way classification: each tracked file vs (base=LastSynced, remote=HEAD, local=working tree).
	// ErrRefNotFound here means LAST_SYNCED points at a commit the cache
	// no longer has (corrupt baseline). Treat that exactly like a missing
	// marker — the recommendation is FirstPull, not a hard failure.
	if err := s.classifyFiles(files, lastSynced, status); err != nil {
		if errors.Is(err, domain.ErrRefNotFound) {
			status.Action = domain.SyncActionFirstPull
			status.LocalChanges = nil
			status.RemoteChanges = nil
			status.Conflicts = nil
			return status, nil
		}
		return nil, err
	}

	// Decide action from the three sets
	switch {
	case len(status.LocalChanges) == 0 && len(status.RemoteChanges) == 0:
		status.Action = domain.SyncActionInSync
	case len(status.Conflicts) > 0:
		status.Action = domain.SyncActionReconcile
	case len(status.LocalChanges) > 0 && len(status.RemoteChanges) > 0:
		status.Action = domain.SyncActionPullThenPush
	case len(status.LocalChanges) > 0:
		status.Action = domain.SyncActionPush
	case len(status.RemoteChanges) > 0:
		status.Action = domain.SyncActionPull
	}

	return status, nil
}

// classifyFiles fills in LocalChanges/RemoteChanges/Conflicts on status.
// readBaseline returns this checkout's last-synced commit. With no marker of
// its own, it adopts the legacy shared marker if the working tree matches
// that commit, i.e. this is the checkout that last synced it.
func (s *Syncer) readBaseline() (string, time.Time) {
	if hash, mtime, _ := s.cache.ReadLastSynced(); hash != "" {
		return hash, mtime
	}
	return s.cache.AdoptLegacyBaseline(s.treeMatchesRef)
}

// treeMatchesRef reports whether every tracked file in the working tree has
// the same content (and existence) as at ref. Any read or decrypt failure
// counts as no match.
func (s *Syncer) treeMatchesRef(ref string) bool {
	files, err := s.discovery.EnvFiles()
	if err != nil || len(files) == 0 {
		return false
	}
	for _, f := range files {
		base, baseExists, err := s.readCacheAtRef(f, ref)
		if err != nil {
			return false
		}
		local, localExists, err := s.readWorkingTree(f)
		if err != nil {
			return false
		}
		if !sameContent(base, baseExists, local, localExists) {
			return false
		}
	}
	return true
}

// FilesAbsentAtRef returns the tracked files that do not exist at ref. A
// rollback to ref deletes these, whether or not they are still present
// locally (e.g. a retry after an earlier rollback push failed).
func (s *Syncer) FilesAbsentAtRef(ref string) ([]string, error) {
	files, err := s.discovery.EnvFiles()
	if err != nil {
		return nil, err
	}
	var absent []string
	for _, f := range files {
		_, err := s.cache.ReadEncryptedAtRef(f, ref)
		switch {
		case err == nil:
		case errors.Is(err, domain.ErrFileNotFound):
			absent = append(absent, f)
		default:
			return nil, err
		}
	}
	return absent, nil
}

// missingLocallyOnly returns the tracked files remote has when NONE of the
// tracked files exist in the working tree, and nil otherwise.
func (s *Syncer) missingLocallyOnly(files []string) ([]string, error) {
	for _, f := range files {
		if s.discovery.FileExists(f) {
			return nil, nil
		}
	}
	var missing []string
	for _, f := range files {
		_, err := s.cache.ReadEncrypted(f)
		switch {
		case err == nil:
			missing = append(missing, f)
		case errors.Is(err, domain.ErrFileNotFound), errors.Is(err, domain.ErrNotInitialized):
			// Not on remote either; nothing to restore
		default:
			return nil, err
		}
	}
	return missing, nil
}

func (s *Syncer) classifyFiles(files []string, baseRef string, status *domain.SyncStatus) error {
	for _, f := range files {
		baseBytes, baseExists, err := s.readCacheAtRef(f, baseRef)
		if err != nil {
			return err
		}
		remoteBytes, remoteExists, err := s.readCacheAtHead(f)
		if err != nil {
			return err
		}
		localBytes, localExists, err := s.readWorkingTree(f)
		if err != nil {
			return err
		}

		localChanged := !sameContent(baseBytes, baseExists, localBytes, localExists)
		remoteChanged := !sameContent(baseBytes, baseExists, remoteBytes, remoteExists)

		if localChanged {
			status.LocalChanges = append(status.LocalChanges, f)
		}
		if remoteChanged {
			status.RemoteChanges = append(status.RemoteChanges, f)
		}
		if localChanged && remoteChanged && !sameContent(localBytes, localExists, remoteBytes, remoteExists) {
			status.Conflicts = append(status.Conflicts, f)
		}
	}
	return nil
}

// readCacheAtRef returns the decrypted file content at a specific ref, plus
// whether the file existed at that ref.
//
// A missing FILE at the ref is not an error (returns nil, false, nil) — that's
// the normal "this file was added on the other side" case in a 3-way diff.
//
// A missing REF, however, IS propagated as an error. ErrRefNotFound here
// almost always means LAST_SYNCED points at a commit the cache no longer
// has (e.g. cache was partially restored, packfile is incomplete, or the
// commit was GC'd). Treating that as "file absent at baseline" silently
// would misclassify every tracked file as remote-only-changed, turning
// every legitimate edit into a fake conflict. Surfacing the error lets
// callers (status, sync) recover by recommending FirstPull.
func (s *Syncer) readCacheAtRef(file, ref string) ([]byte, bool, error) {
	encrypted, err := s.cache.ReadEncryptedAtRef(file, ref)
	if err != nil {
		if errors.Is(err, domain.ErrFileNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	plain, err := s.encrypter.Decrypt(encrypted)
	if err != nil {
		return nil, false, err
	}
	return plain, true, nil
}

// readCacheAtHead returns the decrypted file content at HEAD (= remote after
// SyncFromStorage), plus existence. Missing file is not an error.
func (s *Syncer) readCacheAtHead(file string) ([]byte, bool, error) {
	encrypted, err := s.cache.ReadEncrypted(file)
	if err != nil {
		if errors.Is(err, domain.ErrFileNotFound) || errors.Is(err, domain.ErrNotInitialized) {
			return nil, false, nil
		}
		return nil, false, err
	}
	plain, err := s.encrypter.Decrypt(encrypted)
	if err != nil {
		return nil, false, err
	}
	return plain, true, nil
}

// readWorkingTree returns the project-directory content of file, plus existence.
func (s *Syncer) readWorkingTree(file string) ([]byte, bool, error) {
	if s.discovery == nil {
		return nil, false, nil
	}
	if !s.discovery.FileExists(file) {
		return nil, false, nil
	}
	content, err := s.discovery.ReadFile(file)
	if err != nil {
		return nil, false, err
	}
	return content, true, nil
}

// sameContent reports whether two (bytes, exists) pairs represent the same
// state — both absent, or both present and byte-equal.
func sameContent(a []byte, aExists bool, b []byte, bExists bool) bool {
	if aExists != bExists {
		return false
	}
	if !aExists {
		return true
	}
	return bytes.Equal(a, b)
}

// discoveryEnvFiles returns the tracked file list, propagating ErrNoFilesTracked
// distinctly so callers can treat "nothing to track" as a soft state, not an error.
func (s *Syncer) discoveryEnvFiles() ([]string, error) {
	if s.discovery == nil {
		return nil, domain.ErrNotInRepo
	}
	return s.discovery.EnvFiles()
}

// EnsureCacheInitialized ensures the cache is initialized
func (s *Syncer) EnsureCacheInitialized(ctx context.Context) error {
	if s.cache.Exists() {
		return nil
	}

	// Check if remote exists
	exists, err := s.cache.ExistsRemote(ctx)
	if err != nil {
		return err
	}

	if exists {
		// Sync from remote
		return s.cache.SyncFromStorage(ctx)
	}

	// Initialize new cache
	return s.cache.Init()
}
