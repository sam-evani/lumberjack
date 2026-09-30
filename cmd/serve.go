package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/sam-evani/lumberjack/internal/prune"
	"github.com/sam-evani/lumberjack/internal/scan"
	"github.com/sam-evani/lumberjack/internal/state"
	"github.com/sam-evani/lumberjack/internal/web"
	"github.com/sam-evani/lumberjack/internal/worktree"
	"github.com/spf13/cobra"
)

var (
	serveNoOpen bool
	servePort   int
	servePolicy = worktree.DefaultPolicy()
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Open the web UI (same as running chainsaw with no command)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServe(cmd)
	},
}

var runServe = func(cmd *cobra.Command) error {
	root, err := absRoot()
	if err != nil {
		return err
	}
	srv, err := web.NewServer(web.Config{
		Root:     root,
		Dirs:     state.Default(),
		Policy:   servePolicy,
		Scan:     scan.Default,
		ScanRepo: scan.DefaultRepo,
		Runner:   prune.GitRunner{},
	})
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", servePort))
	if err != nil {
		return err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	srv.SetPort(port)
	url := fmt.Sprintf("http://127.0.0.1:%d/?t=%s", port, srv.Token())
	fmt.Fprintf(cmd.OutOrStdout(), "chainsaw serving %s\n  %s\nCtrl-C to stop.\n", root, url)

	httpSrv := &http.Server{Handler: srv.Handler()}
	errc := make(chan error, 1)
	go func() { errc <- httpSrv.Serve(ln) }()
	srv.StartRescan()
	if !serveNoOpen {
		exec.Command("open", url).Start()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	httpSrv.Shutdown(shutdownCtx)
	srv.WaitIdle()
	return nil
}

func addServeFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&serveNoOpen, "no-open", false, "Don't open the browser")
	cmd.Flags().IntVar(&servePort, "port", 0, "Port to listen on (default: random)")
	addPolicyFlags(cmd, &servePolicy)
}

func init() {
	addServeFlags(serveCmd)
	addServeFlags(rootCmd)
	rootCmd.AddCommand(serveCmd)
}
