package cmd

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/sam-evani/lumberjack/internal/prune"
	"github.com/sam-evani/lumberjack/internal/scan"
	"github.com/sam-evani/lumberjack/internal/size"
	"github.com/sam-evani/lumberjack/internal/state"
	"github.com/sam-evani/lumberjack/internal/worktree"
	"github.com/spf13/cobra"
)

const salvageRetention = 90 * 24 * time.Hour

var (
	pruneDryRun bool
	prunePolicy = worktree.DefaultPolicy()
	pruneNotify bool

	pruneScan                = scan.Default
	pruneRunner prune.Runner = prune.GitRunner{}
	pruneDirs                = state.Default
	pruneNow                 = time.Now
	notify                   = func(msg string) error {
		script := fmt.Sprintf("display notification %q with title \"chainsaw\"", msg)
		return exec.Command("osascript", "-e", script).Run()
	}
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove every merged worktree and its branch (dirty ones are salvaged first)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := absRoot()
		if err != nil {
			return err
		}
		return runPrune(cmd.OutOrStdout(), root)
	},
}

func init() {
	pruneCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false, "Print what would be removed without deleting anything")
	addPolicyFlags(pruneCmd, &prunePolicy)
	pruneCmd.Flags().BoolVar(&pruneNotify, "notify", true, "Post a macOS notification with the result")
	rootCmd.AddCommand(pruneCmd)
}

func runPrune(w io.Writer, root string) error {
	dirs := pruneDirs()
	now := pruneNow()
	logf := func(format string, a ...any) {
		fmt.Fprintf(w, "%s "+format+"\n", append([]any{pruneNow().Format(time.RFC3339)}, a...)...)
	}

	if !pruneDryRun {
		release, err := dirs.Lock(10*time.Minute, func() { logf("waiting for another chainsaw deletion to finish") })
		if err != nil {
			return err
		}
		defer release()
		if err := dirs.CleanSalvage(now, salvageRetention); err != nil {
			logf("clean salvage: %v", err)
		}
	}

	logf("scanning %s", root)
	entries, err := pruneScan(root)
	if err != nil {
		return err
	}
	selected := map[string]bool{}
	var targets []worktree.Entry
	for _, e := range entries {
		if prunePolicy.AutoPrune(e, now) {
			selected[e.Path] = true
			targets = append(targets, e)
		}
	}

	cached := map[string]state.Row{}
	if snap, ok := dirs.LoadFor(root); ok {
		for _, r := range snap.Rows {
			cached[r.Path] = r
		}
	}
	sizes := map[string]int64{}
	var unmeasured []string
	for _, e := range targets {
		if r, ok := cached[e.Path]; ok && r.SizeBytes != nil {
			sizes[e.Path] = *r.SizeBytes
		} else {
			unmeasured = append(unmeasured, e.Path)
		}
	}
	size.MeasureAll(unmeasured, func(r size.Result) {
		if r.Err == nil {
			sizes[r.Path] = r.Bytes
		}
	})

	if pruneDryRun {
		var total int64
		for _, e := range targets {
			label := e.Branch
			var notes []string
			if e.Detached {
				label = "detached, idle since " + e.Activity().Format("2006-01-02")
				if e.HeadOrphan {
					notes = append(notes, "commits on no branch: would keep "+prune.BackupRefName(e))
				}
			}
			if e.Dirty {
				notes = append(notes, "dirty: would salvage + force")
			}
			note := ""
			if len(notes) > 0 {
				note = " (" + strings.Join(notes, "; ") + ")"
			}
			fmt.Fprintf(w, "would remove %s [%s] %s%s\n", e.Path, label, humanize.Bytes(uint64(sizes[e.Path])), note)
			total += sizes[e.Path]
		}
		fmt.Fprintf(w, "%d worktrees, %s\n", len(targets), humanize.Bytes(uint64(total)))
		return nil
	}

	salvageDir := filepath.Join(dirs.Salvage(), now.Format("20060102-150405"))
	res := prune.Execute(entries, selected, pruneRunner, salvageDir, func(ev prune.Event) {
		if ev.Message != "" {
			logf("%s %s [%s]: %s", ev.Kind, ev.Path, ev.Branch, ev.Message)
		} else {
			logf("%s %s [%s]", ev.Kind, ev.Path, ev.Branch)
		}
	})

	stale := map[string]bool{}
	for _, e := range entries {
		if e.Prunable {
			stale[e.RepoPath] = true
		}
	}
	for repoPath := range stale {
		if err := pruneRunner.PruneWorktrees(repoPath); err != nil {
			logf("git worktree prune %s: %v", repoPath, err)
			delete(stale, repoPath)
		} else {
			logf("pruned stale worktree metadata in %s", repoPath)
		}
	}

	removed := map[string]bool{}
	var freed int64
	for _, p := range res.Removed {
		removed[p] = true
		freed += sizes[p]
	}
	summary := &state.PruneSummary{At: now, Removed: len(res.Removed), Failed: len(res.Failed), BytesFreed: freed}

	var remaining []worktree.Entry
	var paths []string
	for _, e := range entries {
		if !removed[e.Path] && !(e.Prunable && stale[e.RepoPath]) {
			remaining = append(remaining, e)
			paths = append(paths, e.Path)
		}
	}
	logf("measuring %d remaining worktrees", len(paths))
	measured := map[string]size.Result{}
	size.MeasureAll(paths, func(r size.Result) { measured[r.Path] = r })
	rows := make([]state.Row, 0, len(remaining))
	for _, e := range remaining {
		row := state.NewRow(e, now, prunePolicy)
		if m := measured[e.Path]; m.Err == nil {
			n := m.Bytes
			row.SizeBytes = &n
		} else {
			row.SizeError = m.Err.Error()
		}
		row.MeasuredAt = pruneNow()
		rows = append(rows, row)
	}
	if err := dirs.Save(state.Snapshot{Root: root, ScannedAt: now, Rows: rows, LastPrune: summary}); err != nil {
		logf("save snapshot: %v", err)
	}

	msg := fmt.Sprintf("Pruned %d worktrees, freed %s", summary.Removed, humanize.Bytes(uint64(freed)))
	if summary.Failed > 0 {
		msg += fmt.Sprintf(", %d failed", summary.Failed)
	}
	logf("%s", msg)
	if pruneNotify && (summary.Removed > 0 || summary.Failed > 0) {
		if err := notify(msg); err != nil {
			logf("notify: %v", err)
		}
	}
	if summary.Failed > 0 {
		return fmt.Errorf("%d worktree(s) failed: %s", summary.Failed, strings.Join(res.Failed, ", "))
	}
	return nil
}
