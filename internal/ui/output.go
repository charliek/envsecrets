package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/domain"
)

// Output handles formatted output to the terminal
type Output struct {
	out     io.Writer
	err     io.Writer
	verbose bool
	json    bool
}

// NewOutput creates a new output handler
func NewOutput(verbose, jsonOutput bool) *Output {
	return &Output{
		out:     os.Stdout,
		err:     os.Stderr,
		verbose: verbose,
		json:    jsonOutput,
	}
}

// NewOutputWithWriters creates an output handler with custom writers (for testing)
func NewOutputWithWriters(out, err io.Writer, verbose, jsonOutput bool) *Output {
	return &Output{
		out:     out,
		err:     err,
		verbose: verbose,
		json:    jsonOutput,
	}
}

// Println prints a message to stdout with a newline
func (o *Output) Println(args ...interface{}) {
	fmt.Fprintln(o.out, args...)
}

// Printf prints a formatted message to stdout
func (o *Output) Printf(format string, args ...interface{}) {
	fmt.Fprintf(o.out, format, args...)
}

// Error prints an error message to stderr
func (o *Output) Error(format string, args ...interface{}) {
	fmt.Fprintf(o.err, "Error: "+format+"\n", args...)
}

// Warn prints a warning message to stderr
func (o *Output) Warn(format string, args ...interface{}) {
	fmt.Fprintf(o.err, "Warning: "+format+"\n", args...)
}

// Verbose prints a message only if verbose mode is enabled
func (o *Output) Verbose(format string, args ...interface{}) {
	if o.verbose {
		fmt.Fprintf(o.err, format+"\n", args...)
	}
}

// PrintDryRunHeader prints the standard dry-run header message
// Skips output in JSON mode to avoid polluting JSON output
func (o *Output) PrintDryRunHeader() {
	if o.json {
		return
	}
	o.Println("Dry run - no changes will be made")
	o.Println()
}

// Success prints a success message
func (o *Output) Success(format string, args ...interface{}) {
	fmt.Fprintf(o.out, format+"\n", args...)
}

// JSON outputs data as JSON
func (o *Output) JSON(data interface{}) error {
	enc := json.NewEncoder(o.out)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// IsJSON returns true if JSON output mode is enabled
func (o *Output) IsJSON() bool {
	return o.json
}

// Status prints a status line
func (o *Output) Status(label, value string) {
	fmt.Fprintf(o.out, "%-20s %s\n", label+":", value)
}

// Table prints a simple table
func (o *Output) Table(headers []string, rows [][]string) {
	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print headers
	for i, h := range headers {
		fmt.Fprintf(o.out, "%-*s  ", widths[i], h)
	}
	fmt.Fprintln(o.out)

	// Print separator
	for i, w := range widths {
		fmt.Fprint(o.out, strings.Repeat("-", w))
		if i < len(widths)-1 {
			fmt.Fprint(o.out, "  ")
		}
	}
	fmt.Fprintln(o.out)

	// Print rows
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				fmt.Fprintf(o.out, "%-*s  ", widths[i], cell)
			}
		}
		fmt.Fprintln(o.out)
	}
}

// PrintCommit prints a commit in a formatted way
func (o *Output) PrintCommit(c domain.Commit, verbose bool) {
	fmt.Fprintf(o.out, "%s %s\n", c.ShortHash, firstLine(c.Message))
	if verbose {
		fmt.Fprintf(o.out, "    Author: %s\n", c.Author)
		fmt.Fprintf(o.out, "    Date:   %s\n", c.Date.Format(time.RFC3339))
		if len(c.Files) > 0 {
			fmt.Fprintf(o.out, "    Files:\n")
			for _, f := range c.Files {
				fmt.Fprintf(o.out, "      - %s\n", f)
			}
		}
		fmt.Fprintln(o.out)
	}
}

// firstLine returns the first line of a string
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// TruncateHash truncates a hash to the standard short length for display
func TruncateHash(hash string) string {
	if len(hash) > constants.ShortHashLength {
		return hash[:constants.ShortHashLength]
	}
	return hash
}

// PrintFileStatus prints file status with short indicator
func (o *Output) PrintFileStatus(status domain.FileStatus) {
	var indicator string
	switch {
	case !status.LocalExists && status.CacheExists:
		indicator = "D"
	case status.LocalExists && !status.CacheExists:
		indicator = "A"
	case status.Modified:
		indicator = "M"
	default:
		indicator = " "
	}
	fmt.Fprintf(o.out, " %s %s\n", indicator, status.Path)
}

// PrintFileStatusDetailed prints file status with detailed description
func (o *Output) PrintFileStatusDetailed(status domain.FileStatus) {
	var indicator string
	switch {
	case !status.LocalExists && status.CacheExists:
		indicator = "(missing locally)"
	case status.LocalExists && !status.CacheExists:
		indicator = "(new)"
	case status.Modified:
		indicator = "(modified)"
	default:
		indicator = "(unchanged)"
	}
	fmt.Fprintf(o.out, "  %-30s %s\n", status.Path, indicator)
}
