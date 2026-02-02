package io

import (
	"fmt"
	"io"

	"github.com/charliek/envsecrets/internal/domain"
)

// LimitedReadAll reads from r up to maxBytes.
// If the reader contains more than maxBytes, it returns an ErrFileSizeTooLarge error.
func LimitedReadAll(r io.Reader, maxBytes int64, context string) ([]byte, error) {
	// Create a limited reader that reads one extra byte to detect overflow
	limitedReader := io.LimitReader(r, maxBytes+1)

	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}

	if int64(len(data)) > maxBytes {
		return nil, domain.Errorf(domain.ErrFileSizeTooLarge,
			"%s exceeds maximum size of %d bytes", context, maxBytes)
	}

	return data, nil
}

// LimitedReader wraps an io.Reader with a size limit.
// It returns an error if more than maxBytes are read.
type LimitedReader struct {
	r         io.Reader
	maxBytes  int64
	bytesRead int64
	context   string
}

// NewLimitedReader creates a new LimitedReader.
func NewLimitedReader(r io.Reader, maxBytes int64, context string) *LimitedReader {
	return &LimitedReader{
		r:        r,
		maxBytes: maxBytes,
		context:  context,
	}
}

// Read implements io.Reader.
func (lr *LimitedReader) Read(p []byte) (n int, err error) {
	n, err = lr.r.Read(p)
	lr.bytesRead += int64(n)

	if lr.bytesRead > lr.maxBytes {
		return n, domain.Errorf(domain.ErrFileSizeTooLarge,
			"%s exceeds maximum size of %d bytes", lr.context, lr.maxBytes)
	}

	return n, err
}

// FormatSize returns a human-readable size string.
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
