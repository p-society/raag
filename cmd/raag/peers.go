package main

import (
	"fmt"
	"os"

	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/rpc"
	"github.com/spf13/cobra"
)

func requireDaemon() *rpc.Client {
	client := newRPCClient()
	if !client.IsAvailable() {
		fmt.Fprintln(os.Stderr, "Daemon not running. Start it with: raag daemon")
		os.Exit(1)
	}
	return client
}

func peersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "peers",
		Short: "Manage peers and discovery",
	}

	cmd.AddCommand(peersListCommand())
	cmd.AddCommand(peersInfoCommand())
	cmd.AddCommand(peersConnectCommand())
	cmd.AddCommand(peersDisconnectCommand())
	cmd.AddCommand(peersTrackerCommand())
	cmd.AddCommand(peersBootstrapCommand())
	return cmd
}

func peersListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List connected peers",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.PeerListResult
			invokeRPC("DaemonService.PeersList", &rpc.EmptyArgs{}, &result)

			logger.Infof("connected peers count=%d", len(result.Peers))
			for _, p := range result.Peers {
				logger.Infof("peer id=%s", p.ID)
			}
		},
	}
}

func peersInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show all peer info",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.PeersInfoResult
			invokeRPC("DaemonService.PeersInfo", &rpc.EmptyArgs{}, &result)

			logger.Infof("self info")
			logger.Infof("peer id id=%v", result.Self["peer_id"])
			logger.Infof("multiaddr addr=%v", result.Self["multiaddr"])
			logger.Infof("connected peers count=%d", result.ConnectedCount)
			for _, p := range result.ConnectedPeers {
				logger.Infof("peer id=%s", p.ID)
			}
			logger.Infof("known peers count=%d", result.KnownCount)
		},
	}
}

func peersConnectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "connect <multiaddr>",
		Short: "Connect to a peer by multiaddr",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.Connect", &rpc.ConnectArgs{Multiaddr: args[0]}, &result)
			logger.Infof("connected to peer")
		},
	}
}

func peersDisconnectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "disconnect <peerID>",
		Short: "Disconnect from a peer",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.Disconnect", &rpc.DisconnectArgs{PeerID: args[0]}, &result)
			logger.Infof("disconnected from peer")
		},
	}
}

func peersTrackerCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tracker <url>",
		Short: "Set tracker URL",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.UpdateTracker", &rpc.TrackerArgs{URL: args[0]}, &result)
			logger.Infof("tracker URL updated successfully url=%s", args[0])
		},
	}
}

func peersBootstrapCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "bootstrap <multiaddr>",
		Short: "Add bootstrap peer",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.AddBootstrap", &rpc.BootstrapArgs{Multiaddr: args[0]}, &result)
			logger.Infof("bootstrap peer added successfully")
		},
	}
}
