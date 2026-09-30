package prune

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sam-evani/lumberjack/internal/git"
	"github.com/sam-evani/lumberjack/internal/worktree"
)

type Kind string

const (
	KindStarted       Kind = "started"
	KindSkipped       Kind = "skipped"
	KindSalvaged      Kind = "salvaged"
	KindBackedUp      Kind = "backed_up"
	KindRemoved       Kind = "removed"
	KindBranchDeleted Kind = "branch_deleted"
	KindBranchKept    Kind = "branch_kept"
	KindFailed        Kind = "failed"
)

type Event struct {
	Kind    Kind   `json:"kind"`
	Repo    string `json:"repo"`
	Path    string `json:"path"`
	Branch  string `json:"branch,omitempty"`
	Message string `json:"message,omitempty"`
}

type Runner interface {
	SalvagePatch(repoPath, worktreePath string) ([]byte, error)
	BackupRef(repoPath, ref, sha string) error
	RemoveWorktree(repoPath, path string, force bool) error
	DeleteBranch(repoPath, branch string) error
	PruneWorktrees(repoPath string) error
}

type GitRunner struct{}

func (GitRunner) SalvagePatch(repoPath, worktreePath string) ([]byte, error) {
	return git.NewExec(repoPath).SalvagePatch(worktreePath)
}

func (GitRunner) BackupRef(repoPath, ref, sha string) error {
	return git.NewExec(repoPath).UpdateRef(ref, sha)
}

func (GitRunner) RemoveWorktree(repoPath, path string, force bool) error {
	return git.NewExec(repoPath).RemoveWorktree(path, force)
}

func (GitRunner) DeleteBranch(repoPath, branch string) error {
	return git.NewExec(repoPath).DeleteBranch(branch)
}

func (GitRunner) PruneWorktrees(repoPath string) error {
	return git.NewExec(repoPath).PruneWorktrees()
}

type Result struct {
	Removed []string
	Failed  []string
	Skipped []string
}

// Execute removes the selected worktrees. Dirty worktrees are salvaged to
// salvageDir first and are never removed if salvage fails. Branches are deleted
// only for merged rows and only after their worktree is gone. Repos run in
// parallel; worktrees within a repo run sequentially.
func Execute(entries []worktree.Entry, selected map[string]bool, runner Runner, salvageDir string, emit func(Event)) Result {
	plan := worktree.PlanDeletion(entries, selected, worktree.DeleteForce)
	byPath := make(map[string]worktree.Entry, len(entries))
	for _, e := range entries {
		byPath[e.Path] = e
	}

	var mu sync.Mutex
	var result Result
	send := func(ev Event) {
		mu.Lock()
		defer mu.Unlock()
		switch ev.Kind {
		case KindRemoved:
			result.Removed = append(result.Removed, ev.Path)
		case KindFailed:
			result.Failed = append(result.Failed, ev.Path)
		case KindSkipped:
			result.Skipped = append(result.Skipped, ev.Path)
		}
		if emit != nil {
			emit(ev)
		}
	}

	for _, s := range plan.Skipped {
		send(Event{Kind: KindSkipped, Repo: s.Entry.RepoName, Path: s.Entry.Path, Branch: s.Entry.Branch, Message: s.Reason})
	}

	byRepo := map[string][]worktree.Entry{}
	var repoOrder []string
	for _, action := range plan.RemoveWorktrees {
		if _, ok := byRepo[action.RepoPath]; !ok {
			repoOrder = append(repoOrder, action.RepoPath)
		}
		byRepo[action.RepoPath] = append(byRepo[action.RepoPath], byPath[action.Path])
	}

	var wg sync.WaitGroup
	for _, repoPath := range repoOrder {
		wg.Add(1)
		go func(repoPath string, rows []worktree.Entry) {
			defer wg.Done()
			for _, e := range rows {
				removeOne(e, runner, salvageDir, send)
			}
			if err := runner.PruneWorktrees(repoPath); err != nil {
				send(Event{Kind: KindFailed, Repo: filepath.Base(repoPath), Path: repoPath, Message: "git worktree prune: " + err.Error()})
			}
		}(repoPath, byRepo[repoPath])
	}
	wg.Wait()
	return result
}

func removeOne(e worktree.Entry, runner Runner, salvageDir string, send func(Event)) {
	base := Event{Repo: e.RepoName, Path: e.Path, Branch: e.Branch}
	with := func(kind Kind, msg string) Event {
		ev := base
		ev.Kind = kind
		ev.Message = msg
		return ev
	}

	send(with(KindStarted, ""))
	if e.HeadOrphan {
		ref := BackupRefName(e)
		if e.HeadSHA == "" {
			send(with(KindFailed, "commits are on no branch and HEAD is unknown, not removed"))
			return
		}
		if err := runner.BackupRef(e.RepoPath, ref, e.HeadSHA); err != nil {
			send(with(KindFailed, "backup ref failed, not removed: "+err.Error()))
			return
		}
		send(with(KindBackedUp, ref))
	}
	if e.Dirty {
		file, err := salvage(e, runner, salvageDir)
		if err != nil {
			send(with(KindFailed, "salvage failed, not removed: "+err.Error()))
			return
		}
		send(with(KindSalvaged, file))
	}
	if err := runner.RemoveWorktree(e.RepoPath, e.Path, e.Dirty); err != nil {
		send(with(KindFailed, err.Error()))
		return
	}
	send(with(KindRemoved, ""))

	if e.Branch == "" {
		return
	}
	if e.Status != worktree.StatusMerged {
		send(with(KindBranchKept, "unmerged"))
		return
	}
	if err := runner.DeleteBranch(e.RepoPath, e.Branch); err != nil {
		send(with(KindFailed, "worktree removed but branch delete failed: "+err.Error()))
		return
	}
	send(with(KindBranchDeleted, ""))
}

// BackupRefName is where a detached worktree's otherwise-unreferenced HEAD is
// kept so its commits survive removal and gc.
func BackupRefName(e worktree.Entry) string {
	sha := e.HeadSHA
	if len(sha) > 8 {
		sha = sha[:8]
	}
	return "refs/chainsaw/backup/" + filepath.Base(e.Path) + "-" + sha
}

func salvage(e worktree.Entry, runner Runner, salvageDir string) (string, error) {
	patch, err := runner.SalvagePatch(e.RepoPath, e.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(salvageDir, 0o755); err != nil {
		return "", err
	}
	label := strings.ReplaceAll(e.Branch, "/", "-")
	if label == "" {
		label = filepath.Base(e.Path)
	}
	name := e.RepoName + "--" + label + ".patch"
	file := filepath.Join(salvageDir, name)
	header := fmt.Sprintf("# chainsaw salvage\n# path: %s\n# branch: %s\n# head: %s\n# restore: git worktree add <dir> %s && git -C <dir> apply %s\n\n",
		e.Path, e.Branch, e.HeadSHA, e.HeadSHA, file)
	if err := os.WriteFile(file, append([]byte(header), patch...), 0o644); err != nil {
		return "", err
	}
	return file, nil
}
