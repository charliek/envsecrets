package cli

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/charliek/envsecrets/internal/cache"
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
	return listRepoFilesWithStorage(ctx, store, out, args[0])
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

func listRepoFilesWithStorage(ctx context.Context, store storage.Storage, out *ui.Output, repo string) error {
	repoInfo, err := project.ParseRepoString(repo)
	if err != nil {
		return err
	}
	c, err := cache.NewCache(repoInfo, store)
	if err != nil {
		return err
	}
	return listCacheFiles(ctx, c, out, repo)
}

// listCacheFiles lists the files in a repository's current remote HEAD.
// Tracked files live inside the encrypted packfile rather than as individual
// bucket objects, so the cache is synced from storage and read from there.
func listCacheFiles(ctx context.Context, c *cache.Cache, out *ui.Output, repo string) error {
	exists, err := c.ExistsRemote(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return printNoFiles(out, repo)
	}

	if err := c.SyncFromStorage(ctx); err != nil {
		return err
	}

	files, err := c.ListTrackedFiles()
	if err != nil {
		return err
	}
	sort.Strings(files)

	if len(files) == 0 {
		return printNoFiles(out, repo)
	}

	// Find when each file last changed by walking history newest-first
	commits, err := c.Log(math.MaxInt, true)
	if err != nil {
		return err
	}
	lastChanged := make(map[string]time.Time)
	for _, commit := range commits {
		for _, f := range commit.Files {
			if _, ok := lastChanged[f]; !ok {
				lastChanged[f] = commit.Date
			}
		}
	}

	type fileInfo struct {
		Name    string `json:"name"`
		Size    int64  `json:"size"`
		Updated string `json:"updated"`
	}
	infos := make([]fileInfo, 0, len(files))
	for _, f := range files {
		stat, err := os.Stat(filepath.Join(c.Path(), f+constants.AgeExtension))
		if err != nil {
			return err
		}
		updated := ""
		if t, ok := lastChanged[f]; ok {
			updated = t.Local().Format("2006-01-02 15:04:05")
		}
		infos = append(infos, fileInfo{Name: f, Size: stat.Size(), Updated: updated})
	}

	if out.IsJSON() {
		return out.JSON(infos)
	}

	nameWidth := 25
	for _, info := range infos {
		nameWidth = max(nameWidth, len(info.Name))
	}

	out.Printf("Files in %s:\n\n", repo)
	for _, info := range infos {
		out.Printf("  %-*s %10s   %s\n", nameWidth, info.Name, formatBytes(info.Size), info.Updated)
	}
	out.Println()
	out.Printf("%d file(s)\n", len(infos))

	return nil
}

// printNoFiles reports an empty listing, as an empty array in JSON mode
func printNoFiles(out *ui.Output, repo string) error {
	if out.IsJSON() {
		return out.JSON([]struct{}{})
	}
	out.Printf("No files found in %s\n", repo)
	return nil
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
