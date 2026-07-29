package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func newTempRepo(t *testing.T) string {
	t.Helper()

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatalf("Mkdir repo: %v", err)
	}
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.name", "Lumberjack Test")
	runGit(t, repo, "config", "user.email", "lumberjack@example.com")

	return repo
}

func writeFile(t *testing.T, dir, name, contents string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", name, err)
	}
}

func commitFile(t *testing.T, repo, name, contents, subject string) {
	t.Helper()

	writeFile(t, repo, name, contents)
	runGit(t, repo, "add", name)
	runGit(t, repo, "commit", "-m", subject)
}

func commitFileAt(t *testing.T, repo, name, contents, subject, unixSeconds string) {
	t.Helper()

	writeFile(t, repo, name, contents)
	runGit(t, repo, "add", name)
	runGitWithEnv(t, repo, []string{
		"GIT_AUTHOR_DATE=" + unixSeconds + " +0000",
		"GIT_COMMITTER_DATE=" + unixSeconds + " +0000",
	}, "commit", "-m", subject)
}

func runGit(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	return runGitWithEnv(t, dir, nil, args...)
}

func runGitWithEnv(t *testing.T, dir string, env []string, args ...string) []byte {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return out
}

func realPath(t *testing.T, path string) string {
	t.Helper()

	got, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks %s: %v", path, err)
	}
	return got
}
