package scan

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sam-evani/lumberjack/internal/worktree"
)

func TestRootDiscoversReposAndClassifiesMergedFeature(t *testing.T) {
	root := t.TempDir()
	repoPath := makeRepo(t, root, "lumberjack")
	featurePath := filepath.Join(repoPath, ".worktrees", "feature")
	commitTime := int64(1712345678)

	git := &fakeGit{
		porcelain: porcelain(
			"worktree "+featurePath,
			"HEAD abc123",
			"branch refs/heads/sam-evani/feature",
		),
		baseRefs: map[string]bool{"origin/main": true},
		merged:   map[string]bool{"sam-evani/feature": true},
		dirty:    map[string]bool{featurePath: true},
		commits: map[string]fakeCommit{
			featurePath: {unix: commitTime, subject: "build scanner"},
		},
	}

	rows, err := Root(root, factoryFor(map[string]*fakeGit{repoPath: git}))
	if err != nil {
		t.Fatalf("Root returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.RepoName != "lumberjack" {
		t.Fatalf("RepoName = %q, want lumberjack", row.RepoName)
	}
	if row.RepoPath != repoPath {
		t.Fatalf("RepoPath = %q, want %q", row.RepoPath, repoPath)
	}
	if row.Path != featurePath {
		t.Fatalf("Path = %q, want %q", row.Path, featurePath)
	}
	if row.DisplayName != "feature" {
		t.Fatalf("DisplayName = %q, want feature", row.DisplayName)
	}
	if !row.Dirty {
		t.Fatalf("Dirty = false, want true")
	}
	if !row.LastCommitTime.Equal(time.Unix(commitTime, 0)) {
		t.Fatalf("LastCommitTime = %v, want %v", row.LastCommitTime, time.Unix(commitTime, 0))
	}
	if row.LastCommitTitle != "build scanner" {
		t.Fatalf("LastCommitTitle = %q, want build scanner", row.LastCommitTitle)
	}
	if row.Status != worktree.StatusMerged {
		t.Fatalf("Status = %q, want %q", row.Status, worktree.StatusMerged)
	}
	if row.Protected {
		t.Fatalf("Protected = true, want false")
	}
	if !reflect.DeepEqual(git.factoryCandidates, baseRefCandidates) {
		t.Fatalf("AvailableBaseRefs candidates = %v, want %v", git.factoryCandidates, baseRefCandidates)
	}
}

func TestRootDeduplicatesRowsByWorktreePath(t *testing.T) {
	root := t.TempDir()
	repoA := makeRepo(t, root, "repo-a")
	repoB := makeRepo(t, root, "repo-b")
	sharedPath := filepath.Join(root, "shared-worktree")

	gits := map[string]*fakeGit{
		repoA: {
			porcelain: porcelain(
				"worktree "+sharedPath,
				"HEAD aaa",
				"branch refs/heads/sam-evani/shared",
			),
			baseRefs: map[string]bool{"main": true},
			merged:   map[string]bool{"sam-evani/shared": true},
		},
		repoB: {
			porcelain: porcelain(
				"worktree "+sharedPath,
				"HEAD bbb",
				"branch refs/heads/sam-evani/shared",
			),
			baseRefs: map[string]bool{"main": true},
			merged:   map[string]bool{"sam-evani/shared": true},
		},
	}

	rows, err := Root(root, factoryFor(gits))
	if err != nil {
		t.Fatalf("Root returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Path != sharedPath {
		t.Fatalf("Path = %q, want %q", rows[0].Path, sharedPath)
	}
}

func TestRootSkipsLinkedWorktreeChildren(t *testing.T) {
	root := t.TempDir()
	linkedPath := filepath.Join(root, "aaa-linked")
	if err := os.MkdirAll(linkedPath, 0o755); err != nil {
		t.Fatalf("make linked worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(linkedPath, ".git"), []byte("gitdir: ../repo/.git/worktrees/aaa-linked\n"), 0o644); err != nil {
		t.Fatalf("write linked worktree .git file: %v", err)
	}
	repoPath := makeRepo(t, root, "zzz-repo")

	git := &fakeGit{
		porcelain: porcelain(
			"worktree "+linkedPath,
			"HEAD abc123",
			"branch refs/heads/feature",
		),
		baseRefs: map[string]bool{"main": true},
		merged:   map[string]bool{"feature": true},
	}

	rows, err := Root(root, factoryFor(map[string]*fakeGit{repoPath: git}))
	if err != nil {
		t.Fatalf("Root returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].RepoPath != repoPath {
		t.Fatalf("RepoPath = %q, want %q", rows[0].RepoPath, repoPath)
	}
	if rows[0].Status != worktree.StatusMerged {
		t.Fatalf("Status = %q, want %q", rows[0].Status, worktree.StatusMerged)
	}
	if rows[0].Protected {
		t.Fatalf("Protected = true, want false")
	}
}

func TestRootMarksFeatureUnknownWhenNoBaseRefs(t *testing.T) {
	root := t.TempDir()
	repoPath := makeRepo(t, root, "repo")
	featurePath := filepath.Join(repoPath, ".worktrees", "feature")
	git := &fakeGit{
		porcelain: porcelain(
			"worktree "+featurePath,
			"HEAD abc123",
			"branch refs/heads/feature",
		),
	}

	rows, err := Root(root, factoryFor(map[string]*fakeGit{repoPath: git}))
	if err != nil {
		t.Fatalf("Root returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Status != worktree.StatusUnknown {
		t.Fatalf("Status = %q, want %q", rows[0].Status, worktree.StatusUnknown)
	}
	if !rows[0].Protected {
		t.Fatalf("Protected = false, want true")
	}
	if len(git.mergeCalls) != 0 {
		t.Fatalf("MergedIntoAny called %d times, want 0", len(git.mergeCalls))
	}
}

func TestRootClassifiesUnmergedFeatureWithBaseRefs(t *testing.T) {
	root := t.TempDir()
	repoPath := makeRepo(t, root, "repo")
	featurePath := filepath.Join(repoPath, ".worktrees", "feature")
	git := &fakeGit{
		porcelain: porcelain(
			"worktree "+featurePath,
			"HEAD abc123",
			"branch refs/heads/feature",
		),
		baseRefs: map[string]bool{"main": true},
		merged:   map[string]bool{"feature": false},
	}

	rows, err := Root(root, factoryFor(map[string]*fakeGit{repoPath: git}))
	if err != nil {
		t.Fatalf("Root returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Status != worktree.StatusUnmerged {
		t.Fatalf("Status = %q, want %q", rows[0].Status, worktree.StatusUnmerged)
	}
	if rows[0].Protected {
		t.Fatalf("Protected = true, want false")
	}
	if !reflect.DeepEqual(git.mergeCalls, []string{"feature"}) {
		t.Fatalf("mergeCalls = %v, want [feature]", git.mergeCalls)
	}
}

func TestRootRecordsMergeErrorsWithoutAborting(t *testing.T) {
	root := t.TempDir()
	mergedRepo := makeRepo(t, root, "merged-repo")
	mergeRepo := makeRepo(t, root, "merge-repo")
	mergedPath := filepath.Join(mergedRepo, ".worktrees", "feature")
	mergePath := filepath.Join(mergeRepo, ".worktrees", "feature")

	mergeErr := errors.New("merge-base failed")
	gits := map[string]*fakeGit{
		mergedRepo: {
			porcelain: porcelain(
				"worktree "+mergedPath,
				"HEAD abc123",
				"branch refs/heads/feature",
			),
			baseRefs: map[string]bool{"main": true},
			merged:   map[string]bool{"feature": true},
		},
		mergeRepo: {
			porcelain: porcelain(
				"worktree "+mergePath,
				"HEAD def456",
				"branch refs/heads/feature",
			),
			baseRefs:  map[string]bool{"main": true},
			mergeErrs: map[string]error{"feature": mergeErr},
		},
	}

	rows, err := Root(root, factoryFor(gits))
	if err != nil {
		t.Fatalf("Root returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].RepoName != "merge-repo" {
		t.Fatalf("first RepoName = %q, want merge-repo", rows[0].RepoName)
	}
	if rows[0].Error == "" {
		t.Fatalf("merge-repo Error is empty, want scan error")
	}
	if rows[0].Status != worktree.StatusUnknown {
		t.Fatalf("merge-repo Status = %q, want %q", rows[0].Status, worktree.StatusUnknown)
	}
	if !rows[0].Protected {
		t.Fatalf("merge-repo Protected = false, want true")
	}

	if rows[1].RepoName != "merged-repo" {
		t.Fatalf("second RepoName = %q, want merged-repo", rows[1].RepoName)
	}
	if rows[1].Error != "" {
		t.Fatalf("merged-repo Error = %q, want empty", rows[1].Error)
	}
	if rows[1].Status != worktree.StatusMerged {
		t.Fatalf("merged-repo Status = %q, want %q", rows[1].Status, worktree.StatusMerged)
	}
	if rows[1].Protected {
		t.Fatalf("merged-repo Protected = true, want false")
	}
}

func TestRootKeepsLockedAndBareEntriesProtected(t *testing.T) {
	root := t.TempDir()
	repoPath := makeRepo(t, root, "repo")
	lockedPath := filepath.Join(repoPath, ".worktrees", "locked")
	barePath := filepath.Join(repoPath, "bare.git")
	git := &fakeGit{
		porcelain: porcelain(
			"worktree "+lockedPath,
			"HEAD abc123",
			"branch refs/heads/locked-feature",
			"locked in use",
			"",
			"worktree "+barePath,
			"HEAD def456",
			"branch refs/heads/main",
			"bare",
		),
		baseRefs: map[string]bool{"main": true},
	}

	rows, err := Root(root, factoryFor(map[string]*fakeGit{repoPath: git}))
	if err != nil {
		t.Fatalf("Root returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if !row.Protected {
			t.Fatalf("%s Protected = false, want true", row.Path)
		}
		if row.Status != worktree.StatusUnknown {
			t.Fatalf("%s Status = %q, want %q", row.Path, row.Status, worktree.StatusUnknown)
		}
	}
	if git.dirtyCalls[barePath] != 0 {
		t.Fatalf("IsDirty called for bare path %d times, want 0", git.dirtyCalls[barePath])
	}
	if git.lastCommitCalls[barePath] != 0 {
		t.Fatalf("LastCommit called for bare path %d times, want 0", git.lastCommitCalls[barePath])
	}
}

type fakeGit struct {
	listErr   error
	baseErr   error
	porcelain []byte
	baseRefs  map[string]bool
	merged    map[string]bool
	mergeErrs map[string]error
	dirty     map[string]bool
	dirtyErrs map[string]error
	commits   map[string]fakeCommit

	factoryCandidates []string
	mergeCalls        []string
	dirtyCalls        map[string]int
	lastCommitCalls   map[string]int
}

type fakeCommit struct {
	unix    int64
	subject string
	err     error
}

func (g *fakeGit) WorktreeListPorcelain() ([]byte, error) {
	if g.listErr != nil {
		return nil, g.listErr
	}
	return g.porcelain, nil
}

func (g *fakeGit) AvailableBaseRefs(candidates []string) ([]string, error) {
	g.factoryCandidates = append([]string(nil), candidates...)
	if g.baseErr != nil {
		return nil, g.baseErr
	}
	var refs []string
	for _, candidate := range candidates {
		if g.baseRefs[candidate] {
			refs = append(refs, candidate)
		}
	}
	return refs, nil
}

func (g *fakeGit) MergedIntoAny(branch string, baseRefs []string) (bool, error) {
	g.mergeCalls = append(g.mergeCalls, branch)
	if err := g.mergeErrs[branch]; err != nil {
		return false, err
	}
	return g.merged[branch], nil
}

func (g *fakeGit) IsDirty(worktreePath string) (bool, error) {
	if g.dirtyCalls == nil {
		g.dirtyCalls = map[string]int{}
	}
	g.dirtyCalls[worktreePath]++
	if err := g.dirtyErrs[worktreePath]; err != nil {
		return false, err
	}
	return g.dirty[worktreePath], nil
}

func (g *fakeGit) LastCommit(worktreePath string) (int64, string, error) {
	if g.lastCommitCalls == nil {
		g.lastCommitCalls = map[string]int{}
	}
	g.lastCommitCalls[worktreePath]++
	commit := g.commits[worktreePath]
	return commit.unix, commit.subject, commit.err
}

func (g *fakeGit) RemoveWorktree(path string, force bool) error {
	panic("RemoveWorktree should not be called by scan")
}

func (g *fakeGit) DeleteBranch(branch string) error {
	panic("DeleteBranch should not be called by scan")
}

func makeRepo(t *testing.T, root string, name string) string {
	t.Helper()
	repoPath := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatalf("make repo: %v", err)
	}
	return repoPath
}

func factoryFor(gits map[string]*fakeGit) GitFactory {
	return func(repoPath string) Git {
		git := gits[repoPath]
		if git == nil {
			panic("unexpected repo path: " + repoPath)
		}
		return git
	}
}

func porcelain(lines ...string) []byte {
	return []byte(strings.Join(lines, "\n") + "\n")
}
