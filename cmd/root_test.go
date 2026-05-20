package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/sam-evani/lumberjack/internal/worktree"
)

func executeRootForTest(args ...string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	originalRootPath := rootPath

	rootCmd.SetArgs(args)
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	defer func() {
		rootPath = originalRootPath
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	}()

	err := rootCmd.Execute()

	return stdout.String(), stderr.String(), err
}

func TestRootCommandIsSilentOnCommandErrors(t *testing.T) {
	_, stderr, err := executeRootForTest("unknown")
	if err == nil {
		t.Fatal("expected unknown command to return an error")
	}
	if stderr != "" {
		t.Fatalf("expected Cobra to leave command error printing to Execute, got stderr %q", stderr)
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	stdout, stderr, err := executeRootForTest("version", "extra")
	if err == nil {
		t.Fatal("expected version with extra arg to return an error")
	}
	if stdout != "" {
		t.Fatalf("expected no version output, got %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected Cobra to leave argument error printing to Execute, got stderr %q", stderr)
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Fatalf("expected error to reject extra arg, got %v", err)
	}
}

func TestHelpIncludesListCommand(t *testing.T) {
	stdout, stderr, err := executeRootForTest("help")
	if err != nil {
		t.Fatalf("expected help to succeed, got %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "list") || !strings.Contains(stdout, "Print worktrees as TSV") {
		t.Fatalf("expected help to include list command, got %q", stdout)
	}
}

func TestListRejectsArgs(t *testing.T) {
	stdout, stderr, err := executeRootForTest("list", "extra")
	if err == nil {
		t.Fatal("expected list with extra arg to return an error")
	}
	if stdout != "" {
		t.Fatalf("expected no list output, got %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected Cobra to leave argument error printing to Execute, got stderr %q", stderr)
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Fatalf("expected error to reject extra arg, got %v", err)
	}
}

func TestListPrintsRowsWithProtectedAndError(t *testing.T) {
	originalListRows := listRows
	originalListNow := listNow
	t.Cleanup(func() {
		listRows = originalListRows
		listNow = originalListNow
	})

	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	listNow = func() time.Time { return now }
	listRows = func(root string) ([]worktree.Entry, error) {
		return []worktree.Entry{
			{
				RepoName:        "repo",
				Path:            "/repo/.worktrees/clean",
				Branch:          "feature",
				Status:          worktree.StatusMerged,
				Dirty:           false,
				LastCommitTime:  now.Add(-2 * time.Minute),
				LastCommitTitle: "normal title",
				Protected:       false,
			},
			{
				RepoName:        "repo",
				Path:            "/repo/.worktrees/protected",
				Branch:          "bugfix",
				Status:          worktree.StatusUnknown,
				Dirty:           true,
				LastCommitTitle: "bad\ttitle\nnext",
				Protected:       true,
				Error:           "scan\tfailed\ntry\ragain",
			},
		}, nil
	}

	stdout, stderr, err := executeRootForTest("list")
	if err != nil {
		t.Fatalf("expected list to succeed, got %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}

	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two TSV rows, got %d from %q", len(lines), stdout)
	}

	first := strings.Split(lines[0], "\t")
	if len(first) != 9 {
		t.Fatalf("expected first row to have 9 fields, got %d: %#v", len(first), first)
	}
	wantFirst := []string{"repo", "/repo/.worktrees/clean", "feature", "merged", "false", "120", "normal title", "false", ""}
	for i := range wantFirst {
		if first[i] != wantFirst[i] {
			t.Fatalf("first row field %d = %q, want %q; row=%#v", i, first[i], wantFirst[i], first)
		}
	}

	second := strings.Split(lines[1], "\t")
	if len(second) != 9 {
		t.Fatalf("expected second row to have 9 fields, got %d: %#v", len(second), second)
	}
	wantSecond := []string{"repo", "/repo/.worktrees/protected", "bugfix", "unknown", "true", "0", "bad title next", "true", "scan failed try again"}
	for i := range wantSecond {
		if second[i] != wantSecond[i] {
			t.Fatalf("second row field %d = %q, want %q; row=%#v", i, second[i], wantSecond[i], second)
		}
	}
}
