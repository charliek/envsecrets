package cli

import (
	"errors"

	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/sync"
	"github.com/spf13/cobra"
)

var (
	pushMessage string
	pushDryRun  bool
	pushForce   bool
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

	// Create syncer
	syncer := sync.NewSyncer(pc.Discovery, pc.RepoInfo, pc.Storage, pc.Encrypter, pc.Cache)

	opts := sync.PushOptions{
		Message: pushMessage,
		DryRun:  pushDryRun,
		Force:   pushForce,
	}

	if pushDryRun {
		out.Println("Dry run - no changes will be made")
		out.Println()
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
		commitDisplay := result.CommitHash
		if len(commitDisplay) > 7 {
			commitDisplay = commitDisplay[:7]
		}
		out.Printf("Commit: %s\n", commitDisplay)
	}

	return nil
}
