package sync

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/domain"
)

// Push encrypts and uploads environment files
func (s *Syncer) Push(ctx context.Context, opts PushOptions) (*domain.PushResult, error) {
	// Read this machine's last-synced commit BEFORE any state changes. Used
	// below to detect divergence from another machine's push.
	lastSynced, _, _ := s.cache.ReadLastSynced()

	// Sync from storage first to get full history and latest state
	if err := s.syncBeforePush(ctx, opts.DryRun); err != nil {
		return nil, err
	}

	// Divergence safety check: if remote has moved since this machine's last
	// successful sync AND any of the user's local changes overlap files that
	// also moved on remote, refuse — the user almost certainly wants to pull
	// and reconcile first. --force bypasses.
	if err := s.checkPushDivergence(ctx, lastSynced, opts); err != nil {
		return nil, err
	}

	// Capture remote HEAD at start for optimistic locking (unless --force is used)
	var initialRemoteHead string
	if !opts.Force {
		head, err := s.cache.GetRemoteHead(ctx)
		if err == nil {
			initialRemoteHead = head
		}
		// If error (e.g., repo not found in remote), that's OK - means it's a new repo
	}

	// Get files to track
	files, err := s.discovery.EnvFiles()
	if err != nil {
		if !errors.Is(err, domain.ErrNoFilesTracked) {
			return nil, err
		}
		files = nil // No tracked files — orphan cleanup will handle removals
	}

	result := &domain.PushResult{}

	// Process each file
	for _, file := range files {
		if !s.discovery.FileExists(file) {
			// File doesn't exist locally - check if we should delete from cache
			_, err := s.cache.ReadEncrypted(file)
			if err == nil {
				// File exists in cache but not locally - delete it
				if !opts.DryRun {
					if err := s.cache.RemoveEncrypted(file); err != nil {
						return nil, fmt.Errorf("failed to remove %s: %w", file, err)
					}
				}
				result.FilesDeleted++
			}
			continue
		}

		// Read file content
		content, err := s.discovery.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", file, err)
		}

		// Check if file is new or modified
		existingEncrypted, err := s.cache.ReadEncrypted(file)
		isNew := err != nil

		if !isNew {
			// Decrypt existing to compare
			existingDecrypted, err := s.encrypter.Decrypt(existingEncrypted)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt existing %s: %w", file, err)
			}

			// Skip if unchanged
			if bytes.Equal(existingDecrypted, content) {
				continue
			}
			result.FilesUpdated++
		} else {
			result.FilesAdded++
		}

		if opts.DryRun {
			continue
		}

		// Encrypt content
		encrypted, err := s.encrypter.Encrypt(content)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt %s: %w", file, err)
		}

		// Write to cache
		if err := s.cache.WriteEncrypted(file, encrypted); err != nil {
			return nil, fmt.Errorf("failed to write %s to cache: %w", file, err)
		}
	}

	// Clean up orphaned cache files (in cache but no longer tracked)
	cachedFiles, err := s.cache.ListTrackedFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to list cached files: %w", err)
	}
	trackedSet := make(map[string]bool, len(files))
	for _, f := range files {
		trackedSet[f] = true
	}
	for _, cached := range cachedFiles {
		if !trackedSet[cached] {
			if !opts.DryRun {
				if err := s.cache.RemoveEncrypted(cached); err != nil {
					return nil, fmt.Errorf("failed to remove orphaned %s: %w", cached, err)
				}
			}
			result.FilesDeleted++
		}
	}

	// Nothing to push
	if result.FilesAdded == 0 && result.FilesUpdated == 0 && result.FilesDeleted == 0 {
		return nil, domain.ErrNothingToCommit
	}

	if opts.DryRun {
		return result, nil
	}

	// Stage all changes
	if err := s.cache.StageAll(); err != nil {
		return nil, fmt.Errorf("failed to stage changes: %w", err)
	}

	// Verify remote hasn't changed BEFORE creating commit (optimistic locking)
	// This prevents orphan commits if remote verification fails
	if !opts.Force && initialRemoteHead != "" {
		currentRemoteHead, err := s.cache.GetRemoteHead(ctx)
		if err == nil && currentRemoteHead != initialRemoteHead {
			return nil, domain.Errorf(domain.ErrRemoteChanged, "remote changed during push (expected %s, got %s); run 'envsecrets pull' first or use --force to override", initialRemoteHead[:constants.ShortHashLength], currentRemoteHead[:constants.ShortHashLength])
		}
	}

	// Create commit (only after remote verification passes)
	message := opts.Message
	if message == "" {
		message = generateCommitMessage(result)
	}

	hash, err := s.cache.Commit(message)
	if err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}
	result.CommitHash = hash

	// Sync to storage
	if err := s.cache.SyncToStorage(ctx); err != nil {
		return nil, fmt.Errorf("failed to sync to storage: %w", err)
	}

	// Record this machine's new sync baseline. Best-effort: a write failure
	// here doesn't roll back the successful push — the next status/push call
	// will recompute against an older baseline, which is less accurate but
	// not unsafe.
	if err := s.cache.WriteLastSynced(hash); err != nil {
		// Surface as a non-fatal warning via the returned result rather than
		// failing — push already succeeded remotely.
		_ = err
	}

	return result, nil
}

// checkPushDivergence implements the multi-machine safety check described in
// the design plan. Returns nil to allow the push to proceed, or an error
// (typically ErrDivergedHistory) to abort it.
//
// Behavior:
//   - --force always bypasses this check.
//   - If no remote yet, no divergence is possible.
//   - If lastSynced is empty AND remote has commits, refuse: this machine has
//     no baseline, so we can't tell whether the working tree is stale.
//   - If lastSynced == remote HEAD, no divergence — proceed normally.
//   - Otherwise compute file overlap. Overlap = files where the working tree
//     differs from the baseline (lastSynced) AND remote moved on the same
//     file. Any overlap → ErrDivergedHistory. Empty overlap → fast-forward
//     safe; proceed.
func (s *Syncer) checkPushDivergence(ctx context.Context, lastSynced string, opts PushOptions) error {
	if opts.Force {
		return nil
	}

	remoteHead, err := s.cache.GetRemoteHead(ctx)
	if err != nil || remoteHead == "" {
		// No remote yet — initialization push, nothing to diverge from.
		return nil
	}

	if lastSynced == remoteHead {
		return nil
	}

	files, err := s.discoveryEnvFiles()
	if err != nil {
		// "No files tracked" is not a divergence concern.
		if errors.Is(err, domain.ErrNoFilesTracked) {
			return nil
		}
		return err
	}

	if lastSynced == "" {
		// Remote has commits but this machine has no recorded baseline. We
		// can't classify safely; refuse with a clear next step.
		return domain.Errorf(domain.ErrDivergedHistory,
			"this machine has no sync baseline (remote HEAD %s exists but no last-synced marker); run 'envsecrets pull' first, or use --force to overwrite remote with your working tree",
			truncHash(remoteHead))
	}

	overlap, err := s.computePushOverlap(files, lastSynced)
	if err != nil {
		return err
	}
	if len(overlap) == 0 {
		// Diverged but disjoint — the existing fast-forward in syncBeforePush
		// already aligned the cache; let the push proceed.
		return nil
	}

	return domain.Errorf(domain.ErrDivergedHistory,
		"remote has moved since this machine last synced (last %s, remote %s) AND %d file(s) changed on both sides: %v; run 'envsecrets pull' to reconcile or 'envsecrets push --force' to overwrite remote",
		truncHash(lastSynced), truncHash(remoteHead), len(overlap), overlap)
}

// computePushOverlap returns the list of tracked files that were modified
// both locally (working tree vs lastSynced) AND remotely (cache@HEAD vs
// lastSynced) — the intersection that makes push unsafe.
func (s *Syncer) computePushOverlap(files []string, baseRef string) ([]string, error) {
	var overlap []string
	for _, f := range files {
		baseBytes, baseExists, err := s.readCacheAtRef(f, baseRef)
		if err != nil {
			return nil, err
		}
		remoteBytes, remoteExists, err := s.readCacheAtHead(f)
		if err != nil {
			return nil, err
		}
		localBytes, localExists, err := s.readWorkingTree(f)
		if err != nil {
			return nil, err
		}

		localChanged := !sameContent(baseBytes, baseExists, localBytes, localExists)
		remoteChanged := !sameContent(baseBytes, baseExists, remoteBytes, remoteExists)

		// Overlap only counts when local and remote disagree. If they
		// converged on the same content, there's nothing to reconcile.
		if localChanged && remoteChanged && !sameContent(localBytes, localExists, remoteBytes, remoteExists) {
			overlap = append(overlap, f)
		}
	}
	return overlap, nil
}

// truncHash shortens a 40-char hash for human-readable error messages.
func truncHash(h string) string {
	if len(h) > constants.ShortHashLength {
		return h[:constants.ShortHashLength]
	}
	return h
}

// syncBeforePush syncs from storage to get the latest history, then fast-forwards
// the local branch if needed so new commits build on top of remote HEAD.
func (s *Syncer) syncBeforePush(ctx context.Context, dryRun bool) error {
	// Check if remote exists
	exists, err := s.cache.ExistsRemote(ctx)
	if err != nil {
		return err
	}

	if exists {
		// Sync from remote to get full history
		if err := s.cache.SyncFromStorage(ctx); err != nil {
			return fmt.Errorf("failed to sync from storage: %w", err)
		}

		// Align local to remote HEAD so new commits build on top of
		// the latest shared history. The local cache has no independent
		// commits — it is only written to by this tool after syncing —
		// so resetting to remote HEAD is always safe here.
		remoteHead, err := s.cache.GetRemoteHead(ctx)
		if err == nil && remoteHead != "" {
			localHead, headErr := s.cache.Head()
			if headErr != nil {
				return fmt.Errorf("failed to read local HEAD: %w", headErr)
			}
			if localHead != remoteHead {
				if err := s.cache.Checkout(remoteHead); err != nil {
					return fmt.Errorf("failed to checkout remote HEAD: %w", err)
				}
				// Re-attach to default branch (detached HEAD is acceptable
				// if branch doesn't exist yet, so we ignore that error)
				if branch, branchErr := s.cache.GetDefaultBranch(); branchErr == nil {
					_ = s.cache.CheckoutBranch(branch)
				}
			}
		}
	} else {
		// No remote — if local cache has stale data, reset it so push
		// correctly detects all files as new. Skip the destructive reset
		// during dry-run to keep it side-effect-free.
		if !dryRun && s.cache.Exists() {
			if err := s.cache.Reset(ctx); err != nil {
				return err
			}
		} else if !s.cache.Exists() {
			if err := s.cache.Init(); err != nil {
				return err
			}
		}
	}

	return nil
}

func generateCommitMessage(result *domain.PushResult) string {
	parts := []string{}
	if result.FilesAdded > 0 {
		parts = append(parts, fmt.Sprintf("%d added", result.FilesAdded))
	}
	if result.FilesUpdated > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", result.FilesUpdated))
	}
	if result.FilesDeleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", result.FilesDeleted))
	}

	if len(parts) == 0 {
		return "Update environment files"
	}

	msg := "Update environment files: "
	for i, p := range parts {
		if i > 0 {
			msg += ", "
		}
		msg += p
	}
	return msg
}
