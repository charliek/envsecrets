package domain

import (
	"errors"
	"fmt"

	"github.com/charliek/envsecrets/internal/constants"
)

// Sentinel errors
var (
	ErrNotConfigured    = errors.New("envsecrets not configured")
	ErrNotInRepo        = errors.New("not in a git repository")
	ErrNoEnvFiles       = errors.New("no .envsecrets file found")
	ErrNoFilesTracked   = errors.New("no files tracked in .envsecrets")
	ErrConflict         = errors.New("conflict between local and remote")
	ErrDecryptFailed    = errors.New("decryption failed")
	ErrEncryptFailed    = errors.New("encryption failed")
	ErrUploadFailed     = errors.New("upload failed")
	ErrDownloadFailed   = errors.New("download failed")
	ErrInvalidConfig    = errors.New("invalid configuration")
	ErrGCSError         = errors.New("GCS error")
	ErrGitError         = errors.New("git error")
	ErrUserCancelled    = errors.New("operation cancelled by user")
	ErrInvalidArgs      = errors.New("invalid arguments")
	ErrFileNotFound     = errors.New("file not found")
	ErrPermissionDenied = errors.New("permission denied")
	ErrNoPassphrase     = errors.New("passphrase not available")
	ErrRepoNotFound     = errors.New("repository not found")
	ErrRefNotFound      = errors.New("reference not found")
	ErrNothingToCommit  = errors.New("nothing to commit")
	ErrNotInitialized   = errors.New("cache not initialized")
	ErrRemoteChanged    = errors.New("remote has changed since last sync")
	ErrFileSizeTooLarge = errors.New("file size exceeds limit")
)

// ExitCodeError wraps an error with an exit code
type ExitCodeError struct {
	Err      error
	ExitCode int
}

func (e *ExitCodeError) Error() string {
	return e.Err.Error()
}

func (e *ExitCodeError) Unwrap() error {
	return e.Err
}

// NewExitCodeError creates a new ExitCodeError
func NewExitCodeError(err error, code int) *ExitCodeError {
	return &ExitCodeError{Err: err, ExitCode: code}
}

// WrapWithExitCode wraps an error with an exit code based on the error type
func WrapWithExitCode(err error) *ExitCodeError {
	if err == nil {
		return nil
	}

	// Check if already wrapped
	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr
	}

	code := errorToExitCode(err)
	return &ExitCodeError{Err: err, ExitCode: code}
}

// errorToExitCode maps errors to exit codes
func errorToExitCode(err error) int {
	switch {
	case errors.Is(err, ErrNotConfigured):
		return constants.ExitNotConfigured
	case errors.Is(err, ErrNotInRepo):
		return constants.ExitNotInRepo
	case errors.Is(err, ErrNoEnvFiles), errors.Is(err, ErrNoFilesTracked):
		return constants.ExitNoEnvFiles
	case errors.Is(err, ErrConflict), errors.Is(err, ErrRemoteChanged):
		return constants.ExitConflict
	case errors.Is(err, ErrDecryptFailed), errors.Is(err, ErrNoPassphrase):
		return constants.ExitDecryptFailed
	case errors.Is(err, ErrUploadFailed), errors.Is(err, ErrEncryptFailed):
		return constants.ExitUploadFailed
	case errors.Is(err, ErrDownloadFailed):
		return constants.ExitDownloadFailed
	case errors.Is(err, ErrInvalidConfig), errors.Is(err, ErrFileSizeTooLarge):
		return constants.ExitInvalidConfig
	case errors.Is(err, ErrGCSError):
		return constants.ExitGCSError
	case errors.Is(err, ErrGitError):
		return constants.ExitGitError
	case errors.Is(err, ErrUserCancelled):
		return constants.ExitUserCancelled
	case errors.Is(err, ErrInvalidArgs):
		return constants.ExitInvalidArgs
	case errors.Is(err, ErrFileNotFound), errors.Is(err, ErrRepoNotFound), errors.Is(err, ErrRefNotFound):
		return constants.ExitFileNotFound
	case errors.Is(err, ErrPermissionDenied):
		return constants.ExitPermissionDenied
	default:
		return constants.ExitUnknownError
	}
}

// GetExitCode returns the exit code for an error
func GetExitCode(err error) int {
	if err == nil {
		return constants.ExitSuccess
	}

	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode
	}

	return errorToExitCode(err)
}

// Errorf creates a formatted error wrapping a sentinel error
func Errorf(sentinel error, format string, args ...interface{}) error {
	return fmt.Errorf("%w: "+format, append([]interface{}{sentinel}, args...)...)
}
