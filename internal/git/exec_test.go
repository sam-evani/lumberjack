package git

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecFetchAllPrune(t *testing.T) {
	t.Run("succeeds with no remotes", func(t *testing.T) {
		repo := newTempRepo(t)
		commitFile(t, repo, "README.md", "initial\n", "initial commit")

		g := NewExec(repo)
		if err := g.FetchAllPrune(); err != nil {
			t.Fatalf("FetchAllPrune returned error: %v", err)
		}
	})

	t.Run("returns error for broken origin", func(t *testing.T) {
		repo := newTempRepo(t)
		commitFile(t, repo, "README.md", "initial\n", "initial commit")

		missingRemote := filepath.Join(t.TempDir(), "missing.git")
		runGit(t, repo, "remote", "add", "origin", missingRemote)

		g := NewExec(repo)
		err := g.FetchAllPrune()
		if err == nil {
			t.Fatalf("FetchAllPrune returned nil error, want broken remote error")
		}
		if got := err.Error(); !strings.Contains(got, "git [fetch --all --prune -q] failed") &&
			!strings.Contains(got, "does not appear to be a git repository") {
			t.Fatalf("FetchAllPrune error = %q, want git fetch command or missing repo output", got)
		}
	})
}

func TestExecWorktreeListPorcelainAndIsDirty(t *testing.T) {
	repo := newTempRepo(t)
	commitFile(t, repo, "README.md", "initial\n", "initial commit")

	worktreePath := filepath.Join(filepath.Dir(repo), "feature")
	runGit(t, repo, "worktree", "add", "-b", "feature", worktreePath, "main")
	writeFile(t, worktreePath, "dirty.txt", "dirty\n")

	g := NewExec(repo)
	list, err := g.WorktreeListPorcelain()
	if err != nil {
		t.Fatalf("WorktreeListPorcelain returned error: %v", err)
	}
	wantPath := realPath(t, worktreePath)
	if !bytes.Contains(list, []byte("worktree "+wantPath)) {
		t.Fatalf("WorktreeListPorcelain output does not include %q:\n%s", wantPath, list)
	}

	dirty, err := g.IsDirty(worktreePath)
	if err != nil {
		t.Fatalf("IsDirty returned error: %v", err)
	}
	if !dirty {
		t.Fatalf("IsDirty(%q) = false, want true", worktreePath)
	}
}

func TestExecAvailableBaseRefsAndMergedIntoAny(t *testing.T) {
	repo := newTempRepo(t)
	commitFile(t, repo, "README.md", "initial\n", "initial commit")
	runGit(t, repo, "branch", "feature")

	g := NewExec(repo)
	got, err := g.AvailableBaseRefs([]string{"missing", "main", "refs/heads/main"})
	if err != nil {
		t.Fatalf("AvailableBaseRefs returned error: %v", err)
	}
	want := []string{"main", "refs/heads/main"}
	if len(got) != len(want) {
		t.Fatalf("AvailableBaseRefs returned %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AvailableBaseRefs returned %v, want %v", got, want)
		}
	}

	merged, err := g.MergedIntoAny("feature", got)
	if err != nil {
		t.Fatalf("MergedIntoAny returned error: %v", err)
	}
	if !merged {
		t.Fatalf("MergedIntoAny(feature, %v) = false, want true", got)
	}
}

func TestExecLastCommitReturnsUnixSecondsAndSubject(t *testing.T) {
	repo := newTempRepo(t)
	commitFileAt(t, repo, "README.md", "initial\n", "known subject", "1700000000")

	worktreePath := filepath.Join(filepath.Dir(repo), "last-commit")
	runGit(t, repo, "worktree", "add", "-b", "last-commit", worktreePath, "main")

	g := NewExec(repo)
	for _, path := range []string{repo, worktreePath} {
		t.Run(path, func(t *testing.T) {
			unixSeconds, subject, err := g.LastCommit(path)
			if err != nil {
				t.Fatalf("LastCommit returned error: %v", err)
			}
			if unixSeconds != 1700000000 {
				t.Fatalf("LastCommit unixSeconds = %d, want 1700000000", unixSeconds)
			}
			if subject != "known subject" {
				t.Fatalf("LastCommit subject = %q, want %q", subject, "known subject")
			}
		})
	}
}

func TestExecRemoveWorktreeAndDeleteBranch(t *testing.T) {
	repo := newTempRepo(t)
	commitFile(t, repo, "README.md", "initial\n", "initial commit")

	worktreePath := filepath.Join(filepath.Dir(repo), "cleanup")
	runGit(t, repo, "worktree", "add", "-b", "cleanup", worktreePath, "main")

	g := NewExec(repo)
	if err := g.RemoveWorktree(worktreePath, false); err != nil {
		t.Fatalf("RemoveWorktree returned error: %v", err)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree path still exists after RemoveWorktree: %v", err)
	}
	if err := g.DeleteBranch("cleanup"); err != nil {
		t.Fatalf("DeleteBranch returned error: %v", err)
	}
	if out := runGit(t, repo, "branch", "--list", "cleanup"); len(bytes.TrimSpace(out)) != 0 {
		t.Fatalf("branch cleanup still exists:\n%s", out)
	}
}
