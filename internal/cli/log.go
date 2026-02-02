package cli

import (
	"context"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/spf13/cobra"
)

var (
	logCount   int
	logVerbose bool
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show commit history",
	Long: `Show the commit history for the current repository.

Displays commits with their hash, message, author, and date.`,
	RunE: runLog,
}

func init() {
	logCmd.Flags().IntVarP(&logCount, "number", "n", constants.DefaultLogCount, "number of commits to show")
	logCmd.Flags().BoolVarP(&logVerbose, "verbose", "", false, "show file changes in each commit")
}

func runLog(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	out := GetOutput()

	// Create project context
	pc, err := NewProjectContext(ctx, cfg)
	if err != nil {
		return err
	}

	// Ensure cache is synced
	if err := pc.Cache.SyncFromStorage(ctx); err != nil {
		return err
	}

	// Get log
	commits, err := pc.Cache.Log(logCount)
	if err != nil {
		return err
	}

	if len(commits) == 0 {
		out.Println("No commits yet")
		return nil
	}

	// Output
	if out.IsJSON() {
		return out.JSON(commits)
	}

	for _, commit := range commits {
		out.PrintCommit(commit, logVerbose)
	}

	return nil
}
