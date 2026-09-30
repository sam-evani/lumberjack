package worktree

import (
	"testing"
	"time"
)

func TestCategorize(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-24 * time.Hour)
	old := now.Add(-45 * 24 * time.Hour)

	tests := []struct {
		name  string
		entry Entry
		want  Category
	}{
		{"merged", Entry{Status: StatusMerged, LastCommitTime: old}, CategoryMerged},
		{"merged dirty", Entry{Status: StatusMerged, Dirty: true}, CategoryMerged},
		{"recent unmerged", Entry{Status: StatusUnmerged, LastCommitTime: recent}, CategoryActive},
		{"stale unmerged", Entry{Status: StatusUnmerged, LastCommitTime: old}, CategoryReview},
		{"protected base", Entry{Status: StatusBase, Protected: true, LastCommitTime: old}, CategoryProtected},
		{"error wins over protected", Entry{Status: StatusUnknown, Protected: true, Error: "boom"}, CategoryReview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Categorize(tt.entry, now, DefaultStaleAfter); got != tt.want {
				t.Fatalf("Categorize = %q, want %q", got, tt.want)
			}
		})
	}
}
