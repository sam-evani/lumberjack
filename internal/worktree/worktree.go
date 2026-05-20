package worktree

import (
	"bufio"
	"bytes"
	"strings"
	"time"
)

type MergeStatus string

const (
	StatusMerged   MergeStatus = "merged"
	StatusUnmerged MergeStatus = "unmerged"
	StatusBase     MergeStatus = "main/master"
	StatusDetached MergeStatus = "detached"
	StatusPrunable MergeStatus = "missing/prunable"
	StatusUnknown  MergeStatus = "unknown"
)

type Entry struct {
	RepoName        string
	RepoPath        string
	Path            string
	DisplayName     string
	Branch          string
	HeadSHA         string
	Detached        bool
	Prunable        bool
	PrunableReason  string
	Status          MergeStatus
	Dirty           bool
	LastCommitTime  time.Time
	LastCommitTitle string
	Protected       bool
	Error           string
}

func ParsePorcelain(out []byte) ([]Entry, error) {
	var entries []Entry
	scanner := bufio.NewScanner(bytes.NewReader(out))
	current := Entry{}
	haveCurrent := false

	flush := func() {
		if !haveCurrent {
			return
		}
		if current.Branch == "" {
			current.Detached = true
		}
		entries = append(entries, current)
		current = Entry{}
		haveCurrent = false
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		switch key {
		case "worktree":
			flush()
			current = Entry{Path: value}
			haveCurrent = true
		case "HEAD":
			current.HeadSHA = value
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "prunable":
			current.Prunable = true
			current.PrunableReason = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flush()
	return entries, nil
}
