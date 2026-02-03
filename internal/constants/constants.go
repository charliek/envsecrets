package constants

import (
	"os"
	"path/filepath"
)

const (
	// ConfigFileName is the default config file name
	ConfigFileName = "config.yaml"

	// EnvSecretsDir is the directory name for envsecrets data
	EnvSecretsDir = ".envsecrets"

	// CacheDir is the subdirectory for cached encrypted files
	CacheDir = "cache"

	// EnvSecretsFile is the project file listing tracked env files
	EnvSecretsFile = ".envsecrets"

	// AgeExtension is the file extension for age-encrypted files
	AgeExtension = ".age"

	// DefaultPassphraseEnv is the default environment variable for passphrase
	DefaultPassphraseEnv = "ENVSECRETS_PASSPHRASE"

	// ConfigEnvVar is the environment variable to override config path
	ConfigEnvVar = "ENVSECRETS_CONFIG"

	// DefaultLogCount is the default number of log entries to show
	DefaultLogCount = 10

	// MaxEnvFileSize is the maximum size of an env file (1 MB)
	MaxEnvFileSize = 1 * 1024 * 1024

	// MaxEncryptedFileSize is the maximum size of an encrypted file (2 MB)
	// This is larger than MaxEnvFileSize to account for encryption overhead
	MaxEncryptedFileSize = 2 * 1024 * 1024

	// ScryptWorkFactor is the age scrypt work factor (2^18 iterations).
	// This provides strong protection against brute-force attacks while
	// keeping decryption time under 1 second on modern hardware.
	// Files encrypted with work factor 17 remain backward compatible.
	ScryptWorkFactor = 18

	// ShortHashLength is the number of characters in a short commit hash
	ShortHashLength = 7

	// BytesPerKB is the number of bytes in a kilobyte
	BytesPerKB = 1024
)

// Exit codes
const (
	ExitSuccess          = 0
	ExitNotConfigured    = 1
	ExitNotInRepo        = 2
	ExitNoEnvFiles       = 3
	ExitConflict         = 4
	ExitDecryptFailed    = 5
	ExitUploadFailed     = 6
	ExitDownloadFailed   = 7
	ExitInvalidConfig    = 8
	ExitGCSError         = 9
	ExitGitError         = 10
	ExitUserCancelled    = 11
	ExitInvalidArgs      = 12
	ExitFileNotFound     = 13
	ExitPermissionDenied = 14
	ExitUnknownError     = 99
)

// DefaultConfigDir returns the default configuration directory path
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, EnvSecretsDir)
}

// DefaultConfigPath returns the default configuration file path
func DefaultConfigPath() string {
	return filepath.Join(DefaultConfigDir(), ConfigFileName)
}

// DefaultCacheDir returns the default cache directory path
func DefaultCacheDir() string {
	return filepath.Join(DefaultConfigDir(), CacheDir)
}
