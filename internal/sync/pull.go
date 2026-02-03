package sync

import (
	"bytes"
	"context"
	"fmt"

	"github.com/charliek/envsecrets/internal/domain"
)

// Pull downloads and decrypts environment files
func (s *Syncer) Pull(ctx context.Context, opts PullOptions) (*domain.PullResult, error) {
	result := &domain.PullResult{}

	// Check if remote exists
	exists, err := s.cache.ExistsRemote(ctx)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, domain.Errorf(domain.ErrRepoNotFound, "repository not found in remote storage")
	}

	// Sync from storage
	if err := s.cache.SyncFromStorage(ctx); err != nil {
		return nil, fmt.Errorf("failed to sync from storage: %w", err)
	}

	// Checkout specific ref if provided
	if opts.Ref != "" {
		if err := s.cache.Checkout(opts.Ref); err != nil {
			return nil, err
		}
		result.Ref = opts.Ref
	} else {
		head, err := s.cache.Head()
		if err != nil {
			return nil, err
		}
		result.Ref = head
	}

	// Get list of files to pull
	files, err := s.discovery.EnvFiles()
	if err != nil {
		return nil, err
	}

	// First pass: detect conflicts (files that would be overwritten)
	type fileToWrite struct {
		file      string
		decrypted []byte
		isNew     bool
	}
	var filesToWrite []fileToWrite

	for _, file := range files {
		// Read encrypted content from cache
		encrypted, err := s.cache.ReadEncrypted(file)
		if err != nil {
			// File doesn't exist in cache
			continue
		}

		// Decrypt
		decrypted, err := s.encrypter.Decrypt(encrypted)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt %s: %w", file, err)
		}

		// Check if file exists and is different
		existingContent, err := s.discovery.ReadFile(file)
		fileExists := err == nil

		if fileExists && bytes.Equal(existingContent, decrypted) {
			// File unchanged
			result.FilesSkipped++
			continue
		}

		// File would be modified - track it
		if fileExists {
			result.FilesWithConflicts = append(result.FilesWithConflicts, file)
		}

		filesToWrite = append(filesToWrite, fileToWrite{
			file:      file,
			decrypted: decrypted,
			isNew:     !fileExists,
		})
	}

	// Handle conflicts
	if len(result.FilesWithConflicts) > 0 && !opts.Force && !opts.DryRun {
		if opts.ConflictResolver == nil {
			// No resolver - abort (current behavior)
			return result, domain.Errorf(domain.ErrConflict, "local files would be overwritten: %v; use --force to overwrite", result.FilesWithConflicts)
		}

		// Resolve each conflict
		resolvedSkips := make(map[string]bool)
		for _, file := range result.FilesWithConflicts {
			action, err := opts.ConflictResolver(file)
			if err != nil {
				return result, fmt.Errorf("conflict resolution failed for %s: %w", file, err)
			}
			switch action {
			case ConflictAbort:
				return result, domain.ErrUserCancelled
			case ConflictSkip:
				resolvedSkips[file] = true
			case ConflictOverwrite:
				// Do nothing, file will be written
			default:
				return result, fmt.Errorf("invalid conflict action %d for %s", action, file)
			}
		}

		// Filter filesToWrite to exclude skipped files
		var filtered []fileToWrite
		for _, ftw := range filesToWrite {
			if resolvedSkips[ftw.file] {
				result.FilesSkippedConflict++
				continue
			}
			filtered = append(filtered, ftw)
		}
		filesToWrite = filtered
		result.FilesWithConflicts = nil
	}

	// In dry-run mode, just count what would be written without actually writing
	if opts.DryRun {
		for _, ftw := range filesToWrite {
			if ftw.isNew {
				result.FilesCreated++
			} else {
				result.FilesUpdated++
			}
		}
		return result, nil
	}

	// Second pass: write files
	for _, ftw := range filesToWrite {
		if err := s.discovery.WriteFile(ftw.file, ftw.decrypted); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", ftw.file, err)
		}

		if ftw.isNew {
			result.FilesCreated++
		} else {
			result.FilesUpdated++
		}
	}

	// Clear conflicts from result if we successfully wrote them (with --force)
	if opts.Force {
		result.FilesWithConflicts = nil
	}

	return result, nil
}

// PullFile downloads and decrypts a single file
func (s *Syncer) PullFile(ctx context.Context, filename string, ref string) ([]byte, error) {
	// Ensure cache is synced
	if err := s.cache.SyncFromStorage(ctx); err != nil {
		return nil, err
	}

	// Read from specific ref or working tree
	var encrypted []byte
	var err error

	if ref != "" {
		encrypted, err = s.cache.ReadEncryptedAtRef(filename, ref)
	} else {
		encrypted, err = s.cache.ReadEncrypted(filename)
	}

	if err != nil {
		return nil, err
	}

	// Decrypt
	decrypted, err := s.encrypter.Decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt %s: %w", filename, err)
	}

	return decrypted, nil
}
