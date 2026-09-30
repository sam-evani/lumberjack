package git

import "time"

type Git interface {
	WorktreeListPorcelain() ([]byte, error)
	AvailableBaseRefs(candidates []string) ([]string, error)
	MergedIntoAny(branch string, baseRefs []string) (bool, error)
	IsDirty(worktreePath string) (bool, error)
	LastCommit(worktreePath string) (unixSeconds int64, subject string, err error)
	HeadContainedIn(worktreePath string, baseRefs []string) (string, error)
	LastActivity(worktreePath string) (time.Time, error)
	RemoveWorktree(path string, force bool) error
	DeleteBranch(branch string) error
}
