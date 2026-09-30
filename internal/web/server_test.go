package web

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sam-evani/lumberjack/internal/size"
	"github.com/sam-evani/lumberjack/internal/state"
	"github.com/sam-evani/lumberjack/internal/worktree"
)

type recordingRunner struct {
	mu      sync.Mutex
	removed []string
}

func (r *recordingRunner) SalvagePatch(repoPath, path string) ([]byte, error) { return nil, nil }
func (r *recordingRunner) BackupRef(repoPath, ref, sha string) error          { return nil }
func (r *recordingRunner) RemoveWorktree(repoPath, path string, force bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removed = append(r.removed, path)
	return nil
}
func (r *recordingRunner) DeleteBranch(repoPath, branch string) error { return nil }
func (r *recordingRunner) PruneWorktrees(repoPath string) error       { return nil }

func testEntries() []worktree.Entry {
	return []worktree.Entry{
		{RepoName: "r", RepoPath: "/r", Path: "/r", Branch: "main", Status: worktree.StatusBase, Protected: true},
		{RepoName: "r", RepoPath: "/r", Path: "/r/wt/a", Branch: "a", Status: worktree.StatusMerged},
		{RepoName: "r", RepoPath: "/r", Path: "/r/wt/b", Branch: "b", Status: worktree.StatusUnmerged, LastCommitTime: time.Now()},
	}
}

func newTestServer(t *testing.T, repoEntries func() []worktree.Entry) (*Server, *httptest.Server, *recordingRunner) {
	t.Helper()
	runner := &recordingRunner{}
	s, err := NewServer(Config{
		Root:     "/co",
		Dirs:     state.Dirs{Root: t.TempDir()},
		Policy:   worktree.DefaultPolicy(),
		Scan:     func(string) ([]worktree.Entry, error) { return testEntries(), nil },
		ScanRepo: func(string) []worktree.Entry { return repoEntries() },
		Runner:   runner,
		Measure: func(paths []string, on func(size.Result)) {
			for _, p := range paths {
				on(size.Result{Path: p, Bytes: 1000})
			}
		},
		LockWait: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	port, _ := strconv.Atoi(ts.URL[strings.LastIndex(ts.URL, ":")+1:])
	s.SetPort(port)

	s.StartRescan()
	waitFor(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return !s.scanning && len(s.rows) == 3
	})
	return s, ts, runner
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for condition")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func do(t *testing.T, s *Server, ts *httptest.Server, method, path, body string, withToken bool) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	if withToken {
		req.Header.Set("X-Chainsaw-Token", s.Token())
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestRejectsMissingTokenAndBadHost(t *testing.T) {
	s, ts, _ := newTestServer(t, testEntries)
	if resp := do(t, s, ts, "GET", "/api/state", "", false); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token status = %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("GET", ts.URL+"/api/state", nil)
	req.Host = "evil.example:80"
	req.Header.Set("X-Chainsaw-Token", s.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad host status = %d", resp.StatusCode)
	}
	if resp := do(t, s, ts, "GET", "/?t="+s.Token(), "", false); resp.StatusCode != http.StatusOK {
		t.Fatalf("index with query token status = %d", resp.StatusCode)
	}
}

func TestStateIncludesSizesAndCategories(t *testing.T) {
	s, ts, _ := newTestServer(t, testEntries)
	resp := do(t, s, ts, "GET", "/api/state", "", true)
	var v stateView
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	cats := map[string]worktree.Category{}
	for _, r := range v.Rows {
		cats[r.Path] = r.Category
		if r.SizeBytes == nil || *r.SizeBytes != 1000 {
			t.Fatalf("row %s size = %v", r.Path, r.SizeBytes)
		}
	}
	if cats["/r"] != worktree.CategoryProtected || cats["/r/wt/a"] != worktree.CategoryMerged || cats["/r/wt/b"] != worktree.CategoryActive {
		t.Fatalf("categories = %v", cats)
	}
}

func TestDeleteStreamsEventsAndRejectsProtectedAndChangedRows(t *testing.T) {
	changed := testEntries()
	changed[1].Status = worktree.StatusUnmerged
	changed[1].LastCommitTime = time.Now()
	calls := 0
	s, ts, runner := newTestServer(t, func() []worktree.Entry {
		calls++
		if calls > 1 {
			return changed
		}
		return testEntries()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events?t="+s.Token(), nil)
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	events := make(chan string, 100)
	go func() {
		sc := bufio.NewScanner(stream.Body)
		var name string
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "event: ") {
				name = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				events <- name + " " + strings.TrimPrefix(line, "data: ")
			}
		}
	}()

	body := `{"items":[{"path":"/r/wt/a","category":"merged"},{"path":"/r","category":"protected"}]}`
	if resp := do(t, s, ts, "POST", "/api/delete", body, true); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	var got []string
	for !strings.HasPrefix(last(got), "delete_done") {
		select {
		case e := <-events:
			got = append(got, e)
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out; events so far: %v", got)
		}
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{`"kind":"skipped","repo":"r","path":"/r"`, `"kind":"removed","repo":"r","path":"/r/wt/a"`, `"kind":"branch_deleted"`, `"removed":1`} {
		if !strings.Contains(joined, want) {
			t.Fatalf("events missing %s:\n%s", want, joined)
		}
	}
	if len(runner.removed) != 1 || runner.removed[0] != "/r/wt/a" {
		t.Fatalf("removed = %v", runner.removed)
	}
	waitFor(t, func() bool { s.mu.Lock(); defer s.mu.Unlock(); return !s.deleting })

	// Second request: the row's category changed on disk, so it must be rejected.
	s.mu.Lock()
	s.rows["/r/wt/a"] = state.NewRow(testEntries()[1], time.Now(), worktree.DefaultPolicy())
	s.mu.Unlock()
	do(t, s, ts, "POST", "/api/delete", `{"items":[{"path":"/r/wt/a","category":"merged"}]}`, true)
	got = nil
	for !strings.HasPrefix(last(got), "delete_done") {
		select {
		case e := <-events:
			got = append(got, e)
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out; events so far: %v", got)
		}
	}
	if !strings.Contains(strings.Join(got, "\n"), "changed since scan") {
		t.Fatalf("expected changed-since-scan rejection:\n%s", strings.Join(got, "\n"))
	}
	if len(runner.removed) != 1 {
		t.Fatalf("changed row was removed: %v", runner.removed)
	}
}

func last(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[len(s)-1]
}
