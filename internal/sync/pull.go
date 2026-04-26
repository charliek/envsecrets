package sync

import (
	"bytes"
	"context"
	"errors"
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

		// Return to default branch to avoid detached HEAD state.
		// The working tree files from the checked-out ref are preserved (Keep: true).
		// If no default branch is detected (new repo without commits), skip this step.
		if branch, err := s.cache.GetDefaultBranch(); err == nil {
			_ = s.cache.CheckoutBranch(branch)
		}
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

	// Read this machine's last-synced baseline so per-file decisions can
	// use the same 3-way classification status uses:
	//
	//   working tree vs cache@LAST_SYNCED vs cache@HEAD (= remote)
	//
	//   no local change, no remote change → skip (already synced)
	//   no local change, remote changed   → overwrite working tree
	//   local changed, no remote change   → preserve working tree (push will publish)
	//   local changed, remote changed     → conflict (require resolver / --force)
	//
	// Without a baseline (lastSynced == "") we fall back to the older,
	// pessimistic behavior: any working-tree disagreement is treated as
	// a conflict. That's correct because we genuinely cannot tell whether
	// a difference is a local edit or just stale content.
	lastSynced, _, _ := s.cache.ReadLastSynced()

	// First pass: classify each tracked file.
	type fileToWrite struct {
		file      string
		decrypted []byte
		isNew     bool
	}
	var filesToWrite []fileToWrite

	for _, file := range files {
		// Read encrypted content at remote HEAD (cache is synced above).
		// Distinguish "file not present at HEAD" (a normal case — the user
		// added a tracked file that no machine has pushed yet) from any
		// other read failure (corrupt cache, IO error, permission). The
		// latter must surface as a hard error so the user knows pull is
		// not silently leaving stale content in place.
		encrypted, err := s.cache.ReadEncrypted(file)
		if err != nil {
			if errors.Is(err, domain.ErrFileNotFound) {
				continue
			}
			return nil, fmt.Errorf("failed to read %s from cache: %w", file, err)
		}

		decrypted, err := s.encrypter.Decrypt(encrypted)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt %s: %w", file, err)
		}

		existingContent, readErr := s.discovery.ReadFile(file)
		fileExists := readErr == nil

		if fileExists && bytes.Equal(existingContent, decrypted) {
			// Working tree already matches remote; nothing to do.
			result.FilesSkipped++
			continue
		}

		// Compute relationship to baseline if we have one.
		var baseExists bool
		var basePlain []byte
		if lastSynced != "" {
			if baseEnc, baseErr := s.cache.ReadEncryptedAtRef(file, lastSynced); baseErr == nil {
				if plain, decErr := s.encrypter.Decrypt(baseEnc); decErr == nil {
					basePlain = plain
					baseExists = true
				}
			}
		}

		localChanged := true
		remoteChanged := true
		if baseExists {
			if fileExists {
				localChanged = !bytes.Equal(existingContent, basePlain)
			} else {
				localChanged = true // user deleted a tracked file vs baseline
			}
			remoteChanged = !bytes.Equal(decrypted, basePlain)
		}

		switch {
		case !localChanged && remoteChanged:
			// User has not edited this file since LAST_SYNCED; remote moved.
			// Overwrite without prompting — this is the natural catch-up case.
			filesToWrite = append(filesToWrite, fileToWrite{
				file:      file,
				decrypted: decrypted,
				isNew:     !fileExists,
			})

		case localChanged && !remoteChanged:
			// User edited this file locally; remote didn't touch it. Pull
			// must NOT overwrite — push will publish the local edit. This
			// is the disjoint-changes leg of pull_then_push.
			result.FilesSkipped++

		case localChanged && remoteChanged:
			// Both sides changed. Real conflict — surface to caller.
			if fileExists {
				result.FilesWithConflicts = append(result.FilesWithConflicts, file)
			}
			filesToWrite = append(filesToWrite, fileToWrite{
				file:      file,
				decrypted: decrypted,
				isNew:     !fileExists,
			})

		default:
			// Pessimistic fallback when no baseline is available: treat
			// any working-tree disagreement as a potential conflict, the
			// historical behavior for fresh / post-Reset machines.
			if fileExists {
				result.FilesWithConflicts = append(result.FilesWithConflicts, file)
			}
			filesToWrite = append(filesToWrite, fileToWrite{
				file:      file,
				decrypted: decrypted,
				isNew:     !fileExists,
			})
		}
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

	// Record this machine's new sync baseline only for full-HEAD pulls (not
	// --ref, which intentionally checks out a historical commit). Best-effort:
	// a write failure here doesn't roll back successfully-pulled files, but
	// it MUST be surfaced — symmetric with push's behavior. Otherwise the
	// next `status` would silently use a stale baseline and could mislead
	// the user about whether they need to push or pull.
	if !opts.DryRun && opts.Ref == "" {
		head, err := s.cache.Head()
		if err == nil && head != "" {
			if wErr := s.cache.WriteLastSynced(head); wErr != nil {
				result.Warning = fmt.Sprintf(
					"pull succeeded but failed to update local sync baseline: %v; subsequent status may show stale baseline info until the next successful push or pull",
					wErr,
				)
			}
		}
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
