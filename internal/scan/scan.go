package scan

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/sam-evani/lumberjack/internal/git"
	"github.com/sam-evani/lumberjack/internal/worktree"
)

type Git = git.Git

type GitFactory func(repoPath string) Git

var baseRefCandidates = []string{"main", "master", "origin/main", "origin/master"}

const worktreeConcurrency = 6

func Default(root string) ([]worktree.Entry, error) {
	return Root(root, func(repoPath string) Git {
		return git.NewExec(repoPath)
	})
}

// DefaultRepo rescans a single repository, e.g. to re-verify rows before deletion.
func DefaultRepo(repoPath string) []worktree.Entry {
	return scanRepo(repoPath, filepath.Base(repoPath), git.NewExec(repoPath))
}

func Root(root string, factory GitFactory) ([]worktree.Entry, error) {
	children, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var repos []string
	for _, child := range children {
		if child.IsDir() && hasGitDir(filepath.Join(root, child.Name())) {
			repos = append(repos, child.Name())
		}
	}
	perRepo := make([][]worktree.Entry, len(repos))
	var wg sync.WaitGroup
	for i, name := range repos {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			repoPath := filepath.Join(root, name)
			perRepo[i] = scanRepo(repoPath, name, factory(repoPath))
		}(i, name)
	}
	wg.Wait()
	var rows []worktree.Entry
	for _, r := range perRepo {
		rows = append(rows, r...)
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

	rows := make([]worktree.Entry, len(entries))
	sem := make(chan struct{}, worktreeConcurrency)
	var wg sync.WaitGroup
	for i, entry := range entries {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, entry worktree.Entry) {
			defer func() { <-sem; wg.Done() }()
			rows[i] = inspect(entry, repoPath, repoName, git, baseRefs, hasBaseRef, baseErr)
		}(i, entry)
	}
	wg.Wait()
	return rows
}

func inspect(entry worktree.Entry, repoPath, repoName string, git Git, baseRefs []string, hasBaseRef bool, baseErr error) worktree.Entry {
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

	if entry.Error == "" && shouldInspectDetached(entry) {
		activity, err := git.LastActivity(entry.Path)
		setErrorIfEmpty(&entry, err)
		entry.LastActivity = activity
		if err == nil {
			in, err := git.HeadContainedIn(entry.Path, baseRefs)
			setErrorIfEmpty(&entry, err)
			entry.HeadContainedIn = in
			entry.HeadOrphan = err == nil && in == ""
		}
	}

	merged := false
	if entry.Error == "" && hasBaseRef && shouldCheckMerged(entry) {
		var err error
		merged, err = git.MergedIntoAny(entry.Branch, baseRefs)
		setErrorIfEmpty(&entry, err)
	}

	return worktree.Classify(entry, merged, hasBaseRef)
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

func shouldInspectDetached(entry worktree.Entry) bool {
	return entry.Detached && entry.Path != entry.RepoPath && !entry.Locked && !entry.Bare && !entry.Prunable
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
