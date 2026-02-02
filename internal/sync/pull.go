package sync

import (
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

		if fileExists && string(existingContent) == string(decrypted) {
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

	// If there are conflicts and --force is not set, abort
	if len(result.FilesWithConflicts) > 0 && !opts.Force {
		return result, domain.Errorf(domain.ErrConflict, "local files would be overwritten: %v; use --force to overwrite", result.FilesWithConflicts)
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
