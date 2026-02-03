package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetNonInteractive(t *testing.T) {
	// Reset state after test
	defer SetNonInteractive(false)

	// Initially should be false
	SetNonInteractive(false)
	require.True(t, !nonInteractiveMode.Load())

	// Set to true
	SetNonInteractive(true)
	require.True(t, nonInteractiveMode.Load())

	// Set back to false
	SetNonInteractive(false)
	require.False(t, nonInteractiveMode.Load())
}

func TestCanPrompt(t *testing.T) {
	// Reset state after test
	defer SetNonInteractive(false)

	// In non-interactive mode, CanPrompt should return false
	SetNonInteractive(true)
	require.False(t, CanPrompt())

	// When not in non-interactive mode, CanPrompt depends on IsInteractive()
	// In test environment, stdin is typically not a terminal
	SetNonInteractive(false)
	// CanPrompt() will return false in tests because IsInteractive() is false
	// (stdin is not a terminal in test environment)
	require.False(t, CanPrompt())
}

func TestNonInteractiveModeThreadSafety(t *testing.T) {
	// Reset state after test
	defer SetNonInteractive(false)

	// Run concurrent reads and writes to verify no race conditions
	done := make(chan bool, 100)

	for i := 0; i < 50; i++ {
		go func() {
			SetNonInteractive(true)
			done <- true
		}()
		go func() {
			_ = CanPrompt()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestTruncateHash(t *testing.T) {
	tests := []struct {
		name     string
		hash     string
		expected string
	}{
		{
			name:     "long hash gets truncated",
			hash:     "abc1234567890def",
			expected: "abc1234",
		},
		{
			name:     "exactly 7 chars unchanged",
			hash:     "abc1234",
			expected: "abc1234",
		},
		{
			name:     "short hash unchanged",
			hash:     "abc",
			expected: "abc",
		},
		{
			name:     "empty hash unchanged",
			hash:     "",
			expected: "",
		},
		{
			name:     "typical git hash",
			hash:     "54e04f4abc1234567890abcdef1234567890ab",
			expected: "54e04f4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateHash(tt.hash)
			require.Equal(t, tt.expected, result)
		})
	}
}
