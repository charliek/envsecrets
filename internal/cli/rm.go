package cli

import (
	"errors"
	"fmt"

	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/project"
	"github.com/charliek/envsecrets/internal/sync"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/spf13/cobra"
)

var (
	rmDryRun bool
	rmYes    bool
)

var rmCmd = &cobra.Command{
	Use:   "rm <file>",
	Short: "Remove a file from tracking",
	Long: `Remove a file from tracking.

This removes the file from .envsecrets and from the encrypted cache.
The local file is not deleted.`,
	Args: cobra.ExactArgs(1),
	RunE: runRm,
}

func init() {
	rmCmd.Flags().BoolVar(&rmDryRun, "dry-run", false, "show what would be removed without removing")
	rmCmd.Flags().BoolVarP(&rmYes, "yes", "y", false, "skip confirmation prompt")
}

func runRm(cmd *cobra.Command, args []string) error {
	ctx, cancel := signalContext()
	defer cancel()
	out := GetOutput()
	filename := args[0]

	if rmDryRun {
		out.PrintDryRunHeader()
	}

	// Create project context
	pc, err := NewProjectContext(ctx, cfg)
	if err != nil {
		return err
	}
	defer pc.Close()

	// Check if file is tracked
	tracked, err := project.IsTracked(pc.Discovery.EnvSecretsFile(), filename)
	if err != nil {
		return err
	}

	if !tracked {
		out.Printf("File not tracked: %s\n", filename)
		return nil
	}

	if rmDryRun {
		out.Printf("Would remove %s from tracking\n", filename)
		out.Println("Local file would not be deleted.")
		return nil
	}

	// Confirm removal
	if !rmYes {
		if ui.CanPrompt() {
			prompt := ui.NewPrompt()
			confirmed, err := prompt.Confirm("Remove "+filename+" from tracking?", false)
			if err != nil {
				return err
			}
			if !confirmed {
				out.Println("Aborted.")
				return nil
			}
		} else {
			return fmt.Errorf("rm requires confirmation; use --yes in non-interactive mode")
		}
	}

	// Remove from .envsecrets
	if err := project.RemoveFromTracked(pc.Discovery.EnvSecretsFile(), filename); err != nil {
		return err
	}

	// Remove from cache if it exists
	if err := pc.Cache.RemoveEncrypted(filename); err != nil {
		// Log cache removal errors at verbose level (file may not exist in cache yet)
		out.Verbose("Note: could not remove from cache: %v", err)
	}

	// Push changes
	syncer := sync.NewSyncer(pc.Discovery, pc.RepoInfo, pc.Storage, pc.Encrypter, pc.Cache)
	_, err = syncer.Push(ctx, sync.PushOptions{
		Message: "Remove " + filename,
	})
	if err != nil {
		// Only ignore "nothing to commit" - warn about other errors
		if errors.Is(err, domain.ErrNothingToCommit) {
			out.Verbose("Note: %v", err)
		} else {
			out.Warn("Warning: failed to push removal to remote: %v", err)
		}
	}

	out.Printf("Removed %s from tracking\n", filename)
	out.Println("Local file was not deleted.")

	return nil
}
