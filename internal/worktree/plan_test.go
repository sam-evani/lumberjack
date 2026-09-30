package worktree

import (
	"reflect"
	"testing"
)

func TestPlanDeletionNormalRemovesSelectedCleanWorktrees(t *testing.T) {
	entries := []Entry{
		{
			RepoPath: "/repo",
			Path:     "/repo/.worktrees/merged",
			Branch:   "sam-evani/merged",
			Status:   StatusMerged,
		},
		{
			RepoPath: "/repo",
			Path:     "/repo/.worktrees/unmerged",
			Branch:   "sam-evani/unmerged",
			Status:   StatusUnmerged,
		},
	}

	got := PlanDeletion(entries, selected(entries...), DeleteNormal)

	wantRemove := []RemoveWorktreeAction{
		{RepoPath: "/repo", Path: "/repo/.worktrees/merged"},
		{RepoPath: "/repo", Path: "/repo/.worktrees/unmerged"},
	}
	if !reflect.DeepEqual(got.RemoveWorktrees, wantRemove) {
		t.Fatalf("RemoveWorktrees = %#v, want %#v", got.RemoveWorktrees, wantRemove)
	}
	wantDelete := []BranchAction{{RepoPath: "/repo", Branch: "sam-evani/merged"}}
	if !reflect.DeepEqual(got.DeleteBranches, wantDelete) {
		t.Fatalf("DeleteBranches = %#v, want %#v", got.DeleteBranches, wantDelete)
	}
	wantKeep := []BranchAction{{RepoPath: "/repo", Branch: "sam-evani/unmerged"}}
	if !reflect.DeepEqual(got.KeepBranches, wantKeep) {
		t.Fatalf("KeepBranches = %#v, want %#v", got.KeepBranches, wantKeep)
	}
	if len(got.Skipped) != 0 {
		t.Fatalf("Skipped = %#v, want none", got.Skipped)
	}
}

func TestPlanDeletionNormalSkipsDirtyAndProtectedSelectedRows(t *testing.T) {
	entries := []Entry{
		{
			RepoPath: "/repo",
			Path:     "/repo/.worktrees/dirty",
			Branch:   "sam-evani/dirty",
			Status:   StatusMerged,
			Dirty:    true,
		},
		{
			RepoPath:  "/repo",
			Path:      "/repo/.worktrees/main",
			Branch:    "main",
			Status:    StatusBase,
			Protected: true,
		},
	}

	got := PlanDeletion(entries, selected(entries...), DeleteNormal)

	if len(got.RemoveWorktrees) != 0 {
		t.Fatalf("RemoveWorktrees = %#v, want none", got.RemoveWorktrees)
	}
	wantSkipped := []SkippedAction{
		{Entry: entries[0], Reason: "dirty"},
		{Entry: entries[1], Reason: "protected: main/master"},
	}
	if !reflect.DeepEqual(got.Skipped, wantSkipped) {
		t.Fatalf("Skipped = %#v, want %#v", got.Skipped, wantSkipped)
	}
}

func TestPlanDeletionForceAllowsDirtyWorktreeRemovalAndKeepsUnmergedBranch(t *testing.T) {
	entries := []Entry{
		{
			RepoPath: "/repo",
			Path:     "/repo/.worktrees/dirty-unmerged",
			Branch:   "sam-evani/dirty-unmerged",
			Status:   StatusUnmerged,
			Dirty:    true,
		},
	}

	got := PlanDeletion(entries, selected(entries...), DeleteForce)

	wantRemove := []RemoveWorktreeAction{
		{RepoPath: "/repo", Path: "/repo/.worktrees/dirty-unmerged", Force: true},
	}
	if !reflect.DeepEqual(got.RemoveWorktrees, wantRemove) {
		t.Fatalf("RemoveWorktrees = %#v, want %#v", got.RemoveWorktrees, wantRemove)
	}
	if len(got.DeleteBranches) != 0 {
		t.Fatalf("DeleteBranches = %#v, want none", got.DeleteBranches)
	}
	wantKeep := []BranchAction{{RepoPath: "/repo", Branch: "sam-evani/dirty-unmerged"}}
	if !reflect.DeepEqual(got.KeepBranches, wantKeep) {
		t.Fatalf("KeepBranches = %#v, want %#v", got.KeepBranches, wantKeep)
	}
}

func TestPlanDeletionIgnoresUnselectedRows(t *testing.T) {
	entries := []Entry{
		{
			RepoPath: "/repo",
			Path:     "/repo/.worktrees/selected",
			Branch:   "sam-evani/selected",
			Status:   StatusMerged,
		},
		{
			RepoPath: "/repo",
			Path:     "/repo/.worktrees/unselected",
			Branch:   "sam-evani/unselected",
			Status:   StatusMerged,
		},
	}

	got := PlanDeletion(entries, map[string]bool{entries[0].Path: true}, DeleteNormal)

	wantRemove := []RemoveWorktreeAction{
		{RepoPath: "/repo", Path: "/repo/.worktrees/selected"},
	}
	if !reflect.DeepEqual(got.RemoveWorktrees, wantRemove) {
		t.Fatalf("RemoveWorktrees = %#v, want %#v", got.RemoveWorktrees, wantRemove)
	}
	wantDelete := []BranchAction{{RepoPath: "/repo", Branch: "sam-evani/selected"}}
	if !reflect.DeepEqual(got.DeleteBranches, wantDelete) {
		t.Fatalf("DeleteBranches = %#v, want %#v", got.DeleteBranches, wantDelete)
	}
}

func TestPlanDeletionProtectedLockedAndBareRowsAreSkippedAsProtected(t *testing.T) {
	entries := []Entry{
		{
			RepoPath:  "/repo",
			Path:      "/repo/.worktrees/locked",
			Branch:    "sam-evani/locked",
			Status:    StatusUnknown,
			Locked:    true,
			Protected: true,
		},
		{
			RepoPath:  "/repo",
			Path:      "/repo/.worktrees/bare.git",
			Branch:    "main",
			Status:    StatusUnknown,
			Bare:      true,
			Protected: true,
		},
	}

	got := PlanDeletion(entries, selected(entries...), DeleteForce)

	wantSkipped := []SkippedAction{
		{Entry: entries[0], Reason: "protected: locked"},
		{Entry: entries[1], Reason: "protected: bare"},
	}
	if !reflect.DeepEqual(got.Skipped, wantSkipped) {
		t.Fatalf("Skipped = %#v, want %#v", got.Skipped, wantSkipped)
	}
}

func selected(entries ...Entry) map[string]bool {
	selected := make(map[string]bool, len(entries))
	for _, entry := range entries {
		selected[entry.Path] = true
	}
	return selected
}
