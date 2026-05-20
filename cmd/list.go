package cmd

import (
	"fmt"
	"time"

	"github.com/sam-evani/lumberjack/internal/scan"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Print worktrees as TSV",
	RunE: func(cmd *cobra.Command, args []string) error {
		rows, err := scan.Default(rootPath)
		if err != nil {
			return err
		}

		w := cmd.OutOrStdout()
		for _, row := range rows {
			age := int64(0)
			if !row.LastCommitTime.IsZero() {
				age = int64(time.Since(row.LastCommitTime).Seconds())
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\t%d\t%s\n", row.RepoName, row.Path, row.Branch, row.Status, row.Dirty, age, row.LastCommitTitle)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
