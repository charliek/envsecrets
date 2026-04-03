package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/project"
	"github.com/charliek/envsecrets/internal/storage"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/spf13/cobra"
)

var listCurrent bool

var listCmd = &cobra.Command{
	Use:   "list [repo]",
	Short: "List repositories or files in the bucket",
	Long: `List repositories or files stored in the bucket.

Without arguments, lists all repositories (owner/repo).
With a repo argument, lists files in that repository.
With --current flag, lists files in the auto-detected current repository.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runList,
}

func init() {
	listCmd.Flags().BoolVar(&listCurrent, "current", false, "list files in current repository")
}

func runList(cmd *cobra.Command, args []string) error {
	ctx, cancel := signalContext()
	defer cancel()
	out := GetOutput()

	// Handle --current flag - only needs discovery + storage, no passphrase required
	if listCurrent {
		discovery, err := project.NewDiscovery("")
		if err != nil {
			return err
		}
		repoInfo, err := discovery.RepoInfo()
		if err != nil {
			return err
		}
		store, err := storage.NewGCSStorage(ctx, cfg.Bucket, cfg.GCSCredentials)
		if err != nil {
			return err
		}
		defer store.Close()
		return listRepoFilesWithStorage(ctx, store, out, repoInfo.String())
	}

	// Create storage client for non-current operations
	store, err := storage.NewGCSStorage(ctx, cfg.Bucket, cfg.GCSCredentials)
	if err != nil {
		return err
	}
	defer store.Close()

	if len(args) == 0 {
		// List all repos
		return listRepos(ctx, store, out)
	}

	// List files in specific repo
	return listRepoFiles(ctx, store, out, args[0])
}

func listRepos(ctx context.Context, store storage.Storage, out *ui.Output) error {
	// List all objects in bucket
	objects, err := store.List(ctx, "")
	if err != nil {
		return err
	}

	// Extract unique owner/repo combinations
	repos := extractReposFromObjects(objects)

	if len(repos) == 0 {
		out.Println("No repositories found")
		return nil
	}

	// Sort repos for deterministic output
	repoList := make([]string, 0, len(repos))
	for repo := range repos {
		repoList = append(repoList, repo)
	}
	sort.Strings(repoList)

	if out.IsJSON() {
		return out.JSON(repoList)
	}

	out.Println("Repositories:")
	for _, repo := range repoList {
		out.Printf("  %s\n", repo)
	}

	return nil
}

// listRepoFilesWithStorage lists files using the Storage interface
func listRepoFilesWithStorage(ctx context.Context, store storage.Storage, out *ui.Output, repo string) error {
	return listRepoFilesImpl(ctx, store, out, repo)
}

func listRepoFiles(ctx context.Context, store *storage.GCSStorage, out *ui.Output, repo string) error {
	return listRepoFilesImpl(ctx, store, out, repo)
}

func listRepoFilesImpl(ctx context.Context, store storage.Storage, out *ui.Output, repo string) error {
	prefix := repo + "/"

	objects, err := store.ListWithMetadata(ctx, prefix)
	if err != nil {
		return err
	}

	if len(objects) == 0 {
		out.Printf("No files found in %s\n", repo)
		return nil
	}

	if out.IsJSON() {
		type fileInfo struct {
			Name    string `json:"name"`
			Size    int64  `json:"size"`
			Updated string `json:"updated"`
		}
		var files []fileInfo
		for _, obj := range objects {
			if isInternalStorageFile(obj.Name) {
				continue
			}
			files = append(files, fileInfo{
				Name:    strings.TrimPrefix(obj.Name, prefix),
				Size:    obj.Size,
				Updated: obj.Updated.Format("2006-01-02 15:04:05"),
			})
		}
		return out.JSON(files)
	}

	out.Printf("Files in %s:\n\n", repo)

	// Count user files (exclude internal storage files)
	fileCount := 0
	for _, obj := range objects {
		if !isInternalStorageFile(obj.Name) {
			fileCount++
		}
	}

	for _, obj := range objects {
		if isInternalStorageFile(obj.Name) {
			continue
		}
		filename := strings.TrimPrefix(obj.Name, prefix)
		out.Printf("  %-25s %10s   %s\n",
			filename,
			formatBytes(obj.Size),
			obj.Updated.Format("2006-01-02 15:04:05"))
	}

	out.Println()
	out.Printf("%d file(s)\n", fileCount)

	return nil
}

// isInternalStorageFile returns true if the object path is an internal
// storage file (FORMAT, HEAD, objects.pack, refs) that should be hidden
// from user-facing list output.
func isInternalStorageFile(name string) bool {
	return strings.HasSuffix(name, "/HEAD") ||
		strings.HasSuffix(name, "/FORMAT") ||
		strings.HasSuffix(name, "/objects.pack") ||
		strings.HasSuffix(name, "/refs")
}

// formatBytes formats bytes in human-readable format
func formatBytes(bytes int64) string {
	if bytes < constants.BytesPerKB {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(constants.BytesPerKB), 0
	for n := bytes / constants.BytesPerKB; n >= constants.BytesPerKB; n /= constants.BytesPerKB {
		div *= constants.BytesPerKB
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
