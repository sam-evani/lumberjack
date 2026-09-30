package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dustin/go-humanize"
	"github.com/sam-evani/lumberjack/internal/launchd"
	"github.com/sam-evani/lumberjack/internal/state"
	"github.com/spf13/cobra"
)

var (
	scheduleHour   int
	scheduleMinute int
	launchctl      launchd.Launchctl = launchd.SystemLaunchctl
)

var scheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Manage the daily auto-prune launchd agent",
}

var scheduleInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and load the daily auto-prune agent",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := absRoot()
		if err != nil {
			return err
		}
		bin, err := os.Executable()
		if err != nil {
			return err
		}
		if bin, err = filepath.EvalSymlinks(bin); err != nil {
			return err
		}
		path, err := launchd.Install(launchd.Config{Binary: bin, Root: root, LogPath: state.Default().Log(), Hour: scheduleHour, Minute: scheduleMinute}, launchctl)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "installed %s\n  runs %s prune --root %s daily at %02d:%02d\n  log: %s\n", path, bin, root, scheduleHour, scheduleMinute, state.Default().Log())
		return nil
	},
}

var scheduleUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Unload and remove the auto-prune agent",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := launchd.Uninstall(launchctl)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", path)
		return nil
	},
}

var scheduleStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether the agent is installed and the last prune result",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		w := cmd.OutOrStdout()
		path, err := launchd.PlistPath()
		if err != nil {
			return err
		}
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintln(w, "not installed (run: chainsaw schedule install)")
		} else {
			fmt.Fprintf(w, "installed: %s (loaded: %t)\n", path, launchd.Loaded(launchctl))
		}
		fmt.Fprintf(w, "log: %s\n", state.Default().Log())
		if snap, err := state.Default().Load(); err == nil && snap.LastPrune != nil {
			lp := snap.LastPrune
			fmt.Fprintf(w, "last prune: %s, removed %d, freed %s, failed %d\n", lp.At.Format("2006-01-02 15:04"), lp.Removed, humanize.Bytes(uint64(lp.BytesFreed)), lp.Failed)
		} else {
			fmt.Fprintln(w, "last prune: never")
		}
		return nil
	},
}

func init() {
	scheduleInstallCmd.Flags().IntVar(&scheduleHour, "hour", 10, "Hour of day to run (0-23)")
	scheduleInstallCmd.Flags().IntVar(&scheduleMinute, "minute", 0, "Minute of hour to run")
	scheduleCmd.AddCommand(scheduleInstallCmd, scheduleUninstallCmd, scheduleStatusCmd)
	rootCmd.AddCommand(scheduleCmd)
}
