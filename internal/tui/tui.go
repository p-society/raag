package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/p-society/raag/rpc"
)

// Start launches the TUI as an RPC client to the daemon.
func Start(client *rpc.Client) error {
	m := NewRPCModel(client)
	program := tea.NewProgram(m, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

// RPCModel is the TUI model that communicates exclusively via RPC.
type RPCModel struct {
	client      *rpc.Client
	keys        KeyMap
	searchInput textinput.Model
	state       *DaemonState
	currentView View
	selectedIdx int
	width       int
	height      int
}

// DaemonState holds the cached state from the daemon.
type DaemonState struct {
	NowPlaying *rpc.NowPlayingResult
	Queue      *rpc.QueueResult
	Library    *rpc.LibraryListResult
	Network    *rpc.NetworkStatusResult
	Status     *rpc.StatusResult
}

func NewRPCModel(client *rpc.Client) *RPCModel {
	searchInput := textinput.New()
	searchInput.Placeholder = "Search library..."
	searchInput.Prompt = "/ "

	return &RPCModel{
		client:      client,
		keys:        DefaultKeyMap(),
		searchInput: searchInput,
		currentView: ViewPlayer,
		state:       &DaemonState{},
	}
}

type (
	tickMsg        time.Time
	stateUpdateMsg *DaemonState
)

func (m *RPCModel) Init() tea.Cmd {
	return tea.Batch(m.fetchState(), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *RPCModel) fetchState() tea.Cmd {
	return func() tea.Msg {
		var fs rpc.FullStateResult
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		if err := m.client.CallWithContext(ctx, "DaemonService.GetFullState", &rpc.EmptyArgs{}, &fs); err != nil {
			return nil
		}
		state := &DaemonState{
			NowPlaying: fs.NowPlaying,
			Queue:      fs.Queue,
			Network:    fs.Network,
			Status:     fs.Status,
			Library:    fs.Library,
		}
		return stateUpdateMsg(state)
	}
}

func (m *RPCModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tickMsg:
		return m, tea.Batch(m.fetchState(), tickCmd())
	case stateUpdateMsg:
		if msg == nil {
			return m, nil
		}
		m.state = msg
		return m, nil
	}
	return m, nil
}

func (m *RPCModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	if m.currentView == ViewLibrary && m.searchInput.Focused() {
		if _, matched := MatchAny(msg, k.Enter, k.Up, k.Down, k.Tab, k.Quit); matched {
			// Let main switch handle navigation keys
		} else {
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}
	}

	switch {
	case key.Matches(msg, k.Quit):
		if m.currentView == ViewPlayer {
			return m, tea.Quit
		}

		m.currentView = ViewPlayer
		m.searchInput.Blur()
		return m, nil
	case key.Matches(msg, k.Tab):
		m.nextView()
		return m, nil
	case key.Matches(msg, k.ViewPlayer):
		m.currentView = ViewPlayer
		m.selectedIdx = 0
		return m, nil
	case key.Matches(msg, k.ViewLibrary):
		m.currentView = ViewLibrary
		m.selectedIdx = 0
		return m, nil
	case key.Matches(msg, k.ViewPlaylist):
		m.currentView = ViewPlaylist
		m.selectedIdx = 0
		return m, nil
	case key.Matches(msg, k.ViewPeers):
		m.currentView = ViewPeers
		m.selectedIdx = 0
		return m, nil
	case key.Matches(msg, k.Help):
		m.currentView = ViewHelp
		m.searchInput.Blur()
		return m, nil
	case key.Matches(msg, k.Search) && m.currentView == ViewLibrary:
		m.searchInput.Focus()
		m.searchInput.SetValue("")
		return m, nil
	case key.Matches(msg, k.Up) && m.selectedIdx > 0:
		m.selectedIdx--
		return m, nil
	case key.Matches(msg, k.Down):
		m.selectedIdx++
		return m, nil
	case key.Matches(msg, k.Enter) && m.currentView == ViewLibrary:
		if m.state.Library != nil && m.selectedIdx < len(m.state.Library.Songs) {
			song := m.state.Library.Songs[m.selectedIdx]
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err := m.client.CallWithContext(ctx, "DaemonService.Play", &rpc.PlayArgs{Song: song.Title}, &rpc.EmptyResult{})
			if err != nil {
				m.state.Status = nil
			}
			return m, m.fetchState()
		}
		return m, nil
	}

	if m.currentView == ViewPlayer {
		if key.Matches(msg, k.PlayPause) {
			if m.state.NowPlaying != nil && m.state.NowPlaying.Playing {
				return m.executeAction("DaemonService.Pause", &rpc.EmptyArgs{})
			}
			return m.executeAction("DaemonService.Resume", &rpc.EmptyArgs{})
		}
		if key.Matches(msg, k.Next) {
			return m.executeAction("DaemonService.Next", &rpc.EmptyArgs{})
		}
		if key.Matches(msg, k.Prev) {
			return m.executeAction("DaemonService.Previous", &rpc.EmptyArgs{})
		}
		if key.Matches(msg, k.VolUp) {
			return m.executeAction("DaemonService.SetVolume", &rpc.VolumeArgs{Level: 5})
		}
		if key.Matches(msg, k.VolDown) {
			return m.executeAction("DaemonService.SetVolume", &rpc.VolumeArgs{Level: -5})
		}
	}
	return m, nil
}

func (m *RPCModel) executeAction(method string, args any) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var result rpc.EmptyResult
	if err := m.client.CallWithContext(ctx, method, args, &result); err != nil {
		m.state.Status = nil
	}
	return m, m.fetchState()
}

func (m *RPCModel) nextView() {
	switch m.currentView {
	case ViewPlayer:
		m.currentView = ViewLibrary
	case ViewLibrary:
		m.currentView = ViewPlaylist
	case ViewPlaylist:
		m.currentView = ViewPeers
	case ViewPeers:
		m.currentView = ViewPlayer
	default:
		m.currentView = ViewPlayer
	}
	m.selectedIdx = 0
}

func (m *RPCModel) View() string {
	if m.state == nil {
		return "Connecting to daemon..."
	}
	if m.state.Status == nil {
		return "Daemon not responding. Start it with: raag daemon"
	}

	var b strings.Builder
	b.WriteString("══════════════ RAAG ══════════════\n\n")

	switch m.currentView {
	case ViewPlayer:
		m.renderPlayerView(&b)
	case ViewLibrary:
		m.renderLibraryView(&b)
	case ViewPlaylist:
		m.renderPlaylistView(&b)
	case ViewPeers:
		m.renderPeersView(&b)
	case ViewHelp:
		m.renderHelpView(&b)
	}

	b.WriteString("\n[q]uit [tab]view [space]play/pause [n]ext [p]rev [+/-]vol")
	if m.currentView == ViewLibrary {
		b.WriteString(" [/]search")
	}
	return b.String()
}

func (m *RPCModel) renderPlayerView(b *strings.Builder) {
	if m.state.NowPlaying != nil && m.state.NowPlaying.Title != "" {
		np := m.state.NowPlaying
		status := "Playing"
		if np.Paused {
			status = "Paused"
		} else if !np.Playing {
			status = "Stopped"
		}

		fmt.Fprintf(b, "Now Playing: %s\n", status)
		fmt.Fprintf(b, "Title:  %s\n", np.Title)
		fmt.Fprintf(b, "Artist: %s\n", np.Artist)
		fmt.Fprintf(b, "Album:  %s\n", np.Album)

		if np.Duration > 0 {
			progress := float64(np.Position) / float64(np.Duration)
			barWidth := 30
			filled := int(progress * float64(barWidth))
			bar := strings.Repeat("█", filled) + strings.Repeat("─", barWidth-filled)
			fmt.Fprintf(b, "\n%s %s/%s\n", bar, formatTime(np.Position), formatTime(np.Duration))
		}

		var volBar strings.Builder
		for i := range 10 {
			if float64(i)*10 < np.Volume {
				volBar.WriteString("█")
			} else {
				volBar.WriteString("░")
			}
		}
		fmt.Fprintf(b, "Volume: %s %.0f%%\n", volBar.String(), np.Volume)
	} else {
		fmt.Fprintln(b, "No song playing")
	}
}

func (m *RPCModel) renderLibraryView(b *strings.Builder) {
	fmt.Fprintf(b, "Search: %s\n\n", m.searchInput.View())
	if m.state.Library == nil {
		b.WriteString("Loading library...\n")
		return
	}
	if len(m.state.Library.Songs) == 0 {
		b.WriteString("Library is empty\n")
		return
	}
	fmt.Fprintf(b, "Library: %d songs\n\n", len(m.state.Library.Songs))
	for i, song := range m.state.Library.Songs {
		if i >= 20 {
			fmt.Fprintf(b, "... and %d more\n", len(m.state.Library.Songs)-20)
			break
		}
		prefix := "  "
		if i == m.selectedIdx {
			prefix = "> "
		}
		fmt.Fprintf(b, "%s%s - %s\n", prefix, song.Title, song.Artist)
	}
}

func (m *RPCModel) renderPeersView(b *strings.Builder) {
	if m.state.Network == nil {
		b.WriteString("Network status loading...\n")
		return
	}

	ns := m.state.Network.State
	fmt.Fprintf(b, "Self: %s\n", ns.SelfID)
	fmt.Fprintf(b, "Tracker: %s\n", ns.TrackerURL)
	fmt.Fprintf(b, "DHT: %v (peers: %d)\n", ns.DHTEnabled, ns.DHTPeers)
	fmt.Fprintf(b, "Connected Peers: %d\n", len(ns.ConnectedPeers))
	fmt.Fprintf(b, "Known Peers: %d\n", len(ns.KnownPeers))
}

func (m *RPCModel) renderPlaylistView(b *strings.Builder) {
	b.WriteString("Playlists:\n")
	b.WriteString("(manage via CLI commands: raag playlist list/add/remove)\n")
}

func (m *RPCModel) renderHelpView(b *strings.Builder) {
	b.WriteString("Keyboard Shortcuts:\n\n")
	b.WriteString("  q / Ctrl+C: quit\n")
	b.WriteString("  Tab:        next view\n")
	b.WriteString("  Space:      play/pause\n")
	b.WriteString("  n:          next song\n")
	b.WriteString("  p:          previous song\n")
	b.WriteString("  +/-:        volume up/down\n")
	b.WriteString("  /:          search library\n")
	b.WriteString("  1-4:        switch views\n")
}

func formatTime(seconds int) string {
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}
