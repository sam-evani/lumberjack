package worktree

import "time"

type Category string

const (
	CategoryMerged    Category = "merged"
	CategoryReview    Category = "review"
	CategoryActive    Category = "active"
	CategoryProtected Category = "protected"
)

const DefaultStaleAfter = 30 * 24 * time.Hour

func Categorize(entry Entry, now time.Time, staleAfter time.Duration) Category {
	switch {
	case entry.Error != "":
		return CategoryReview
	case entry.Protected:
		return CategoryProtected
	case entry.Status == StatusMerged:
		return CategoryMerged
	case entry.Status == StatusUnmerged && !entry.LastCommitTime.IsZero() && now.Sub(entry.LastCommitTime) > staleAfter:
		return CategoryReview
	default:
		return CategoryActive
	}
}
