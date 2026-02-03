package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPassphraseResolver_ResolveFromEnv(t *testing.T) {
	t.Setenv("TEST_PASSPHRASE", "my-secret")

	cfg := &Config{
		Bucket:        "test",
		PassphraseEnv: "TEST_PASSPHRASE",
	}

	resolver := NewPassphraseResolver(cfg)
	pass, err := resolver.Resolve()

	require.NoError(t, err)
	require.Equal(t, "my-secret", pass)
}

func TestPassphraseResolver_ResolveFromCommandArgs(t *testing.T) {
	cfg := &Config{
		Bucket:                "test",
		PassphraseCommandArgs: []string{"echo", "secure-passphrase"},
	}

	resolver := NewPassphraseResolver(cfg)
	pass, err := resolver.Resolve()

	require.NoError(t, err)
	require.Equal(t, "secure-passphrase", pass)
}

func TestPassphraseResolver_CommandArgsWorks(t *testing.T) {
	cfg := &Config{
		Bucket:                "test",
		PassphraseCommandArgs: []string{"echo", "from-args"},
	}

	resolver := NewPassphraseResolver(cfg)
	pass, err := resolver.Resolve()

	require.NoError(t, err)
	require.Equal(t, "from-args", pass)
}

func TestPassphraseResolver_CommandArgsWithMultipleArgs(t *testing.T) {
	// Test that command args handles multiple arguments correctly
	cfg := &Config{
		Bucket:                "test",
		PassphraseCommandArgs: []string{"printf", "%s", "pass123"},
	}

	resolver := NewPassphraseResolver(cfg)
	pass, err := resolver.Resolve()

	require.NoError(t, err)
	require.Equal(t, "pass123", pass)
}

func TestPassphraseResolver_EmptyCommandArgs(t *testing.T) {
	cfg := &Config{
		Bucket:                "test",
		PassphraseCommandArgs: []string{},
	}

	resolver := NewPassphraseResolver(cfg)
	_, err := resolver.Resolve()

	// Should fall through to interactive prompt which fails without terminal
	require.Error(t, err)
}

func TestPassphraseResolver_CommandArgsFailure(t *testing.T) {
	cfg := &Config{
		Bucket:                "test",
		PassphraseCommandArgs: []string{"nonexistent-command-xyz"},
	}

	resolver := NewPassphraseResolver(cfg)
	_, err := resolver.Resolve()

	require.Error(t, err)
	require.Contains(t, err.Error(), "passphrase command failed")
}

func TestPassphraseResolver_CommandArgsReturnsEmpty(t *testing.T) {
	cfg := &Config{
		Bucket:                "test",
		PassphraseCommandArgs: []string{"echo", ""},
	}

	resolver := NewPassphraseResolver(cfg)
	_, err := resolver.Resolve()

	require.Error(t, err)
	require.Contains(t, err.Error(), "empty passphrase")
}

func TestPassphraseResolver_NoConfig(t *testing.T) {
	// Ensure stdin is not a terminal for this test
	cfg := &Config{
		Bucket: "test",
	}

	resolver := NewPassphraseResolver(cfg)
	_, err := resolver.Resolve()

	// Should fail because no config and not a terminal
	require.Error(t, err)
}

func TestPassphraseResolver_EnvTakesPrecedenceOverCommand(t *testing.T) {
	t.Setenv("TEST_PASS_PRECEDENCE", "from-env")

	cfg := &Config{
		Bucket:                "test",
		PassphraseEnv:         "TEST_PASS_PRECEDENCE",
		PassphraseCommandArgs: []string{"echo", "from-command"},
	}

	resolver := NewPassphraseResolver(cfg)
	pass, err := resolver.Resolve()

	require.NoError(t, err)
	require.Equal(t, "from-env", pass, "env should take precedence over command")
}
