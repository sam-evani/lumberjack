package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sam-evani/lumberjack/internal/worktree"
)

type Dirs struct {
	Root string
}

func Default() Dirs {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return Dirs{Root: filepath.Join(xdg, "chainsaw")}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Dirs{Root: filepath.Join(os.TempDir(), "chainsaw")}
	}
	return Dirs{Root: filepath.Join(home, ".local", "state", "chainsaw")}
}

func (d Dirs) Snapshot() string { return filepath.Join(d.Root, "last-scan.json") }
func (d Dirs) Log() string      { return filepath.Join(d.Root, "prune.log") }
func (d Dirs) Salvage() string  { return filepath.Join(d.Root, "salvage") }
func (d Dirs) lockFile() string { return filepath.Join(d.Root, "lock") }

type Row struct {
	Repo            string            `json:"repo"`
	RepoPath        string            `json:"repo_path"`
	Path            string            `json:"path"`
	Name            string            `json:"name"`
	Branch          string            `json:"branch"`
	Status          string            `json:"status"`
	Category        worktree.Category `json:"category"`
	Dirty           bool              `json:"dirty"`
	Protected       bool              `json:"protected"`
	Primary         bool              `json:"primary"`
	Reason          string            `json:"reason,omitempty"`
	Error           string            `json:"error,omitempty"`
	LastCommitTime  time.Time         `json:"last_commit_time"`
	LastCommitTitle string            `json:"last_commit_title"`
	SizeBytes       *int64            `json:"size_bytes"`
	SizeError       string            `json:"size_error,omitempty"`
	MeasuredAt      time.Time         `json:"measured_at"`
}

func NewRow(entry worktree.Entry, now time.Time, staleAfter time.Duration) Row {
	row := Row{
		Repo:            entry.RepoName,
		RepoPath:        entry.RepoPath,
		Path:            entry.Path,
		Name:            entry.DisplayName,
		Branch:          entry.Branch,
		Status:          string(entry.Status),
		Category:        worktree.Categorize(entry, now, staleAfter),
		Dirty:           entry.Dirty,
		Protected:       entry.Protected,
		Primary:         entry.Path == entry.RepoPath,
		Error:           entry.Error,
		LastCommitTime:  entry.LastCommitTime,
		LastCommitTitle: entry.LastCommitTitle,
	}
	if entry.Protected {
		row.Reason = worktree.ProtectionReason(entry)
	}
	return row
}

type PruneSummary struct {
	At         time.Time `json:"at"`
	Removed    int       `json:"removed"`
	Failed     int       `json:"failed"`
	BytesFreed int64     `json:"bytes_freed"`
}

type Snapshot struct {
	Root      string        `json:"root"`
	ScannedAt time.Time     `json:"scanned_at"`
	Rows      []Row         `json:"rows"`
	LastPrune *PruneSummary `json:"last_prune,omitempty"`
}

func (d Dirs) Load() (Snapshot, error) {
	var snap Snapshot
	data, err := os.ReadFile(d.Snapshot())
	if err != nil {
		return snap, err
	}
	err = json.Unmarshal(data, &snap)
	return snap, err
}

// LoadFor returns the cached snapshot only if it was taken for root.
func (d Dirs) LoadFor(root string) (Snapshot, bool) {
	snap, err := d.Load()
	if err != nil || snap.Root != root {
		return Snapshot{}, false
	}
	return snap, true
}

func (d Dirs) Save(snap Snapshot) error {
	if err := os.MkdirAll(d.Root, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(d.Root, "last-scan-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), d.Snapshot())
}

var ErrLockTimeout = errors.New("timed out waiting for another chainsaw deletion to finish")

// Lock takes the exclusive deletion lock, calling onWait once if it has to
// wait. The returned func releases it.
func (d Dirs) Lock(timeout time.Duration, onWait func()) (func(), error) {
	if err := os.MkdirAll(d.Root, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(d.lockFile(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	waited := false
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				f.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			f.Close()
			return nil, fmt.Errorf("lock %s: %w", d.lockFile(), err)
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, ErrLockTimeout
		}
		if !waited && onWait != nil {
			onWait()
		}
		waited = true
		time.Sleep(500 * time.Millisecond)
	}
}

// CleanSalvage removes salvage batch directories older than maxAge.
func (d Dirs) CleanSalvage(now time.Time, maxAge time.Duration) error {
	entries, err := os.ReadDir(d.Salvage())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !e.IsDir() {
			continue
		}
		if now.Sub(info.ModTime()) > maxAge {
			if err := os.RemoveAll(filepath.Join(d.Salvage(), e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
