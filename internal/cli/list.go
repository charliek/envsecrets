package cli

import (
	"context"
	"path"
	"strings"

	"github.com/charliek/envsecrets/internal/storage"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list [repo]",
	Short: "List repositories or files in the bucket",
	Long: `List repositories or files stored in the bucket.

Without arguments, lists all repositories (owner/repo).
With a repo argument, lists files in that repository.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	out := GetOutput()

	// Create storage client
	store, err := storage.NewGCSStorage(ctx, cfg.Bucket, cfg.GCSCredentials)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		// List all repos
		return listRepos(ctx, store, out)
	}

	// List files in specific repo
	return listRepoFiles(ctx, store, out, args[0])
}

func listRepos(ctx context.Context, store *storage.GCSStorage, out *ui.Output) error {
	// List all objects in bucket
	objects, err := store.List(ctx, "")
	if err != nil {
		return err
	}

	// Extract unique owner/repo combinations
	repos := make(map[string]bool)
	for _, obj := range objects {
		parts := strings.Split(obj, "/")
		if len(parts) >= 2 {
			repo := parts[0] + "/" + parts[1]
			repos[repo] = true
		}
	}

	if len(repos) == 0 {
		out.Println("No repositories found")
		return nil
	}

	if out.IsJSON() {
		var repoList []string
		for repo := range repos {
			repoList = append(repoList, repo)
		}
		return out.JSON(repoList)
	}

	out.Println("Repositories:")
	for repo := range repos {
		out.Printf("  %s\n", repo)
	}

	return nil
}

func listRepoFiles(ctx context.Context, store *storage.GCSStorage, out *ui.Output, repo string) error {
	prefix := repo + "/"

	objects, err := store.List(ctx, prefix)
	if err != nil {
		return err
	}

	if len(objects) == 0 {
		out.Printf("No files found in %s\n", repo)
		return nil
	}

	if out.IsJSON() {
		var files []string
		for _, obj := range objects {
			// Skip HEAD file
			if strings.HasSuffix(obj, "/HEAD") {
				continue
			}
			files = append(files, path.Base(obj))
		}
		return out.JSON(files)
	}

	out.Printf("Files in %s:\n", repo)
	for _, obj := range objects {
		// Skip HEAD file
		if strings.HasSuffix(obj, "/HEAD") {
			continue
		}
		filename := strings.TrimPrefix(obj, prefix)
		out.Printf("  %s\n", filename)
	}

	return nil
}
