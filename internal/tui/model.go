package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// KeyMap defines semantic keybindings for the TUI.
type KeyMap struct {
	Quit         key.Binding
	Tab          key.Binding
	PlayPause    key.Binding
	Next         key.Binding
	Prev         key.Binding
	VolUp        key.Binding
	VolDown      key.Binding
	Up           key.Binding
	Down         key.Binding
	Enter        key.Binding
	Search       key.Binding
	Backspace    key.Binding
	ViewPlayer   key.Binding
	ViewLibrary  key.Binding
	ViewPlaylist key.Binding
	ViewPeers    key.Binding
	Help         key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit:         key.NewBinding(key.WithKeys("ctrl+c", "q"), key.WithHelp("q/ctrl+c", "quit")),
		Tab:          key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next view")),
		PlayPause:    key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "play/pause")),
		Next:         key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next")),
		Prev:         key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "previous")),
		VolUp:        key.NewBinding(key.WithKeys("+", "="), key.WithHelp("+", "vol up")),
		VolDown:      key.NewBinding(key.WithKeys("-"), key.WithHelp("-", "vol down")),
		Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("up/k", "up")),
		Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("down/j", "down")),
		Enter:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Backspace:    key.NewBinding(key.WithKeys("backspace"), key.WithHelp("backspace", "delete")),
		ViewPlayer:   key.NewBinding(key.WithKeys("1")),
		ViewLibrary:  key.NewBinding(key.WithKeys("2")),
		ViewPlaylist: key.NewBinding(key.WithKeys("3")),
		ViewPeers:    key.NewBinding(key.WithKeys("4")),
		Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	}
}

// View represents a TUI view.
type View string

const (
	ViewPlayer   View = "player"
	ViewLibrary  View = "library"
	ViewPlaylist View = "playlist"
	ViewPeers    View = "peers"
	ViewHelp     View = "help"
)

// MatchAny checks if a key message matches any of the provided bindings.
// Returns the index of the first match and true, or -1 and false if no match.
func MatchAny(msg tea.KeyMsg, bindings ...key.Binding) (int, bool) {
	for i, b := range bindings {
		if key.Matches(msg, b) {
			return i, true
		}
	}
	return -1, false
}
