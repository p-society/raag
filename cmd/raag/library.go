package main

import (
	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/rpc"
	"github.com/spf13/cobra"
)

func libraryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "library",
		Short: "Manage music library",
	}

	cmd.AddCommand(libraryListCommand())
	cmd.AddCommand(librarySearchCommand())
	cmd.AddCommand(libraryRescanCommand())
	cmd.AddCommand(libraryAddCommand())
	cmd.AddCommand(libraryRemoveCommand())
	return cmd
}

func libraryListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all songs in library",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.LibraryListResult
			invokeRPC("DaemonService.LibraryList", &rpc.EmptyArgs{}, &result)
			if len(result.Songs) == 0 {
				logger.Infof("library is empty")
				return
			}

			logger.Infof("library contents count=%d", len(result.Songs))
			for i, song := range result.Songs {
				logger.Infof("song index=%d title=%s artist=%s album=%s", i+1, song.Title, song.Artist, song.Album)
			}
		},
	}
}

func librarySearchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search songs in library",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.SearchResult
			invokeRPC("DaemonService.LibrarySearch", &rpc.SearchArgs{Query: args[0]}, &result)
			if len(result.Songs) == 0 {
				logger.Infof("no songs found matching query=%s", args[0])
				return
			}

			logger.Infof("matching songs found count=%d", len(result.Songs))
			for _, song := range result.Songs {
				logger.Infof("result song=%s - %s", song.Title, song.Artist)
			}
		},
	}
}

func libraryRescanCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rescan",
		Short: "Rescan music directory",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.RescanResult
			invokeRPC("DaemonService.LibraryRescan", &rpc.EmptyArgs{}, &result)
			logger.Infof("library rescanned song_count=%d", result.Count)
		},
	}
}

func libraryAddCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "add <path>",
		Short: "Add a song to library",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.LibraryAdd", &rpc.AddArgs{Path: args[0]}, &result)
			logger.Infof("song added path=%s", args[0])
		},
	}
}

func libraryRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <title>",
		Short: "Remove a song from library",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.LibraryRemove", &rpc.RemoveArgs{Title: args[0]}, &result)
			logger.Infof("song removed title=%s", args[0])
		},
	}
}
