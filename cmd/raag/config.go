package main

import (
	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/rpc"
	"github.com/spf13/cobra"
)

func configCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}

	cmd.AddCommand(configShowCommand())
	cmd.AddCommand(configSetCommand())
	cmd.AddCommand(configResetCommand())
	return cmd
}

func configShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			client := requireDaemon()
			var result rpc.ConfigGetResult
			if err := client.Call("DaemonService.ConfigGet", &rpc.EmptyArgs{}, &result); err != nil {
				logger.Errorf("failed to get config error=%v", err)
				return
			}

			logger.Infof("current configuration")
			for key, val := range result.Config {
				logger.Infof("config key=%s value=%v", key, val)
			}
		},
	}
}

func configSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			client := requireDaemon()
			var result rpc.EmptyResult
			if err := client.Call("DaemonService.ConfigSet", &rpc.ConfigSetArgs{Key: args[0], Value: args[1]}, &result); err != nil {
				logger.Errorf("failed to set config error=%v", err)
				return
			}
			logger.Infof("config updated key=%s value=%s", args[0], args[1])
		},
	}
}

func configResetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Reset configuration to defaults",
		Run: func(cmd *cobra.Command, args []string) {
			logger.Infof("config reset requires daemon restart")
		},
	}
}
