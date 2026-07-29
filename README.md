# lumberjack

`lumberjack` builds `chainsaw`, a TUI for manually selecting and pruning git worktrees.

It is designed for cleaning up worktrees under `$HOME/co`.

## Install

```bash
go install github.com/sam-evani/lumberjack/cmd/chainsaw@latest
```

## Usage

Open the TUI:

```bash
chainsaw
```

Scan a different root:

```bash
chainsaw --root ~/co
```

Print TSV output:

```bash
chainsaw list --root ~/co
```

Build locally:

```bash
go build -o chainsaw ./cmd/chainsaw
```

The TSV columns are:

```text
repo	path	branch	status	dirty	age_seconds	last_commit	protected	error
```

## TUI Keys

| Key | Action |
|---|---|
| `up/down` or `j/k` | move cursor |
| `space` | toggle selected worktree |
| `a` | select all visible safe candidates |
| `c` | clear selection |
| `/` | filter |
| `s` | cycle sort |
| `r` | refresh scan |
| `d` | delete selected clean worktrees |
| `D` | force-delete selected dirty worktrees |
| `enter` or `y` | confirm deletion |
| `esc` or `n` | cancel confirmation |
| `q` | quit |

## Safety Rules

- Delete actions only operate on manually selected rows.
- Normal delete skips dirty worktrees.
- Force delete is a separate action for dirty worktrees.
- When normal delete skips selected dirty worktrees, press `D` from the confirmation screen to switch to force delete for the same selection.
- Merged branches are deleted after their worktree is removed.
- Branch deletion only runs after the matching worktree removal succeeds.
- Unmerged branches are kept.
- Remote branches are not deleted.
- Protected, detached, prunable, unknown, locked, bare, and scan-error rows are skipped.
