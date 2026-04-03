package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/charliek/envsecrets/internal/cache"
	"github.com/charliek/envsecrets/internal/config"
	"github.com/charliek/envsecrets/internal/crypto"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/project"
	"github.com/charliek/envsecrets/internal/storage"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Test decryption across all repositories",
	Long: `Test that the current passphrase can decrypt files in all repositories.

This command checks that all encrypted files in all repositories can be
decrypted with the current passphrase. This is useful for verifying that
your passphrase is correct before making changes.`,
	RunE: runVerify,
}

func runVerify(cmd *cobra.Command, args []string) error {
	ctx, cancel := signalContext()
	defer cancel()
	out := GetOutput()

	// Get passphrase
	resolver := config.NewPassphraseResolver(cfg)
	passphrase, err := resolver.Resolve()
	if err != nil {
		return err
	}

	// Create encrypter
	enc, err := crypto.NewAgeEncrypter(passphrase)
	if err != nil {
		return err
	}

	// Create storage client
	store, err := storage.NewGCSStorage(ctx, cfg.Bucket, cfg.GCSCredentials)
	if err != nil {
		return err
	}
	defer store.Close()

	// List all repos
	objects, err := store.List(ctx, "")
	if err != nil {
		return err
	}

	// Extract unique repos
	repos := extractReposFromObjects(objects)

	if len(repos) == 0 {
		out.Println("No repositories found")
		return nil
	}

	out.Printf("Verifying %d repositories...\n\n", len(repos))

	allOK := true
	results := make(map[string]verifyResult)

	for repoPath := range repos {
		repoInfo, err := project.ParseRepoString(repoPath)
		if err != nil {
			results[repoPath] = verifyResult{Error: "invalid repo path"}
			continue
		}

		result := verifyRepo(ctx, store, repoInfo, enc)
		results[repoPath] = result

		if result.Error != "" {
			allOK = false
		}
	}

	// Output results
	if out.IsJSON() {
		return out.JSON(results)
	}

	for repo, result := range results {
		if result.Error != "" {
			out.Printf("FAIL  %s\n", repo)
			out.Printf("      %s\n", result.Error)
		} else {
			out.Printf("OK    %s (v%d, %d files)\n", repo, result.FormatVersion, result.FilesVerified)
		}
	}

	out.Println()
	if allOK {
		out.Success("All repositories verified successfully!")
	} else {
		return fmt.Errorf("some repositories failed verification")
	}

	return nil
}

type verifyResult struct {
	FormatVersion int    `json:"format_version,omitempty"`
	FilesVerified int    `json:"files_verified,omitempty"`
	Error         string `json:"error,omitempty"`
}

func verifyRepo(ctx context.Context, store storage.Storage, repoInfo *domain.RepoInfo, enc crypto.Encrypter) verifyResult {
	// Create cache
	cacheRepo, err := cache.NewCache(repoInfo, store)
	if err != nil {
		return verifyResult{Error: err.Error()}
	}

	// Detect format version
	formatInfo, err := cacheRepo.DetectRemoteVersion(ctx)
	if err != nil {
		return verifyResult{Error: fmt.Sprintf("format detection failed: %v", err)}
	}

	result := verifyResult{FormatVersion: formatInfo.Version}

	// Check version compatibility — report as verification failure, not crash
	if err := cache.CheckVersionCompatibility(formatInfo); err != nil {
		if errors.Is(err, domain.ErrVersionUnknown) {
			result.Error = "no FORMAT marker — storage format unknown"
		} else {
			result.Error = fmt.Sprintf("incompatible format: %v", err)
		}
		return result
	}

	// Sync from storage
	if err := cacheRepo.SyncFromStorage(ctx); err != nil {
		result.Error = fmt.Sprintf("sync failed: %v", err)
		return result
	}

	// Get all encrypted files
	files, err := cacheRepo.ListTrackedFiles()
	if err != nil {
		result.Error = fmt.Sprintf("list failed: %v", err)
		return result
	}

	// Verify each file
	for _, file := range files {
		encrypted, err := cacheRepo.ReadEncrypted(file)
		if err != nil {
			result.Error = fmt.Sprintf("read %s failed: %v", file, err)
			return result
		}

		_, err = enc.Decrypt(encrypted)
		if err != nil {
			result.Error = fmt.Sprintf("decrypt %s failed: %v", file, err)
			return result
		}
	}

	result.FilesVerified = len(files)
	return result
}
