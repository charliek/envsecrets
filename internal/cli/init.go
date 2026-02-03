package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charliek/envsecrets/internal/config"
	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize envsecrets configuration",
	Long: `Initialize envsecrets configuration interactively.

This command creates the configuration file at ~/.envsecrets/config.yaml
with your GCS bucket and passphrase settings.`,
	RunE: runInit,
}

// parseShellArgs splits a command string into arguments, respecting quotes.
// Handles both single and double quotes for arguments with spaces.
func parseShellArgs(s string) ([]string, error) {
	var args []string
	var current strings.Builder
	var inQuote rune
	escaped := false

	for _, r := range s {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		switch {
		case r == '\\' && inQuote != '\'':
			escaped = true
		case r == '"' || r == '\'':
			if inQuote == 0 {
				inQuote = r
			} else if inQuote == r {
				inQuote = 0
			} else {
				current.WriteRune(r)
			}
		case r == ' ' || r == '\t':
			if inQuote != 0 {
				current.WriteRune(r)
			} else if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if inQuote != 0 {
		return nil, fmt.Errorf("unclosed quote")
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	if len(args) == 0 {
		return nil, fmt.Errorf("no command specified")
	}

	return args, nil
}

func runInit(cmd *cobra.Command, args []string) error {
	// Init requires interactive mode
	if !ui.CanPrompt() {
		return fmt.Errorf("init requires interactive mode")
	}

	prompt := ui.NewPrompt()
	out := ui.NewOutput(verbose, jsonOut)

	configPath := config.ConfigPath(cfgFile)

	// Check if config already exists
	if config.Exists(configPath) {
		confirmed, err := prompt.Confirm("Configuration already exists. Overwrite?", false)
		if err != nil {
			return err
		}
		if !confirmed {
			out.Println("Aborted.")
			return nil
		}
	}

	out.Println("Setting up envsecrets configuration...")
	out.Println()

	// Get bucket name
	bucket, err := prompt.String("GCS bucket name", "")
	if err != nil {
		return err
	}
	if bucket == "" {
		return fmt.Errorf("bucket name is required")
	}

	// Get passphrase method
	out.Println()
	out.Println("How would you like to provide the passphrase?")
	out.Println("  1. Environment variable")
	out.Println("  2. Command (e.g., 1Password CLI)")
	out.Println("  3. Enter manually each time")

	selection, err := prompt.String("Selection", "1")
	if err != nil {
		return err
	}

	cfg := &config.Config{
		Bucket: bucket,
	}

	switch selection {
	case "1":
		envVar, err := prompt.String("Environment variable name", constants.DefaultPassphraseEnv)
		if err != nil {
			return err
		}
		cfg.PassphraseEnv = envVar
	case "2":
		out.Println("Enter command and arguments (space-separated, e.g., 'pass show envsecrets'):")
		out.Println("Use quotes for arguments with spaces (e.g., 'op read \"my secret\"'):")
		cmdStr, err := prompt.String("Command", "")
		if err != nil {
			return err
		}
		cmdStr = strings.TrimSpace(cmdStr)
		if cmdStr == "" {
			return fmt.Errorf("command is required")
		}
		// Parse command string into args (handles quoted arguments)
		args, err := parseShellArgs(cmdStr)
		if err != nil {
			return fmt.Errorf("invalid command: %w", err)
		}
		cfg.PassphraseCommandArgs = args
	case "3":
		// No passphrase config - will prompt each time
		out.Println("Passphrase will be requested when needed.")
	default:
		return fmt.Errorf("invalid selection: %s", selection)
	}

	// Ask about GCS credentials
	out.Println()
	out.Println("GCS Authentication:")
	out.Println("  1. Use Application Default Credentials (gcloud auth)")
	out.Println("  2. Use service account JSON file")

	credSelection, err := prompt.String("Selection", "1")
	if err != nil {
		return err
	}

	if credSelection == "2" {
		credPath, err := prompt.String("Path to service account JSON", "")
		if err != nil {
			return err
		}
		if credPath != "" {
			// Read and encode the file
			encoded, err := encodeServiceAccountFile(credPath)
			if err != nil {
				return fmt.Errorf("failed to encode service account file: %w", err)
			}
			cfg.GCSCredentials = encoded
		}
	}

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Save config
	if err := cfg.Save(configPath); err != nil {
		return err
	}

	out.Println()
	out.Success("Configuration saved to %s", configPath)
	out.Println()
	out.Println("Next steps:")
	out.Println("  1. Create a .envsecrets file in your project listing files to track")
	out.Println("  2. Run 'envsecrets doctor' to verify your setup")
	out.Println("  3. Run 'envsecrets push' to encrypt and upload your files")

	return nil
}
