package git

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

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

func TestExecMergedIntoAnyDetectsPatchEquivalentCommit(t *testing.T) {
	repo := newTempRepo(t)
	commitFile(t, repo, "README.md", "initial\n", "initial commit")

	runGit(t, repo, "checkout", "-b", "feature")
	commitFile(t, repo, "feature.txt", "feature\n", "feature change")

	runGit(t, repo, "checkout", "main")
	commitFile(t, repo, "feature.txt", "feature\n", "squash feature")

	g := NewExec(repo)
	merged, err := g.MergedIntoAny("feature", []string{"main"})
	if err != nil {
		t.Fatalf("MergedIntoAny returned error: %v", err)
	}
	if !merged {
		t.Fatalf("MergedIntoAny(feature, [main]) = false, want true")
	}
}

func TestExecMergedIntoAnyDetectsPatchEquivalentSquash(t *testing.T) {
	repo := newTempRepo(t)
	commitFile(t, repo, "README.md", "initial\n", "initial commit")

	runGit(t, repo, "checkout", "-b", "feature")
	commitFile(t, repo, "one.txt", "one\n", "first feature change")
	commitFile(t, repo, "two.txt", "two\n", "second feature change")

	runGit(t, repo, "checkout", "main")
	writeFile(t, repo, "one.txt", "one\n")
	writeFile(t, repo, "two.txt", "two\n")
	runGit(t, repo, "add", "one.txt", "two.txt")
	runGit(t, repo, "commit", "-m", "squash feature")

	g := NewExec(repo)
	merged, err := g.MergedIntoAny("feature", []string{"main"})
	if err != nil {
		t.Fatalf("MergedIntoAny returned error: %v", err)
	}
	if !merged {
		t.Fatalf("MergedIntoAny(feature, [main]) = false, want true")
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

func TestExecDeleteBranchDeletesPatchEquivalentBranch(t *testing.T) {
	repo := newTempRepo(t)
	commitFile(t, repo, "README.md", "initial\n", "initial commit")

	runGit(t, repo, "checkout", "-b", "feature")
	commitFile(t, repo, "feature.txt", "feature\n", "feature change")

	runGit(t, repo, "checkout", "main")
	commitFile(t, repo, "feature.txt", "feature\n", "squash feature")

	g := NewExec(repo)
	if err := g.DeleteBranch("feature"); err != nil {
		t.Fatalf("DeleteBranch returned error: %v", err)
	}
	if out := runGit(t, repo, "branch", "--list", "feature"); len(bytes.TrimSpace(out)) != 0 {
		t.Fatalf("branch feature still exists:\n%s", out)
	}
}
