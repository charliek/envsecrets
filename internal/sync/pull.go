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

	// First pass: classify each tracked file using the same 3-way logic
	// status uses, treating absence on either side as a real state. This
	// covers the deletion cases (remote removed a file, user removed a
	// file) which a content-only diff would silently mishandle.
	type fileToWrite struct {
		file      string
		decrypted []byte
		isNew     bool
	}
	var filesToWrite []fileToWrite
	var filesToDelete []string

	for _, file := range files {
		// Remote state (cache@HEAD).
		var (
			decrypted    []byte
			remoteExists bool
		)
		encrypted, err := s.cache.ReadEncrypted(file)
		switch {
		case err == nil:
			plain, decErr := s.encrypter.Decrypt(encrypted)
			if decErr != nil {
				return nil, fmt.Errorf("failed to decrypt %s: %w", file, decErr)
			}
			decrypted = plain
			remoteExists = true
		case errors.Is(err, domain.ErrFileNotFound):
			// Tracked file is absent at remote HEAD. Either the user added
			// it locally and never pushed, or another machine deleted it.
			// The 3-way diff below distinguishes the two.
			remoteExists = false
		default:
			// IO / corruption — must surface, not silently skip.
			return nil, fmt.Errorf("failed to read %s from cache: %w", file, err)
		}

		// Working-tree state.
		existingContent, readErr := s.discovery.ReadFile(file)
		fileExists := readErr == nil

		// Cheap fast-path: both already in identical state.
		if remoteExists && fileExists && bytes.Equal(existingContent, decrypted) {
			result.FilesSkipped++
			continue
		}
		if !remoteExists && !fileExists {
			result.FilesSkipped++
			continue
		}

		// Baseline state (cache@LAST_SYNCED) if available.
		var (
			basePlain  []byte
			baseExists bool
		)
		if lastSynced != "" {
			if baseEnc, baseErr := s.cache.ReadEncryptedAtRef(file, lastSynced); baseErr == nil {
				if plain, decErr := s.encrypter.Decrypt(baseEnc); decErr == nil {
					basePlain = plain
					baseExists = true
				}
			}
			// baseErr non-nil here is acceptable: file was absent at the
			// baseline, or decrypt failed (rare, transient). Falling
			// through with baseExists=false produces the pessimistic
			// fallback further down — strictly safer than silently
			// "deciding" with corrupt baseline data.
		}

		var localChanged, remoteChanged bool
		if baseExists || lastSynced != "" {
			localChanged = !sameContent(existingContent, fileExists, basePlain, baseExists)
			remoteChanged = !sameContent(decrypted, remoteExists, basePlain, baseExists)
		} else {
			// No baseline available at all — pessimistic fallback below.
			localChanged = true
			remoteChanged = true
		}

		switch {
		case !localChanged && remoteChanged:
			// Catch-up: remote moved, user has no local edit. Apply remote.
			if remoteExists {
				filesToWrite = append(filesToWrite, fileToWrite{file: file, decrypted: decrypted, isNew: !fileExists})
			} else if fileExists {
				// Remote deleted; user's working tree still has the
				// stale content. Delete locally so the next push doesn't
				// resurrect the file.
				filesToDelete = append(filesToDelete, file)
			}

		case localChanged && !remoteChanged:
			// User has a local-only edit (or local-only delete). Preserve
			// — push will publish it. Counted distinctly so CLI output
			// isn't misleading ("unchanged" would lie).
			result.FilesKeptLocal++

		case localChanged && remoteChanged:
			// Both sides changed. Conflict only when the resulting states
			// disagree — both deleting is a no-op convergence, and both
			// editing to identical content is also fine.
			bothEqualNow := sameContent(existingContent, fileExists, decrypted, remoteExists)
			if bothEqualNow {
				if remoteExists {
					result.FilesSkipped++
				} else {
					// both-deleted: nothing to do
					result.FilesSkipped++
				}
				continue
			}
			result.FilesWithConflicts = append(result.FilesWithConflicts, file)
			if remoteExists {
				filesToWrite = append(filesToWrite, fileToWrite{file: file, decrypted: decrypted, isNew: !fileExists})
			} else if fileExists {
				// Local edited, remote deleted → unresolved conflict.
				// On forced/overwrite resolution, mirror the remote
				// state by deleting locally.
				filesToDelete = append(filesToDelete, file)
			}

		default:
			// Pessimistic fallback (no baseline): any disagreement is a
			// potential conflict. Matches the historical behavior for
			// fresh / post-Reset machines.
			result.FilesWithConflicts = append(result.FilesWithConflicts, file)
			if remoteExists {
				filesToWrite = append(filesToWrite, fileToWrite{file: file, decrypted: decrypted, isNew: !fileExists})
			} else if fileExists {
				filesToDelete = append(filesToDelete, file)
			}
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
				// Do nothing, file will be written / deleted
			default:
				return result, fmt.Errorf("invalid conflict action %d for %s", action, file)
			}
		}

		// Filter both write and delete lists to exclude skipped files
		var filteredWrites []fileToWrite
		for _, ftw := range filesToWrite {
			if resolvedSkips[ftw.file] {
				result.FilesSkippedConflict++
				continue
			}
			filteredWrites = append(filteredWrites, ftw)
		}
		filesToWrite = filteredWrites

		var filteredDeletes []string
		for _, f := range filesToDelete {
			if resolvedSkips[f] {
				result.FilesSkippedConflict++
				continue
			}
			filteredDeletes = append(filteredDeletes, f)
		}
		filesToDelete = filteredDeletes

		result.FilesWithConflicts = nil
	}

	// In dry-run mode, just count what would be written/deleted without
	// actually touching the working tree.
	if opts.DryRun {
		for _, ftw := range filesToWrite {
			if ftw.isNew {
				result.FilesCreated++
			} else {
				result.FilesUpdated++
			}
		}
		result.FilesDeleted += len(filesToDelete)
		return result, nil
	}

	// Second pass: apply writes.
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

	// Third pass: apply deletes (after writes so write+delete on the same
	// file in different order is consistent — though by construction a
	// single file is in at most one of the two slices).
	for _, f := range filesToDelete {
		if err := s.discovery.RemoveFile(f); err != nil {
			return nil, fmt.Errorf("failed to remove %s: %w", f, err)
		}
		result.FilesDeleted++
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
