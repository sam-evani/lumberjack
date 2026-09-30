package worktree

func Classify(entry Entry, mergedIntoBase bool, hasBaseRef bool) Entry {
	if entry.DisplayName == "" {
		entry.DisplayName = baseName(entry.Path)
	}

	switch {
	case entry.Path == entry.RepoPath:
		entry.Status = StatusUnknown
		entry.Protected = true
	case entry.Error != "":
		entry.Status = StatusUnknown
		entry.Protected = true
	case entry.Locked:
		entry.Status = StatusUnknown
		entry.Protected = true
	case entry.Bare:
		entry.Status = StatusUnknown
		entry.Protected = true
	case entry.Prunable:
		entry.Status = StatusPrunable
		entry.Protected = true
	case entry.Detached:
		entry.Status = StatusDetached
		entry.Protected = false
	case entry.Branch == "main" || entry.Branch == "master":
		entry.Status = StatusBase
		entry.Protected = true
	case !hasBaseRef:
		entry.Status = StatusUnknown
		entry.Protected = true
	case mergedIntoBase:
		entry.Status = StatusMerged
		entry.Protected = false
	default:
		entry.Status = StatusUnmerged
		entry.Protected = false
	}

	return entry
}

func baseName(path string) string {
	if path == "" {
		return ""
	}
	end := len(path)
	for end > 1 && path[end-1] == '/' {
		end--
	}
	start := end
	for start > 0 && path[start-1] != '/' {
		start--
	}
	return path[start:end]
}
