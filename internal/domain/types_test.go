package domain

import "testing"

// TestCommit_AuthorDisplay covers the formatter that combines Name + email
// host so cross-machine attribution shows up in status / log output.
func TestCommit_AuthorDisplay(t *testing.T) {
	tests := []struct {
		name        string
		authorName  string
		authorEmail string
		want        string
	}{
		{"name plus machine_id email", "charliek", "charliek@alice-e2e", "charliek@alice-e2e"},
		{"name plus hostname email", "charliek", "charliek@laptop.local", "charliek@laptop.local"},
		{"missing email falls back to name", "envsecrets", "", "envsecrets"},
		{"malformed email (no @) falls back", "envsecrets", "bogus", "envsecrets"},
		{"email host equals name avoids duplication", "alice", "alice@alice", "alice"},
		{"empty name uses host only", "", "user@host", "host"},
		{"trailing @ is malformed", "x", "user@", "x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := Commit{Author: tc.authorName, AuthorEmail: tc.authorEmail}
			if got := c.AuthorDisplay(); got != tc.want {
				t.Errorf("AuthorDisplay() = %q, want %q", got, tc.want)
			}
		})
	}
}
