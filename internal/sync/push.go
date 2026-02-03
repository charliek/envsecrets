package sync

import (
	"bytes"
	"context"
	"fmt"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/domain"
)

// Push encrypts and uploads environment files
func (s *Syncer) Push(ctx context.Context, opts PushOptions) (*domain.PushResult, error) {
	// Ensure cache is initialized
	if err := s.EnsureCacheInitialized(ctx); err != nil {
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
		return nil, err
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
