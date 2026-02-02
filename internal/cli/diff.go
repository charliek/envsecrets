package cli

import (
	"errors"
	"strings"

	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/sync"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff [ref1] [ref2]",
	Short: "Show changes between versions",
	Long: `Show changes between versions or between local and remote.

If no refs are provided, shows diff between local files and latest remote.
If one ref is provided, shows diff between that ref and current local.
If two refs are provided, shows diff between those refs.`,
	Args: cobra.MaximumNArgs(2),
	RunE: runDiff,
}

func runDiff(cmd *cobra.Command, args []string) error {
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

	// Ensure cache is synced
	if err := pc.Cache.SyncFromStorage(ctx); err != nil {
		out.Verbose("Note: Could not sync from remote: %v", err)
	}

	// Get files to compare
	files, err := pc.EnvFiles()
	if err != nil {
		return err
	}

	var ref1, ref2 string
	switch len(args) {
	case 0:
		// Compare local to HEAD
		ref2 = "HEAD"
	case 1:
		// Compare local to specified ref
		ref2 = args[0]
	case 2:
		// Compare two refs
		ref1 = args[0]
		ref2 = args[1]
	}

	hasChanges := false

	for _, file := range files {
		var content1, content2 []byte

		if ref1 == "" {
			// Get local content
			content1, err = pc.ReadProjectFile(file)
			if err != nil {
				if !errors.Is(err, domain.ErrFileNotFound) {
					out.Verbose("Warning: could not read local file %s: %v", file, err)
				}
				content1 = nil
			}
		} else {
			// Get content from ref1
			content1, err = syncer.PullFile(ctx, file, ref1)
			if err != nil {
				if !errors.Is(err, domain.ErrFileNotFound) {
					out.Verbose("Warning: could not read %s at %s: %v", file, ref1, err)
				}
				content1 = nil
			}
		}

		// Get content from ref2
		content2, err = syncer.PullFile(ctx, file, ref2)
		if err != nil {
			if !errors.Is(err, domain.ErrFileNotFound) {
				out.Verbose("Warning: could not read %s at %s: %v", file, ref2, err)
			}
			content2 = nil
		}

		// Compare
		if string(content1) == string(content2) {
			continue
		}

		hasChanges = true

		// Print diff header
		out.Println("---", file)
		if ref1 == "" {
			out.Println("+++ (local)")
		} else {
			out.Printf("+++ %s (%s)\n", file, ref1)
		}

		// Simple line-by-line diff
		printSimpleDiff(out, string(content2), string(content1))
		out.Println()
	}

	if !hasChanges {
		out.Println("No changes")
	}

	return nil
}

func printSimpleDiff(out *Output, old, new string) {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")

	// Very simple diff - just show added/removed lines
	oldSet := make(map[string]bool)
	for _, line := range oldLines {
		oldSet[line] = true
	}

	newSet := make(map[string]bool)
	for _, line := range newLines {
		newSet[line] = true
	}

	// Show removed lines
	for _, line := range oldLines {
		if !newSet[line] && line != "" {
			out.Printf("- %s\n", line)
		}
	}

	// Show added lines
	for _, line := range newLines {
		if !oldSet[line] && line != "" {
			out.Printf("+ %s\n", line)
		}
	}
}

// Output is a type alias for the UI output
type Output = ui.Output
