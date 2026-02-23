package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charliek/envsecrets/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestPrintCommit(t *testing.T) {
	fixedTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	commit := domain.Commit{
		Hash:      "abc1234567890abcdef1234567890abcdef123456",
		ShortHash: "abc1234",
		Message:   "add env files\n",
		Author:    "envsecrets",
		Date:      fixedTime,
		Files:     []string{".env", ".env.production"},
	}

	t.Run("non-verbose shows short hash and message", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewOutputWithWriters(&buf, &buf, false, false)
		out.PrintCommit(commit, false)

		output := buf.String()
		require.Equal(t, "abc1234 add env files\n", output)
	})

	t.Run("verbose shows author date and files", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewOutputWithWriters(&buf, &buf, false, false)
		out.PrintCommit(commit, true)

		output := buf.String()
		require.Contains(t, output, "abc1234 add env files")
		require.Contains(t, output, "Author: envsecrets")
		require.Contains(t, output, "Date:   2025-01-15T10:30:00Z")
		require.Contains(t, output, "- .env")
		require.Contains(t, output, "- .env.production")
	})

	t.Run("verbose with no files omits files section", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewOutputWithWriters(&buf, &buf, false, false)
		noFilesCommit := commit
		noFilesCommit.Files = nil
		out.PrintCommit(noFilesCommit, true)

		output := buf.String()
		require.Contains(t, output, "Author: envsecrets")
		require.False(t, strings.Contains(output, "Files:"))
	})
}
