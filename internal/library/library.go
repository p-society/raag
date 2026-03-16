package library

import (
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/p-society/raag/internal/metadata"
)

// Library uses atomic.Pointer<sync.Map> for lockless concurrent access to songs.
type Library struct {
	Songs    atomic.Pointer[sync.Map]
	musicDir string
	musicMu  sync.RWMutex // protects musicDir only
	watcher  *fsnotify.Watcher
	closed   chan struct{}
}

func NewLibrary(musicDir string) (*Library, error) {
	lib := &Library{
		musicDir: musicDir,
		closed:   make(chan struct{}),
	}

	lib.Songs.Store(&sync.Map{})
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		return nil, fmt.Errorf("error creating music directory: %w", err)
	}
	if err := lib.ScanMusicLibrary(musicDir); err != nil {
		return nil, fmt.Errorf("error scanning music library: %w", err)
	}
	if err := lib.StartWatcher(); err != nil {
		fmt.Printf("Warning: failed to start file watcher: %v\n", err)
	}
	return lib, nil
}

func (l *Library) Close() error {
	close(l.closed)
	if l.watcher != nil {
		return l.watcher.Close()
	}
	return nil
}

func (l *Library) StartWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	l.watcher = watcher
	if err := watcher.Add(l.musicDir); err != nil {
		watcher.Close()
		return fmt.Errorf("failed to watch directory: %w", err)
	}

	go func() {
		var timerMu sync.Mutex
		pendingFiles := make(map[string]*time.Timer)
		for {
			select {
			case <-l.closed:
				timerMu.Lock()
				for _, t := range pendingFiles {
					t.Stop()
				}
				timerMu.Unlock()
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) {
					if isAudioFile(event.Name) {
						timerMu.Lock()
						if existingTimer := pendingFiles[event.Name]; existingTimer != nil {
							existingTimer.Stop()
						}
						pendingFiles[event.Name] = time.AfterFunc(500*time.Millisecond, func() {
							timerMu.Lock()
							delete(pendingFiles, event.Name)
							timerMu.Unlock()
							fmt.Printf("New audio file detected: %s\n", event.Name)
							if err := l.AddSong(event.Name); err != nil {
								fmt.Printf("Error adding new song: %v\n", err)
							}
						})
						timerMu.Unlock()
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Printf("Watcher error: %v\n", err)
			}
		}
	}()

	return nil
}

func (l *Library) ScanMusicLibrary(musicDir string) error {
	info, err := os.Stat(musicDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", musicDir)
	}

	newSongs := &sync.Map{}
	err = filepath.WalkDir(musicDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && isAudioFile(d.Name()) {
			song, err := metadata.ExtractMetadata(path)
			if err != nil {
				fmt.Printf("Warning: skipping unreadable file %s: %v\n", path, err)
				return nil
			}
			newSongs.Store(song.Hash, song)
		}
		return nil
	})
	if err != nil {
		return err
	}

	l.Songs.Store(newSongs)
	return nil
}

func isAudioFile(filename string) bool {
	lower := strings.ToLower(filename)
	exts := []string{".mp3", ".flac", ".wav", ".ogg", ".ogv"}
	for _, ext := range exts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// AllSongs returns a lockless iterator over all songs using sync.Map.Range.
func (l *Library) AllSongs() iter.Seq[metadata.Song] {
	songsMap := l.Songs.Load()
	if songsMap == nil {
		return func(yield func(metadata.Song) bool) {}
	}
	return func(yield func(metadata.Song) bool) {
		songsMap.Range(func(key, value any) bool {
			song, ok := value.(metadata.Song)
			if !ok {
				return true // skip invalid entries
			}
			return yield(song)
		})
	}
}

func (l *Library) FindSong(title string) (metadata.Song, error) {
	target := strings.ToLower(title)
	songsMap := l.Songs.Load()
	if songsMap == nil {
		return metadata.Song{}, fmt.Errorf("song not found: %s", title)
	}

	var found metadata.Song
	foundOk := false
	songsMap.Range(func(key, value any) bool {
		song, ok := value.(metadata.Song)
		if !ok {
			return true
		}
		if strings.ToLower(song.Title) == target {
			found = song
			foundOk = true
			return false
		}
		return true
	})

	if !foundOk {
		return metadata.Song{}, fmt.Errorf("song not found: %s", title)
	}
	return found, nil
}

func (l *Library) AddSong(path string) error {
	if !isAudioFile(path) {
		return fmt.Errorf("unsupported audio file: %s", path)
	}

	song, err := metadata.ExtractMetadata(path)
	if err != nil {
		return fmt.Errorf("error extracting metadata: %w", err)
	}

	songsMap := l.Songs.Load()
	if songsMap != nil {
		songsMap.Store(song.Hash, song)
	}
	return nil
}

func (l *Library) RemoveSong(title string) error {
	target := strings.ToLower(title)
	songsMap := l.Songs.Load()
	if songsMap == nil {
		return fmt.Errorf("song not found: %s", title)
	}

	var hashToDelete string
	found := false
	songsMap.Range(func(key, value any) bool {
		song := value.(metadata.Song)
		if strings.ToLower(song.Title) == target {
			hashToDelete = song.Hash
			found = true
			return false
		}
		return true
	})

	if !found {
		return fmt.Errorf("song not found: %s", title)
	}

	songsMap.Delete(hashToDelete)
	return nil
}

func (l *Library) filterSongs(predicate func(song metadata.Song) bool) []metadata.Song {
	var songs []metadata.Song
	for song := range l.AllSongs() {
		if predicate(song) {
			songs = append(songs, song)
		}
	}
	return songs
}

func (l *Library) GetByArtist(artist string) []metadata.Song {
	q := strings.ToLower(artist)
	return l.filterSongs(func(s metadata.Song) bool {
		return strings.Contains(strings.ToLower(s.Artist), q)
	})
}

func (l *Library) GetByAlbum(album string) []metadata.Song {
	q := strings.ToLower(album)
	return l.filterSongs(func(s metadata.Song) bool {
		return strings.Contains(strings.ToLower(s.Album), q)
	})
}

func (l *Library) GetMusicDir() string {
	l.musicMu.RLock()
	defer l.musicMu.RUnlock()
	return l.musicDir
}
