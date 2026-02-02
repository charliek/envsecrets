package project

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
