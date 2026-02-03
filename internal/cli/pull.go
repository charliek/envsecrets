package cli

import (
	"fmt"

	"github.com/charliek/envsecrets/internal/sync"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/spf13/cobra"
)

var (
	pullRef           string
	pullForce         bool
	pullDryRun        bool
	pullSkipConflicts bool
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Download and decrypt environment files",
	Long: `Download and decrypt environment files from GCS.

Files are downloaded from the configured GCS bucket, decrypted, and written
to the project directory.`,
	RunE: runPull,
}

func init() {
	pullCmd.Flags().StringVar(&pullRef, "ref", "", "pull specific version (commit hash)")
	pullCmd.Flags().BoolVar(&pullForce, "force", false, "overwrite local files without confirmation")
	pullCmd.Flags().BoolVar(&pullDryRun, "dry-run", false, "show what would be pulled without pulling")
	pullCmd.Flags().BoolVar(&pullSkipConflicts, "skip-conflicts", false, "skip conflicting files instead of aborting")
}

func runPull(cmd *cobra.Command, args []string) error {
	ctx, cancel := signalContext()
	defer cancel()
	out := GetOutput()

	// Validate flag combinations
	if pullForce && pullSkipConflicts {
		return fmt.Errorf("--force and --skip-conflicts cannot be used together")
	}

	// Create project context
	pc, err := NewProjectContext(ctx, cfg)
	if err != nil {
		return err
	}
	defer pc.Close()

	// Create syncer
	syncer := sync.NewSyncer(pc.Discovery, pc.RepoInfo, pc.Storage, pc.Encrypter, pc.Cache)

	opts := sync.PullOptions{
		Ref:    pullRef,
		Force:  pullForce,
		DryRun: pullDryRun,
	}

	// Set up conflict resolver
	if pullSkipConflicts {
		// Skip all conflicts automatically
		opts.ConflictResolver = func(f string) (sync.ConflictAction, error) {
			return sync.ConflictSkip, nil
		}
	} else if !pullForce && ui.CanPrompt() {
		// Interactive conflict resolution
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

	if pullDryRun {
		out.PrintDryRunHeader()
	}

	result, err := syncer.Pull(ctx, opts)
	if err != nil {
		return err
	}

	// Output results
	if out.IsJSON() {
		return out.JSON(result)
	}

	if pullDryRun {
		out.Println("Would pull:")
	} else {
		out.Println("Pulled:")
	}
	if result.FilesCreated > 0 {
		out.Printf("  %d file(s) created\n", result.FilesCreated)
	}
	if result.FilesUpdated > 0 {
		out.Printf("  %d file(s) updated\n", result.FilesUpdated)
	}
	if result.FilesSkipped > 0 {
		out.Printf("  %d file(s) unchanged\n", result.FilesSkipped)
	}
	if result.FilesSkippedConflict > 0 {
		out.Printf("  %d file(s) skipped (conflicts)\n", result.FilesSkippedConflict)
	}

	if result.Ref != "" {
		out.Println()
		out.Printf("At ref: %s\n", ui.TruncateHash(result.Ref))
	}

	return nil
}
