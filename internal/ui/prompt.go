package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"golang.org/x/term"
)

// nonInteractiveMode tracks whether prompts should be disabled.
// Uses atomic.Bool for thread-safe access from concurrent operations.
var nonInteractiveMode atomic.Bool

// SetNonInteractive sets non-interactive mode (disables prompts).
// This function is thread-safe.
func SetNonInteractive(v bool) {
	nonInteractiveMode.Store(v)
}

// CanPrompt returns true if interactive prompts are allowed
// (terminal is available and non-interactive mode is not set).
// This function is thread-safe.
func CanPrompt() bool {
	return !nonInteractiveMode.Load() && IsInteractive()
}

// Prompt handles interactive prompts
type Prompt struct {
	reader *bufio.Reader
}

// NewPrompt creates a new prompt handler
func NewPrompt() *Prompt {
	return &Prompt{
		reader: bufio.NewReader(os.Stdin),
	}
}

// Confirm asks for a yes/no confirmation
func (p *Prompt) Confirm(message string, defaultYes bool) (bool, error) {
	defaultStr := "y/N"
	if defaultYes {
		defaultStr = "Y/n"
	}

	fmt.Fprintf(os.Stderr, "%s [%s]: ", message, defaultStr)

	input, err := p.reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		return defaultYes, nil
	}

	return input == "y" || input == "yes", nil
}

// ConfirmDanger asks for a yes/no confirmation for dangerous operations
// Requires typing "yes" explicitly
func (p *Prompt) ConfirmDanger(message string) (bool, error) {
	fmt.Fprintf(os.Stderr, "%s\nType 'yes' to confirm: ", message)

	input, err := p.reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(strings.ToLower(input)) == "yes", nil
}

// String asks for a string input
func (p *Prompt) String(message string, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", message, defaultValue)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", message)
	}

	input, err := p.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}

	return input, nil
}

// Password asks for a password (hidden input)
func (p *Prompt) Password(message string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", message)

	pass, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr) // Print newline after password input

	if err != nil {
		return "", err
	}

	return string(pass), nil
}

// Select asks user to select from a list of options
func (p *Prompt) Select(message string, options []string) (int, error) {
	if len(options) == 0 {
		return -1, fmt.Errorf("no options provided")
	}

	fmt.Fprintln(os.Stderr, message)
	for i, opt := range options {
		fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, opt)
	}
	fmt.Fprint(os.Stderr, "Selection: ")

	var selection int
	_, err := fmt.Fscanf(p.reader, "%d\n", &selection)
	if err != nil {
		return -1, err
	}

	if selection < 1 || selection > len(options) {
		return -1, fmt.Errorf("invalid selection: %d", selection)
	}

	return selection - 1, nil
}

// IsInteractive returns true if stdin is a terminal
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// ConflictChoice prompts the user to choose how to handle a file conflict
// Returns "o" for overwrite, "s" for skip, or "a" for abort
func (p *Prompt) ConflictChoice(filename string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s exists locally and differs from remote.\n", filename)
	fmt.Fprint(os.Stderr, "  [o]verwrite / [s]kip / [a]bort? ")

	input, err := p.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	switch strings.TrimSpace(strings.ToLower(input)) {
	case "o", "overwrite":
		return "o", nil
	case "s", "skip":
		return "s", nil
	default:
		return "a", nil
	}
}
