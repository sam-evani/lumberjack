package size

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMeasureCountsHardLinksOnceAndSkipsNestedAndSymlinks(t *testing.T) {
	root := t.TempDir()
	big := make([]byte, 64*1024)
	if err := os.WriteFile(filepath.Join(root, "a"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	baseline, err := Measure(root, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Link(filepath.Join(root, "a"), filepath.Join(root, "a-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/usr", filepath.Join(root, "sym")); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "b"), big, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Measure(root, map[string]bool{nested: true})
	if err != nil {
		t.Fatal(err)
	}
	// Allow for the directory entry itself plus the symlink's blocks.
	if got < baseline || got > baseline+16*1024 {
		t.Fatalf("Measure = %d, want about %d", got, baseline)
	}

	withNested, err := Measure(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if withNested < got+int64(len(big)) {
		t.Fatalf("Measure without skip = %d, want at least %d", withNested, got+int64(len(big)))
	}
}

func TestMeasureMissingPathErrors(t *testing.T) {
	if _, err := Measure(filepath.Join(t.TempDir(), "gone"), nil); err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestMeasureAllReportsEveryPath(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	got := map[string]bool{}
	MeasureAll([]string{a, b}, func(r Result) {
		if r.Err != nil {
			t.Errorf("%s: %v", r.Path, r.Err)
		}
		got[r.Path] = true
	})
	if !got[a] || !got[b] {
		t.Fatalf("results = %v", got)
	}
}
