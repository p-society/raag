package main

import (
	"strconv"

	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/rpc"
	"github.com/spf13/cobra"
)

func playCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "play <song>",
		Short: "Play a song from the library",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.Play", &rpc.PlayArgs{Song: args[0]}, &result)
			logger.Infof("playing song=%s", args[0])
		},
	}
}

func pauseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "pause",
		Short: "Pause playback",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.Pause", &rpc.EmptyArgs{}, &result)
			logger.Infof("playback paused")
		},
	}
}

func resumeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Resume playback",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.Resume", &rpc.EmptyArgs{}, &result)
			logger.Infof("playback resumed")
		},
	}
}

func stopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop playback and clear queue",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.StopPlayback", &rpc.EmptyArgs{}, &result)
			logger.Infof("playback stopped")
		},
	}
}

func nextCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "next",
		Short: "Skip to next song",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.Next", &rpc.EmptyArgs{}, &result)
			logger.Infof("skipped to next song")
		},
	}
}

func previousCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "previous",
		Short: "Go to previous song",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.EmptyResult
			invokeRPC("DaemonService.Previous", &rpc.EmptyArgs{}, &result)
			logger.Infof("went to previous song")
		},
	}
}

func queueCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "queue",
		Short: "Show current queue",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.QueueResult
			invokeRPC("DaemonService.GetQueue", &rpc.EmptyArgs{}, &result)
			if len(result.Songs) == 0 {
				logger.Infof("queue is empty")
				return
			}

			logger.Infof("current queue count=%d", len(result.Songs))
			for i, song := range result.Songs {
				marker := "  "
				if i == result.Current {
					marker = "> "
				}
				logger.Infof("song index=%d marker=%s title=%s artist=%s", i+1, marker, song.Title, song.Artist)
			}
		},
	}
}

func volumeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "volume [0-100]",
		Short: "Set volume (0-100) or show current volume",
		Args:  cobra.RangeArgs(0, 1),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				var result rpc.NowPlayingResult
				invokeRPC("DaemonService.NowPlaying", &rpc.EmptyArgs{}, &result)
				logger.Infof("current volume volume=%.0f", result.Volume)
				return
			}

			vol, err := strconv.ParseFloat(args[0], 64)
			if err != nil {
				logger.Errorf("invalid volume level error=%v", err)
				return
			}
			var result rpc.EmptyResult
			invokeRPC("DaemonService.SetVolume", &rpc.VolumeArgs{Level: vol}, &result)
			logger.Infof("volume set to level=%.0f", vol)
		},
	}
}

func seekCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "seek <seconds>",
		Short: "Seek to position in seconds",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			secs, err := strconv.Atoi(args[0])
			if err != nil {
				logger.Errorf("invalid position error=%v", err)
				return
			}

			var result rpc.EmptyResult
			invokeRPC("DaemonService.Seek", &rpc.SeekArgs{Seconds: secs}, &result)
			logger.Infof("seeked to seconds=%d", secs)
		},
	}
}

func nowplayingCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "nowplaying",
		Short: "Show current playing song",
		Run: func(cmd *cobra.Command, args []string) {
			var result rpc.NowPlayingResult
			invokeRPC("DaemonService.NowPlaying", &rpc.EmptyArgs{}, &result)
			if result.Title == "" {
				logger.Infof("no song playing")
				return
			}

			status := "Playing"
			if result.Paused {
				status = "Paused"
			} else if !result.Playing {
				status = "Stopped"
			}

			logger.Infof("now playing status=%s", status)
			logger.Infof("title title=%s", result.Title)
			logger.Infof("artist artist=%s", result.Artist)
			logger.Infof("album album=%s", result.Album)
			logger.Infof("position position=%d duration=%d", result.Position, result.Duration)
			logger.Infof("volume volume=%.0f", result.Volume)
		},
	}
}
