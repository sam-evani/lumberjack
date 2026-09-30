package worktree

import "time"

type Category string

const (
	CategoryMerged    Category = "merged"
	CategoryDetached  Category = "detached"
	CategoryReview    Category = "review"
	CategoryActive    Category = "active"
	CategoryProtected Category = "protected"
)

type Policy struct {
	// Unmerged worktrees with no commits for this long are flagged for review.
	StaleAfter time.Duration
	// Detached worktrees idle for this long are auto-pruned.
	DetachedAfter time.Duration
}

func DefaultPolicy() Policy {
	return Policy{StaleAfter: 30 * 24 * time.Hour, DetachedAfter: 30 * 24 * time.Hour}
}

func (p Policy) Categorize(entry Entry, now time.Time) Category {
	switch {
	case entry.Error != "":
		return CategoryReview
	case entry.Protected:
		return CategoryProtected
	case entry.Status == StatusMerged:
		return CategoryMerged
	case entry.Status == StatusDetached:
		return CategoryDetached
	case entry.Status == StatusUnmerged && !entry.LastCommitTime.IsZero() && now.Sub(entry.LastCommitTime) > p.StaleAfter:
		return CategoryReview
	default:
		return CategoryActive
	}
}

// AutoPrune reports whether the scheduled prune should remove entry: merged
// worktrees always, detached ones once idle longer than DetachedAfter.
func (p Policy) AutoPrune(entry Entry, now time.Time) bool {
	switch p.Categorize(entry, now) {
	case CategoryMerged:
		return true
	case CategoryDetached:
		activity := entry.Activity()
		return !activity.IsZero() && now.Sub(activity) > p.DetachedAfter
	default:
		return false
	}
}
