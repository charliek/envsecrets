package sync

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConflictAction_Constants(t *testing.T) {
	// Verify the constant values are distinct
	require.NotEqual(t, ConflictOverwrite, ConflictSkip)
	require.NotEqual(t, ConflictOverwrite, ConflictAbort)
	require.NotEqual(t, ConflictSkip, ConflictAbort)
}

func TestConflictResolver_SkipAll(t *testing.T) {
	resolver := func(filename string) (ConflictAction, error) {
		return ConflictSkip, nil
	}

	action, err := resolver("test.env")
	require.NoError(t, err)
	require.Equal(t, ConflictSkip, action)
}

func TestConflictResolver_OverwriteAll(t *testing.T) {
	resolver := func(filename string) (ConflictAction, error) {
		return ConflictOverwrite, nil
	}

	action, err := resolver("test.env")
	require.NoError(t, err)
	require.Equal(t, ConflictOverwrite, action)
}

func TestConflictResolver_Abort(t *testing.T) {
	resolver := func(filename string) (ConflictAction, error) {
		return ConflictAbort, nil
	}

	action, err := resolver("test.env")
	require.NoError(t, err)
	require.Equal(t, ConflictAbort, action)
}

func TestConflictResolver_Error(t *testing.T) {
	testErr := errors.New("user cancelled")
	resolver := func(filename string) (ConflictAction, error) {
		return ConflictAbort, testErr
	}

	_, err := resolver("test.env")
	require.ErrorIs(t, err, testErr)
}

func TestConflictResolver_PerFile(t *testing.T) {
	// Resolver that handles different files differently
	resolver := func(filename string) (ConflictAction, error) {
		switch filename {
		case ".env":
			return ConflictOverwrite, nil
		case ".env.local":
			return ConflictSkip, nil
		default:
			return ConflictAbort, nil
		}
	}

	action, err := resolver(".env")
	require.NoError(t, err)
	require.Equal(t, ConflictOverwrite, action)

	action, err = resolver(".env.local")
	require.NoError(t, err)
	require.Equal(t, ConflictSkip, action)

	action, err = resolver(".env.production")
	require.NoError(t, err)
	require.Equal(t, ConflictAbort, action)
}
