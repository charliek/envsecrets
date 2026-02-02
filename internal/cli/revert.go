package cli

import (
	"context"
	"fmt"

	"github.com/charliek/envsecrets/internal/sync"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/spf13/cobra"
)

var revertCmd = &cobra.Command{
	Use:   "revert <ref>",
	Short: "Restore files from a previous version",
	Long: `Restore environment files from a previous version.

This pulls files from the specified ref and writes them to your project directory.
It does not automatically push the changes - you can review them first.`,
	Args: cobra.ExactArgs(1),
	RunE: runRevert,
}

func runRevert(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	out := GetOutput()
	ref := args[0]

	// Create project context
	pc, err := NewProjectContext(ctx, cfg)
	if err != nil {
		return err
	}

	// Confirm the revert
	if ui.IsInteractive() {
		prompt := ui.NewPrompt()
		confirmed, err := prompt.Confirm(fmt.Sprintf("Restore files from %s?", ref), false)
		if err != nil {
			return err
		}
		if !confirmed {
			out.Println("Aborted.")
			return nil
		}
	}

	// Create syncer and pull at specific ref
	syncer := sync.NewSyncer(pc.Discovery, pc.RepoInfo, pc.Storage, pc.Encrypter, pc.Cache)

	opts := sync.PullOptions{
		Ref:   ref,
		Force: true, // Force overwrite for revert
	}

	result, err := syncer.Pull(ctx, opts)
	if err != nil {
		return err
	}

	// Output results
	if out.IsJSON() {
		return out.JSON(map[string]interface{}{
			"ref":           ref,
			"files_updated": result.FilesUpdated,
			"files_created": result.FilesCreated,
		})
	}

	out.Printf("Reverted to %s\n", ref)
	if result.FilesCreated > 0 {
		out.Printf("  %d file(s) created\n", result.FilesCreated)
	}
	if result.FilesUpdated > 0 {
		out.Printf("  %d file(s) updated\n", result.FilesUpdated)
	}

	out.Println()
	out.Println("Review the changes, then run 'envsecrets push' to save.")

	return nil
}
