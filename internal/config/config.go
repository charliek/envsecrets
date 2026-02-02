package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/domain"
	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	// Bucket is the GCS bucket name
	Bucket string `yaml:"bucket"`

	// PassphraseEnv is the environment variable containing the passphrase
	PassphraseEnv string `yaml:"passphrase_env,omitempty"`

	// PassphraseCommand is a command to retrieve the passphrase (DEPRECATED: use passphrase_command_args)
	// This uses shell execution and is kept for backward compatibility
	PassphraseCommand string `yaml:"passphrase_command,omitempty"`

	// PassphraseCommandArgs is the preferred way to specify a passphrase command
	// It executes the command directly without shell interpolation
	// Example: ["pass", "show", "envsecrets"]
	PassphraseCommandArgs []string `yaml:"passphrase_command_args,omitempty"`

	// GCSCredentials is base64-encoded service account JSON
	GCSCredentials string `yaml:"gcs_credentials,omitempty"`

	// configPath is the path this config was loaded from (not serialized)
	configPath string `yaml:"-"`
}

// Load reads configuration from the specified path
func Load(path string) (*Config, error) {
	if path == "" {
		path = getConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domain.Errorf(domain.ErrNotConfigured, "config file not found at %s", path)
		}
		return nil, domain.Errorf(domain.ErrInvalidConfig, "failed to read config: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, domain.Errorf(domain.ErrInvalidConfig, "failed to parse config: %v", err)
	}

	cfg.configPath = path

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Save writes the configuration to the specified path using atomic write
func (c *Config) Save(path string) error {
	if path == "" {
		path = getConfigPath()
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return domain.Errorf(domain.ErrInvalidConfig, "failed to create config directory: %v", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return domain.Errorf(domain.ErrInvalidConfig, "failed to marshal config: %v", err)
	}

	// Atomic write: write to temp file, then rename
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return domain.Errorf(domain.ErrInvalidConfig, "failed to write config: %v", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath) // Clean up on failure
		return domain.Errorf(domain.ErrInvalidConfig, "failed to save config: %v", err)
	}

	c.configPath = path
	return nil
}

// Validate checks that the configuration is valid
func (c *Config) Validate() error {
	if c.Bucket == "" {
		return domain.Errorf(domain.ErrInvalidConfig, "bucket is required")
	}

	// Validate that both passphrase command formats aren't set simultaneously
	if c.PassphraseCommand != "" && len(c.PassphraseCommandArgs) > 0 {
		return domain.Errorf(domain.ErrInvalidConfig, "cannot set both passphrase_command and passphrase_command_args")
	}

	// At least one passphrase method should be configured, but we allow
	// interactive input as fallback, so this is not strictly required
	return nil
}

// Path returns the path this config was loaded from
func (c *Config) Path() string {
	return c.configPath
}

// HasPassphraseConfig returns true if a passphrase retrieval method is configured
func (c *Config) HasPassphraseConfig() bool {
	return c.PassphraseEnv != "" || c.PassphraseCommand != "" || len(c.PassphraseCommandArgs) > 0
}

// getConfigPath returns the config path from env var or default
func getConfigPath() string {
	if path := os.Getenv(constants.ConfigEnvVar); path != "" {
		return path
	}
	return constants.DefaultConfigPath()
}

// Exists checks if a config file exists at the default or specified path
func Exists(path string) bool {
	if path == "" {
		path = getConfigPath()
	}
	_, err := os.Stat(path)
	return err == nil
}

// ConfigPath returns the path that would be used for config
func ConfigPath(override string) string {
	if override != "" {
		return override
	}
	return getConfigPath()
}

// String returns a string representation (for debugging, hides sensitive data)
func (c *Config) String() string {
	creds := ""
	if c.GCSCredentials != "" {
		creds = "[set]"
	}
	passEnv := ""
	if c.PassphraseEnv != "" {
		passEnv = "[set]"
	}
	passCmd := ""
	if c.PassphraseCommand != "" {
		passCmd = "[set:deprecated]"
	}
	passCmdArgs := ""
	if len(c.PassphraseCommandArgs) > 0 {
		passCmdArgs = "[set]"
	}
	return fmt.Sprintf("Config{Bucket: %q, PassphraseEnv: %s, PassphraseCommand: %s, PassphraseCommandArgs: %s, GCSCredentials: %s}",
		c.Bucket, passEnv, passCmd, passCmdArgs, creds)
}
