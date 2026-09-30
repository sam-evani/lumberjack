package prune

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/sam-evani/lumberjack/internal/worktree"
)

type fakeRunner struct {
	mu         sync.Mutex
	calls      []string
	salvageErr error
	removeErr  map[string]error
	patch      string
}

func (f *fakeRunner) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *fakeRunner) SalvagePatch(repoPath, path string) ([]byte, error) {
	f.record("salvage " + path)
	return []byte(f.patch), f.salvageErr
}

func (f *fakeRunner) RemoveWorktree(repoPath, path string, force bool) error {
	if force {
		f.record("remove --force " + path)
	} else {
		f.record("remove " + path)
	}
	return f.removeErr[path]
}

func (f *fakeRunner) DeleteBranch(repoPath, branch string) error {
	f.record("branch -D " + branch)
	return nil
}

func (f *fakeRunner) PruneWorktrees(repoPath string) error {
	f.record("prune " + repoPath)
	return nil
}

func all(entries []worktree.Entry) map[string]bool {
	m := map[string]bool{}
	for _, e := range entries {
		m[e.Path] = true
	}
	return m
}

func TestExecuteMergedCleanDirtyUnmergedAndProtected(t *testing.T) {
	entries := []worktree.Entry{
		{RepoName: "r", RepoPath: "/r", Path: "/r/wt/clean", Branch: "clean", Status: worktree.StatusMerged},
		{RepoName: "r", RepoPath: "/r", Path: "/r/wt/dirty", Branch: "sam/dirty", Status: worktree.StatusMerged, Dirty: true, HeadSHA: "abc"},
		{RepoName: "r", RepoPath: "/r", Path: "/r/wt/open", Branch: "open", Status: worktree.StatusUnmerged},
		{RepoName: "r", RepoPath: "/r", Path: "/r", Branch: "main", Status: worktree.StatusBase, Protected: true},
	}
	runner := &fakeRunner{patch: "diff --git a/x b/x\n"}
	salvageDir := t.TempDir()

	res := Execute(entries, all(entries), runner, salvageDir, nil)

	wantCalls := []string{
		"remove /r/wt/clean",
		"branch -D clean",
		"salvage /r/wt/dirty",
		"remove --force /r/wt/dirty",
		"branch -D sam/dirty",
		"remove /r/wt/open",
		"prune /r",
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v\nwant %#v", runner.calls, wantCalls)
	}
	if len(res.Removed) != 3 || len(res.Failed) != 0 || !reflect.DeepEqual(res.Skipped, []string{"/r"}) {
		t.Fatalf("result = %#v", res)
	}
	data, err := os.ReadFile(filepath.Join(salvageDir, "r--sam-dirty.patch"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# head: abc") || !strings.Contains(string(data), "diff --git a/x b/x") {
		t.Fatalf("salvage file = %q", data)
	}
}

func TestExecuteSalvageFailureSkipsRemoval(t *testing.T) {
	entries := []worktree.Entry{
		{RepoName: "r", RepoPath: "/r", Path: "/r/wt/dirty", Branch: "dirty", Status: worktree.StatusMerged, Dirty: true},
	}
	runner := &fakeRunner{salvageErr: errors.New("nope")}
	var events []Event
	res := Execute(entries, all(entries), runner, t.TempDir(), func(e Event) { events = append(events, e) })

	for _, c := range runner.calls {
		if strings.HasPrefix(c, "remove") || strings.HasPrefix(c, "branch") {
			t.Fatalf("unexpected call %q after salvage failure", c)
		}
	}
	if len(res.Failed) != 1 || len(res.Removed) != 0 {
		t.Fatalf("result = %#v", res)
	}
	last := events[len(events)-1]
	if last.Kind != KindFailed || !strings.Contains(last.Message, "salvage failed") {
		t.Fatalf("last event = %#v", last)
	}
}

func TestExecuteRemoveFailureKeepsBranch(t *testing.T) {
	entries := []worktree.Entry{
		{RepoName: "r", RepoPath: "/r", Path: "/r/wt/a", Branch: "a", Status: worktree.StatusMerged},
	}
	runner := &fakeRunner{removeErr: map[string]error{"/r/wt/a": errors.New("locked")}}
	Execute(entries, all(entries), runner, t.TempDir(), nil)
	for _, c := range runner.calls {
		if strings.HasPrefix(c, "branch") {
			t.Fatalf("branch deleted after failed removal: %v", runner.calls)
		}
	}
}
