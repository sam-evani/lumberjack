package size

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
)

// Measure returns on-disk usage of root in bytes, counting each inode once and
// not descending into any directory listed in skip (other worktrees nested
// inside a primary checkout).
func Measure(root string, skip map[string]bool) (int64, error) {
	var total int64
	seen := map[uint64]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			return nil
		}
		if d.IsDir() && path != root && skip[path] {
			return filepath.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			total += info.Size()
			return nil
		}
		if st.Nlink > 1 {
			if seen[st.Ino] {
				return nil
			}
			seen[st.Ino] = true
		}
		total += st.Blocks * 512
		return nil
	})
	return total, err
}

type Result struct {
	Path  string
	Bytes int64
	Err   error
}

// MeasureAll measures each path concurrently and calls onResult as each
// finishes. Every path in paths is skipped when measuring the others.
func MeasureAll(paths []string, onResult func(Result)) {
	skip := make(map[string]bool, len(paths))
	for _, p := range paths {
		skip[p] = true
	}
	work := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := 0; i < runtime.GOMAXPROCS(0); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range work {
				n, err := Measure(path, skip)
				mu.Lock()
				onResult(Result{Path: path, Bytes: n, Err: err})
				mu.Unlock()
			}
		}()
	}
	for _, p := range paths {
		work <- p
	}
	close(work)
	wg.Wait()
}
