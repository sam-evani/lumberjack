package web

import (
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/sam-evani/lumberjack/internal/prune"
	"github.com/sam-evani/lumberjack/internal/size"
	"github.com/sam-evani/lumberjack/internal/state"
	"github.com/sam-evani/lumberjack/internal/worktree"
)

//go:embed static/index.html
var indexHTML []byte

type Config struct {
	Root     string
	Dirs     state.Dirs
	Policy   worktree.Policy
	Scan     func(root string) ([]worktree.Entry, error)
	ScanRepo func(repoPath string) []worktree.Entry
	Runner   prune.Runner
	Measure  func(paths []string, onResult func(size.Result))
	Now      func() time.Time
	LockWait time.Duration
}

type Server struct {
	cfg   Config
	token string
	port  int
	hub   *hub

	mu        sync.Mutex
	rows      map[string]state.Row
	scannedAt time.Time
	scanning  bool
	measured  int
	toMeasure int
	deleting  bool
	lastPrune *state.PruneSummary
	scanErr   string
}

func NewServer(cfg Config) (*Server, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Measure == nil {
		cfg.Measure = size.MeasureAll
	}
	if cfg.LockWait == 0 {
		cfg.LockWait = 10 * time.Minute
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, token: hex.EncodeToString(buf), hub: newHub(), rows: map[string]state.Row{}}
	if snap, ok := cfg.Dirs.LoadFor(cfg.Root); ok {
		for _, r := range snap.Rows {
			s.rows[r.Path] = r
		}
		s.scannedAt = snap.ScannedAt
		s.lastPrune = snap.LastPrune
	}
	return s, nil
}

func (s *Server) Token() string { return s.token }

// WaitIdle blocks until any in-flight delete batch has finished.
func (s *Server) WaitIdle() {
	for {
		s.mu.Lock()
		busy := s.deleting
		s.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// SetPort must be called with the bound port before serving; it is used for
// the Host header check.
func (s *Server) SetPort(port int) { s.port = port }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("POST /api/rescan", s.handleRescan)
	mux.HandleFunc("POST /api/delete", s.handleDelete)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	return s.guard(mux)
}

func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != fmt.Sprintf("127.0.0.1:%d", s.port) && r.Host != fmt.Sprintf("localhost:%d", s.port) {
			http.Error(w, "bad host", http.StatusForbidden)
			return
		}
		tok := r.Header.Get("X-Chainsaw-Token")
		if tok == "" {
			tok = r.URL.Query().Get("t")
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) != 1 {
			http.Error(w, "missing or invalid token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(indexHTML)
}

type stateView struct {
	Root      string              `json:"root"`
	ScannedAt time.Time           `json:"scanned_at"`
	Scanning  bool                `json:"scanning"`
	Measured  int                 `json:"measured"`
	ToMeasure int                 `json:"to_measure"`
	Deleting  bool                `json:"deleting"`
	ScanError string              `json:"scan_error,omitempty"`
	LastPrune *state.PruneSummary `json:"last_prune,omitempty"`
	Rows      []state.Row         `json:"rows"`
}

func (s *Server) viewLocked() stateView {
	return stateView{
		Root:      s.cfg.Root,
		ScannedAt: s.scannedAt,
		Scanning:  s.scanning,
		Measured:  s.measured,
		ToMeasure: s.toMeasure,
		Deleting:  s.deleting,
		ScanError: s.scanErr,
		LastPrune: s.lastPrune,
		Rows:      s.sortedRowsLocked(),
	}
}

func (s *Server) sortedRowsLocked() []state.Row {
	rows := make([]state.Row, 0, len(s.rows))
	for _, r := range s.rows {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Repo != rows[j].Repo {
			return rows[i].Repo < rows[j].Repo
		}
		return rows[i].Path < rows[j].Path
	})
	return rows
}

func (s *Server) broadcastState() {
	s.mu.Lock()
	v := s.viewLocked()
	s.mu.Unlock()
	s.hub.send("state", v)
}

func (s *Server) saveSnapshot() {
	s.mu.Lock()
	snap := state.Snapshot{Root: s.cfg.Root, ScannedAt: s.scannedAt, Rows: s.sortedRowsLocked(), LastPrune: s.lastPrune}
	s.mu.Unlock()
	s.cfg.Dirs.Save(snap)
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	v := s.viewLocked()
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleRescan(w http.ResponseWriter, r *http.Request) {
	s.StartRescan()
	w.WriteHeader(http.StatusAccepted)
}

// StartRescan scans in the background unless a scan or delete is running.
func (s *Server) StartRescan() {
	s.mu.Lock()
	if s.scanning || s.deleting {
		s.mu.Unlock()
		return
	}
	s.scanning = true
	s.scanErr = ""
	s.mu.Unlock()
	s.broadcastState()
	go s.rescan()
}

func (s *Server) rescan() {
	entries, err := s.cfg.Scan(s.cfg.Root)
	now := s.cfg.Now()

	s.mu.Lock()
	if err != nil {
		s.scanning = false
		s.scanErr = err.Error()
		s.mu.Unlock()
		s.broadcastState()
		return
	}
	next := make(map[string]state.Row, len(entries))
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		row := state.NewRow(e, now, s.cfg.Policy)
		if prev, ok := s.rows[e.Path]; ok {
			row.SizeBytes, row.SizeError, row.MeasuredAt = prev.SizeBytes, prev.SizeError, prev.MeasuredAt
		}
		next[e.Path] = row
		paths = append(paths, e.Path)
	}
	s.rows = next
	s.scannedAt = now
	s.measured = 0
	s.toMeasure = len(paths)
	s.mu.Unlock()
	s.broadcastState()

	s.cfg.Measure(paths, func(res size.Result) {
		s.mu.Lock()
		s.measured++
		row, ok := s.rows[res.Path]
		if ok {
			if res.Err != nil {
				row.SizeBytes, row.SizeError = nil, res.Err.Error()
			} else {
				n := res.Bytes
				row.SizeBytes, row.SizeError = &n, ""
			}
			row.MeasuredAt = s.cfg.Now()
			s.rows[res.Path] = row
		}
		progress := map[string]int{"measured": s.measured, "to_measure": s.toMeasure}
		s.mu.Unlock()
		if ok {
			s.hub.send("row", row)
		}
		s.hub.send("scan_progress", progress)
	})

	s.mu.Lock()
	s.scanning = false
	s.mu.Unlock()
	s.saveSnapshot()
	s.broadcastState()
}

type deleteItem struct {
	Path     string            `json:"path"`
	Category worktree.Category `json:"category"`
}

type deleteRequest struct {
	Items []deleteItem `json:"items"`
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req deleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Items) == 0 {
		http.Error(w, "expected {\"items\":[{\"path\",\"category\"}]}", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if s.deleting {
		s.mu.Unlock()
		http.Error(w, "a delete is already running", http.StatusConflict)
		return
	}
	repos := map[string]bool{}
	for _, item := range req.Items {
		row, ok := s.rows[item.Path]
		if !ok {
			s.mu.Unlock()
			http.Error(w, "unknown path "+item.Path, http.StatusBadRequest)
			return
		}
		repos[row.RepoPath] = true
	}
	s.deleting = true
	s.mu.Unlock()
	s.broadcastState()

	go s.runDelete(req.Items, repos)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) runDelete(items []deleteItem, repos map[string]bool) {
	defer func() {
		s.mu.Lock()
		s.deleting = false
		s.mu.Unlock()
		s.saveSnapshot()
		s.broadcastState()
	}()

	release, err := s.cfg.Dirs.Lock(s.cfg.LockWait, func() { s.hub.send("lock_wait", nil) })
	if err != nil {
		for _, item := range items {
			s.hub.send("delete", prune.Event{Kind: prune.KindFailed, Path: item.Path, Message: err.Error()})
		}
		s.hub.send("delete_done", map[string]int{"removed": 0, "failed": len(items)})
		return
	}
	defer release()

	now := s.cfg.Now()
	var fresh []worktree.Entry
	for repo := range repos {
		fresh = append(fresh, s.cfg.ScanRepo(repo)...)
	}
	byPath := map[string]worktree.Entry{}
	for _, e := range fresh {
		byPath[e.Path] = e
	}

	selected := map[string]bool{}
	failed := 0
	for _, item := range items {
		e, ok := byPath[item.Path]
		if !ok {
			s.mu.Lock()
			delete(s.rows, item.Path)
			s.mu.Unlock()
			s.hub.send("delete", prune.Event{Kind: prune.KindRemoved, Path: item.Path, Message: "already gone"})
			continue
		}
		row := state.NewRow(e, now, s.cfg.Policy)
		s.mu.Lock()
		if prev, ok := s.rows[e.Path]; ok {
			row.SizeBytes, row.SizeError, row.MeasuredAt = prev.SizeBytes, prev.SizeError, prev.MeasuredAt
		}
		s.rows[e.Path] = row
		s.mu.Unlock()
		if row.Category != item.Category {
			failed++
			s.hub.send("row", row)
			s.hub.send("delete", prune.Event{Kind: prune.KindFailed, Repo: e.RepoName, Path: e.Path, Message: fmt.Sprintf("changed since scan (%s → %s), rescan", item.Category, row.Category)})
			continue
		}
		selected[e.Path] = true
	}

	salvageDir := filepath.Join(s.cfg.Dirs.Salvage(), now.Format("20060102-150405"))
	res := prune.Execute(fresh, selected, s.cfg.Runner, salvageDir, func(ev prune.Event) {
		if ev.Kind == prune.KindRemoved {
			s.mu.Lock()
			delete(s.rows, ev.Path)
			s.mu.Unlock()
		}
		s.hub.send("delete", ev)
	})
	s.hub.send("delete_done", map[string]int{"removed": len(res.Removed), "failed": failed + len(res.Failed) + len(res.Skipped)})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			w.Write(msg)
			flusher.Flush()
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

type hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func newHub() *hub { return &hub{subs: map[chan []byte]struct{}{}} }

func (h *hub) subscribe() chan []byte {
	ch := make(chan []byte, 1024)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *hub) send(event string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	msg := []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, payload))
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}
