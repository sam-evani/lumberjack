package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sam-evani/lumberjack/internal/worktree"
	"github.com/spf13/cobra"
)

var rootPath string

var rootCmd = &cobra.Command{
	Use:           "chainsaw",
	Short:         "Prune git worktrees safely",
	Long:          "chainsaw finds git worktrees under a root directory, auto-prunes merged ones, and serves a local web UI for the rest.",
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServe(cmd)
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

func addPolicyFlags(cmd *cobra.Command, p *worktree.Policy) {
	cmd.Flags().DurationVar(&p.StaleAfter, "stale-after", p.StaleAfter, "Flag unmerged worktrees with no commits for this long")
	cmd.Flags().DurationVar(&p.DetachedAfter, "detached-after", p.DetachedAfter, "Auto-prune detached worktrees idle for this long")
}

func absRoot() (string, error) {
	return filepath.Abs(rootPath)
}
