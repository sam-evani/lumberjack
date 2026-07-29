package tui

import "github.com/charmbracelet/lipgloss"

var styles = struct {
	title    lipgloss.Style
	header   lipgloss.Style
	selected lipgloss.Style
	muted    lipgloss.Style
	warning  lipgloss.Style
}{
	title:    lipgloss.NewStyle().Bold(true),
	header:   lipgloss.NewStyle().Bold(true).Underline(true),
	selected: lipgloss.NewStyle().Bold(true),
	muted:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	warning:  lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
}
