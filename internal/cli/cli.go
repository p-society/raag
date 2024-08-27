package cli

import (
	p2p "github.com/p-society/raag/internal/p2p"
	player "github.com/p-society/raag/internal/player"
	streaming "github.com/p-society/raag/internal/streaming"
	tui "github.com/p-society/raag/internal/tui"
	cli "github.com/urfave/cli/v2"
)

func NewApp() *cli.App {
	return &cli.App{
		Name:  "raag",
		Usage: "CLI Tool for Offline Music Playback, Streaming with local P2P support",
		Commands: []*cli.Command{
			{
				Name:  "play",
				Usage: "Play a local file",
				Action: func(ctx *cli.Context) error {
					return player.PlayMusic(ctx.Args().First())
				},
			},
			{
				Name:  "p2p",
				Usage: "Start a P2P node",
				Action: func(ctx *cli.Context) error {
					return p2p.StartP2PNode()
				},
			},
			{
				Name:  "stream",
				Usage: "Start a music streaming server",
				Action: func(ctx *cli.Context) error {
					return streaming.StartStreaming()
				},
			},
			{
				Name:  "ui",
				Usage: "Start TUI",
				Action: func(ctx *cli.Context) error {
					return tui.StartTUI()
				},
			},
		},
	}
}
