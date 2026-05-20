package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sam-evani/lumberjack/internal/scan"
	"github.com/sam-evani/lumberjack/internal/tui"
	"github.com/sam-evani/lumberjack/internal/worktree"
	"github.com/spf13/cobra"
)

var rootPath string

var (
	rootRows = scan.Default
	runTUI   = func(rows []worktree.Entry, deleter tui.Deleter, refresh tui.RefreshFunc) error {
		_, err := tea.NewProgram(tui.NewModelWithOptions(rows, deleter, refresh), tea.WithAltScreen()).Run()
		return err
	}
)

var rootCmd = &cobra.Command{
	Use:           "chainsaw",
	Short:         "Prune git worktrees safely",
	Long:          "chainsaw is an interactive TUI for inspecting and pruning git worktrees.",
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		requestedRoot := rootPath
		rows, err := rootRows(requestedRoot)
		if err != nil {
			return err
		}
		refresh := func() ([]worktree.Entry, error) {
			return rootRows(requestedRoot)
		}
		return runTUI(rows, tui.GitDeleter{}, refresh)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&rootPath, "root", defaultRoot(), "Root path containing git repositories")
}

func defaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "co")
}
