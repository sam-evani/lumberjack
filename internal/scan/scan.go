package scan

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sam-evani/lumberjack/internal/git"
	"github.com/sam-evani/lumberjack/internal/worktree"
)

type Git = git.Git

type GitFactory func(repoPath string) Git

var baseRefCandidates = []string{"main", "master", "origin/main", "origin/master"}

func Default(root string) ([]worktree.Entry, error) {
	return Root(root, func(repoPath string) Git {
		return git.NewExec(repoPath)
	})
}

func Root(root string, factory GitFactory) ([]worktree.Entry, error) {
	children, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var rows []worktree.Entry
	for _, child := range children {
		if !child.IsDir() {
			continue
		}
		repoPath := filepath.Join(root, child.Name())
		if !hasGitDir(repoPath) {
			continue
		}
		rows = append(rows, scanRepo(repoPath, child.Name(), factory(repoPath))...)
	}
	rows = dedupeRows(rows)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].RepoName == rows[j].RepoName {
			return rows[i].Path < rows[j].Path
		}
		return rows[i].RepoName < rows[j].RepoName
	})
	return rows, nil
}

func hasGitDir(repoPath string) bool {
	info, err := os.Stat(filepath.Join(repoPath, ".git"))
	return err == nil && info.IsDir()
}

func scanRepo(repoPath string, repoName string, git Git) []worktree.Entry {
	out, err := git.WorktreeListPorcelain()
	if err != nil {
		return []worktree.Entry{errorRow(repoPath, repoName, err)}
	}
	entries, err := worktree.ParsePorcelain(out)
	if err != nil {
		return []worktree.Entry{errorRow(repoPath, repoName, err)}
	}

	baseRefs, baseErr := git.AvailableBaseRefs(baseRefCandidates)
	hasBaseRef := baseErr == nil && len(baseRefs) > 0

	rows := make([]worktree.Entry, 0, len(entries))
	for _, entry := range entries {
		entry.RepoName = repoName
		entry.RepoPath = repoPath
		setErrorIfEmpty(&entry, baseErr)

		if shouldInspect(entry) {
			dirty, err := git.IsDirty(entry.Path)
			if err != nil {
				setErrorIfEmpty(&entry, err)
			} else {
				entry.Dirty = dirty
			}

			unixSeconds, subject, err := git.LastCommit(entry.Path)
			if err != nil {
				setErrorIfEmpty(&entry, err)
			} else {
				entry.LastCommitTime = time.Unix(unixSeconds, 0)
				entry.LastCommitTitle = subject
			}
		}

		merged := false
		if entry.Error == "" && hasBaseRef && shouldCheckMerged(entry) {
			var err error
			merged, err = git.MergedIntoAny(entry.Branch, baseRefs)
			setErrorIfEmpty(&entry, err)
		}

		rows = append(rows, worktree.Classify(entry, merged, hasBaseRef))
	}
	return rows
}

func errorRow(repoPath string, repoName string, err error) worktree.Entry {
	return worktree.Classify(worktree.Entry{
		RepoName: repoName,
		RepoPath: repoPath,
		Path:     repoPath,
		Error:    err.Error(),
	}, false, false)
}

func dedupeRows(rows []worktree.Entry) []worktree.Entry {
	seen := map[string]bool{}
	deduped := make([]worktree.Entry, 0, len(rows))
	for _, row := range rows {
		if seen[row.Path] {
			continue
		}
		seen[row.Path] = true
		deduped = append(deduped, row)
	}
	return deduped
}

func shouldInspect(entry worktree.Entry) bool {
	return !entry.Prunable && !entry.Bare
}

func shouldCheckMerged(entry worktree.Entry) bool {
	return entry.Path != entry.RepoPath &&
		!entry.Locked &&
		!entry.Bare &&
		!entry.Prunable &&
		!entry.Detached &&
		entry.Branch != "" &&
		entry.Branch != "main" &&
		entry.Branch != "master"
}

func setErrorIfEmpty(entry *worktree.Entry, err error) {
	if err == nil || entry.Error != "" {
		return
	}
	entry.Error = err.Error()
}
