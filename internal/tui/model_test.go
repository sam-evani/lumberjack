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

func TestViewUsesCheckboxesForSelection(t *testing.T) {
	model := NewModel([]worktree.Entry{row("repo", "/repo/.worktrees/feature", worktree.StatusMerged)})

	if view := model.View(); !strings.Contains(view, "[ ]") {
		t.Fatalf("View() missing unchecked box:\n%s", view)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)

	if view := model.View(); !strings.Contains(view, "[x]") {
		t.Fatalf("View() missing checked box:\n%s", view)
	}
}

func TestViewShowsProtectedReason(t *testing.T) {
	rows := []worktree.Entry{
		rowWith("repo", "/repo", worktree.StatusUnknown, func(entry *worktree.Entry) {
			entry.RepoPath = "/repo"
			entry.Protected = true
		}),
		rowWith("repo", "/repo/.worktrees/error", worktree.StatusUnknown, func(entry *worktree.Entry) {
			entry.Protected = true
			entry.Error = "scan failed"
		}),
		rowWith("repo", "/repo/.worktrees/no-base", worktree.StatusUnknown, func(entry *worktree.Entry) {
			entry.Protected = true
		}),
		rowWith("repo", "/repo/.worktrees/main", worktree.StatusBase, func(entry *worktree.Entry) {
			entry.Branch = "main"
			entry.Protected = true
		}),
	}
	model := NewModel(rows)

	view := model.View()
	for _, want := range []string{
		"protected: primary checkout",
		"protected: scan error",
		"protected: no base ref",
		"protected: main/master",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q:\n%s", want, view)
		}
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

func TestFilterModeTreatsPrintableQuitAndCancelKeysAsInput(t *testing.T) {
	model := NewModel([]worktree.Entry{row("repo", "/repo/.worktrees/feature", worktree.StatusMerged)})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("Update(q) returned a command, want nil")
	}
	if got := model.filter; got != "nq" {
		t.Fatalf("filter = %q, want nq", got)
	}
	if !model.filtering {
		t.Fatal("filtering = false, want true")
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

func TestConfirmationShowsProtectedSkipReason(t *testing.T) {
	rows := []worktree.Entry{
		rowWith("repo", "/repo", worktree.StatusUnknown, func(entry *worktree.Entry) {
			entry.RepoPath = "/repo"
			entry.Protected = true
		}),
	}
	model := NewModel(rows)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)

	view := model.View()
	if !strings.Contains(view, "skip /repo: protected: primary checkout") {
		t.Fatalf("View() missing protected reason:\n%s", view)
	}
}

func TestNormalConfirmationCanSwitchToForceDelete(t *testing.T) {
	rows := []worktree.Entry{
		rowWith("repo", "/repo/.worktrees/dirty", worktree.StatusMerged, func(entry *worktree.Entry) {
			entry.Dirty = true
		}),
	}
	model := NewModel(rows)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)

	normalView := model.View()
	if !strings.Contains(normalView, "skip /repo/.worktrees/dirty: dirty; press D to force delete") {
		t.Fatalf("normal confirmation missing dirty force prompt:\n%s", normalView)
	}
	if !strings.Contains(normalView, "D force delete selected") {
		t.Fatalf("normal confirmation missing force footer:\n%s", normalView)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("D")})
	model = updated.(Model)

	forceView := model.View()
	if !strings.Contains(forceView, "Confirm force delete") {
		t.Fatalf("force confirmation missing title:\n%s", forceView)
	}
	if !strings.Contains(forceView, "remove /repo/.worktrees/dirty") {
		t.Fatalf("force confirmation missing dirty removal:\n%s", forceView)
	}
}

func TestConfirmationFooterUsesDeletionWording(t *testing.T) {
	model := NewModel([]worktree.Entry{row("repo", "/repo/.worktrees/feature", worktree.StatusMerged)})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)

	view := model.View()
	if strings.Contains(view, "preview only") {
		t.Fatalf("View() contains preview-only wording:\n%s", view)
	}
	if !strings.Contains(view, "enter/y delete") {
		t.Fatalf("View() missing deletion confirmation wording:\n%s", view)
	}
}

func TestConfirmationForceFooterUsesDeletionWording(t *testing.T) {
	model := NewModel([]worktree.Entry{row("repo", "/repo/.worktrees/feature", worktree.StatusMerged)})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("D")})
	model = updated.(Model)

	view := model.View()
	if !strings.Contains(view, "Confirm force delete") {
		t.Fatalf("View() missing force confirmation title:\n%s", view)
	}
	if strings.Contains(view, "preview only") {
		t.Fatalf("View() contains preview-only wording:\n%s", view)
	}
	if !strings.Contains(view, "enter/y delete") {
		t.Fatalf("View() missing deletion confirmation wording:\n%s", view)
	}
}

func TestConfirmDeletionShowsDeletingMessageBeforeCommandRuns(t *testing.T) {
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
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if cmd == nil {
		t.Fatal("confirm returned nil command, want delete command")
	}
	if len(deleter.removed) != 0 {
		t.Fatalf("removed before running command = %#v, want none", deleter.removed)
	}
	if got := model.Message(); got != "deleting 1 worktree(s), 1 branch(es)..." {
		t.Fatalf("Message() = %q, want deleting message", got)
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
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
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
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
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
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
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

func TestRefreshReplacesRowsAndClearsSelection(t *testing.T) {
	oldRow := row("repo", "/repo/.worktrees/old", worktree.StatusMerged)
	newRow := row("repo", "/repo/.worktrees/new", worktree.StatusMerged)
	model := NewModelWithOptions([]worktree.Entry{oldRow}, noopDeleter{}, func() ([]worktree.Entry, error) {
		return []worktree.Entry{newRow}, nil
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = updated.(Model)

	if !reflect.DeepEqual(model.rows, []worktree.Entry{newRow}) {
		t.Fatalf("rows = %#v, want new row", model.rows)
	}
	if len(model.visible) != 1 || model.visible[0] != 0 {
		t.Fatalf("visible = %#v, want [0]", model.visible)
	}
	if got := model.SelectedPaths(); len(got) != 0 {
		t.Fatalf("SelectedPaths() = %#v, want empty", got)
	}
	if got := model.Message(); got != "refreshed 1 worktree(s)" {
		t.Fatalf("Message() = %q, want refreshed message", got)
	}
}

func TestRefreshUnavailableShowsMessage(t *testing.T) {
	model := NewModel([]worktree.Entry{row("repo", "/repo/.worktrees/feature", worktree.StatusMerged)})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = updated.(Model)

	if got := model.Message(); got != "refresh is not available" {
		t.Fatalf("Message() = %q, want unavailable message", got)
	}
}

func TestRefreshFailureKeepsRowsAndSelection(t *testing.T) {
	oldRow := row("repo", "/repo/.worktrees/old", worktree.StatusMerged)
	model := NewModelWithOptions([]worktree.Entry{oldRow}, noopDeleter{}, func() ([]worktree.Entry, error) {
		return nil, errors.New("scan failed")
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = updated.(Model)

	if !reflect.DeepEqual(model.rows, []worktree.Entry{oldRow}) {
		t.Fatalf("rows = %#v, want old row", model.rows)
	}
	wantSelection := []string{"/repo/.worktrees/old"}
	if got := model.SelectedPaths(); !reflect.DeepEqual(got, wantSelection) {
		t.Fatalf("SelectedPaths() = %#v, want %#v", got, wantSelection)
	}
	if got := model.Message(); !strings.HasPrefix(got, "refresh failed:") {
		t.Fatalf("Message() = %q, want refresh failure prefix", got)
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
