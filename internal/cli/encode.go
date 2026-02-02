package cli

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var encodeCmd = &cobra.Command{
	Use:   "encode <path>",
	Short: "Base64 encode a service account JSON file",
	Long: `Base64 encode a service account JSON file for use in configuration.

The encoded string can be used as the gcs_credentials value in your config file.`,
	Args: cobra.ExactArgs(1),
	RunE: runEncode,
}

func runEncode(cmd *cobra.Command, args []string) error {
	path := args[0]

	encoded, err := encodeServiceAccountFile(path)
	if err != nil {
		return err
	}

	fmt.Println(encoded)
	return nil
}

// encodeServiceAccountFile reads and base64 encodes a file
func encodeServiceAccountFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return base64.StdEncoding.EncodeToString(data), nil
}
