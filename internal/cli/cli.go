package cli

import (
	"fmt"

	"github.com/p-society/raag/internal/library"
	"github.com/p-society/raag/internal/network"
	"github.com/p-society/raag/internal/player"
	"github.com/spf13/cobra"
)

type CLI struct {
	library *library.Library
	player  *player.Player
	network *network.Network
	rootCmd *cobra.Command
}

func NewCLI(lib *library.Library, p *player.Player, net *network.Network) *CLI {
	cli := &CLI{
		library: lib,
		player:  p,
		network: net,
	}

	cli.rootCmd = &cobra.Command{
		Use:   "raag",
		Short: "Raag allows you to play, share, and discover music in a peer-to-peer network.",
		Run:   func(cmd *cobra.Command, args []string) {},
	}

	cli.rootCmd.CompletionOptions.DisableDefaultCmd = true
	cli.rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	cli.addCommands()

	return cli
}

func (c *CLI) addCommands() {
	c.rootCmd.AddCommand(
		c.listCommand(),
		c.playCommand(),
		c.pauseCommand(),
		c.resumeCommand(),
		c.stopCommand(),
		c.requestCommand(),
		c.shareCommand(),
	)
}

func (c *CLI) Start() error {
	return c.rootCmd.Execute()
}

func (c *CLI) listCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all songs in your library",
		Run: func(cmd *cobra.Command, args []string) {
			songs := c.library.ListSongs()
			for i, song := range songs {
				fmt.Printf("%d. %s - %s (%s)\n", i+1, song.Title, song.Artist, song.Album)
			}
		},
	}
}

func (c *CLI) playCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "play [song title]",
		Short: "Play a song",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			song, err := c.library.FindSong(args[0])
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			if err := c.player.Play(song); err != nil {
				fmt.Printf("Error playing song: %v\n", err)
			}
		},
	}
}

func (c *CLI) pauseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "pause",
		Short: "Pause the current playback",
		Run: func(cmd *cobra.Command, args []string) {
			c.player.Pause()
		},
	}
}

func (c *CLI) resumeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Resume paused playback",
		Run: func(cmd *cobra.Command, args []string) {
			c.player.Resume()
		},
	}
}

func (c *CLI) stopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the current playback",
		Run: func(cmd *cobra.Command, args []string) {
			c.player.Stop()
		},
	}
}

func (c *CLI) requestCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "request [song title]",
		Short: "Request a song from peers",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Requesting song: %s\n", args[0])
			// TODO :- Implement song request logic here
		},
	}
}

func (c *CLI) shareCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "share [song title]",
		Short: "Share a song with peers",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Sharing song: %s\n", args[0])
			// TODO :- Implement song sharing logic here
		},
	}
}
