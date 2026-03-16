package main

import (
	"strconv"

	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/rpc"
	"github.com/spf13/cobra"
)

func playlistCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "playlist",
		Short: "Manage playlists",
	}

	cmd.AddCommand(playlistCreateCommand())
	cmd.AddCommand(playlistDeleteCommand())
	cmd.AddCommand(playlistListCommand())
	cmd.AddCommand(playlistAddCommand())
	cmd.AddCommand(playlistRemoveCommand())
	cmd.AddCommand(playlistSongsCommand())
	cmd.AddCommand(playlistPlayCommand())
	return cmd
}

func playlistCreateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new playlist",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.PlaylistCreate", &rpc.PlaylistCreateArgs{Name: args[0]}, &result)
			logger.Infof("playlist created name=%s", args[0])
		},
	}
}

func playlistDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a playlist",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.PlaylistDelete", &rpc.PlaylistDeleteArgs{Name: args[0]}, &result)
			logger.Infof("playlist deleted name=%s", args[0])
		},
	}
}

func playlistListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all playlists",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.PlaylistListResult
			invokeRPC("DaemonService.PlaylistList", &rpc.EmptyArgs{}, &result)
			if len(result.Names) == 0 {
				logger.Infof("no playlists found")
				return
			}

			logger.Infof("playlists")
			for _, name := range result.Names {
				logger.Infof("playlist name=%s", name)
			}
		},
	}
}

func playlistAddCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "add <playlist> <song>",
		Short: "Add a song to a playlist",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.PlaylistAdd", &rpc.PlaylistAddArgs{Playlist: args[0], Song: args[1]}, &result)
			logger.Infof("song added to playlist song=%s playlist=%s", args[1], args[0])
		},
	}
}

func playlistRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <playlist> <index>",
		Short: "Remove a song from playlist by index",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			idx, err := strconv.Atoi(args[1])
			if err != nil {
				logger.Errorf("invalid index error=%v", err)
				return
			}

			var result rpc.EmptyResult
			invokeRPC("DaemonService.PlaylistRemove", &rpc.PlaylistRemoveArgs{Playlist: args[0], Index: idx}, &result)
			logger.Infof("song removed from playlist index=%d playlist=%s", idx, args[0])
		},
	}
}

func playlistSongsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "songs <playlist>",
		Short: "List songs in a playlist",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.PlaylistSongsResult
			invokeRPC("DaemonService.PlaylistSongs", &rpc.PlaylistDeleteArgs{Name: args[0]}, &result)
			if len(result.Songs) == 0 {
				logger.Infof("playlist is empty playlist=%s", args[0])
				return
			}

			logger.Infof("songs in playlist playlist=%s", args[0])
			for i, song := range result.Songs {
				logger.Infof("song index=%d title=%s artist=%s", i+1, song.Title, song.Artist)
			}
		},
	}
}

func playlistPlayCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "play <playlist>",
		Short: "Play all songs in a playlist",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var songsResult rpc.PlaylistSongsResult
			invokeRPC("DaemonService.PlaylistSongs", &rpc.PlaylistDeleteArgs{Name: args[0]}, &songsResult)
			for _, song := range songsResult.Songs {
				var playResult rpc.EmptyResult
				invokeRPC("DaemonService.Play", &rpc.PlayArgs{Song: song.Title}, &playResult)
			}
			logger.Infof("playing playlist playlist=%s song_count=%d", args[0], len(songsResult.Songs))
		},
	}
}
