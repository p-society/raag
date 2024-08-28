package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dhowden/tag"
)

type Song struct {
	Title  string
	Artist string
	Album  string
	Path   string
}

type Library struct {
	Songs []Song
	mutex sync.RWMutex
}

func NewLibrary(musicDir string) (*Library, error) {
	lib := &Library{}
	err := lib.ScanMusicLibrary(musicDir)
	return lib, err
}

func (l *Library) ScanMusicLibrary(musicDir string) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	return filepath.Walk(musicDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".mp3") {
			song, err := l.extractMetadata(path)
			if err != nil {
				return err
			}
			l.Songs = append(l.Songs, song)
		}
		return nil
	})
}

func (l *Library) extractMetadata(path string) (Song, error) {
	file, err := os.Open(path)
	if err != nil {
		return Song{}, err
	}
	defer file.Close()

	metadata, err := tag.ReadFrom(file)
	if err != nil {
		return Song{}, err
	}

	return Song{
		Title:  metadata.Title(),
		Artist: metadata.Artist(),
		Album:  metadata.Album(),
		Path:   path,
	}, nil
}

func (l *Library) ListSongs() []Song {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.Songs
}

func (l *Library) FindSong(title string) (Song, error) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	for _, song := range l.Songs {
		if strings.EqualFold(song.Title, title) {
			return song, nil
		}
	}
	return Song{}, fmt.Errorf("song not found: %s", title)
}
