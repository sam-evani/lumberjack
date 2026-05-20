package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type Exec struct {
	dir string
}

func NewExec(dir string) *Exec {
	return &Exec{dir: dir}
}

func (g *Exec) FetchAllPrune() error {
	_, err := g.run(g.dir, "fetch", "--all", "--prune", "-q")
	if err == nil || isNoRemoteFetchError(err) {
		return nil
	}
	return err
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
		_, err := g.run(g.dir, "merge-base", "--is-ancestor", branch, base)
		if err == nil {
			return true, nil
		}
		if exitCode(err) == 1 {
			continue
		}
		return false, err
	}
	return false, nil
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
	_, err := g.run(g.dir, "branch", "-d", branch)
	return err
}

func (g *Exec) run(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
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

func isNoRemoteFetchError(err error) bool {
	var cmdErr commandError
	if !errors.As(err, &cmdErr) {
		return false
	}
	out := string(cmdErr.output)
	return strings.Contains(out, "No remote repository specified") ||
		strings.Contains(out, "does not appear to be a git repository") ||
		strings.Contains(out, "Could not read from remote repository")
}
