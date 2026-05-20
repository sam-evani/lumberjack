package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sam-evani/lumberjack/internal/worktree"
)

func TestSelectionStartsEmptyAndSpaceSelectsCurrentRow(t *testing.T) {
	model := NewModel([]worktree.Entry{row("repo", "/repo/.worktrees/feature", worktree.StatusMerged)})

	if got := model.SelectedPaths(); len(got) != 0 {
		t.Fatalf("SelectedPaths() = %#v, want empty", got)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)

	want := []string{"/repo/.worktrees/feature"}
	if got := model.SelectedPaths(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SelectedPaths() = %#v, want %#v", got, want)
	}
}

func TestDeleteWithNoSelectionShowsMessageAndDoesNotConfirm(t *testing.T) {
	model := NewModel([]worktree.Entry{row("repo", "/repo/.worktrees/feature", worktree.StatusMerged)})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)

	if got := model.Message(); got != "select one or more worktrees first" {
		t.Fatalf("Message() = %q, want no-selection message", got)
	}
	if model.Confirming() {
		t.Fatal("Confirming() = true, want false")
	}
}

func TestSelectAllVisibleSkipsUnsafeRows(t *testing.T) {
	rows := []worktree.Entry{
		row("repo", "/repo/.worktrees/clean", worktree.StatusMerged),
		rowWith("repo", "/repo/.worktrees/dirty", worktree.StatusMerged, func(entry *worktree.Entry) {
			entry.Dirty = true
		}),
		rowWith("repo", "/repo/.worktrees/protected", worktree.StatusMerged, func(entry *worktree.Entry) {
			entry.Protected = true
		}),
		row("repo", "/repo/.worktrees/unknown", worktree.StatusUnknown),
		row("repo", "/repo/.worktrees/detached", worktree.StatusDetached),
		row("repo", "/repo/.worktrees/prunable", worktree.StatusPrunable),
	}
	model := NewModel(rows)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(Model)

	want := []string{"/repo/.worktrees/clean"}
	if got := model.SelectedPaths(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SelectedPaths() = %#v, want %#v", got, want)
	}
}

func TestForceDeleteWithNoSelectionShowsMessageAndDoesNotConfirm(t *testing.T) {
	model := NewModel([]worktree.Entry{row("repo", "/repo/.worktrees/feature", worktree.StatusMerged)})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("D")})
	model = updated.(Model)

	if got := model.Message(); got != "select one or more worktrees first" {
		t.Fatalf("Message() = %q, want no-selection message", got)
	}
	if model.Confirming() {
		t.Fatal("Confirming() = true, want false")
	}
}

func TestConfirmationDisclosesPendingBranchActions(t *testing.T) {
	rows := []worktree.Entry{
		rowWith("repo", "/repo/.worktrees/merged", worktree.StatusMerged, func(entry *worktree.Entry) {
			entry.Branch = "sam-evani/merged"
		}),
		rowWith("repo", "/repo/.worktrees/unmerged", worktree.StatusUnmerged, func(entry *worktree.Entry) {
			entry.Branch = "sam-evani/unmerged"
		}),
	}
	model := NewModel(rows)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)

	view := model.View()
	if !strings.Contains(view, "delete branch sam-evani/merged") {
		t.Fatalf("View() missing delete branch disclosure:\n%s", view)
	}
	if !strings.Contains(view, "keep branch sam-evani/unmerged") {
		t.Fatalf("View() missing keep branch disclosure:\n%s", view)
	}
}

func TestConfirmDeletionRemovesMergedWorktreeAndBranch(t *testing.T) {
	rows := []worktree.Entry{
		rowWith("repo", "/repo/.worktrees/merged", worktree.StatusMerged, func(entry *worktree.Entry) {
			entry.Branch = "sam-evani/merged"
		}),
	}
	deleter := &fakeDeleter{}
	model := NewModelWithDeleter(rows, deleter)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	wantRemove := []worktree.RemoveWorktreeAction{{RepoPath: "/repo", Path: "/repo/.worktrees/merged"}}
	if !reflect.DeepEqual(deleter.removed, wantRemove) {
		t.Fatalf("removed = %#v, want %#v", deleter.removed, wantRemove)
	}
	wantBranches := []worktree.BranchAction{{RepoPath: "/repo", Branch: "sam-evani/merged"}}
	if !reflect.DeepEqual(deleter.branches, wantBranches) {
		t.Fatalf("branches = %#v, want %#v", deleter.branches, wantBranches)
	}
	if len(model.rows) != 0 {
		t.Fatalf("rows = %#v, want none", model.rows)
	}
	if got := model.Message(); got != "deleted 1 worktree(s), 1 branch(es)" {
		t.Fatalf("Message() = %q, want success message", got)
	}
}

func TestConfirmDeletionFailureLeavesRowAndDoesNotDeleteBranch(t *testing.T) {
	rows := []worktree.Entry{
		rowWith("repo", "/repo/.worktrees/merged", worktree.StatusMerged, func(entry *worktree.Entry) {
			entry.Branch = "sam-evani/merged"
		}),
	}
	deleter := &fakeDeleter{removeErr: errors.New("remove failed")}
	model := NewModelWithDeleter(rows, deleter)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if len(model.rows) != 1 {
		t.Fatalf("rows = %#v, want original row", model.rows)
	}
	if len(deleter.branches) != 0 {
		t.Fatalf("branches = %#v, want none", deleter.branches)
	}
	if got := model.Message(); !strings.HasPrefix(got, "errors:") {
		t.Fatalf("Message() = %q, want errors prefix", got)
	}
}

func TestConfirmDeletionKeepsUnmergedBranch(t *testing.T) {
	rows := []worktree.Entry{
		rowWith("repo", "/repo/.worktrees/unmerged", worktree.StatusUnmerged, func(entry *worktree.Entry) {
			entry.Branch = "sam-evani/unmerged"
		}),
	}
	deleter := &fakeDeleter{}
	model := NewModelWithDeleter(rows, deleter)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	wantRemove := []worktree.RemoveWorktreeAction{{RepoPath: "/repo", Path: "/repo/.worktrees/unmerged"}}
	if !reflect.DeepEqual(deleter.removed, wantRemove) {
		t.Fatalf("removed = %#v, want %#v", deleter.removed, wantRemove)
	}
	if len(deleter.branches) != 0 {
		t.Fatalf("branches = %#v, want none", deleter.branches)
	}
	if len(model.rows) != 0 {
		t.Fatalf("rows = %#v, want none", model.rows)
	}
}

type fakeDeleter struct {
	removed   []worktree.RemoveWorktreeAction
	branches  []worktree.BranchAction
	removeErr error
	branchErr error
}

func (d *fakeDeleter) RemoveWorktree(action worktree.RemoveWorktreeAction) error {
	d.removed = append(d.removed, action)
	return d.removeErr
}

func (d *fakeDeleter) DeleteBranch(action worktree.BranchAction) error {
	d.branches = append(d.branches, action)
	return d.branchErr
}

func row(repoName string, path string, status worktree.MergeStatus) worktree.Entry {
	return rowWith(repoName, path, status, func(entry *worktree.Entry) {})
}

func rowWith(repoName string, path string, status worktree.MergeStatus, mutate func(*worktree.Entry)) worktree.Entry {
	entry := worktree.Entry{
		RepoName:        repoName,
		RepoPath:        "/repo",
		Path:            path,
		DisplayName:     path,
		Branch:          "feature",
		Status:          status,
		LastCommitTitle: "commit",
	}
	mutate(&entry)
	return entry
}
