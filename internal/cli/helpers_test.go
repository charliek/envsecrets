package cli

import (
	"testing"

	"github.com/charliek/envsecrets/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestCountFileStatuses(t *testing.T) {
	tests := []struct {
		name     string
		statuses []domain.FileStatus
		want     statusCounts
	}{
		{
			name:     "empty",
			statuses: nil,
			want:     statusCounts{},
		},
		{
			name: "all unchanged",
			statuses: []domain.FileStatus{
				{Path: ".env", LocalExists: true, CacheExists: true, Modified: false},
				{Path: ".env.prod", LocalExists: true, CacheExists: true, Modified: false},
			},
			want: statusCounts{Unchanged: 2},
		},
		{
			name: "mixed states",
			statuses: []domain.FileStatus{
				{Path: "a", LocalExists: true, CacheExists: true, Modified: false},   // unchanged
				{Path: "b", LocalExists: true, CacheExists: true, Modified: true},    // modified
				{Path: "c", LocalExists: true, CacheExists: false, Modified: true},   // added
				{Path: "d", LocalExists: false, CacheExists: true, Modified: true},   // deleted
				{Path: "e", LocalExists: false, CacheExists: false, Modified: false}, // not synced
			},
			want: statusCounts{
				Unchanged: 1,
				Modified:  1,
				Added:     1,
				Deleted:   1,
				NotSynced: 1,
			},
		},
		{
			name: "all not synced",
			statuses: []domain.FileStatus{
				{Path: ".env", LocalExists: false, CacheExists: false},
				{Path: ".env.prod", LocalExists: false, CacheExists: false},
			},
			want: statusCounts{NotSynced: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countFileStatuses(tt.statuses)
			require.Equal(t, tt.want, got)
		})
	}
}
