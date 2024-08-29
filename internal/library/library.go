package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/p-society/raag/internal/metadata"
)

type Library struct {
	Songs map[string]metadata.Song
	mutex sync.RWMutex
}

func NewLibrary(musicDir string) (*Library, error) {
	lib := &Library{
		Songs: make(map[string]metadata.Song),
	}

	if err := lib.ScanMusicLibrary(musicDir); err != nil {
		return nil, fmt.Errorf("error scanning music library: %w", err)
	}
	return lib, nil
}

func (l *Library) ScanMusicLibrary(musicDir string) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.Songs = make(map[string]metadata.Song)
	return filepath.Walk(musicDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".mp3") {
			song, err := metadata.ExtractMetadata(path)
			if err != nil {
				return fmt.Errorf("error extracting metadata from %s: %w", path, err)
			}
			l.Songs[strings.ToLower(song.Title)] = song
		}
		return nil
	})
}

func (l *Library) ListSongs() []metadata.Song {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	songs := make([]metadata.Song, 0, len(l.Songs))
	for _, song := range l.Songs {
		songs = append(songs, song)
	}
	return songs
}

func (l *Library) FindSong(title string) (metadata.Song, error) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	song, exists := l.Songs[strings.ToLower(title)]
	if !exists {
		return metadata.Song{}, fmt.Errorf("song not found: %s", title)
	}
	return song, nil
}
