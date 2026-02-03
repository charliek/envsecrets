package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charliek/envsecrets/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestParseEnvSecretsFile(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantRepo     string
		wantFiles    []string
		wantErr      bool
		wantErrMatch string
	}{
		{
			name:      "simple file list",
			content:   ".env\n.env.local\n",
			wantFiles: []string{".env", ".env.local"},
		},
		{
			name:      "with comments and blank lines",
			content:   "# This is a comment\n.env\n\n.env.local\n",
			wantFiles: []string{".env", ".env.local"},
		},
		{
			name:      "with repo directive",
			content:   "repo: custom/project\n.env\n.env.local\n",
			wantRepo:  "custom/project",
			wantFiles: []string{".env", ".env.local"},
		},
		{
			name:      "repo directive with extra whitespace",
			content:   "repo:   custom/project  \n.env\n",
			wantRepo:  "custom/project",
			wantFiles: []string{".env"},
		},
		{
			name:      "repo directive only",
			content:   "repo: owner/name\n",
			wantRepo:  "owner/name",
			wantFiles: nil,
		},
		{
			name:         "invalid repo directive",
			content:      "repo: invalid\n.env\n",
			wantErr:      true,
			wantErrMatch: "invalid repo directive",
		},
		{
			name:         "path traversal attack",
			content:      "../../../etc/passwd\n",
			wantErr:      true,
			wantErrMatch: "path traversal",
		},
		{
			name:         "absolute path",
			content:      "/etc/passwd\n",
			wantErr:      true,
			wantErrMatch: "absolute path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, ".envsecrets")
			err := os.WriteFile(path, []byte(tt.content), 0644)
			require.NoError(t, err)

			config, err := ParseEnvSecretsFile(path)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrMatch != "" {
					require.Contains(t, err.Error(), tt.wantErrMatch)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantRepo, config.RepoOverride)
			require.Equal(t, tt.wantFiles, config.Files)
		})
	}
}

func TestParseEnvSecretsFile_NotFound(t *testing.T) {
	_, err := ParseEnvSecretsFile("/nonexistent/path/.envsecrets")
	require.Error(t, err)
}

func TestParseGitignoreMarker(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantFiles []string
	}{
		{
			name:      "marker with files",
			content:   "node_modules\n# envsecrets\n.env\n.env.local\n\nother_stuff\n",
			wantFiles: []string{".env", ".env.local"},
		},
		{
			name:      "marker at end of file",
			content:   "node_modules\n# envsecrets\n.env\n.env.local",
			wantFiles: []string{".env", ".env.local"},
		},
		{
			name:      "section ends with comment",
			content:   "# envsecrets\n.env\n# Other section\nstuff\n",
			wantFiles: []string{".env"},
		},
		{
			name:      "no marker",
			content:   "node_modules\n.env\n.env.local\n",
			wantFiles: nil,
		},
		{
			name:      "empty file",
			content:   "",
			wantFiles: nil,
		},
		{
			name:      "marker only, no files",
			content:   "# envsecrets\n\n",
			wantFiles: nil,
		},
		{
			name:      "skips invalid paths",
			content:   "# envsecrets\n.env\n../escape\n.env.local\n",
			wantFiles: []string{".env", ".env.local"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, ".gitignore")
			err := os.WriteFile(path, []byte(tt.content), 0644)
			require.NoError(t, err)

			files, err := ParseGitignoreMarker(path)
			require.NoError(t, err)
			require.Equal(t, tt.wantFiles, files)
		})
	}
}

func TestParseGitignoreMarker_NotFound(t *testing.T) {
	files, err := ParseGitignoreMarker("/nonexistent/path/.gitignore")
	require.NoError(t, err)
	require.Nil(t, files)
}

func TestWriteEnvSecretsFileWithConfig(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".envsecrets")

	tests := []struct {
		name     string
		repo     string
		files    []string
		expected string
	}{
		{
			name:     "files only",
			files:    []string{".env", ".env.local"},
			expected: ".env\n.env.local\n",
		},
		{
			name:     "with repo directive",
			repo:     "custom/project",
			files:    []string{".env"},
			expected: "repo: custom/project\n.env\n",
		},
		{
			name:     "repo only",
			repo:     "owner/name",
			expected: "repo: owner/name\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &domain.EnvSecretsConfig{
				RepoOverride: tt.repo,
				Files:        tt.files,
			}
			err := WriteEnvSecretsFileWithConfig(path, config)
			require.NoError(t, err)

			content, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, tt.expected, string(content))
		})
	}
}
