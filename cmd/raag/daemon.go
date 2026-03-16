package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/p-society/raag/app"
	"github.com/p-society/raag/internal/config"
	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/rpc"
	"github.com/spf13/cobra"
)

var daemonTrackerURL string

func daemonCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run raag as a background daemon",
		Long: `Start the raag daemon in background mode.

The daemon maintains live peer connections and exposes a Unix socket
for fast CLI queries. Use 'raag peers list' and other commands to query.

Examples:
  raag daemon --tracker http://localhost:8080
  raag daemon --tracker http://raag-production.up.railway.app`,
		Run: func(cmd *cobra.Command, args []string) {
			runDaemon(cmd)
		},
	}
	cmd.Flags().StringVar(&daemonTrackerURL, "tracker", "", "Tracker URL")
	return cmd
}

func runDaemon(cmd *cobra.Command) {
	logger.Infof("starting Raag daemon")
	v, err := config.InitViper(cmd)
	if err != nil {
		logger.Errorf("failed to initialize config error=%v", err)
		os.Exit(1)
	}

	cfg, err := config.LoadConfig(v)
	if err != nil {
		logger.Errorf("failed to load config error=%v", err)
		os.Exit(1)
	}
	if logLevel, _ := cmd.Flags().GetString("log-level"); logLevel == "" {
		logger.SetLevel(cfg.LogLevel)
	}
	if daemonTrackerURL != "" {
		cfg.TrackerURL = daemonTrackerURL
		config.SaveConfig(v, cfg)
	}

	a, err := app.NewApp(v, cfg)
	if err != nil {
		logger.Errorf("failed to create app error=%v", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a.StartNetwork(ctx)
	rpcServer := rpc.NewServer(config.SocketPath(), a, func() {
		cancel()
	})
	if err := rpcServer.Start(); err != nil {
		logger.Errorf("failed to start RPC server error=%v", err)
		os.Exit(1)
	}

	logger.Infof("Raag daemon started. Use Ctrl+C to stop.")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		logger.Infof("received termination signal")
	case <-rpcServer.ShutdownRequested():
		logger.Infof("shutdown requested via RPC")
	case <-ctx.Done():
		logger.Infof("shutdown requested via RPC")
	}

	logger.Infof("shutting down daemon")
	rpcServer.Stop()
	if err := a.NetMgr.Close(); err != nil {
		logger.Warnf("failed to close network manager error=%v", err)
	}

	a.SaveState()
	logger.Infof("daemon stopped")
}
