# chainsaw: scheduled auto-prune + browser UI

Date: 2026-09-30
Status: approved design, pending implementation plan

## Problem

`~/co` holds ~400 GB of repos and linked worktrees (a full `du` takes ~96 s). Most of the waste is worktrees whose PRs already merged. The current `chainsaw` TUI works but is hard to read and requires manual sessions. We want:

1. A daily unattended run that removes merged worktrees and their branches.
2. A browser page to inspect everything else (sizes, status, resizable columns), select rows, delete them, and watch the result live.

## Goals / non-goals

Goals:
- Auto-delete every merged worktree, including dirty ones, without losing uncommitted work silently.
- Never touch protected worktrees.
- Show per-worktree size, share of total, and totals by category.
- Single Go binary, no Node toolchain.

Non-goals:
- Deleting remote branches.
- Deleting unmerged branches (worktree may be removed manually; branch is kept).
- Recovering gitignored files (build outputs, `node_modules`, etc.) from removed worktrees.
- Multi-user or remote access to the web UI.

## Commands

| Command | Purpose |
|---|---|
| `chainsaw prune [--root DIR] [--dry-run] [--stale-after 30d]` | Unattended cleanup of merged worktrees |
| `chainsaw serve [--root DIR] [--no-open]` | Local web UI |
| `chainsaw schedule install\|uninstall\|status` | Manage the launchd agent |
| `chainsaw list [--root DIR]` | Unchanged TSV output |

Running bare `chainsaw` prints help. The TUI (`internal/tui`) and the bubbletea/bubbles/lipgloss dependencies are removed.

## Classification

Existing `worktree.Classify` is unchanged. A new derived `Category` is computed in Go (so it is unit-testable and the UI only renders it):

Rules are evaluated top to bottom; first match wins.

| Category | Rule | Auto-pruned | Selectable in UI | Colour |
|---|---|---|---|---|
| `review` | `Error != ""` | no | no (row is protected by `Classify`) | amber |
| `protected` | `Protected == true` (primary checkout, main/master, locked, bare, detached, prunable, no base ref) | no | no | grey |
| `merged` | `Status == merged` | yes | yes | green |
| `review` | `Status == unmerged` and last commit older than `--stale-after` (default 30 days) | no | yes | amber |
| `active` | `Status == unmerged`, recent | no | yes | neutral |

Selectability comes from `Protected`, not from category. `Dirty` is shown as a separate pill, not a category.

## Deletion engine (`internal/prune`)

Shared by `prune` and `serve`. Replaces `internal/tui/git_deleter.go`.

For each worktree to delete, in order:
1. If dirty, write a salvage patch (below). On failure, skip this worktree and report the error. Never force-remove without a successful salvage.
2. `git worktree remove --force <path>` if dirty, else without `--force`.
3. Only if step 2 succeeded and the row is `merged`: `git branch -D <branch>`. Unmerged branches are kept and reported.

After all removals in a repo, run `git worktree prune` for that repo.

Execution: repos in parallel, worktrees within one repo sequentially (avoids contention on the repo's worktree metadata). Progress is emitted as events (`started`, `salvaged`, `removed`, `branch_deleted`, `branch_kept`, `failed`, `done`) to a callback so both the CLI logger and the SSE stream can consume them.

`worktree.PlanDeletion` is kept as the gate: every request (CLI or web) goes through it, so protected/unknown/detached/prunable rows are rejected regardless of caller. The "press D to force delete" skip reason is removed since dirty rows are always handled via salvage + force.

### Salvage patch

Path: `~/.local/state/chainsaw/salvage/<YYYYMMDD-HHMMSS>/<repo>--<branch-with-slashes-as-dashes>.patch`

Contents: tracked changes and untracked (non-ignored) files, captured without touching the worktree's real index:

```
GIT_INDEX_FILE=<tmpfile> git read-tree HEAD
GIT_INDEX_FILE=<tmpfile> git add -A
GIT_INDEX_FILE=<tmpfile> git diff --cached --binary HEAD
```

Header comment lines record the original path, branch, and HEAD SHA. Restore with `git apply` on a checkout of that SHA. Salvage directories older than 90 days are deleted at the start of each `prune` run.

## `chainsaw prune`

1. Acquire the lock (below). If held, wait up to 10 minutes, then exit non-zero.
2. Scan `--root` with the existing scanner.
3. Select all rows with category `merged`.
4. With `--dry-run`: print the plan and exit.
5. Otherwise run the deletion engine; log each event to stdout (launchd redirects to the log file).
6. Measure sizes for remaining rows and write `last-scan.json` (with `last_prune` summary: time, count removed, bytes freed, failures).
7. Post a macOS notification via `osascript -e 'display notification ...'`: "Pruned N worktrees, freed X GB" plus ", K failed" if any.
8. Exit non-zero if any worktree failed.

Bytes freed = sum of the cached sizes of removed worktrees (measured before removal if no cache entry exists).

## `chainsaw schedule`

`install` writes `~/Library/LaunchAgents/com.sam-evani.chainsaw.plist` and runs `launchctl bootstrap gui/$UID <plist>` (after `bootout` if already loaded):
- `ProgramArguments`: absolute path from `os.Executable()` at install time, `prune`, `--root`, the resolved root.
- `StartCalendarInterval`: Hour 10, Minute 0. launchd runs missed intervals on wake.
- `EnvironmentVariables.PATH`: `/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin` so `git` resolves.
- `StandardOutPath` / `StandardErrorPath`: `~/.local/state/chainsaw/prune.log`.

`uninstall` boots out and removes the plist. `status` reports installed/loaded, the binary path in the plist, and the `last_prune` summary from `last-scan.json`.

## Sizes (`internal/size`)

- Measured with `filepath.WalkDir`, summing on-disk usage (`Stat_t.Blocks * 512`), counting each inode once per worktree (hard links), not following symlinks.
- Linked worktrees don't contain the object store, so their size is their checkout plus artifacts. Primary checkouts include `.git`; they are shown (grey) for context and counted in the "total" but not in "reclaimable".
- Bounded worker pool (default `GOMAXPROCS`), one worktree per worker.
- Unreadable or vanished paths yield an unknown size (`null` in JSON, "?" in UI), counted as 0 with a warning.
- Results are cached in `last-scan.json` keyed by path, with a `measured_at` timestamp. Every scan re-measures; the cache exists so the UI can render instantly.

## State (`internal/state`)

Directory: `$XDG_STATE_HOME/chainsaw`, default `~/.local/state/chainsaw`.

- `last-scan.json`: root, scan time, rows (entry fields + category + size + measured_at), `last_prune` summary.
- `prune.log`: launchd output.
- `salvage/`: patches.
- `lock`: `flock`-based exclusive lock held for the duration of any deletion (prune run or web delete batch).

Writes to `last-scan.json` are atomic (write temp + rename).

## `chainsaw serve` (`internal/web`)

Startup:
1. Bind `127.0.0.1:0`, generate a 32-byte random token.
2. Load `last-scan.json` if its root matches; start a background rescan.
3. Open `http://127.0.0.1:<port>/?t=<token>` with `open` (skip with `--no-open`), print the URL.
4. Ctrl-C shuts down gracefully (waits for an in-flight delete batch).

Security:
- Every request needs the token (`X-Chainsaw-Token` header, or `t` query param for the page and the SSE stream since `EventSource` can't set headers).
- `Host` header must be `127.0.0.1:<port>` or `localhost:<port>` (DNS rebinding guard).
- Mutating endpoints are POST with JSON bodies only.

API:

| Method | Path | Purpose |
|---|---|---|
| GET | `/` | Embedded HTML page |
| GET | `/api/state` | Current rows, totals, scan status, last prune summary |
| POST | `/api/rescan` | Start a rescan (no-op if one is running) |
| POST | `/api/delete` | Body `{"paths": [...]}`; returns 202 and a batch id |
| GET | `/api/events` | SSE stream |

SSE events: `row` (added/updated row), `size` (path, bytes), `scan_progress` (measured/total), `scan_done`, `delete` (path, phase, error), `delete_done` (batch summary), `lock_wait`.

Delete handling:
1. Re-scan only the repos owning the requested paths.
2. Reject any path whose category changed since the client's view (client sends the category it saw; mismatch returns per-row error "changed since scan, rescan") or that is now protected.
3. Acquire the lock (emit `lock_wait` if blocked), run the deletion engine, stream events, remove deleted rows from state, update `last-scan.json`.

## Browser page

Single `index.html` with inline CSS/JS, embedded via `embed`. No framework, no build step.

Summary bar:
- Total size and a stacked bar by category (merged/review/active/protected).
- Reclaimable (non-protected) total.
- Selected count and size.
- Last scan time, last auto-prune summary, Rescan button, "measuring N / M" indicator.

Table columns: checkbox, repo, worktree, branch, category badge, dirty pill, size, share of total (inline bar), last commit age, last commit subject, path, protection reason / error.
- Column widths resizable by dragging header edges; cells wrap when widened; widths persisted in `localStorage`.
- Click header to sort; default sort size descending.
- Text filter across repo/branch/path/subject; chips to toggle each category.
- "Select all merged" and "Clear" buttons. Protected rows have disabled checkboxes.

Delete flow:
1. "Delete selected" opens a confirm dialog listing: worktrees to remove, branches to delete (merged), branches kept (unmerged), dirty worktrees that will be salvaged then force-removed.
2. On confirm, POST `/api/delete`. Rows show "deleting…", then fade out on success or turn red with the inline error on failure.
3. Totals update as rows are removed.

## Error handling summary

| Failure | Behaviour |
|---|---|
| Repo scan fails | One amber, unselectable error row for that repo; rest of scan continues |
| Size unreadable | "?" with warning, counted as 0 |
| Salvage fails | Worktree skipped, error reported, nothing removed |
| Worktree remove fails | Branch kept, error reported |
| Branch delete fails | Worktree already gone; error reported, branch listed |
| Lock held | `prune` waits ≤10 min then fails; web shows "waiting for lock" |
| Row changed since UI loaded | That row rejected with "changed since scan, rescan" |
| `osascript` fails | Logged, does not affect exit code |

## Package layout

```
cmd/            prune.go, serve.go, schedule.go, list.go, root.go
internal/git    + SalvagePatch, WorktreePrune
internal/scan   unchanged
internal/worktree + Category
internal/prune  deletion engine + events
internal/size   parallel disk usage
internal/state  paths, cache, lock
internal/launchd plist generation + launchctl wrapper
internal/web    handlers, SSE hub, embedded index.html
```

## Testing

Unit (temp git repos via existing `testhelp_test.go` helpers):
- `Category` rules including stale threshold and error-before-protected precedence.
- Deletion engine: merged-clean removed + branch deleted; merged-dirty salvaged then force-removed; salvage failure skips; unmerged removal keeps branch; protected rejected; per-repo sequential ordering.
- Salvage patch includes tracked edits and untracked files and applies cleanly onto the recorded SHA.
- Size: fake tree with hard links and symlinks; unreadable dir yields unknown.
- State: atomic write, cache load with mismatched root ignored, lock exclusion.
- launchd: generated plist content (no `launchctl` calls in tests; command runner injected).

HTTP (`httptest`):
- Missing/wrong token → 401; bad Host → 403.
- Delete of protected path rejected; category-mismatch rejected.
- SSE event order for a delete batch.

End-to-end: temp root with merged-clean, merged-dirty, squash-merged, unmerged, and main worktrees; run `prune`; assert remaining directories, branches, and salvage files.

Frontend: manual verification in the browser.
