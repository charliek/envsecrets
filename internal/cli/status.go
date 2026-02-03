package cli

import (
	"context"

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
	existsRemote, err := pc.Cache.ExistsRemote(ctx)
	if err != nil {
		out.Warn("Could not check remote status: %v", err)
	} else if existsRemote {
		out.Println("Remote: initialized")

		remoteHead, err := pc.Cache.GetRemoteHead(ctx)
		if err == nil {
			if len(remoteHead) > constants.ShortHashLength {
				remoteHead = remoteHead[:constants.ShortHashLength]
			}
			out.Println("Remote HEAD:", remoteHead)
		}
	} else {
		out.Println("Remote: not initialized (run 'envsecrets push' to initialize)")
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
	var added, modified, deleted, unchanged int
	for _, s := range statuses {
		switch {
		case s.LocalExists && !s.CacheExists:
			added++
		case !s.LocalExists && s.CacheExists:
			deleted++
		case s.Modified:
			modified++
		default:
			unchanged++
		}
	}

	out.Println()
	out.Printf("Summary: %d added, %d modified, %d deleted, %d unchanged\n",
		added, modified, deleted, unchanged)

	return nil
}

func outputStatusJSON(pc *ProjectContext, ctx context.Context) error {
	out := GetOutput()

	statuses, err := pc.GetFileStatuses()
	if err != nil {
		return err
	}

	// For JSON output, we still want to include the data even if remote checks fail
	// but we include any errors in the output
	var remoteError string
	existsRemote, err := pc.Cache.ExistsRemote(ctx)
	if err != nil {
		remoteError = err.Error()
	}

	remoteHead := ""
	if existsRemote {
		remoteHead, err = pc.Cache.GetRemoteHead(ctx)
		if err != nil && remoteError == "" {
			remoteError = err.Error()
		}
	}

	data := map[string]interface{}{
		"repository":    pc.RepoInfo.String(),
		"bucket":        cfg.Bucket,
		"remote_exists": existsRemote,
		"remote_head":   remoteHead,
		"files":         statuses,
	}

	// Include error in JSON output if there was one
	if remoteError != "" {
		data["remote_error"] = remoteError
	}

	return out.JSON(data)
}
