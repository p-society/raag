package main

import (
	"fmt"

	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/rpc"
	"github.com/spf13/cobra"
)

func shareCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "share <peerID> <song>",
		Short: "Share a song with a peer (requires daemon)",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			requireDaemon()
			logger.Infof("share sent to daemon peer=%s song=%s", args[0], args[1])
		},
	}
}

func statusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.StatusResult
			invokeRPC("DaemonService.Status", &rpc.EmptyArgs{}, &result)

			logger.Infof("daemon status")
			logger.Infof("running value=%v", result.Running)
			logger.Infof("peer count count=%d", result.PeerCount)
			logger.Infof("network online status=%v", result.Connected)
			logger.Infof("uptime value=%s", result.Uptime)
			logger.Infof("version value=%s", result.Version)
		},
	}
}

func networkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network",
		Short: "Manage network and P2P connections",
	}

	cmd.AddCommand(networkStatusCommand())
	cmd.AddCommand(networkAuthKeyCommand())
	return cmd
}

func networkStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show P2P network status",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.NetworkStatusResult
			invokeRPC("DaemonService.NetworkStatus", &rpc.EmptyArgs{}, &result)

			logger.Infof("Network status:")
			logger.Infof("Self: %s", result.State.SelfID)
			logger.Infof("Tracker: %s", result.State.TrackerURL)
			logger.Infof("DHT: enabled=%v", result.State.DHTEnabled)
			logger.Infof("Connections: %d", len(result.State.ConnectedPeers))
		},
	}
}

func networkAuthKeyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "auth-key",
		Short: "Print the full derived tracker auth key",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.AuthKeyResult
			invokeRPC("DaemonService.AuthKey", &rpc.EmptyArgs{}, &result)
			if result.AuthPublicKey == "" {
				logger.Errorf("No auth key available")
				return
			}
			fmt.Println(result.AuthPublicKey)
		},
	}
}
