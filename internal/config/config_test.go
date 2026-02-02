package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig_Load(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config",
			content: `bucket: test-bucket
passphrase_env: MY_PASS
`,
			wantErr: false,
		},
		{
			name: "valid config with command",
			content: `bucket: test-bucket
passphrase_command: echo secret
`,
			wantErr: false,
		},
		{
			name:        "missing bucket",
			content:     `passphrase_env: MY_PASS`,
			wantErr:     true,
			errContains: "bucket is required",
		},
		{
			name:        "invalid yaml",
			content:     `bucket: [invalid`,
			wantErr:     true,
			errContains: "failed to parse config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp config file
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			err := os.WriteFile(path, []byte(tt.content), 0600)
			require.NoError(t, err)

			cfg, err := Load(path)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)
		})
	}
}

func TestConfig_Save(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.yaml")

	cfg := &Config{
		Bucket:        "my-bucket",
		PassphraseEnv: "MY_SECRET",
	}

	err := cfg.Save(path)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Load it back
	loaded, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, cfg.Bucket, loaded.Bucket)
	require.Equal(t, cfg.PassphraseEnv, loaded.PassphraseEnv)
}

func TestConfig_Load_NotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "config file not found")
}

func TestConfig_HasPassphraseConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   bool
	}{
		{
			name:   "no config",
			config: &Config{Bucket: "test"},
			want:   false,
		},
		{
			name:   "env set",
			config: &Config{Bucket: "test", PassphraseEnv: "MY_PASS"},
			want:   true,
		},
		{
			name:   "command set",
			config: &Config{Bucket: "test", PassphraseCommand: "echo secret"},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.config.HasPassphraseConfig())
		})
	}
}

func TestConfigPath(t *testing.T) {
	// Test with override
	require.Equal(t, "/custom/path", ConfigPath("/custom/path"))

	// Test with env var
	t.Setenv("ENVSECRETS_CONFIG", "/env/path")
	require.Equal(t, "/env/path", ConfigPath(""))
}

func TestConfig_PassphraseCommandArgs(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid passphrase_command_args",
			content: `bucket: test-bucket
passphrase_command_args:
  - echo
  - secret
`,
			wantErr: false,
		},
		{
			name: "cannot set both passphrase_command and passphrase_command_args",
			content: `bucket: test-bucket
passphrase_command: echo legacy
passphrase_command_args:
  - echo
  - new
`,
			wantErr:     true,
			errContains: "cannot set both passphrase_command and passphrase_command_args",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			err := os.WriteFile(path, []byte(tt.content), 0600)
			require.NoError(t, err)

			cfg, err := Load(path)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)
		})
	}
}

func TestConfig_HasPassphraseConfig_WithArgs(t *testing.T) {
	cfg := &Config{
		Bucket:                "test",
		PassphraseCommandArgs: []string{"pass", "show", "secret"},
	}
	require.True(t, cfg.HasPassphraseConfig())
}
