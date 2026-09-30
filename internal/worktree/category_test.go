package worktree

import (
	"testing"
	"time"
)

func TestCategorizeAndAutoPrune(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-24 * time.Hour)
	old := now.Add(-45 * 24 * time.Hour)

	tests := []struct {
		name  string
		entry Entry
		want  Category
		auto  bool
	}{
		{"merged", Entry{Status: StatusMerged, LastCommitTime: old}, CategoryMerged, true},
		{"merged dirty", Entry{Status: StatusMerged, Dirty: true}, CategoryMerged, true},
		{"recent unmerged", Entry{Status: StatusUnmerged, LastCommitTime: recent}, CategoryActive, false},
		{"stale unmerged", Entry{Status: StatusUnmerged, LastCommitTime: old}, CategoryReview, false},
		{"protected base", Entry{Status: StatusBase, Protected: true, LastCommitTime: old}, CategoryProtected, false},
		{"error wins over protected", Entry{Status: StatusUnknown, Protected: true, Error: "boom"}, CategoryReview, false},
		{"detached idle", Entry{Status: StatusDetached, LastCommitTime: old, LastActivity: old}, CategoryDetached, true},
		{"detached old commit but recent checkout", Entry{Status: StatusDetached, LastCommitTime: old, LastActivity: recent}, CategoryDetached, false},
		{"detached unknown activity", Entry{Status: StatusDetached}, CategoryDetached, false},
		{"detached with scan error", Entry{Status: StatusDetached, LastCommitTime: old, Error: "boom"}, CategoryReview, false},
	}
	p := DefaultPolicy()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.Categorize(tt.entry, now); got != tt.want {
				t.Fatalf("Categorize = %q, want %q", got, tt.want)
			}
			if got := p.AutoPrune(tt.entry, now); got != tt.auto {
				t.Fatalf("AutoPrune = %v, want %v", got, tt.auto)
			}
		})
	}
}
