package cli

import (
	"fmt"

	"github.com/charliek/envsecrets/internal/cache"
	"github.com/charliek/envsecrets/internal/config"
	"github.com/charliek/envsecrets/internal/crypto"
	"github.com/charliek/envsecrets/internal/project"
	"github.com/charliek/envsecrets/internal/storage"
	"github.com/spf13/cobra"
)

var (
	doctorFix bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Verify configuration and connectivity",
	Long: `Verify that envsecrets is properly configured.

This command checks:
- Configuration file exists and is valid
- GCS bucket is accessible
- Passphrase is available
- Current directory is a git repository (optional)
- Local cache health

Use --fix to attempt automatic repair of cache issues.`,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "attempt to fix cache issues by resetting")
}

func runDoctor(cmd *cobra.Command, args []string) error {
	ctx, cancel := signalContext()
	defer cancel()
	out := GetOutput()
	allOK := true

	out.Println("Checking envsecrets configuration...")
	out.Println()

	// Check config file
	configPath := config.ConfigPath(cfgFile)
	out.Printf("Config file (%s): ", configPath)
	if cfg == nil {
		out.Println("MISSING")
		out.Println("  Run 'envsecrets init' to create configuration")
		return nil
	}
	out.Println("OK")

	// Check bucket configuration
	out.Printf("Bucket configured: ")
	if cfg.Bucket == "" {
		out.Println("MISSING")
		allOK = false
	} else {
		out.Println(cfg.Bucket)
	}

	// Check GCS connectivity
	out.Printf("GCS connectivity: ")
	store, err := storage.NewGCSStorage(ctx, cfg.Bucket, cfg.GCSCredentials)
	if err != nil {
		out.Println("FAILED")
		out.Printf("  Error: %v\n", err)
		allOK = false
	} else {
		defer store.Close()
		// Try to list objects to verify access
		_, err := store.List(ctx, "")
		if err != nil {
			out.Println("FAILED")
			out.Printf("  Error: %v\n", err)
			allOK = false
		} else {
			out.Println("OK")
		}
	}

	// Check passphrase availability
	out.Printf("Passphrase: ")
	resolver := config.NewPassphraseResolver(cfg)
	passphrase, err := resolver.Resolve()
	if err != nil {
		out.Println("NOT AVAILABLE")
		if cfg.PassphraseEnv != "" {
			out.Printf("  Set environment variable: %s\n", cfg.PassphraseEnv)
		} else if len(cfg.PassphraseCommandArgs) > 0 {
			out.Println("  Passphrase command failed to execute")
		} else {
			out.Println("  Configure passphrase_env or passphrase_command_args in config")
		}
		allOK = false
	} else {
		out.Println("OK")

		// Test encryption/decryption
		out.Printf("Encryption: ")
		{
			encrypter, err := crypto.NewAgeEncrypter(passphrase)
			if err != nil {
				out.Println("FAILED")
				out.Printf("  Error: %v\n", err)
				allOK = false
			} else {
				testData := []byte("test encryption")
				encrypted, err := encrypter.Encrypt(testData)
				if err != nil {
					out.Println("FAILED")
					out.Printf("  Encrypt error: %v\n", err)
					allOK = false
				} else {
					decrypted, err := encrypter.Decrypt(encrypted)
					if err != nil {
						out.Println("FAILED")
						out.Printf("  Decrypt error: %v\n", err)
						allOK = false
					} else if string(decrypted) != string(testData) {
						out.Println("FAILED")
						out.Println("  Round-trip verification failed")
						allOK = false
					} else {
						out.Println("OK")
					}
				}
			}
		}
	}

	// Check git repository (optional)
	out.Printf("Git repository: ")
	discovery, err := project.NewDiscovery("")
	var repoInfoForCache *project.Discovery
	if err != nil {
		out.Println("NOT IN REPO")
		out.Println("  (This is OK - you can still use envsecrets globally)")
	} else {
		repoInfoForCache = discovery
		repoInfo, err := discovery.RepoInfo()
		if err != nil {
			out.Println("NO REMOTE")
			out.Printf("  Error: %v\n", err)
			repoInfoForCache = nil
		} else {
			out.Println(repoInfo.String())
		}

		// Check .envsecrets file
		out.Printf(".envsecrets file: ")
		files, err := discovery.EnvFiles()
		if err != nil {
			out.Println("NOT FOUND")
			out.Println("  Create a .envsecrets file listing files to track")
		} else {
			out.Printf("OK (%d files)\n", len(files))
			for _, f := range files {
				exists := "missing"
				if discovery.FileExists(f) {
					exists = "exists"
				}
				out.Printf("    %s (%s)\n", f, exists)
			}
		}

		// Check cache health if we have repo info and storage
		if repoInfoForCache != nil && store != nil {
			repoInfo, _ := repoInfoForCache.RepoInfo()
			out.Printf("Local cache: ")
			cacheRepo, err := cache.NewCache(repoInfo, store)
			if err != nil {
				out.Println("ERROR")
				out.Printf("  Error: %v\n", err)
				allOK = false
			} else {
				health := cacheRepo.Validate()
				if !health.Exists {
					out.Println("NOT INITIALIZED")
					out.Println("  (This is OK - cache will be created on first push/pull)")
				} else if health.Error != nil {
					out.Println("CORRUPTED")
					out.Printf("  Error: %v\n", health.Error)
					out.Println("  Run 'envsecrets doctor --fix' to reset the cache")
					allOK = false

					if doctorFix {
						out.Println()
						out.Printf("Attempting to reset cache...")
						if err := cacheRepo.Reset(ctx); err != nil {
							out.Println(" FAILED")
							out.Printf("  Error: %v\n", err)
							out.Printf("  Manual fix: rm -rf ~/.envsecrets/cache/%s/%s\n", repoInfo.Owner, repoInfo.Name)
						} else {
							out.Println(" OK")
							allOK = true // Fixed!
						}
					}
				} else {
					var status string
					if health.HeadValid {
						status = fmt.Sprintf("OK (%d files)", health.FileCount)
					} else {
						status = "OK (empty)"
					}
					out.Println(status)
				}
			}
		}
	}

	out.Println()
	if allOK {
		out.Success("All checks passed!")
	} else {
		return fmt.Errorf("some checks failed")
	}

	return nil
}
