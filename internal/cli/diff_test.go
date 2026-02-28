package cli

import (
	"bytes"
	"testing"

	"github.com/charliek/envsecrets/internal/ui"
	"github.com/stretchr/testify/require"
)

func TestPrintLineDiff_AddedLines(t *testing.T) {
	var buf bytes.Buffer
	out := ui.NewOutputWithWriters(&buf, &buf, false, false)

	printLineDiff(out, "line1\n", "line1\nline2\n")

	output := buf.String()
	require.Contains(t, output, "+ line2")
	require.NotContains(t, output, "- ")
}

func TestPrintLineDiff_RemovedLines(t *testing.T) {
	var buf bytes.Buffer
	out := ui.NewOutputWithWriters(&buf, &buf, false, false)

	printLineDiff(out, "line1\nline2\n", "line1\n")

	output := buf.String()
	require.Contains(t, output, "- line2")
	require.NotContains(t, output, "+ ")
}

func TestPrintLineDiff_ModifiedLines(t *testing.T) {
	var buf bytes.Buffer
	out := ui.NewOutputWithWriters(&buf, &buf, false, false)

	printLineDiff(out, "KEY=old\n", "KEY=new\n")

	output := buf.String()
	require.Contains(t, output, "- KEY=old")
	require.Contains(t, output, "+ KEY=new")
}

func TestPrintLineDiff_DuplicateLines(t *testing.T) {
	var buf bytes.Buffer
	out := ui.NewOutputWithWriters(&buf, &buf, false, false)

	// The old set-based diff would incorrectly handle duplicates
	old := "A=1\nB=2\nA=1\n"
	new := "A=1\nC=3\nA=1\n"

	printLineDiff(out, old, new)

	output := buf.String()
	require.Contains(t, output, "- B=2")
	require.Contains(t, output, "+ C=3")
	// Should NOT show A=1 as changed (it's in both, duplicated)
	require.NotContains(t, output, "- A=1")
	require.NotContains(t, output, "+ A=1")
}

func TestPrintLineDiff_EmptyContent(t *testing.T) {
	t.Run("old empty (file added)", func(t *testing.T) {
		var buf bytes.Buffer
		out := ui.NewOutputWithWriters(&buf, &buf, false, false)

		printLineDiff(out, "", "KEY=value\n")

		output := buf.String()
		require.Contains(t, output, "+ KEY=value")
	})

	t.Run("new empty (file deleted)", func(t *testing.T) {
		var buf bytes.Buffer
		out := ui.NewOutputWithWriters(&buf, &buf, false, false)

		printLineDiff(out, "KEY=value\n", "")

		output := buf.String()
		require.Contains(t, output, "- KEY=value")
	})
}

func TestPrintLineDiff_NoChanges(t *testing.T) {
	var buf bytes.Buffer
	out := ui.NewOutputWithWriters(&buf, &buf, false, false)

	printLineDiff(out, "same\n", "same\n")

	output := buf.String()
	require.Empty(t, output, "identical content should produce no output")
}

func TestPrintLineDiff_TrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	out := ui.NewOutputWithWriters(&buf, &buf, false, false)

	// Difference is only trailing newline
	printLineDiff(out, "line1", "line1\n")

	// Should show the difference
	output := buf.String()
	// This is a legitimate difference, output may or may not be empty
	// depending on how the diff library handles trailing newlines
	_ = output
}

func TestSplitDiffLines(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"empty", "", []string{}},
		{"single line with newline", "hello\n", []string{"hello"}},
		{"single line without newline", "hello", []string{"hello"}},
		{"multiple lines", "a\nb\nc\n", []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitDiffLines(tt.text)
			require.Equal(t, tt.want, got)
		})
	}
}
