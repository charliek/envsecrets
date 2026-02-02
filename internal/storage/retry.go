package storage

import (
	"context"
	"errors"
	"io"
	"math"
	"net"
	"time"

	"cloud.google.com/go/storage"
	"github.com/charliek/envsecrets/internal/domain"
	"google.golang.org/api/googleapi"
)

const (
	// DefaultMaxRetries is the default number of retry attempts
	DefaultMaxRetries = 3
	// DefaultInitialBackoff is the initial backoff duration
	DefaultInitialBackoff = 500 * time.Millisecond
	// DefaultMaxBackoff is the maximum backoff duration
	DefaultMaxBackoff = 30 * time.Second
)

// RetryConfig configures retry behavior
type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

// DefaultRetryConfig returns the default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     DefaultMaxRetries,
		InitialBackoff: DefaultInitialBackoff,
		MaxBackoff:     DefaultMaxBackoff,
	}
}

// isRetryableError determines if an error is transient and worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for context cancellation - don't retry
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check for GCS-specific retryable error
	if errors.Is(err, storage.ErrObjectNotExist) || errors.Is(err, storage.ErrBucketNotExist) {
		return false
	}

	// Check for network errors (timeout only, Temporary() is deprecated)
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	// Check for Google API errors with retryable status codes
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case 408, // Request Timeout
			429, // Too Many Requests
			500, // Internal Server Error
			502, // Bad Gateway
			503, // Service Unavailable
			504: // Gateway Timeout
			return true
		}
		return false
	}

	// Check for wrapped domain errors - don't retry file not found
	if errors.Is(err, domain.ErrFileNotFound) {
		return false
	}

	return false
}

// calculateBackoff calculates the backoff duration for a given attempt
func calculateBackoff(attempt int, cfg RetryConfig) time.Duration {
	backoff := time.Duration(float64(cfg.InitialBackoff) * math.Pow(2, float64(attempt)))
	if backoff > cfg.MaxBackoff {
		backoff = cfg.MaxBackoff
	}
	return backoff
}

// WithRetry wraps a function with retry logic
func WithRetry[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	var lastErr error
	var zero T

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if we should retry
		if !isRetryableError(err) {
			return zero, err
		}

		// Don't retry if this was the last attempt
		if attempt == cfg.MaxRetries {
			break
		}

		// Calculate backoff and wait
		backoff := calculateBackoff(attempt, cfg)

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(backoff):
			// Continue to next attempt
		}
	}

	return zero, lastErr
}

// WithRetryNoResult wraps a function that returns only an error with retry logic
func WithRetryNoResult(ctx context.Context, cfg RetryConfig, fn func() error) error {
	_, err := WithRetry(ctx, cfg, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

// RetryingStorage wraps a Storage implementation with retry logic
type RetryingStorage struct {
	inner Storage
	cfg   RetryConfig
}

// NewRetryingStorage creates a new retrying storage wrapper
func NewRetryingStorage(inner Storage, cfg RetryConfig) *RetryingStorage {
	return &RetryingStorage{inner: inner, cfg: cfg}
}

// Upload implements Storage.Upload with retry
func (s *RetryingStorage) Upload(ctx context.Context, path string, r io.Reader) error {
	// Note: We can't retry Upload with a plain io.Reader because it may be consumed
	// on first attempt. The caller should handle retry at a higher level if needed.
	return s.inner.Upload(ctx, path, r)
}

// Download implements Storage.Download with retry
func (s *RetryingStorage) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	return WithRetry(ctx, s.cfg, func() (io.ReadCloser, error) {
		return s.inner.Download(ctx, path)
	})
}

// List implements Storage.List with retry
func (s *RetryingStorage) List(ctx context.Context, prefix string) ([]string, error) {
	return WithRetry(ctx, s.cfg, func() ([]string, error) {
		return s.inner.List(ctx, prefix)
	})
}

// Delete implements Storage.Delete with retry
func (s *RetryingStorage) Delete(ctx context.Context, path string) error {
	return WithRetryNoResult(ctx, s.cfg, func() error {
		return s.inner.Delete(ctx, path)
	})
}

// Exists implements Storage.Exists with retry
func (s *RetryingStorage) Exists(ctx context.Context, path string) (bool, error) {
	return WithRetry(ctx, s.cfg, func() (bool, error) {
		return s.inner.Exists(ctx, path)
	})
}

// Close closes the underlying storage if it implements io.Closer
func (s *RetryingStorage) Close() error {
	if closer, ok := s.inner.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}
