package worktree

type DeleteMode int

const (
	DeleteNormal DeleteMode = iota
	DeleteForce
)

type RemoveWorktreeAction struct {
	RepoPath string
	Path     string
	Force    bool
}

type BranchAction struct {
	RepoPath string
	Branch   string
}

type SkippedAction struct {
	Entry  Entry
	Reason string
}

type DeletionPlan struct {
	Mode            DeleteMode
	RemoveWorktrees []RemoveWorktreeAction
	DeleteBranches  []BranchAction
	KeepBranches    []BranchAction
	Skipped         []SkippedAction
}

func PlanDeletion(entries []Entry, selected map[string]bool, mode DeleteMode) DeletionPlan {
	plan := DeletionPlan{Mode: mode}
	for _, entry := range entries {
		if !selected[entry.Path] {
			continue
		}
		if entry.Protected {
			plan.Skipped = append(plan.Skipped, SkippedAction{Entry: entry, Reason: "protected: " + ProtectionReason(entry)})
			continue
		}
		if entry.Status == StatusDetached || entry.Status == StatusUnknown || entry.Status == StatusPrunable {
			plan.Skipped = append(plan.Skipped, SkippedAction{Entry: entry, Reason: string(entry.Status)})
			continue
		}
		if entry.Dirty && mode == DeleteNormal {
			plan.Skipped = append(plan.Skipped, SkippedAction{Entry: entry, Reason: "dirty; press D to force delete"})
			continue
		}

		plan.RemoveWorktrees = append(plan.RemoveWorktrees, RemoveWorktreeAction{
			RepoPath: entry.RepoPath,
			Path:     entry.Path,
			Force:    mode == DeleteForce,
		})

		if entry.Branch == "" {
			continue
		}
		branch := BranchAction{RepoPath: entry.RepoPath, Branch: entry.Branch}
		if entry.Status == StatusMerged {
			plan.DeleteBranches = append(plan.DeleteBranches, branch)
		} else {
			plan.KeepBranches = append(plan.KeepBranches, branch)
		}
	}
	return plan
}

func ProtectionReason(entry Entry) string {
	switch {
	case entry.Error != "":
		return "scan error"
	case entry.Path == entry.RepoPath:
		return "primary checkout"
	case entry.Locked:
		return "locked"
	case entry.Bare:
		return "bare"
	case entry.Prunable || entry.Status == StatusPrunable:
		return "prunable"
	case entry.Detached || entry.Status == StatusDetached:
		return "detached"
	case entry.Branch == "main" || entry.Branch == "master" || entry.Status == StatusBase:
		return "main/master"
	case entry.Status == StatusUnknown:
		return "no base ref"
	default:
		return "protected"
	}
}
