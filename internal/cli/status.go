package cli

import (
	"context"
	"fmt"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show repository info and file status",
	Long: `Show the current repository information and status of tracked files.

Displays:
- Repository identification (owner/repo)
- List of tracked files and their status
- Sync status with remote`,
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

	// Output repository info
	if out.IsJSON() {
		return outputStatusJSON(pc, ctx)
	}

	out.Println("Repository:", pc.RepoInfo.String())
	out.Println("Bucket:", cfg.Bucket)
	out.Println()

	// Check remote status
	existsRemote, remoteErr := pc.Cache.ExistsRemote(ctx)
	if remoteErr != nil {
		out.Warn("Could not check remote status: %v", remoteErr)
	} else if existsRemote {
		out.Println("Remote: initialized")

		remoteHead, err := pc.Cache.GetRemoteHead(ctx)
		if err == nil {
			if len(remoteHead) > constants.ShortHashLength {
				remoteHead = remoteHead[:constants.ShortHashLength]
			}
			out.Println("Remote HEAD:", remoteHead)
		}

		formatInfo, err := pc.Cache.DetectRemoteVersion(ctx)
		if err == nil {
			if formatInfo.Detected {
				out.Printf("Storage format: v%d\n", formatInfo.Version)
			} else {
				out.Println("Storage format: unknown (no FORMAT marker)")
			}
		}
	} else {
		out.Println("Remote: not initialized (run 'envsecrets push' to initialize)")
	}

	// Sync cache for accurate file status
	if existsRemote {
		if err := pc.Cache.SyncFromStorage(ctx); err != nil {
			out.Warn("Could not sync from remote (status may be stale): %v", err)
		}
	} else if remoteErr == nil && pc.Cache.Exists() {
		// Remote is confirmed empty (not just unreachable) but cache has stale data
		if err := pc.Cache.Reset(ctx); err != nil {
			out.Warn("Could not reset stale cache: %v", err)
		}
	}

	out.Println()
	out.Println("Tracked files:")

	// Get file statuses
	statuses, err := pc.GetFileStatuses()
	if err != nil {
		return err
	}

	if len(statuses) == 0 {
		out.Println("  (no files tracked)")
		return nil
	}

	for _, status := range statuses {
		out.PrintFileStatusDetailed(status)
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

	return nil
}

func outputStatusJSON(pc *ProjectContext, ctx context.Context) error {
	out := GetOutput()

	// For JSON output, we still want to include the data even if remote checks fail
	// but we include any errors in the output
	var remoteError string
	existsRemote, err := pc.Cache.ExistsRemote(ctx)
	if err != nil {
		remoteError = err.Error()
	}

	// Sync cache for accurate file status
	if existsRemote {
		if err := pc.Cache.SyncFromStorage(ctx); err != nil {
			if remoteError == "" {
				remoteError = err.Error()
			}
		}
	} else if remoteError == "" && pc.Cache.Exists() {
		// Remote is confirmed empty (not just unreachable) but cache has stale data
		if err := pc.Cache.Reset(ctx); err != nil {
			if remoteError == "" {
				remoteError = err.Error()
			}
		}
	}

	statuses, err := pc.GetFileStatuses()
	if err != nil {
		return err
	}

	remoteHead := ""
	if existsRemote {
		remoteHead, err = pc.Cache.GetRemoteHead(ctx)
		if err != nil && remoteError == "" {
			remoteError = err.Error()
		}
	}

	var formatVersion interface{}
	if existsRemote {
		formatInfo, err := pc.Cache.DetectRemoteVersion(ctx)
		if err == nil {
			formatVersion = formatInfo
		} else if remoteError == "" {
			remoteError = err.Error()
		}
	}

	counts := countFileStatuses(statuses)

	data := map[string]interface{}{
		"repository":     pc.RepoInfo.String(),
		"bucket":         cfg.Bucket,
		"remote_exists":  existsRemote,
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
	}

	// Include error in JSON output if there was one
	if remoteError != "" {
		data["remote_error"] = remoteError
	}

	return out.JSON(data)
}
