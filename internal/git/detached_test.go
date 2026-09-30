package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecHeadContainedIn(t *testing.T) {
	repo := newTempRepo(t)
	commitFile(t, repo, "a.txt", "a\n", "base")
	g := NewExec(repo)

	onMain := filepath.Join(t.TempDir(), "on-main")
	runGit(t, repo, "worktree", "add", "--detach", onMain, "main")
	if got, err := g.HeadContainedIn(onMain, []string{"main"}); err != nil || got != "main" {
		t.Fatalf("on main: got %q, %v", got, err)
	}

	onBranch := filepath.Join(t.TempDir(), "on-branch")
	runGit(t, repo, "branch", "feature", "main")
	runGit(t, repo, "worktree", "add", "--detach", onBranch, "feature")
	commitFile(t, onBranch, "b.txt", "b\n", "detached work")
	runGit(t, onBranch, "branch", "-f", "feature", "HEAD")
	if got, err := g.HeadContainedIn(onBranch, []string{"main"}); err != nil || got != "feature" {
		t.Fatalf("on branch: got %q, %v", got, err)
	}

	orphan := filepath.Join(t.TempDir(), "orphan")
	runGit(t, repo, "worktree", "add", "--detach", orphan, "main")
	commitFile(t, orphan, "c.txt", "c\n", "orphan work")
	if got, err := g.HeadContainedIn(orphan, []string{"main"}); err != nil || got != "" {
		t.Fatalf("orphan: got %q, %v", got, err)
	}

	head := strings.TrimSpace(string(runGit(t, orphan, "rev-parse", "HEAD")))
	if err := g.UpdateRef("refs/chainsaw/backup/orphan", head); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(runGit(t, repo, "rev-parse", "refs/chainsaw/backup/orphan"))); got != head {
		t.Fatalf("backup ref = %q, want %q", got, head)
	}
}

func TestExecLastActivityUsesHeadAndDirtyFiles(t *testing.T) {
	repo := newTempRepo(t)
	commitFile(t, repo, "a.txt", "a\n", "base")
	wt := filepath.Join(t.TempDir(), "wt")
	runGit(t, repo, "worktree", "add", "--detach", wt, "main")
	g := NewExec(repo)

	adminDir := strings.TrimSpace(string(runGit(t, wt, "rev-parse", "--absolute-git-dir")))
	old := time.Now().Add(-90 * 24 * time.Hour)
	for _, p := range []string{filepath.Join(adminDir, "HEAD"), filepath.Join(adminDir, "logs", "HEAD")} {
		os.Chtimes(p, old, old)
	}
	got, err := g.LastActivity(wt)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(got) < 80*24*time.Hour {
		t.Fatalf("clean worktree activity = %v, want ~90 days ago", got)
	}

	writeFile(t, wt, "new.txt", "edit\n")
	got, err = g.LastActivity(wt)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(got) > time.Hour {
		t.Fatalf("dirty worktree activity = %v, want now", got)
	}
}
