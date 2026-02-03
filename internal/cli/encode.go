package cli

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var encodeCopy bool

var encodeCmd = &cobra.Command{
	Use:   "encode <path>",
	Short: "Base64 encode a service account JSON file",
	Long: `Base64 encode a service account JSON file for use in configuration.

The encoded string can be used as the gcs_credentials value in your config file.
Use --copy to copy the result to your clipboard.`,
	Args: cobra.ExactArgs(1),
	RunE: runEncode,
}

func init() {
	encodeCmd.Flags().BoolVar(&encodeCopy, "copy", false, "copy to clipboard")
}

func runEncode(cmd *cobra.Command, args []string) error {
	path := args[0]
	out := GetOutput()

	encoded, err := encodeServiceAccountFile(path)
	if err != nil {
		return err
	}

	if encodeCopy {
		if err := copyToClipboard(encoded); err != nil {
			out.Warn("Failed to copy to clipboard: %v", err)
			out.Println("Falling back to stdout:")
			fmt.Println(encoded)
		} else {
			out.Println("Copied to clipboard.")
		}
	} else {
		fmt.Println(encoded)
	}
	return nil
}

// copyToClipboard copies text to the system clipboard
func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, fall back to xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard utility found (install xclip or xsel)")
		}
	case "windows":
		cmd = exec.Command("clip")
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// encodeServiceAccountFile reads and base64 encodes a file
func encodeServiceAccountFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return base64.StdEncoding.EncodeToString(data), nil
}
