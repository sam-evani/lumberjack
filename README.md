# lumberjack

`lumberjack` builds `chainsaw`, which reclaims disk space from git worktrees under `$HOME/co`:

- `chainsaw prune` removes every worktree whose branch is merged into main/master (including squash merges), then deletes the branch. Meant to run daily via launchd.
- `chainsaw serve` opens a local web UI showing every worktree's status and size, so you can pick the rest by hand and watch them disappear.

## Install

```bash
go install github.com/sam-evani/lumberjack/cmd/chainsaw@latest
chainsaw schedule install      # daily auto-prune at 10:00
```

## Usage

```bash
chainsaw serve                 # web UI (random localhost port, opens browser)
chainsaw prune --dry-run       # show what the daily run would remove
chainsaw prune                 # run the cleanup now
chainsaw schedule status       # is the agent loaded, and what did the last run do
chainsaw schedule uninstall
chainsaw list                  # TSV output
```

All commands accept `--root DIR` (default `~/co`). `prune` and `serve` accept `--stale-after` (default `720h`) to control when unmerged worktrees are flagged for review.

## Categories

| Category | Meaning | Auto-pruned | Deletable in UI |
|---|---|---|---|
| merged (green) | Branch merged into main/master | yes | yes |
| needs review (amber) | Unmerged with no commits in `--stale-after`, or the scan hit an error | no | stale: yes, error: no |
| active (blue) | Unmerged, recent | no | yes |
| protected (grey) | Primary checkout, main/master, locked, bare, detached, prunable | no | no |

## Safety rules

- Protected rows are never deleted, by `prune` or the UI; the server re-checks every request.
- Worktrees with uncommitted changes are salvaged before being force-removed: tracked edits and untracked (non-ignored) files are written to `~/.local/state/chainsaw/salvage/<timestamp>/<repo>--<branch>.patch`. If salvage fails, the worktree is left alone. Gitignored files (build output, `node_modules`) are not saved. Salvage batches older than 90 days are deleted.
- Branches are deleted only if merged and only after their worktree is removed. Unmerged branches are kept. Remote branches are never touched.
- The UI re-scans affected repos right before deleting and rejects any row whose status changed since the page loaded.
- `prune` and UI deletions share a lock, so they never run at the same time.
- The web server binds to `127.0.0.1` only and requires the random token in the URL it opens.

To restore salvaged work:

```bash
git -C <repo> worktree add <dir> <head-sha-from-patch-header>
git -C <dir> apply <patch>
```

## Files

`~/.local/state/chainsaw/` (or `$XDG_STATE_HOME/chainsaw`):

- `last-scan.json` — cached scan and sizes so the UI opens instantly
- `prune.log` — output of scheduled runs
- `salvage/` — patches of uncommitted work from removed worktrees

## Build

```bash
go build -o chainsaw ./cmd/chainsaw
go test ./...
```
