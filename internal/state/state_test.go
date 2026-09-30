package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadRoundTripAndRootMismatch(t *testing.T) {
	d := Dirs{Root: t.TempDir()}
	n := int64(42)
	snap := Snapshot{Root: "/co", ScannedAt: time.Unix(100, 0).UTC(), Rows: []Row{{Path: "/co/a", SizeBytes: &n}}}
	if err := d.Save(snap); err != nil {
		t.Fatal(err)
	}
	got, ok := d.LoadFor("/co")
	if !ok || len(got.Rows) != 1 || *got.Rows[0].SizeBytes != 42 {
		t.Fatalf("LoadFor = %#v, %v", got, ok)
	}
	if _, ok := d.LoadFor("/elsewhere"); ok {
		t.Fatal("expected mismatched root to be ignored")
	}
	leftovers, _ := filepath.Glob(filepath.Join(d.Root, "last-scan-*.json"))
	if len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}

func TestLockIsExclusive(t *testing.T) {
	d := Dirs{Root: t.TempDir()}
	release, err := d.Lock(time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	waited := false
	if _, err := d.Lock(600*time.Millisecond, func() { waited = true }); err != ErrLockTimeout {
		t.Fatalf("second Lock err = %v, want timeout", err)
	}
	if !waited {
		t.Fatal("expected onWait to be called")
	}
	release()
	release2, err := d.Lock(time.Second, nil)
	if err != nil {
		t.Fatalf("Lock after release: %v", err)
	}
	release2()
}

func TestCleanSalvageRemovesOldBatches(t *testing.T) {
	d := Dirs{Root: t.TempDir()}
	oldDir := filepath.Join(d.Salvage(), "old")
	newDir := filepath.Join(d.Salvage(), "new")
	for _, dir := range []string{oldDir, newDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	past := now.Add(-100 * 24 * time.Hour)
	if err := os.Chtimes(oldDir, past, past); err != nil {
		t.Fatal(err)
	}
	if err := d.CleanSalvage(now, 90*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("old batch still present: %v", err)
	}
	if _, err := os.Stat(newDir); err != nil {
		t.Fatalf("new batch removed: %v", err)
	}
}
