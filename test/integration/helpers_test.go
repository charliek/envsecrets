//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// createTestProject creates a temporary project directory with test files
func createTestProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Create .git directory (minimal)
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0755))

	// Create .envsecrets file
	envSecretsContent := ".env\n.env.local\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".envsecrets"),
		[]byte(envSecretsContent),
		0644,
	))

	// Create test env files
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".env"),
		[]byte("DATABASE_URL=postgres://localhost/test\nSECRET_KEY=test123\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".env.local"),
		[]byte("DEBUG=true\nLOCAL_ONLY=yes\n"),
		0644,
	))

	return dir
}

// createTestConfig creates a temporary config file
func createTestConfig(t *testing.T, bucket, passphrase string) string {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	content := "bucket: " + bucket + "\npassphrase_env: TEST_PASSPHRASE\n"
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0600))

	t.Setenv("TEST_PASSPHRASE", passphrase)

	return configPath
}
