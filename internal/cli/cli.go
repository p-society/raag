package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/p-society/raag/internal/library"
	"github.com/p-society/raag/internal/network"
	"github.com/p-society/raag/internal/player"
)

type CLI struct {
	library *library.Library
	player  *player.Player
	network *network.Network
}

func NewCLI(lib *library.Library, p *player.Player, net *network.Network) *CLI {
	return &CLI{
		library: lib,
		player:  p,
		network: net,
	}
}

func (c *CLI) Start() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		input, _ := reader.ReadString('\n')
		c.handleCommand(strings.TrimSpace(input))
	}
}

func (c *CLI) handleCommand(input string) {
	parts := strings.SplitN(input, " ", 2)
	command := parts[0]

	switch command {
	case "list":
		c.listSongs()
	case "play":
		if len(parts) > 1 {
			c.playSong(parts[1])
		}
	case "pause":
		c.player.Pause()
	case "resume":
		c.player.Resume()
	case "stop":
		c.player.Stop()
	case "request":
		if len(parts) > 1 {
			c.requestSong(parts[1])
		}
	case "share":
		if len(parts) > 1 {
			c.shareSong(parts[1])
		}
	case "help":
		c.printHelp()
	default:
		fmt.Println("Unknown command. Type 'help' for available commands.")
	}
}

func (c *CLI) listSongs() {
	songs := c.library.ListSongs()
	for i, song := range songs {
		fmt.Printf("%d. %s - %s (%s)\n", i+1, song.Title, song.Artist, song.Album)
	}
}

func (c *CLI) playSong(title string) {
	song, err := c.library.FindSong(title)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	if err := c.player.Play(song); err != nil {
		fmt.Printf("Error playing song: %v\n", err)
	}
}

func (c *CLI) requestSong(title string) {
	// Implement song request logic
	fmt.Printf("Requesting song: %s\n", title)
}

func (c *CLI) shareSong(title string) {
	// Implement song sharing logic
	fmt.Printf("Sharing song: %s\n", title)
}

func (c *CLI) printHelp() {
	fmt.Println("Available commands:")
	fmt.Println("  list                 - List all songs in your library")
	fmt.Println("  play <song title>    - Play a song")
	fmt.Println("  pause                - Pause the current playback")
	fmt.Println("  resume               - Resume paused playback")
	fmt.Println("  stop                 - Stop the current playback")
	fmt.Println("  request <song title> - Request a song from peers")
	fmt.Println("  share <song title>   - Share a song with peers")
	fmt.Println("  help                 - Show this help message")
}
