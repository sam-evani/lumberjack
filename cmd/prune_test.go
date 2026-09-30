package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sam-evani/lumberjack/internal/state"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPruneEndToEnd(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "repo")
	os.Mkdir(repo, 0o755)
	git(t, repo, "init", "-q", "-b", "main")
	git(t, repo, "config", "user.name", "t")
	git(t, repo, "config", "user.email", "t@example.com")
	write(t, filepath.Join(repo, "base.txt"), "base\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "base")

	wt := func(name string) string { return filepath.Join(root, "repo-worktrees", name) }
	addBranch := func(name, file string) {
		git(t, repo, "worktree", "add", "-q", "-b", name, wt(name), "main")
		write(t, filepath.Join(wt(name), file), name+"\n")
		git(t, wt(name), "add", ".")
		git(t, wt(name), "commit", "-qm", name)
	}

	addBranch("merged-clean", "a.txt")
	git(t, repo, "merge", "-q", "--no-ff", "-m", "merge clean", "merged-clean")

	addBranch("merged-dirty", "b.txt")
	git(t, repo, "merge", "-q", "--no-ff", "-m", "merge dirty", "merged-dirty")
	write(t, filepath.Join(wt("merged-dirty"), "wip.txt"), "unsaved work\n")

	addBranch("squashed", "c.txt")
	git(t, repo, "merge", "-q", "--squash", "squashed")
	git(t, repo, "commit", "-qm", "squash")

	addBranch("open", "d.txt")

	// Detached, idle for 60 days, with a commit on no branch: auto-pruned with a backup ref.
	idle := wt("idle-review")
	git(t, repo, "worktree", "add", "-q", "--detach", idle, "main")
	write(t, filepath.Join(idle, "e.txt"), "orphan\n")
	git(t, idle, "add", ".")
	oldDate := time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339)
	cmd := exec.Command("git", "commit", "-qm", "orphan work")
	cmd.Dir = idle
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+oldDate, "GIT_COMMITTER_DATE="+oldDate)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
	orphanSHA := strings.TrimSpace(git(t, idle, "rev-parse", "HEAD"))
	adminDir := strings.TrimSpace(git(t, idle, "rev-parse", "--absolute-git-dir"))
	old := time.Now().Add(-60 * 24 * time.Hour)
	for _, p := range []string{filepath.Join(adminDir, "HEAD"), filepath.Join(adminDir, "logs", "HEAD")} {
		os.Chtimes(p, old, old)
	}

	// Detached but checked out just now: kept.
	fresh := wt("fresh-review")
	git(t, repo, "worktree", "add", "-q", "--detach", fresh, "main~1")

	dirs := state.Dirs{Root: t.TempDir()}
	origDirs, origNotify := pruneDirs, pruneNotify
	pruneDirs = func() state.Dirs { return dirs }
	pruneNotify = false
	t.Cleanup(func() { pruneDirs, pruneNotify = origDirs, origNotify })

	var out bytes.Buffer
	if err := runPrune(&out, root); err != nil {
		t.Fatalf("runPrune: %v\n%s", err, out.String())
	}

	for _, name := range []string{"merged-clean", "merged-dirty", "squashed"} {
		if _, err := os.Stat(wt(name)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists\n%s", name, out.String())
		}
	}
	if _, err := os.Stat(wt("open")); err != nil {
		t.Fatalf("unmerged worktree removed: %v", err)
	}
	if _, err := os.Stat(idle); !os.IsNotExist(err) {
		t.Fatalf("idle detached worktree still exists\n%s", out.String())
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh detached worktree removed: %v", err)
	}
	backups := git(t, repo, "for-each-ref", "--format=%(refname) %(objectname)", "refs/chainsaw/backup")
	if !strings.Contains(backups, "idle-review-") || !strings.Contains(backups, orphanSHA) {
		t.Fatalf("backup refs = %q, want idle-review -> %s", backups, orphanSHA)
	}
	branches := git(t, repo, "branch", "--format=%(refname:short)")
	if strings.Contains(branches, "merged") || strings.Contains(branches, "squashed") || !strings.Contains(branches, "open") {
		t.Fatalf("branches = %q", branches)
	}

	patches, _ := filepath.Glob(filepath.Join(dirs.Salvage(), "*", "repo--merged-dirty.patch"))
	if len(patches) != 1 {
		t.Fatalf("salvage patches = %v", patches)
	}
	data, _ := os.ReadFile(patches[0])
	if !strings.Contains(string(data), "unsaved work") {
		t.Fatalf("salvage patch missing wip:\n%s", data)
	}

	snap, err := dirs.Load()
	if err != nil {
		t.Fatal(err)
	}
	if snap.LastPrune == nil || snap.LastPrune.Removed != 4 || snap.LastPrune.BytesFreed <= 0 {
		t.Fatalf("last prune = %#v", snap.LastPrune)
	}
	if len(snap.Rows) != 3 {
		t.Fatalf("snapshot rows = %d, want primary + open + fresh", len(snap.Rows))
	}
}
