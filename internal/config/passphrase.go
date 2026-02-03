package config

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charliek/envsecrets/internal/domain"
	"golang.org/x/term"
)

// PassphraseCommandTimeout is the maximum time allowed for passphrase commands to complete
const PassphraseCommandTimeout = 30 * time.Second

// PassphraseResolver handles passphrase retrieval from various sources
type PassphraseResolver struct {
	config *Config
}

// NewPassphraseResolver creates a new resolver for the given config
func NewPassphraseResolver(cfg *Config) *PassphraseResolver {
	return &PassphraseResolver{config: cfg}
}

// Resolve attempts to get the passphrase using the configured method
// Resolution order:
// 1. Environment variable (if passphrase_env is set)
// 2. Command args (if passphrase_command_args is set)
// 3. Interactive prompt (if terminal is available)
func (r *PassphraseResolver) Resolve() (string, error) {
	// Try environment variable first
	if r.config.PassphraseEnv != "" {
		if pass := os.Getenv(r.config.PassphraseEnv); pass != "" {
			return pass, nil
		}
	}

	// Try command args
	if len(r.config.PassphraseCommandArgs) > 0 {
		pass, err := r.runCommandArgs()
		if err != nil {
			return "", domain.Errorf(domain.ErrNoPassphrase, "passphrase command failed: %v", err)
		}
		return pass, nil
	}

	// Try interactive prompt
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return r.promptInteractive()
	}

	return "", domain.ErrNoPassphrase
}

// runCommandArgs executes the passphrase command with explicit arguments (secure method)
func (r *PassphraseResolver) runCommandArgs() (string, error) {
	args := r.config.PassphraseCommandArgs
	if len(args) == 0 {
		return "", fmt.Errorf("no command arguments specified")
	}

	ctx, cancel := context.WithTimeout(context.Background(), PassphraseCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("passphrase command timed out after %v", PassphraseCommandTimeout)
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("%v: %s", err, errMsg)
		}
		return "", err
	}

	pass := strings.TrimSpace(stdout.String())
	if pass == "" {
		return "", fmt.Errorf("command returned empty passphrase")
	}

	return pass, nil
}

// promptInteractive prompts the user for the passphrase
func (r *PassphraseResolver) promptInteractive() (string, error) {
	fmt.Fprint(os.Stderr, "Enter passphrase: ")
	pass, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr) // Print newline after password input
	if err != nil {
		return "", domain.Errorf(domain.ErrNoPassphrase, "failed to read passphrase: %v", err)
	}

	passStr := string(pass)
	if passStr == "" {
		return "", domain.Errorf(domain.ErrNoPassphrase, "passphrase cannot be empty")
	}

	return passStr, nil
}

// PromptNewPassphrase prompts for a new passphrase with confirmation
func PromptNewPassphrase() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", domain.Errorf(domain.ErrNoPassphrase, "cannot prompt for passphrase in non-interactive mode")
	}

	fmt.Fprint(os.Stderr, "Enter new passphrase: ")
	pass1, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", domain.Errorf(domain.ErrNoPassphrase, "failed to read passphrase: %v", err)
	}

	fmt.Fprint(os.Stderr, "Confirm passphrase: ")
	pass2, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", domain.Errorf(domain.ErrNoPassphrase, "failed to read passphrase: %v", err)
	}

	if string(pass1) != string(pass2) {
		return "", domain.Errorf(domain.ErrNoPassphrase, "passphrases do not match")
	}

	passStr := string(pass1)
	if passStr == "" {
		return "", domain.Errorf(domain.ErrNoPassphrase, "passphrase cannot be empty")
	}

	return passStr, nil
}
