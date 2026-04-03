package domain

import (
	"fmt"
	"testing"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/stretchr/testify/require"
)

func TestErrorToExitCode_VersionTooNew(t *testing.T) {
	err := fmt.Errorf("%w: remote v99", ErrVersionTooNew)
	code := errorToExitCode(err)
	require.Equal(t, constants.ExitVersionIncompatible, code)
}

func TestErrorToExitCode_VersionUnknown(t *testing.T) {
	err := fmt.Errorf("%w: no FORMAT marker", ErrVersionUnknown)
	code := errorToExitCode(err)
	require.Equal(t, constants.ExitVersionIncompatible, code)
}
