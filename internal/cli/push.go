package cli

import (
	"errors"
	"fmt"

	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/sync"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/spf13/cobra"
)

var (
	pushMessage      string
	pushDryRun       bool
	pushForce        bool
	pushAllowMissing bool
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Encrypt and upload environment files",
	Long: `Encrypt and upload environment files to GCS.

Files listed in .envsecrets are encrypted with age and uploaded to the
configured GCS bucket.`,
	RunE: runPush,
}

func init() {
	pushCmd.Flags().StringVarP(&pushMessage, "message", "m", "", "commit message")
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "show what would be pushed without pushing")
	pushCmd.Flags().BoolVar(&pushForce, "force", false, "force push even with conflicts")
	pushCmd.Flags().BoolVar(&pushAllowMissing, "allow-missing", false, "allow push with missing tracked files (for non-interactive mode)")
}

func runPush(cmd *cobra.Command, args []string) error {
	ctx, cancel := signalContext()
	defer cancel()
	out := GetOutput()

	// Create project context
	pc, err := NewProjectContext(ctx, cfg)
	if err != nil {
		return err
	}
	defer pc.Close()

	// Check for missing tracked files
	files, err := pc.Discovery.EnvFiles()
	if err != nil {
		return err
	}

	var missing []string
	for _, f := range files {
		if !pc.Discovery.FileExists(f) {
			missing = append(missing, f)
		}
	}

	if len(missing) > 0 {
		out.Warn("Missing tracked files:")
		for _, f := range missing {
			out.Printf("  %s\n", f)
		}
		out.Println()

		existing := len(files) - len(missing)
		if existing == 0 {
			return fmt.Errorf("no files to push")
		}

		// In dry-run mode, just warn and continue
		if !pushDryRun && !pushForce && !pushAllowMissing {
			if ui.CanPrompt() {
				prompt := ui.NewPrompt()
				ok, err := prompt.Confirm(fmt.Sprintf("Push %d of %d files anyway?", existing, len(files)), false)
				if err != nil {
					return err
				}
				if !ok {
					out.Println("Aborted.")
					return nil
				}
			} else {
				return fmt.Errorf("push requires confirmation; use --allow-missing in non-interactive mode")
			}
		}
	}

	// Create syncer
	syncer := sync.NewSyncer(pc.Discovery, pc.RepoInfo, pc.Storage, pc.Encrypter, pc.Cache)

	opts := sync.PushOptions{
		Message: pushMessage,
		DryRun:  pushDryRun,
		Force:   pushForce,
	}

	if pushDryRun {
		out.PrintDryRunHeader()
	}

	result, err := syncer.Push(ctx, opts)
	if err != nil {
		if errors.Is(err, domain.ErrNothingToCommit) {
			out.Println("Nothing to push - all files are up to date")
			return nil
		}
		return err
	}

	// Output results
	if out.IsJSON() {
		return out.JSON(result)
	}

	if pushDryRun {
		out.Println("Would push:")
	} else {
		out.Println("Pushed:")
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

	if !pushDryRun && result.CommitHash != "" {
		out.Println()
		out.Printf("Commit: %s\n", ui.TruncateHash(result.CommitHash))
	}

	return nil
}
