package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/sync"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/spf13/cobra"
)

// staleBaselineThreshold is how long a machine can be in-sync before status
// emits the "you may have unpushed work elsewhere" nudge.
const staleBaselineThreshold = 7 * 24 * time.Hour

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show repository info, file status, and recommended next action",
	Long: `Show the current repository information, status of tracked files, and a
recommended next action (push / pull / reconcile / in-sync).

Displays:
- Repository identification (owner/repo)
- Remote status and HEAD provenance (who pushed it, when)
- This machine's last-synced commit and how stale it is
- Per-file status against the local cache
- A recommendation that tells you exactly what to run next`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := signalContext()
	defer cancel()
	out := GetOutput()

	// Create project context
	pc, err := NewProjectContext(ctx, cfg)
	if err != nil {
		return err
	}
	defer pc.Close()

	// Compute the full sync picture once. GetSyncStatus internally calls
	// SyncFromStorage when a remote exists, so the cache reflects remote
	// state by the time we read per-file statuses below.
	syncer := sync.NewSyncer(pc.Discovery, pc.RepoInfo, pc.Storage, pc.Encrypter, pc.Cache)
	syncStatus, syncErr := syncer.GetSyncStatus(ctx)

	if out.IsJSON() {
		return outputStatusJSON(pc, ctx, syncStatus, syncErr)
	}

	out.Println("Repository:", pc.RepoInfo.String())
	out.Println("Bucket:", cfg.Bucket)
	out.Println()

	if syncErr != nil {
		out.Warn("Could not compute sync status: %v", syncErr)
	}

	// Remote status block
	if syncStatus != nil && syncStatus.RemoteHead != "" {
		out.Println("Remote: initialized")
		line := fmt.Sprintf("Remote HEAD: %s", ui.TruncateHash(syncStatus.RemoteHead))
		if syncStatus.RemoteAuthor != "" {
			line += fmt.Sprintf(" — pushed by %s", syncStatus.RemoteAuthor)
		}
		if !syncStatus.RemoteCommittedAt.IsZero() {
			line += fmt.Sprintf(" %s", humanizeAgo(syncStatus.RemoteCommittedAt))
		}
		out.Println(line)

		formatInfo, err := pc.Cache.DetectRemoteVersion(ctx)
		if err == nil {
			if formatInfo.Detected {
				out.Printf("Storage format: v%d\n", formatInfo.Version)
			} else {
				out.Println("Storage format: unknown (no FORMAT marker)")
			}
		}
	} else if syncStatus != nil {
		out.Println("Remote: not initialized (run 'envsecrets push' to initialize)")
	}

	// This machine's sync baseline
	if syncStatus != nil {
		if syncStatus.LastSynced != "" {
			line := fmt.Sprintf("Last synced: %s", ui.TruncateHash(syncStatus.LastSynced))
			if !syncStatus.LastSyncedAt.IsZero() {
				line += fmt.Sprintf(" %s", humanizeAgo(syncStatus.LastSyncedAt))
			}
			out.Println(line)
		} else if syncStatus.RemoteHead != "" {
			out.Println("Last synced: (never on this machine)")
		}
	}

	out.Println()
	out.Println("Tracked files:")

	// Get file statuses (cache is now in sync with remote thanks to GetSyncStatus)
	statuses, err := pc.GetFileStatuses()
	if err != nil {
		return err
	}

	if len(statuses) == 0 {
		out.Println("  (no files tracked)")
	} else {
		for _, status := range statuses {
			out.PrintFileStatusDetailed(status)
		}
	}

	// Summary
	counts := countFileStatuses(statuses)
	out.Println()
	summary := fmt.Sprintf("Summary: %d added, %d modified, %d deleted, %d unchanged",
		counts.Added, counts.Modified, counts.Deleted, counts.Unchanged)
	if counts.NotSynced > 0 {
		summary += fmt.Sprintf(", %d not synced", counts.NotSynced)
	}
	out.Println(summary)

	// Recommendation block
	if syncStatus != nil {
		out.Println()
		out.Println("Sync status:")
		printSyncRecommendation(out, syncStatus)
	}

	return nil
}

// printSyncRecommendation prints a one- or multi-line "do this next" block.
// The branches mirror domain.SyncAction values.
func printSyncRecommendation(out *ui.Output, s *domain.SyncStatus) {
	switch s.Action {
	case domain.SyncActionInSync:
		out.Println("  ✓ In sync. Nothing to do.")
		// Stale-baseline hint: in-sync but it's been a while since this
		// machine touched remote — other machines may have unpushed work.
		if !s.LastSyncedAt.IsZero() && time.Since(s.LastSyncedAt) > staleBaselineThreshold {
			out.Printf("  Note: this machine has been in sync for %s. If you've been working\n",
				humanizeDuration(time.Since(s.LastSyncedAt)))
			out.Println("  on other machines, run `envsecrets status` there before assuming nothing changed.")
		}
	case domain.SyncActionPush:
		out.Printf("  → You have %d local change(s). Run: envsecrets push\n", len(s.LocalChanges))
		printFileList(out, "    ", s.LocalChanges)
	case domain.SyncActionPull:
		out.Printf("  → Remote has %d new change(s). Run: envsecrets pull\n", len(s.RemoteChanges))
		printFileList(out, "    ", s.RemoteChanges)
	case domain.SyncActionPullThenPush:
		out.Printf("  → Both sides changed (no overlap). Run: envsecrets pull && envsecrets push\n")
		out.Printf("    Local-only changes (%d):\n", len(s.LocalChanges))
		printFileList(out, "      ", s.LocalChanges)
		out.Printf("    Remote-only changes (%d):\n", len(s.RemoteChanges))
		printFileList(out, "      ", s.RemoteChanges)
	case domain.SyncActionReconcile:
		out.Printf("  ! Reconcile needed: %d file(s) changed on both sides:\n", len(s.Conflicts))
		printFileList(out, "      ", s.Conflicts)
		out.Println("    1. Review with: envsecrets diff <file>")
		out.Println("    2. Resolve with: envsecrets pull   (interactive: keep-local or overwrite)")
		out.Println("    3. Publish with: envsecrets push")
	case domain.SyncActionFirstPushInit:
		out.Println("  → Remote not initialized. Run: envsecrets push")
	case domain.SyncActionFirstPull:
		out.Println("  → No sync baseline on this machine. Run: envsecrets pull")
	case domain.SyncActionNothingTracked:
		out.Println("  Nothing tracked yet — add files to .envsecrets and run: envsecrets push")
	default:
		// Unknown action — fall back to head equality
		if s.InSync {
			out.Println("  ✓ In sync.")
		} else {
			out.Printf("  Local: %s, Remote: %s\n", ui.TruncateHash(s.LocalHead), ui.TruncateHash(s.RemoteHead))
		}
	}
}

// printFileList prints up to 10 paths indented; collapses the rest with a count.
func printFileList(out *ui.Output, indent string, files []string) {
	const maxShow = 10
	for i, f := range files {
		if i >= maxShow {
			out.Printf("%s... (%d more)\n", indent, len(files)-maxShow)
			break
		}
		out.Printf("%s- %s\n", indent, f)
	}
}

// humanizeAgo returns a parenthesized "(3m ago)"-style suffix.
func humanizeAgo(t time.Time) string {
	return "(" + humanizeDuration(time.Since(t)) + " ago)"
}

// humanizeDuration formats a duration with the largest reasonable unit.
func humanizeDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func outputStatusJSON(pc *ProjectContext, ctx context.Context, syncStatus *domain.SyncStatus, syncErr error) error {
	out := GetOutput()

	statuses, err := pc.GetFileStatuses()
	if err != nil {
		return err
	}

	var formatVersion interface{}
	if syncStatus != nil && syncStatus.RemoteHead != "" {
		formatInfo, err := pc.Cache.DetectRemoteVersion(ctx)
		if err == nil {
			formatVersion = formatInfo
		}
	}

	counts := countFileStatuses(statuses)

	remoteHead := ""
	if syncStatus != nil {
		remoteHead = syncStatus.RemoteHead
	}

	data := map[string]interface{}{
		"repository":     pc.RepoInfo.String(),
		"bucket":         cfg.Bucket,
		"remote_exists":  remoteHead != "",
		"remote_head":    remoteHead,
		"storage_format": formatVersion,
		"files":          statuses,
		"summary": map[string]int{
			"added":      counts.Added,
			"modified":   counts.Modified,
			"deleted":    counts.Deleted,
			"unchanged":  counts.Unchanged,
			"not_synced": counts.NotSynced,
		},
		"sync": syncStatus,
	}

	if syncErr != nil {
		data["sync_error"] = syncErr.Error()
	}

	return out.JSON(data)
}
