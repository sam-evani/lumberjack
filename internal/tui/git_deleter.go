package tui

import (
	"github.com/sam-evani/lumberjack/internal/git"
	"github.com/sam-evani/lumberjack/internal/worktree"
)

type GitDeleter struct{}

func (GitDeleter) RemoveWorktree(action worktree.RemoveWorktreeAction) error {
	return git.NewExec(action.RepoPath).RemoveWorktree(action.Path, action.Force)
}

func (GitDeleter) DeleteBranch(action worktree.BranchAction) error {
	return git.NewExec(action.RepoPath).DeleteBranch(action.Branch)
}
