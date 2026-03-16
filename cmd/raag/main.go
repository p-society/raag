package main

import (
	"github.com/p-society/raag/internal/constants"
	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/internal/tui"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "raag",
		Short: "Raag - Decentralized Music Streaming",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if jsonMode, _ := cmd.Flags().GetBool("json"); jsonMode {
				logger.SetJSONMode(true)
			}
			if logLevel, _ := cmd.Flags().GetString("log-level"); logLevel != "" {
				logger.SetLevel(logLevel)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	rootCmd.PersistentFlags().String("config", "", "config file (default is $HOME/.config/raag/config.yaml)")
	rootCmd.PersistentFlags().MarkHidden("config")
	rootCmd.PersistentFlags().String("music-dir", "", "Directory containing music files (default: ~/.config/raag/music)")
	rootCmd.PersistentFlags().Bool("network", false, "Enable network mode for peer discovery")
	rootCmd.PersistentFlags().Bool("tui", false, "Start in TUI mode")
	rootCmd.PersistentFlags().String("tracker", constants.DefaultTrackerURL, "Centralized tracker URL for peer discovery")
	rootCmd.PersistentFlags().Int("port", constants.DefaultPort, "Node listen port (use 0 for random)")
	rootCmd.PersistentFlags().Bool("dht", true, "Enable DHT discovery (default enabled)")
	rootCmd.PersistentFlags().Int("max-peers", constants.DefaultMaxPeers, "Maximum number of peers to maintain")
	rootCmd.PersistentFlags().StringSlice("bootstrap", []string{}, "DHT bootstrap peers (multiaddr)")
	rootCmd.PersistentFlags().String("host", constants.DefaultHost, "The host address to listen on")
	rootCmd.PersistentFlags().String("rendezvous", constants.DefaultRendezvous, "Unique string to identify Raag nodes")
	rootCmd.PersistentFlags().Bool("json", false, "Output logs in JSON format")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("auth-secret", "", "Shared secret for tracker authentication")

	rootCmd.AddCommand(playCommand())
	rootCmd.AddCommand(pauseCommand())
	rootCmd.AddCommand(resumeCommand())
	rootCmd.AddCommand(stopCommand())
	rootCmd.AddCommand(nextCommand())
	rootCmd.AddCommand(previousCommand())
	rootCmd.AddCommand(queueCommand())
	rootCmd.AddCommand(volumeCommand())
	rootCmd.AddCommand(seekCommand())
	rootCmd.AddCommand(nowplayingCommand())
	rootCmd.AddCommand(peersCommand())
	rootCmd.AddCommand(libraryCommand())
	rootCmd.AddCommand(playlistCommand())
	rootCmd.AddCommand(shareCommand())
	rootCmd.AddCommand(configCommand())
	rootCmd.AddCommand(daemonCommand())
	rootCmd.AddCommand(statusCommand())
	rootCmd.AddCommand(networkCommand())
	rootCmd.AddCommand(tuiCommand())

	if err := rootCmd.Execute(); err != nil {
		logger.Errorf("Command execution failed error=%v", err)
	}
}

func tuiCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Start terminal UI (requires daemon)",
		Run: func(cmd *cobra.Command, args []string) {
			client := requireDaemon()
			if err := tui.Start(client); err != nil {
				logger.Errorf("TUI error error=%v", err)
			}
		},
	}
}
