package playlist

import (
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/p-society/raag/internal/metadata"
)

type Playlist struct {
	Name  string
	Songs []metadata.Song
}

type Manager struct {
	Playlists map[string]*Playlist
	mutex     sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		Playlists: make(map[string]*Playlist),
	}
}

func (m *Manager) Create(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.Playlists[name]; exists {
		return fmt.Errorf("playlist already exists: %s", name)
	}

	m.Playlists[name] = &Playlist{Name: name, Songs: []metadata.Song{}}
	return nil
}

func (m *Manager) Delete(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.Playlists[name]; !exists {
		return fmt.Errorf("playlist not found: %s", name)
	}

	delete(m.Playlists, name)
	return nil
}

func (m *Manager) AddSong(playlistName string, song metadata.Song) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	pl, exists := m.Playlists[playlistName]
	if !exists {
		return fmt.Errorf("playlist not found: %s", playlistName)
	}

	pl.Songs = append(pl.Songs, song)
	return nil
}

func (m *Manager) RemoveSong(playlistName string, index int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	pl, exists := m.Playlists[playlistName]
	if !exists {
		return fmt.Errorf("playlist not found: %s", playlistName)
	}
	if index < 0 || index >= len(pl.Songs) {
		return fmt.Errorf("invalid index: %d", index)
	}

	pl.Songs = append(pl.Songs[:index], pl.Songs[index+1:]...)
	return nil
}

func (m *Manager) List() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return slices.Sorted(maps.Keys(m.Playlists))
}

func (m *Manager) GetSongs(playlistName string) ([]metadata.Song, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	pl, exists := m.Playlists[playlistName]
	if !exists {
		return nil, fmt.Errorf("playlist not found: %s", playlistName)
	}

	songs := make([]metadata.Song, len(pl.Songs))
	copy(songs, pl.Songs)
	return songs, nil
}

func (m *Manager) Get(playlistName string) (*Playlist, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	pl, exists := m.Playlists[playlistName]
	if !exists {
		return nil, fmt.Errorf("playlist not found: %s", playlistName)
	}
	return pl, nil
}
