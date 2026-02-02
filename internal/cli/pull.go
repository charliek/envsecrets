package cli

import (
	"github.com/charliek/envsecrets/internal/sync"
	"github.com/spf13/cobra"
)

var (
	pullRef   string
	pullForce bool
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
}

func runPull(cmd *cobra.Command, args []string) error {
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

	opts := sync.PullOptions{
		Ref:   pullRef,
		Force: pullForce,
	}

	result, err := syncer.Pull(ctx, opts)
	if err != nil {
		return err
	}

	// Output results
	if out.IsJSON() {
		return out.JSON(result)
	}

	out.Println("Pulled:")
	if result.FilesCreated > 0 {
		out.Printf("  %d file(s) created\n", result.FilesCreated)
	}
	if result.FilesUpdated > 0 {
		out.Printf("  %d file(s) updated\n", result.FilesUpdated)
	}
	if result.FilesSkipped > 0 {
		out.Printf("  %d file(s) unchanged\n", result.FilesSkipped)
	}

	if result.Ref != "" {
		out.Println()
		refDisplay := result.Ref
		if len(refDisplay) > 7 {
			refDisplay = refDisplay[:7]
		}
		out.Printf("At ref: %s\n", refDisplay)
	}

	return nil
}
