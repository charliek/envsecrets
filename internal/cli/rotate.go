package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charliek/envsecrets/internal/cache"
	"github.com/charliek/envsecrets/internal/config"
	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/crypto"
	"github.com/charliek/envsecrets/internal/domain"
	limitedio "github.com/charliek/envsecrets/internal/io"
	"github.com/charliek/envsecrets/internal/storage"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/spf13/cobra"
)

// readAllLimited reads data with a size limit
func readAllLimited(r io.Reader) ([]byte, error) {
	return limitedio.LimitedReadAll(r, constants.MaxEncryptedFileSize, "encrypted file")
}

var rotateCmd = &cobra.Command{
	Use:   "rotate-passphrase",
	Short: "Re-encrypt all repositories with a new passphrase",
	Long: `Re-encrypt all repositories with a new passphrase.

This command:
1. Lists all repositories in the bucket
2. Decrypts all files with the current passphrase
3. Re-encrypts all files with a new passphrase
4. Uploads the re-encrypted files

WARNING: This is a destructive operation. Make sure you have the current
passphrase available and choose a strong new passphrase.`,
	RunE: runRotate,
}

func runRotate(cmd *cobra.Command, args []string) error {
	ctx, cancel := signalContext()
	defer cancel()
	out := GetOutput()

	if !ui.IsInteractive() {
		return fmt.Errorf("rotate-passphrase requires interactive mode")
	}

	// Get current passphrase
	out.Println("First, verify your current passphrase...")
	resolver := config.NewPassphraseResolver(cfg)
	currentPassphrase, err := resolver.Resolve()
	if err != nil {
		return fmt.Errorf("failed to get current passphrase: %w", err)
	}

	// Create encrypter to verify current passphrase
	currentEnc, err := crypto.NewAgeEncrypter(currentPassphrase)
	if err != nil {
		return err
	}

	// Get new passphrase
	out.Println()
	out.Println("Now, enter a new passphrase...")
	newPassphrase, err := config.PromptNewPassphrase()
	if err != nil {
		return err
	}

	// Create new encrypter
	newEnc, err := crypto.NewAgeEncrypter(newPassphrase)
	if err != nil {
		return err
	}

	// Create storage client
	store, err := storage.NewGCSStorage(ctx, cfg.Bucket, cfg.GCSCredentials)
	if err != nil {
		return err
	}

	// List all repos
	objects, err := store.List(ctx, "")
	if err != nil {
		return err
	}

	// Verify current passphrase can decrypt at least one file before proceeding
	out.Println()
	out.Printf("Verifying current passphrase...")
	verified := false
	for _, obj := range objects {
		if strings.HasSuffix(obj, ".age") && !strings.HasSuffix(obj, "/HEAD") {
			// Try to download and decrypt one file
			r, err := store.Download(ctx, obj)
			if err != nil {
				continue
			}
			data, err := readAllLimited(r)
			r.Close()
			if err != nil {
				continue
			}
			_, err = currentEnc.Decrypt(data)
			if err != nil {
				out.Println(" FAILED")
				return fmt.Errorf("current passphrase cannot decrypt existing files: %w", err)
			}
			verified = true
			break
		}
	}
	if !verified && len(objects) > 0 {
		// There are objects but none are .age files - unusual but not an error
		out.Println(" OK (no encrypted files found)")
	} else if verified {
		out.Println(" OK")
	}

	// Extract unique repos
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

	// Confirm
	prompt := ui.NewPrompt()
	confirmed, err := prompt.ConfirmDanger(
		fmt.Sprintf("This will re-encrypt %d repositories with the new passphrase.", len(repos)))
	if err != nil {
		return err
	}
	if !confirmed {
		out.Println("Aborted.")
		return nil
	}

	// Process each repo
	for repoPath := range repos {
		out.Printf("Processing %s...\n", repoPath)

		repoInfo, err := parseRepoPath(repoPath)
		if err != nil {
			out.Warn("Skipping invalid repo path: %s", repoPath)
			continue
		}

		if err := rotateRepo(ctx, store, repoInfo, currentEnc, newEnc); err != nil {
			out.Error("Failed to rotate %s: %v", repoPath, err)
			continue
		}

		out.Printf("  Rotated %s\n", repoPath)
	}

	out.Println()
	out.Success("Passphrase rotation complete!")
	out.Println()
	out.Println("IMPORTANT: Update your passphrase configuration to use the new passphrase.")

	return nil
}

func rotateRepo(ctx context.Context, store storage.Storage, repoInfo *domain.RepoInfo, oldEnc, newEnc crypto.Encrypter) error {
	// Create cache
	cacheRepo, err := cache.NewCache(repoInfo, store)
	if err != nil {
		return err
	}

	// Sync from storage
	if err := cacheRepo.SyncFromStorage(ctx); err != nil {
		return err
	}

	// Get all encrypted files
	files, err := cacheRepo.ListTrackedFiles()
	if err != nil {
		return err
	}

	// Re-encrypt each file
	for _, file := range files {
		// Read encrypted content
		encrypted, err := cacheRepo.ReadEncrypted(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}

		// Decrypt with old passphrase
		decrypted, err := oldEnc.Decrypt(encrypted)
		if err != nil {
			return fmt.Errorf("failed to decrypt %s: %w", file, err)
		}

		// Re-encrypt with new passphrase
		reencrypted, err := newEnc.Encrypt(decrypted)
		if err != nil {
			return fmt.Errorf("failed to re-encrypt %s: %w", file, err)
		}

		// Write back
		if err := cacheRepo.WriteEncrypted(file, reencrypted); err != nil {
			return fmt.Errorf("failed to write %s: %w", file, err)
		}
	}

	// Stage and commit
	if err := cacheRepo.StageAll(); err != nil {
		return err
	}

	_, err = cacheRepo.Commit("Rotate passphrase")
	if err != nil {
		return err
	}

	// Sync back to storage
	return cacheRepo.SyncToStorage(ctx)
}
