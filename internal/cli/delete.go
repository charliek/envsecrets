package cli

import (
	"fmt"

	"github.com/charliek/envsecrets/internal/cache"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/project"
	"github.com/charliek/envsecrets/internal/storage"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/spf13/cobra"
)

var (
	yesDeletePermanently bool
	deleteDryRun         bool
)

var deleteCmd = &cobra.Command{
	Use:   "delete <repo>",
	Short: "Delete an entire repository from GCS",
	Long: `Delete an entire repository from GCS.

This permanently deletes all encrypted files and history for the specified
repository from the GCS bucket. This action cannot be undone.

The repo argument should be in the format "owner/repo".

In non-interactive mode (scripts, CI/CD), use --yes-delete-permanently to confirm.`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete,
}

func init() {
	deleteCmd.Flags().BoolVar(&yesDeletePermanently, "yes-delete-permanently", false, "confirm permanent deletion in non-interactive mode")
	deleteCmd.Flags().BoolVar(&deleteDryRun, "dry-run", false, "show what would be deleted without deleting")
}

func runDelete(cmd *cobra.Command, args []string) error {
	ctx, cancel := signalContext()
	defer cancel()
	out := GetOutput()
	repoPath := args[0]

	if deleteDryRun {
		out.PrintDryRunHeader()
	}

	// Parse repo path
	repoInfo, err := project.ParseRepoString(repoPath)
	if err != nil {
		return err
	}

	// Create storage client
	store, err := storage.NewGCSStorage(ctx, cfg.Bucket, cfg.GCSCredentials)
	if err != nil {
		return err
	}
	defer store.Close()

	// Check if repo exists
	prefix := repoInfo.CachePath() + "/"
	objects, err := store.List(ctx, prefix)
	if err != nil {
		return err
	}

	if len(objects) == 0 {
		return domain.Errorf(domain.ErrRepoNotFound, "repository not found: %s", repoPath)
	}

	if deleteDryRun {
		out.Printf("Would delete %s (%d files)\n", repoPath, len(objects))
		return nil
	}

	// Confirm deletion
	if ui.CanPrompt() {
		prompt := ui.NewPrompt()
		confirmed, err := prompt.ConfirmDanger(
			fmt.Sprintf("This will permanently delete %s and all its history (%d files).",
				repoPath, len(objects)))
		if err != nil {
			return err
		}
		if !confirmed {
			out.Println("Aborted.")
			return nil
		}
	} else {
		// In non-interactive mode, require explicit flag
		if !yesDeletePermanently {
			return fmt.Errorf("delete requires confirmation; use --yes-delete-permanently in non-interactive mode")
		}
	}

	// Create cache and delete remote
	cacheRepo, err := cache.NewCache(repoInfo, store)
	if err != nil {
		return err
	}

	if err := cacheRepo.DeleteRemote(ctx); err != nil {
		return fmt.Errorf("failed to delete repository: %w", err)
	}

	out.Printf("Deleted %s (%d files)\n", repoPath, len(objects))

	return nil
}
