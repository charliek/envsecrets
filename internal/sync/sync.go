package sync

import (
	"context"

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
}

// PullOptions configures a pull operation.
type PullOptions struct {
	// Ref specifies a specific version (commit hash) to pull
	Ref string
	// Force overwrites local files that have different content without prompting.
	// When false, pull will abort with ErrConflict if local files would be overwritten.
	Force bool
}

// GetSyncStatus returns the sync status between local and remote
func (s *Syncer) GetSyncStatus(ctx context.Context) (*domain.SyncStatus, error) {
	status := &domain.SyncStatus{}

	// Get local head
	localHead, err := s.cache.Head()
	if err != nil {
		if err == domain.ErrNotInitialized {
			localHead = ""
		} else {
			return nil, err
		}
	}
	status.LocalHead = localHead

	// Get remote head
	remoteHead, err := s.cache.GetRemoteHead(ctx)
	if err != nil {
		// Remote doesn't exist yet
		status.RemoteHead = ""
	} else {
		status.RemoteHead = remoteHead
	}

	// Determine sync status
	status.InSync = status.LocalHead == status.RemoteHead

	return status, nil
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
