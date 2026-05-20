package tui

import (
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
