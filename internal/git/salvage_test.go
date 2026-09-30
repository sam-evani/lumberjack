package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecSalvagePatchCapturesTrackedAndUntrackedWithoutTouchingIndex(t *testing.T) {
	repo := newTempRepo(t)
	commitFile(t, repo, "tracked.txt", "one\n", "initial")
	worktreePath := filepath.Join(t.TempDir(), "wt")
	runGit(t, repo, "worktree", "add", "-b", "feature", worktreePath, "main")

	writeFile(t, worktreePath, "tracked.txt", "two\n")
	writeFile(t, worktreePath, "new.txt", "brand new\n")
	writeFile(t, worktreePath, ".gitignore", "ignored.txt\n")
	writeFile(t, worktreePath, "ignored.txt", "skip me\n")

	patch, err := NewExec(repo).SalvagePatch(worktreePath)
	if err != nil {
		t.Fatalf("SalvagePatch: %v", err)
	}
	text := string(patch)
	for _, want := range []string{"tracked.txt", "+two", "new.txt", "+brand new"} {
		if !strings.Contains(text, want) {
			t.Fatalf("patch missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "skip me") {
		t.Fatalf("patch should not include ignored files:\n%s", text)
	}
	if staged := strings.TrimSpace(string(runGit(t, worktreePath, "diff", "--cached", "--name-only"))); staged != "" {
		t.Fatalf("real index was modified: %q", staged)
	}

	clone := filepath.Join(t.TempDir(), "clone")
	runGit(t, repo, "worktree", "add", "--detach", clone, "feature")
	patchFile := filepath.Join(t.TempDir(), "salvage.patch")
	if err := os.WriteFile(patchFile, patch, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "apply", patchFile)
	cmd.Dir = clone
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git apply: %v\n%s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(clone, "new.txt"))
	if err != nil || string(got) != "brand new\n" {
		t.Fatalf("restored new.txt = %q, %v", got, err)
	}
}
