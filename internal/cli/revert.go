package cli

import (
	"fmt"

	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/sync"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/spf13/cobra"
)

var (
	revertDryRun  bool
	revertPush    bool
	revertMessage string
	revertYes     bool
)

var revertCmd = &cobra.Command{
	Use:   "revert [ref]",
	Short: "Restore files from a previous version",
	Long: `Restore environment files from a previous version.

This pulls files from the specified ref and writes them to your project directory.
Use --push to automatically push the reverted state as a new commit.

If no ref is provided in interactive mode, you can pick from recent commits.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRevert,
}

func init() {
	revertCmd.Flags().BoolVar(&revertDryRun, "dry-run", false, "show what would be reverted without reverting")
	revertCmd.Flags().BoolVarP(&revertPush, "push", "p", false, "push reverted state as new commit")
	revertCmd.Flags().StringVarP(&revertMessage, "message", "m", "", "commit message (used with --push)")
	revertCmd.Flags().BoolVarP(&revertYes, "yes", "y", false, "skip confirmation prompt")
}

func runRevert(cmd *cobra.Command, args []string) error {
	ctx, cancel := signalContext()
	defer cancel()
	out := GetOutput()

	if revertDryRun {
		out.PrintDryRunHeader()
	}

	// Create project context
	pc, err := NewProjectContext(ctx, cfg)
	if err != nil {
		return err
	}
	defer pc.Close()

	// Determine the ref to revert to
	var ref string
	if len(args) == 0 {
		// Interactive mode: show recent commits and let user pick
		if !ui.CanPrompt() {
			return fmt.Errorf("ref argument required in non-interactive mode")
		}

		// Ensure cache is synced so we have latest commits
		if err := pc.Cache.SyncFromStorage(ctx); err != nil {
			return fmt.Errorf("failed to sync cache: %w", err)
		}

		// Get recent commits
		commits, err := pc.Cache.Log(5)
		if err != nil {
			return fmt.Errorf("failed to get history: %w", err)
		}
		if len(commits) == 0 {
			return fmt.Errorf("no commits to revert to")
		}

		// Format options for selection
		options := make([]string, len(commits))
		for i, c := range commits {
			options[i] = fmt.Sprintf("%s  %s  %s", ui.TruncateHash(c.Hash), c.Date.Format("2006-01-02"), c.Message)
		}

		prompt := ui.NewPrompt()
		idx, err := prompt.Select("Revert to:", options)
		if err != nil {
			return err
		}
		ref = commits[idx].Hash
	} else {
		ref = args[0]
	}

	// Confirm the revert (skip in dry-run mode or with --yes)
	if !revertDryRun && !revertYes {
		if ui.CanPrompt() {
			prompt := ui.NewPrompt()
			confirmed, err := prompt.Confirm(fmt.Sprintf("Restore files from %s?", ui.TruncateHash(ref)), false)
			if err != nil {
				return err
			}
			if !confirmed {
				out.Println("Aborted.")
				return nil
			}
		} else {
			return fmt.Errorf("revert requires confirmation; use --yes in non-interactive mode")
		}
	}

	// Create syncer and pull at specific ref
	syncer := sync.NewSyncer(pc.Discovery, pc.RepoInfo, pc.Storage, pc.Encrypter, pc.Cache)

	opts := sync.PullOptions{
		Ref:    ref,
		Force:  true, // Force overwrite for revert
		DryRun: revertDryRun,
	}

	result, err := syncer.Pull(ctx, opts)
	if err != nil {
		return err
	}

	// Display revert results
	refDisplay := ui.TruncateHash(ref)

	// Handle --push flag
	var pushResult *domain.PushResult
	if revertPush && !revertDryRun {
		commitMessage := revertMessage
		if commitMessage == "" {
			commitMessage = fmt.Sprintf("Revert to %s", refDisplay)
		}

		pushOpts := sync.PushOptions{
			Message: commitMessage,
			Force:   true,
		}

		pushResult, err = syncer.Push(ctx, pushOpts)
		if err != nil {
			return fmt.Errorf("push failed: %w", err)
		}
	}

	// Output JSON if requested (before text output to avoid mixed output)
	if out.IsJSON() {
		jsonResult := map[string]interface{}{
			"ref":           ref,
			"files_updated": result.FilesUpdated,
			"files_created": result.FilesCreated,
			"dry_run":       revertDryRun,
		}
		if revertPush && revertDryRun {
			jsonResult["would_push"] = true
		} else if revertPush && pushResult != nil {
			jsonResult["pushed"] = true
			jsonResult["commit_hash"] = pushResult.CommitHash
		}
		return out.JSON(jsonResult)
	}

	// Text output
	if revertDryRun {
		out.Printf("Would revert to %s\n", refDisplay)
		if result.FilesCreated > 0 {
			out.Printf("  %d file(s) would be created\n", result.FilesCreated)
		}
		if result.FilesUpdated > 0 {
			out.Printf("  %d file(s) would be updated\n", result.FilesUpdated)
		}
		if revertPush {
			out.Println()
			out.Println("Would push reverted state as new commit.")
		}
	} else {
		out.Printf("Reverted to %s\n", refDisplay)
		if result.FilesCreated > 0 {
			out.Printf("  %d file(s) created\n", result.FilesCreated)
		}
		if result.FilesUpdated > 0 {
			out.Printf("  %d file(s) updated\n", result.FilesUpdated)
		}

		if pushResult != nil {
			out.Println()
			out.Printf("Pushed: commit %s\n", ui.TruncateHash(pushResult.CommitHash))
		} else if !revertPush {
			out.Println()
			out.Println("Review the changes, then run 'envsecrets push' to save.")
		}
	}

	return nil
}
