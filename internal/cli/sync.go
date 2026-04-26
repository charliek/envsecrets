package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/sync"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/spf13/cobra"
)

var (
	syncMessage string
	syncDryRun  bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Run the recommended push/pull/reconcile action automatically",
	Long: `Run the recommended action based on this machine's sync state.

sync inspects the same signals 'status' uses, then performs the safe action:

  in_sync          -> nothing to do
  push             -> envsecrets push
  pull             -> envsecrets pull
  pull_then_push   -> pull, then push (no overlapping changes)
  reconcile        -> print guidance and exit non-zero (sync does NOT
                      auto-resolve overlapping conflicts; resolve with
                      'envsecrets pull' interactive prompts, then push)
  first_push_init  -> print "remote not initialized; run push" and exit non-zero
                      (initialization is intentional, not a side effect)

Use 'envsecrets push --force' directly when you want to override divergence.`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "show what would be done without doing it")
	syncCmd.Flags().StringVarP(&syncMessage, "message", "m", "", "commit message used when sync runs push")
}

func runSync(cmd *cobra.Command, args []string) error {
	ctx, cancel := signalContext()
	defer cancel()
	out := GetOutput()

	pc, err := NewProjectContext(ctx, cfg)
	if err != nil {
		return err
	}
	defer pc.Close()

	syncer := sync.NewSyncer(pc.Discovery, pc.RepoInfo, pc.Storage, pc.Encrypter, pc.Cache)

	status, err := syncer.GetSyncStatus(ctx)
	if err != nil {
		return err
	}

	if syncDryRun {
		out.PrintDryRunHeader()
		describeSyncAction(out, status)
		return nil
	}

	switch status.Action {
	case domain.SyncActionInSync:
		out.Println("Already in sync.")
		return nil

	case domain.SyncActionNothingTracked:
		out.Println("Nothing tracked yet — add files to .envsecrets and run: envsecrets push")
		return nil

	case domain.SyncActionFirstPushInit:
		out.Println("Remote not initialized. Run: envsecrets push")
		return domain.NewExitCodeError(domain.ErrActionRequired, exitCodeForActionRequired())

	case domain.SyncActionFirstPull, domain.SyncActionPull:
		return runSyncPull(ctx, syncer, status, out)

	case domain.SyncActionPush:
		return runSyncPush(ctx, syncer, status, out)

	case domain.SyncActionPullThenPush:
		if err := runSyncPull(ctx, syncer, status, out); err != nil {
			return err
		}
		// Re-fetch status after pull so the second leg works against the
		// updated baseline. Pull may have written LAST_SYNCED, and a
		// user-resolved conflict (skip) can leave us in an unexpected
		// state — dispatch on the new action rather than blindly pushing.
		updated, err := syncer.GetSyncStatus(ctx)
		if err != nil {
			return err
		}
		switch updated.Action {
		case domain.SyncActionInSync:
			return nil
		case domain.SyncActionPush:
			return runSyncPush(ctx, syncer, updated, out)
		case domain.SyncActionReconcile:
			out.Warn("reconcile required after pull: %d file(s) changed on both sides", len(updated.Conflicts))
			for _, f := range updated.Conflicts {
				out.Printf("  - %s\n", f)
			}
			out.Println("  Resolve with: envsecrets diff <file>, then envsecrets pull again, then envsecrets push")
			return domain.NewExitCodeError(domain.ErrActionRequired, exitCodeForActionRequired())
		default:
			// Pull, PullThenPush, FirstPull, FirstPushInit, NothingTracked —
			// shouldn't happen right after a successful pull, but if they
			// do, fall back to dispatching through the main loop.
			return fmt.Errorf("unexpected post-pull action: %q (re-run 'envsecrets sync' or 'envsecrets status' to inspect)", updated.Action)
		}

	case domain.SyncActionReconcile:
		out.Warn("reconcile required: %d file(s) changed on both sides", len(status.Conflicts))
		for _, f := range status.Conflicts {
			out.Printf("  - %s\n", f)
		}
		out.Println("  1. Review with: envsecrets diff <file>")
		out.Println("  2. Resolve with: envsecrets pull   (interactive: keep-local or overwrite per file)")
		out.Println("  3. Publish with: envsecrets push")
		return domain.NewExitCodeError(domain.ErrActionRequired, exitCodeForActionRequired())

	default:
		return fmt.Errorf("unknown sync action: %q", status.Action)
	}
}

func runSyncPull(ctx context.Context, syncer *sync.Syncer, status *domain.SyncStatus, out *ui.Output) error {
	if status.Action == domain.SyncActionFirstPull {
		// FirstPull returns from GetSyncStatus before per-file
		// classification runs, so RemoteChanges is empty even though
		// pull may write/update many files. Avoid misleading "0 remote
		// change(s)" messaging in that case.
		out.Println("Pulling remote state (first sync on this machine)...")
	} else {
		out.Printf("Pulling %d remote change(s)...\n", len(status.RemoteChanges))
	}
	opts := sync.PullOptions{}
	// Wire interactive conflict resolution exactly like 'pull' does, so the
	// user keeps the same UX they're used to if conflicts surface mid-pull.
	if ui.CanPrompt() {
		prompt := ui.NewPrompt()
		opts.ConflictResolver = func(f string) (sync.ConflictAction, error) {
			choice, err := prompt.ConflictChoice(f)
			if err != nil {
				return sync.ConflictAbort, err
			}
			switch choice {
			case "o":
				return sync.ConflictOverwrite, nil
			case "s":
				return sync.ConflictSkip, nil
			default:
				return sync.ConflictAbort, nil
			}
		}
	}

	result, err := syncer.Pull(ctx, opts)
	if err != nil {
		return err
	}
	if result.FilesCreated > 0 {
		out.Printf("  %d file(s) created\n", result.FilesCreated)
	}
	if result.FilesUpdated > 0 {
		out.Printf("  %d file(s) updated\n", result.FilesUpdated)
	}
	if result.FilesDeleted > 0 {
		out.Printf("  %d file(s) deleted (remote dropped them)\n", result.FilesDeleted)
	}
	if result.FilesKeptLocal > 0 {
		out.Printf("  %d file(s) kept (local edits preserved; push to publish)\n", result.FilesKeptLocal)
	}
	if result.FilesSkipped > 0 {
		out.Printf("  %d file(s) unchanged\n", result.FilesSkipped)
	}
	if result.Warning != "" {
		out.Warn("%s", result.Warning)
	}
	return nil
}

func runSyncPush(ctx context.Context, syncer *sync.Syncer, status *domain.SyncStatus, out *ui.Output) error {
	out.Printf("Pushing %d local change(s)...\n", len(status.LocalChanges))
	opts := sync.PushOptions{Message: syncMessage}
	result, err := syncer.Push(ctx, opts)
	if err != nil {
		if errors.Is(err, domain.ErrNothingToCommit) {
			out.Println("  nothing to push (already up to date)")
			return nil
		}
		if errors.Is(err, domain.ErrDivergedHistory) {
			// Diverged history has multiple causes; let the wrapped error
			// describe the specific one rather than presuming overlap.
			out.Warn("push refused: %v", err)
			out.Println("  Resolve with: envsecrets pull   (auto-handles non-overlapping changes; prompts on real conflicts)")
			out.Println("  Then re-run: envsecrets sync")
			out.Println("  Or override: envsecrets push --force")
		}
		return err
	}
	if result.FilesAdded > 0 {
		out.Printf("  %d file(s) added\n", result.FilesAdded)
	}
	if result.FilesUpdated > 0 {
		out.Printf("  %d file(s) updated\n", result.FilesUpdated)
	}
	if result.FilesDeleted > 0 {
		out.Printf("  %d file(s) deleted\n", result.FilesDeleted)
	}
	if result.CommitHash != "" {
		out.Printf("  commit: %s\n", ui.TruncateHash(result.CommitHash))
	}
	if result.Warning != "" {
		out.Warn("%s", result.Warning)
	}
	return nil
}

// describeSyncAction is the dry-run printer: it explains what sync *would*
// do without invoking push/pull. Mirrors the cases in runSync.
func describeSyncAction(out *ui.Output, s *domain.SyncStatus) {
	switch s.Action {
	case domain.SyncActionInSync:
		out.Println("Would do nothing — already in sync.")
	case domain.SyncActionNothingTracked:
		out.Println("Would do nothing — no files tracked.")
	case domain.SyncActionFirstPushInit:
		out.Println("Would refuse — remote not initialized; run 'envsecrets push' to initialize.")
	case domain.SyncActionFirstPull:
		// Status returns FirstPull before per-file classification runs, so
		// RemoteChanges is empty even though the pull will materialize the
		// entire remote state on first sync. Don't print "0 remote
		// change(s)" — say what's actually about to happen.
		out.Println("Would perform initial pull and materialize the entire remote state.")
	case domain.SyncActionPull:
		out.Printf("Would pull %d remote change(s).\n", len(s.RemoteChanges))
	case domain.SyncActionPush:
		out.Printf("Would push %d local change(s).\n", len(s.LocalChanges))
	case domain.SyncActionPullThenPush:
		out.Printf("Would pull %d remote change(s), then push %d local change(s).\n",
			len(s.RemoteChanges), len(s.LocalChanges))
	case domain.SyncActionReconcile:
		out.Printf("Would refuse — %d file(s) changed on both sides; reconcile manually.\n",
			len(s.Conflicts))
	default:
		out.Printf("Unknown action: %q\n", s.Action)
	}
}

// exitCodeForActionRequired is the exit code used when sync needs the user
// to take a manual step (reconcile, init).
func exitCodeForActionRequired() int {
	return constants.ExitActionRequired
}
