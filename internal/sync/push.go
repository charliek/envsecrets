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
	// Sync from storage first to get full history and latest state
	if err := s.syncBeforePush(ctx); err != nil {
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

	return result, nil
}

// syncBeforePush syncs from storage to get the latest history, then fast-forwards
// the local branch if needed so new commits build on top of remote HEAD.
func (s *Syncer) syncBeforePush(ctx context.Context) error {
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
		// correctly detects all files as new.
		if s.cache.Exists() {
			if err := s.cache.Reset(ctx); err != nil {
				return err
			}
		} else {
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
