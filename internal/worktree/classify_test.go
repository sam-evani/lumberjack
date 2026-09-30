package worktree

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name           string
		entry          Entry
		mergedIntoBase bool
		hasBaseRef     bool
		wantStatus     MergeStatus
		wantProtected  bool
	}{
		{
			name: "primary repo checkout",
			entry: Entry{
				RepoPath: "/repo",
				Path:     "/repo",
				Branch:   "feature",
			},
			hasBaseRef:    true,
			wantStatus:    StatusUnknown,
			wantProtected: true,
		},
		{
			name: "main branch",
			entry: Entry{
				RepoPath: "/repo",
				Path:     "/repo/.worktrees/main",
				Branch:   "main",
			},
			hasBaseRef:    true,
			wantStatus:    StatusBase,
			wantProtected: true,
		},
		{
			name: "master branch",
			entry: Entry{
				RepoPath: "/repo",
				Path:     "/repo/.worktrees/master",
				Branch:   "master",
			},
			hasBaseRef:    true,
			wantStatus:    StatusBase,
			wantProtected: true,
		},
		{
			name: "detached",
			entry: Entry{
				RepoPath: "/repo",
				Path:     "/repo/.worktrees/detached",
				Detached: true,
				Branch:   "feature",
			},
			hasBaseRef:    true,
			wantStatus:    StatusDetached,
			wantProtected: false,
		},
		{
			name: "prunable",
			entry: Entry{
				RepoPath: "/repo",
				Path:     "/repo/.worktrees/missing",
				Branch:   "feature",
				Prunable: true,
			},
			hasBaseRef:    true,
			wantStatus:    StatusPrunable,
			wantProtected: true,
		},
		{
			name: "scan error",
			entry: Entry{
				RepoPath: "/repo",
				Path:     "/repo/.worktrees/error",
				Branch:   "feature",
				Error:    "failed to scan",
			},
			hasBaseRef:    true,
			wantStatus:    StatusUnknown,
			wantProtected: true,
		},
		{
			name: "locked",
			entry: Entry{
				RepoPath: "/repo",
				Path:     "/repo/.worktrees/locked",
				Branch:   "feature",
				Locked:   true,
			},
			hasBaseRef:    true,
			wantStatus:    StatusUnknown,
			wantProtected: true,
		},
		{
			name: "bare",
			entry: Entry{
				RepoPath: "/repo",
				Path:     "/repo/.worktrees/bare.git",
				Branch:   "main",
				Bare:     true,
			},
			hasBaseRef:    true,
			wantStatus:    StatusUnknown,
			wantProtected: true,
		},
		{
			name: "feature branch merged into base",
			entry: Entry{
				RepoPath: "/repo",
				Path:     "/repo/.worktrees/merged",
				Branch:   "feature",
			},
			mergedIntoBase: true,
			hasBaseRef:     true,
			wantStatus:     StatusMerged,
			wantProtected:  false,
		},
		{
			name: "feature branch not merged into base",
			entry: Entry{
				RepoPath: "/repo",
				Path:     "/repo/.worktrees/unmerged",
				Branch:   "feature",
			},
			mergedIntoBase: false,
			hasBaseRef:     true,
			wantStatus:     StatusUnmerged,
			wantProtected:  false,
		},
		{
			name: "no base refs",
			entry: Entry{
				RepoPath: "/repo",
				Path:     "/repo/.worktrees/unknown",
				Branch:   "feature",
			},
			hasBaseRef:    false,
			wantStatus:    StatusUnknown,
			wantProtected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.entry, tt.mergedIntoBase, tt.hasBaseRef)
			if got.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.Protected != tt.wantProtected {
				t.Fatalf("Protected = %v, want %v", got.Protected, tt.wantProtected)
			}
		})
	}
}

func TestClassifySetsDisplayNameFromPath(t *testing.T) {
	got := Classify(Entry{
		RepoPath: "/repo",
		Path:     "/repo/.worktrees/feature/",
		Branch:   "feature",
	}, false, true)

	if got.DisplayName != "feature" {
		t.Fatalf("DisplayName = %q, want %q", got.DisplayName, "feature")
	}
}
