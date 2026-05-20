package git

type Git interface {
	FetchAllPrune() error
	WorktreeListPorcelain() ([]byte, error)
	AvailableBaseRefs(candidates []string) ([]string, error)
	MergedIntoAny(branch string, baseRefs []string) (bool, error)
	IsDirty(worktreePath string) (bool, error)
	LastCommit(worktreePath string) (unixSeconds int64, subject string, err error)
	RemoveWorktree(path string, force bool) error
	DeleteBranch(branch string) error
}
