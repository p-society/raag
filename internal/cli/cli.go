package cli

import (
	"log"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/p-society/raag/internal/library"
	"github.com/p-society/raag/internal/network"
	"github.com/p-society/raag/internal/player"
	"github.com/spf13/cobra"
)

type CLI struct {
	library *library.Library
	player  *player.Player
	network *network.NetworkManager
	rootCmd *cobra.Command
}

func NewCLI(lib *library.Library, p *player.Player, net *network.NetworkManager) *CLI {
	cli := &CLI{
		library: lib,
		player:  p,
		network: net,
	}
	cli.rootCmd = &cobra.Command{
		Use:   "raag",
		Short: "Raag CLI for decentralized music streaming",
		Run:   func(cmd *cobra.Command, args []string) {},
	}

	cli.rootCmd.CompletionOptions.DisableDefaultCmd = true
	cli.rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	cli.rootCmd.AddCommand(cli.shareCommand())

	return cli
}

func (c *CLI) shareCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "share [peer ID] [song title]",
		Short: "Share a song with a peer",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			peerID := args[0]
			songTitle := args[1]

			log.Printf("Attempting to share song '%s' with peer %s\n", songTitle, peerID)
			song, err := c.library.FindSong(songTitle)
			if err != nil {
				log.Printf("Error: Song not found in library: %s\n", err)
				return
			}
			log.Printf("Song found in library: %+v\n", song)

			peerInfo, err := peer.AddrInfoFromString(peerID)
			if err != nil {
				log.Printf("Error parsing peer ID: %v\n", err)
				return
			}
			log.Printf("Peer info parsed: %+v\n", peerInfo)

			if err := c.network.ShareSong(peerInfo, song); err != nil {
				log.Printf("Error sharing song: %v\n", err)
				return
			}

			log.Printf("Song '%s' shared successfully with peer %s\n", songTitle, peerID)
		},
	}
}

func (c *CLI) Start() error {
	return c.rootCmd.Execute()
}
