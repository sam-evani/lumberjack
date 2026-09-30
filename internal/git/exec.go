package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Exec struct {
	dir string
}

// Keep squash detection bounded; exact per-commit matches are handled by git cherry first.
const squashPatchScanLimit = 100

func NewExec(dir string) *Exec {
	return &Exec{dir: dir}
}

func (g *Exec) WorktreeListPorcelain() ([]byte, error) {
	return g.run(g.dir, "worktree", "list", "--porcelain")
}

func (g *Exec) AvailableBaseRefs(candidates []string) ([]string, error) {
	var refs []string
	for _, candidate := range candidates {
		_, err := g.run(g.dir, "rev-parse", "--verify", "--quiet", candidate+"^{commit}")
		if err == nil {
			refs = append(refs, candidate)
			continue
		}
		if exitCode(err) == 1 {
			continue
		}
		return nil, err
	}
	return refs, nil
}

func (g *Exec) MergedIntoAny(branch string, baseRefs []string) (bool, error) {
	for _, base := range baseRefs {
		merged, err := g.isAncestor(branch, base)
		if err != nil {
			return false, err
		}
		if merged {
			return true, nil
		}

		merged, err = g.hasEquivalentPatches(branch, base)
		if err != nil {
			return false, err
		}
		if merged {
			return true, nil
		}
	}
	return false, nil
}

func (g *Exec) isAncestor(branch string, base string) (bool, error) {
	_, err := g.run(g.dir, "merge-base", "--is-ancestor", branch, base)
	if err == nil {
		return true, nil
	}
	if exitCode(err) == 1 {
		return false, nil
	}
	return false, err
}

func (g *Exec) hasEquivalentPatches(branch string, base string) (bool, error) {
	merged, err := g.allBranchCommitsHaveEquivalentPatch(branch, base)
	if err != nil || merged {
		return merged, err
	}
	return g.branchDiffMatchesBaseCommit(branch, base)
}

func (g *Exec) allBranchCommitsHaveEquivalentPatch(branch string, base string) (bool, error) {
	out, err := g.run(g.dir, "cherry", base, branch)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "+") {
			return false, nil
		}
		if !strings.HasPrefix(line, "-") {
			return false, fmt.Errorf("parse git cherry output line %q", line)
		}
	}
	return true, nil
}

func (g *Exec) branchDiffMatchesBaseCommit(branch string, base string) (bool, error) {
	out, err := g.run(g.dir, "merge-base", branch, base)
	if err != nil {
		return false, err
	}
	mergeBase := strings.TrimSpace(string(out))
	if mergeBase == "" {
		return false, fmt.Errorf("merge-base %s %s returned empty output", branch, base)
	}

	diff, err := g.run(g.dir, "diff", mergeBase, branch)
	if err != nil {
		return false, err
	}
	branchPatchID, ok, err := g.patchID(diff)
	if err != nil || !ok {
		return ok, err
	}

	out, err = g.run(g.dir, "log", "--first-parent", "--max-count="+strconv.Itoa(squashPatchScanLimit), "--format=medium", "--patch", mergeBase+".."+base)
	if err != nil {
		return false, err
	}
	basePatchIDs, err := g.patchIDs(out)
	if err != nil {
		return false, err
	}
	return basePatchIDs[branchPatchID], nil
}

func (g *Exec) patchID(patch []byte) (string, bool, error) {
	patchIDs, err := g.patchIDs(patch)
	if err != nil {
		return "", false, err
	}
	for patchID := range patchIDs {
		return patchID, true, nil
	}
	return "", false, nil
}

func (g *Exec) patchIDs(patch []byte) (map[string]bool, error) {
	patchIDs := map[string]bool{}
	if len(bytes.TrimSpace(patch)) == 0 {
		return patchIDs, nil
	}
	cmd := exec.Command("git", "patch-id", "--stable")
	cmd.Dir = g.dir
	cmd.Stdin = bytes.NewReader(patch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, commandError{args: []string{"patch-id", "--stable"}, output: out, err: err}
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		patchIDs[fields[0]] = true
	}
	return patchIDs, nil
}

func (g *Exec) IsDirty(worktreePath string) (bool, error) {
	out, err := g.run(worktreePath, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return len(bytes.TrimSpace(out)) != 0, nil
}

func (g *Exec) LastCommit(worktreePath string) (int64, string, error) {
	out, err := g.run(worktreePath, "log", "-1", "--format=%ct%x00%s")
	if err != nil {
		return 0, "", err
	}

	parts := bytes.SplitN(bytes.TrimSuffix(out, []byte("\n")), []byte{0}, 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("parse last commit: expected unix seconds and subject, got %q", out)
	}
	unixSeconds, err := strconv.ParseInt(string(parts[0]), 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("parse last commit unix seconds %q: %w", parts[0], err)
	}
	return unixSeconds, string(parts[1]), nil
}

func (g *Exec) RemoveWorktree(path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	_, err := g.run(g.dir, args...)
	return err
}

func (g *Exec) DeleteBranch(branch string) error {
	_, err := g.run(g.dir, "branch", "-D", branch)
	return err
}

func (g *Exec) PruneWorktrees() error {
	_, err := g.run(g.dir, "worktree", "prune")
	return err
}

// SalvagePatch returns a binary diff of tracked and untracked (non-ignored)
// changes against HEAD, using a throwaway index so the worktree is untouched.
func (g *Exec) SalvagePatch(worktreePath string) ([]byte, error) {
	tmp, err := os.MkdirTemp("", "chainsaw-index-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}

	if _, err := g.runEnv(worktreePath, env, "read-tree", "HEAD"); err != nil {
		return nil, err
	}
	if _, err := g.runEnv(worktreePath, env, "add", "-A"); err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "diff", "--cached", "--binary", "HEAD")
	cmd.Dir = worktreePath
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, commandError{args: []string{"diff", "--cached", "--binary", "HEAD"}, output: stderr.Bytes(), err: err}
	}
	return out, nil
}

func (g *Exec) run(dir string, args ...string) ([]byte, error) {
	return g.runEnv(dir, nil, args...)
}

func (g *Exec) runEnv(dir string, env []string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, commandError{args: args, output: out, err: err}
	}
	return out, nil
}

type commandError struct {
	args   []string
	output []byte
	err    error
}

func (e commandError) Error() string {
	out := strings.TrimSpace(string(e.output))
	if out == "" {
		return fmt.Sprintf("git %v failed: %v", e.args, e.err)
	}
	return fmt.Sprintf("git %v failed: %v\n%s", e.args, e.err, out)
}

func (e commandError) Unwrap() error {
	return e.err
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
