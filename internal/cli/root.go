package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charliek/envsecrets/internal/config"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/charliek/envsecrets/internal/version"
	"github.com/spf13/cobra"
)

const (
	// DefaultOperationTimeout is the default timeout for operations (5 minutes)
	DefaultOperationTimeout = 5 * time.Minute
)

var (
	// Global flags
	cfgFile        string
	verbose        bool
	jsonOut        bool
	repo           string
	nonInteractive bool

	// Shared state
	cfg    *config.Config
	output *ui.Output
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "envsecrets",
	Short: "Manage encrypted environment files",
	Long: `envsecrets is a CLI tool for managing encrypted environment files
using GCS and age encryption.

Environment files are encrypted locally and synced to Google Cloud Storage,
providing secure team-wide access with version history.`,
	Version: version.Version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Initialize output handler
		output = ui.NewOutput(verbose, jsonOut)

		// Set non-interactive mode
		ui.SetNonInteractive(nonInteractive)

		// Skip config loading for commands that don't need it
		if !needsConfig(cmd) {
			return nil
		}

		// Load configuration
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return err
		}

		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// needsConfig returns true if the command requires configuration
func needsConfig(cmd *cobra.Command) bool {
	// Commands that don't need config
	noConfigCmds := map[string]bool{
		"init":       true,
		"encode":     true,
		"help":       true,
		"completion": true,
		"version":    true,
	}

	return !noConfigCmds[cmd.Name()]
}

// Execute runs the root command
func Execute() error {
	err := rootCmd.Execute()
	if err != nil {
		// Print error if output is available
		if output != nil {
			output.Error("%v", err)
		} else {
			// Fallback if output isn't initialized
			ui.NewOutput(false, false).Error("%v", err)
		}

		// Return exit code error
		return domain.WrapWithExitCode(err)
	}
	return nil
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.envsecrets/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "output in JSON format")
	rootCmd.PersistentFlags().StringVarP(&repo, "repo", "r", "", "override repository (owner/name)")
	rootCmd.PersistentFlags().BoolVar(&nonInteractive, "non-interactive", false, "disable interactive prompts (for CI/CD)")

	// Set version template
	rootCmd.SetVersionTemplate("envsecrets {{.Version}}\n")

	// Add commands
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(encodeCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(logCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(revertCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(rmCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(rotateCmd)
	rootCmd.AddCommand(verifyCmd)
}

// GetConfig returns the loaded configuration (for use by subcommands)
func GetConfig() *config.Config {
	return cfg
}

// GetOutput returns the output handler (for use by subcommands)
func GetOutput() *ui.Output {
	return output
}

// GetRepo returns the repo override flag value (for use by subcommands)
func GetRepo() string {
	return repo
}

// ExitWithError prints an error and exits with the appropriate code
func ExitWithError(err error) {
	if output != nil {
		output.Error("%v", err)
	}
	code := domain.GetExitCode(err)
	os.Exit(code)
}

// signalContext returns a context that is cancelled on SIGINT, SIGTERM, or timeout
func signalContext() (context.Context, context.CancelFunc) {
	// Create context with timeout
	ctx, timeoutCancel := context.WithTimeout(context.Background(), DefaultOperationTimeout)

	// Create cancellable context for signal handling
	ctx, signalCancel := context.WithCancel(ctx)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-c:
			signalCancel()
		case <-ctx.Done():
		}
		signal.Stop(c)
		// Drain any pending signal to prevent goroutine leak
		select {
		case <-c:
		default:
		}
	}()

	// Return a combined cancel function
	return ctx, func() {
		signalCancel()
		timeoutCancel()
	}
}
