package project

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRepoString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantName  string
		wantErr   bool
	}{
		{
			name:      "valid owner/name",
			input:     "acme/myapp",
			wantOwner: "acme",
			wantName:  "myapp",
		},
		{
			name:      "with whitespace",
			input:     "  acme/myapp  ",
			wantOwner: "acme",
			wantName:  "myapp",
		},
		{
			name:      "with nested path",
			input:     "acme/myapp/subdir",
			wantOwner: "acme",
			wantName:  "myapp/subdir",
		},
		{
			name:    "missing name",
			input:   "acme/",
			wantErr: true,
		},
		{
			name:    "missing owner",
			input:   "/myapp",
			wantErr: true,
		},
		{
			name:    "no slash",
			input:   "acme",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "only slash",
			input:   "/",
			wantErr: true,
		},
		{
			name:    "invalid char in owner - dollar sign",
			input:   "owner$/repo",
			wantErr: true,
		},
		{
			name:    "invalid char in owner - at sign",
			input:   "owner@name/repo",
			wantErr: true,
		},
		{
			name:    "invalid char in name - dollar sign",
			input:   "owner/repo$name",
			wantErr: true,
		},
		{
			name:    "invalid char in name - space",
			input:   "owner/repo name",
			wantErr: true,
		},
		{
			name:      "valid chars - dots hyphens underscores",
			input:     "my-org_name.co/my-repo_v2.0",
			wantOwner: "my-org_name.co",
			wantName:  "my-repo_v2.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseRepoString(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantOwner, info.Owner)
			require.Equal(t, tt.wantName, info.Name)
		})
	}
}

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantName  string
		wantErr   bool
	}{
		{
			name:      "SSH GitHub",
			url:       "git@github.com:acme/myapp.git",
			wantOwner: "acme",
			wantName:  "myapp",
		},
		{
			name:      "SSH GitHub without .git",
			url:       "git@github.com:acme/myapp",
			wantOwner: "acme",
			wantName:  "myapp",
		},
		{
			name:      "HTTPS GitHub",
			url:       "https://github.com/acme/myapp.git",
			wantOwner: "acme",
			wantName:  "myapp",
		},
		{
			name:      "HTTPS GitHub without .git",
			url:       "https://github.com/acme/myapp",
			wantOwner: "acme",
			wantName:  "myapp",
		},
		{
			name:      "SSH GitLab",
			url:       "git@gitlab.com:team/project.git",
			wantOwner: "team",
			wantName:  "project",
		},
		{
			name:      "HTTPS GitLab",
			url:       "https://gitlab.com/team/project.git",
			wantOwner: "team",
			wantName:  "project",
		},
		{
			name:      "SSH Bitbucket",
			url:       "git@bitbucket.org:company/repo.git",
			wantOwner: "company",
			wantName:  "repo",
		},
		{
			name:      "HTTP (insecure)",
			url:       "http://github.com/acme/myapp.git",
			wantOwner: "acme",
			wantName:  "myapp",
		},
		{
			name:    "Invalid URL",
			url:     "not-a-valid-url",
			wantErr: true,
		},
		{
			name:    "URL without repo",
			url:     "https://github.com/acme",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseRemoteURL(tt.url)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantOwner, info.Owner)
			require.Equal(t, tt.wantName, info.Name)
			require.Equal(t, tt.url, info.RemoteURL)
		})
	}
}
