package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/sam-evani/lumberjack/internal/scan"
	"github.com/spf13/cobra"
)

var (
	listRows = scan.Default
	listNow  = time.Now
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Print worktrees as TSV",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		rows, err := listRows(rootPath)
		if err != nil {
			return err
		}

		w := cmd.OutOrStdout()
		for _, row := range rows {
			age := int64(0)
			if !row.LastCommitTime.IsZero() {
				age = int64(listNow().Sub(row.LastCommitTime).Seconds())
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\t%d\t%s\t%t\t%s\n", row.RepoName, row.Path, row.Branch, row.Status, row.Dirty, age, tsvCell(row.LastCommitTitle), row.Protected, tsvCell(row.Error))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func tsvCell(value string) string {
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "\r", " ")
}
