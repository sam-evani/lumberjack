package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dustin/go-humanize"
	"github.com/sam-evani/lumberjack/internal/worktree"
)

const noSelectionMessage = "select one or more worktrees first"

type Deleter interface {
	RemoveWorktree(action worktree.RemoveWorktreeAction) error
	DeleteBranch(action worktree.BranchAction) error
}

type noopDeleter struct{}

func (noopDeleter) RemoveWorktree(worktree.RemoveWorktreeAction) error { return nil }
func (noopDeleter) DeleteBranch(worktree.BranchAction) error           { return nil }

type sortMode int

const (
	sortRepo sortMode = iota
	sortPath
	sortStatus
	sortAge
)

type Model struct {
	rows     []worktree.Entry
	visible  []int
	selected map[string]bool

	cursor int
	offset int
	width  int
	height int

	filtering bool
	filter    string
	sortMode  sortMode

	message    string
	confirming bool
	force      bool
	pending    worktree.DeletionPlan
	deleter    Deleter
}

func NewModel(rows []worktree.Entry) Model {
	return NewModelWithDeleter(rows, noopDeleter{})
}

func NewModelWithDeleter(rows []worktree.Entry, deleter Deleter) Model {
	if deleter == nil {
		deleter = noopDeleter{}
	}
	model := Model{
		rows:     append([]worktree.Entry(nil), rows...),
		selected: map[string]bool{},
		width:    120,
		height:   24,
		deleter:  deleter,
	}
	model.rebuildVisible()
	return model
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) SelectedPaths() []string {
	paths := make([]string, 0, len(m.selected))
	for _, row := range m.rows {
		if m.selected[row.Path] {
			paths = append(paths, row.Path)
		}
	}
	return paths
}

func (m Model) Message() string {
	return m.message
}

func (m Model) Confirming() bool {
	return m.confirming
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampCursor()
		return m, nil
	case tea.KeyMsg:
		if m.confirming {
			return m.updateConfirming(msg)
		}
		if m.filtering {
			return m.updateFiltering(msg)
		}
		return m.updateTable(msg)
	default:
		return m, nil
	}
}

func (m Model) View() string {
	if m.confirming {
		return m.confirmationView()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", styles.title.Render("chainsaw"))
	if m.filtering || m.filter != "" {
		fmt.Fprintf(&b, "filter: %s\n", m.filter)
	}
	if m.message != "" {
		fmt.Fprintf(&b, "%s\n", styles.warning.Render(m.message))
	}
	fmt.Fprintf(&b, "%s\n", styles.header.Render(m.header()))

	bodyHeight := m.bodyHeight()
	for row := 0; row < bodyHeight && m.offset+row < len(m.visible); row++ {
		index := m.visible[m.offset+row]
		line := m.renderRow(index)
		if m.offset+row == m.cursor {
			line = styles.selected.Render(line)
		}
		fmt.Fprintf(&b, "%s\n", line)
	}

	if len(m.visible) == 0 {
		fmt.Fprintf(&b, "%s\n", styles.muted.Render("no worktrees"))
	}

	fmt.Fprintf(&b, "%s", styles.muted.Render("space select  a select safe visible  c clear  / filter  s sort  d delete  D force  q quit"))
	return b.String()
}

func (m Model) updateTable(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, keys.Quit):
		return m, tea.Quit
	case keyMatches(msg, keys.Up):
		m.cursor--
	case keyMatches(msg, keys.Down):
		m.cursor++
	case keyMatches(msg, keys.PageUp):
		m.cursor -= m.bodyHeight()
	case keyMatches(msg, keys.PageDown):
		m.cursor += m.bodyHeight()
	case keyMatches(msg, keys.Home):
		m.cursor = 0
	case keyMatches(msg, keys.End):
		m.cursor = len(m.visible) - 1
	case keyMatches(msg, keys.Select):
		m.toggleCurrent()
	case keyMatches(msg, keys.SelectAll):
		m.selectAllVisible()
	case keyMatches(msg, keys.Clear):
		m.selected = map[string]bool{}
		m.message = "selection cleared"
	case keyMatches(msg, keys.Filter):
		m.filtering = true
		m.message = ""
	case keyMatches(msg, keys.Sort):
		m.sortMode = (m.sortMode + 1) % 4
		m.rebuildVisible()
	case keyMatches(msg, keys.Delete):
		m.beginConfirm(worktree.DeleteNormal)
	case keyMatches(msg, keys.ForceDelete):
		m.beginConfirm(worktree.DeleteForce)
	}
	m.clampCursor()
	return m, nil
}

func (m Model) updateFiltering(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch {
	case keyMatches(msg, keys.Quit):
		return m, tea.Quit
	case keyMatches(msg, keys.Cancel), key == "enter":
		m.filtering = false
	case key == "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.rebuildVisible()
		}
	case key == "ctrl+w":
		m.filter = ""
		m.rebuildVisible()
	case len(msg.Runes) > 0:
		m.filter += string(msg.Runes)
		m.rebuildVisible()
	}
	m.clampCursor()
	return m, nil
}

func (m Model) updateConfirming(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, keys.Quit):
		return m, tea.Quit
	case keyMatches(msg, keys.Cancel):
		m.confirming = false
		m.message = "delete canceled"
	case keyMatches(msg, keys.Confirm):
		m.confirming = false
		m.executePendingDeletion()
	}
	return m, nil
}

func (m *Model) toggleCurrent() {
	if len(m.visible) == 0 {
		return
	}
	path := m.rows[m.visible[m.cursor]].Path
	if m.selected[path] {
		delete(m.selected, path)
		m.message = "row unselected"
		return
	}
	m.selected[path] = true
	m.message = "row selected"
}

func (m *Model) selectAllVisible() {
	count := 0
	for _, index := range m.visible {
		row := m.rows[index]
		if !safeForSelectAll(row) {
			continue
		}
		m.selected[row.Path] = true
		count++
	}
	if count == 0 {
		m.message = "no safe visible worktrees to select"
		return
	}
	m.message = fmt.Sprintf("selected %d safe visible worktrees", count)
}

func (m *Model) beginConfirm(mode worktree.DeleteMode) {
	if len(m.selected) == 0 {
		m.message = noSelectionMessage
		m.confirming = false
		return
	}
	m.force = mode == worktree.DeleteForce
	m.pending = worktree.PlanDeletion(m.rows, m.selected, mode)
	m.confirming = true
	m.message = ""
}

func (m *Model) executePendingDeletion() {
	var errors []string
	removedPaths := map[string]bool{}

	for _, action := range m.pending.RemoveWorktrees {
		if err := m.deleter.RemoveWorktree(action); err != nil {
			errors = append(errors, fmt.Sprintf("remove %s: %v", action.Path, err))
			continue
		}
		removedPaths[action.Path] = true
	}

	deletedBranches := 0
	for _, action := range m.pending.DeleteBranches {
		if !m.branchWorktreeRemoved(action, removedPaths) {
			continue
		}
		if err := m.deleter.DeleteBranch(action); err != nil {
			errors = append(errors, fmt.Sprintf("delete branch %s in %s: %v", action.Branch, action.RepoPath, err))
			continue
		}
		deletedBranches++
	}

	if len(removedPaths) > 0 {
		m.dropRemovedRows(removedPaths)
	}
	if len(errors) > 0 {
		m.message = "errors: " + strings.Join(errors, "; ")
		return
	}
	m.message = fmt.Sprintf("deleted %d worktree(s), %d branch(es)", len(removedPaths), deletedBranches)
}

func (m Model) branchWorktreeRemoved(action worktree.BranchAction, removedPaths map[string]bool) bool {
	for _, row := range m.rows {
		if row.RepoPath == action.RepoPath && row.Branch == action.Branch && removedPaths[row.Path] {
			return true
		}
	}
	return false
}

func (m *Model) dropRemovedRows(removedPaths map[string]bool) {
	rows := m.rows[:0]
	for _, row := range m.rows {
		if removedPaths[row.Path] {
			delete(m.selected, row.Path)
			continue
		}
		rows = append(rows, row)
	}
	m.rows = rows
	m.rebuildVisible()
}

func (m *Model) rebuildVisible() {
	filter := strings.ToLower(strings.TrimSpace(m.filter))
	m.visible = m.visible[:0]
	for i, row := range m.rows {
		if filter == "" || rowMatches(row, filter) {
			m.visible = append(m.visible, i)
		}
	}

	sort.SliceStable(m.visible, func(i, j int) bool {
		left := m.rows[m.visible[i]]
		right := m.rows[m.visible[j]]
		switch m.sortMode {
		case sortPath:
			return left.Path < right.Path
		case sortStatus:
			if left.Status == right.Status {
				return left.Path < right.Path
			}
			return left.Status < right.Status
		case sortAge:
			return left.LastCommitTime.Before(right.LastCommitTime)
		default:
			if left.RepoName == right.RepoName {
				return left.Path < right.Path
			}
			return left.RepoName < right.RepoName
		}
	})
	m.clampCursor()
}

func (m *Model) clampCursor() {
	if len(m.visible) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}

	height := m.bodyHeight()
	if height < 1 {
		height = 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+height {
		m.offset = m.cursor - height + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m Model) bodyHeight() int {
	height := m.height - 6
	if m.message != "" {
		height--
	}
	if m.filtering || m.filter != "" {
		height--
	}
	if height < 3 {
		return 3
	}
	return height
}

func (m Model) header() string {
	return fmt.Sprintf("%-3s %-28s %-16s %-18s %-16s %-5s %-10s %s", "sel", "path", "repo", "branch", "status", "dirty", "age", "last commit")
}

func (m Model) renderRow(index int) string {
	row := m.rows[index]
	selected := " "
	if m.selected[row.Path] {
		selected = "*"
	}
	status := string(row.Status)
	if row.Protected {
		status = "protected"
	}
	if row.Error != "" {
		status = "error"
	}
	return fmt.Sprintf("%-3s %-28s %-16s %-18s %-16s %-5t %-10s %s",
		selected,
		truncate(displayPath(row), 28),
		truncate(row.RepoName, 16),
		truncate(branch(row), 18),
		truncate(status, 16),
		row.Dirty,
		truncate(age(row.LastCommitTime), 10),
		truncate(row.LastCommitTitle, max(20, m.width-106)),
	)
}

func (m Model) confirmationView() string {
	var b strings.Builder
	mode := "delete"
	if m.force {
		mode = "force delete"
	}
	fmt.Fprintf(&b, "%s\n\n", styles.title.Render("Confirm "+mode))
	fmt.Fprintf(&b, "worktrees: %d\n", len(m.pending.RemoveWorktrees))
	fmt.Fprintf(&b, "branches to delete: %d\n", len(m.pending.DeleteBranches))
	fmt.Fprintf(&b, "branches to keep: %d\n", len(m.pending.KeepBranches))
	fmt.Fprintf(&b, "skipped: %d\n\n", len(m.pending.Skipped))
	for _, action := range m.pending.RemoveWorktrees {
		fmt.Fprintf(&b, "remove %s\n", action.Path)
	}
	for _, action := range m.pending.DeleteBranches {
		fmt.Fprintf(&b, "delete branch %s in %s\n", action.Branch, action.RepoPath)
	}
	for _, action := range m.pending.KeepBranches {
		fmt.Fprintf(&b, "keep branch %s in %s\n", action.Branch, action.RepoPath)
	}
	for _, skipped := range m.pending.Skipped {
		fmt.Fprintf(&b, "skip %s: %s\n", skipped.Entry.Path, skipped.Reason)
	}
	fmt.Fprintf(&b, "\n%s", styles.muted.Render("enter/y preview only  esc/n cancel  q quit"))
	return b.String()
}

func rowMatches(row worktree.Entry, filter string) bool {
	values := []string{
		row.RepoName,
		row.Path,
		row.DisplayName,
		row.Branch,
		string(row.Status),
		row.LastCommitTitle,
		row.Error,
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), filter) {
			return true
		}
	}
	return false
}

func safeForSelectAll(row worktree.Entry) bool {
	if row.Dirty || row.Protected || row.Locked || row.Bare {
		return false
	}
	switch row.Status {
	case worktree.StatusDetached, worktree.StatusUnknown, worktree.StatusPrunable:
		return false
	default:
		return true
	}
}

func displayPath(row worktree.Entry) string {
	if row.DisplayName != "" {
		return row.DisplayName
	}
	return row.Path
}

func branch(row worktree.Entry) string {
	if row.Branch != "" {
		return row.Branch
	}
	if row.Detached {
		return "(detached)"
	}
	return ""
}

func age(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return humanize.Time(t)
}

func truncate(value string, width int) string {
	if width <= 0 || len(value) <= width {
		return value
	}
	if width <= 1 {
		return value[:width]
	}
	return value[:width-1] + "."
}
