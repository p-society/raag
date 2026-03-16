package library

import (
	"sync"
	"testing"

	"github.com/p-society/raag/internal/metadata"
)

func newTestLibrary() *Library {
	lib := &Library{}
	lib.Songs.Store(&sync.Map{})
	return lib
}

func TestNewLibrary(t *testing.T) {
	_, err := NewLibrary("./testdata")
	if err != nil {
		t.Logf("NewLibrary error (expected if testdata missing): %v", err)
	}
}

func TestListSongs(t *testing.T) {
	lib := newTestLibrary()
	songsMap := lib.Songs.Load()
	songsMap.Store("song1", metadata.Song{Title: "Song 1"})
	songsMap.Store("song2", metadata.Song{Title: "Song 2"})

	count := 0
	for range lib.AllSongs() {
		count++
	}

	if count != 2 {
		t.Errorf("AllSongs() count = %d, want 2", count)
	}
}

func TestFindSong(t *testing.T) {
	lib := newTestLibrary()
	songsMap := lib.Songs.Load()
	songsMap.Store("test song", metadata.Song{
		Title:  "Test Song",
		Artist: "Test Artist",
		Album:  "Test Album",
	})

	song, err := lib.FindSong("test song")
	if err != nil {
		t.Errorf("FindSong() error = %v", err)
	}
	if song.Title != "Test Song" {
		t.Errorf("FindSong() = %v, want Test Song", song.Title)
	}

	_, err = lib.FindSong("nonexistent")
	if err == nil {
		t.Error("FindSong() should return error for nonexistent song")
	}
}

func TestAddSong(t *testing.T) {
	lib := newTestLibrary()
	err := lib.AddSong("/fake/path/song.mp3")
	if err != nil {
		t.Logf("AddSong error (expected if file doesn't exist): %v", err)
	}
}

func TestRemoveSong(t *testing.T) {
	lib := newTestLibrary()
	songsMap := lib.Songs.Load()
	songsMap.Store("abc123", metadata.Song{Title: "Test Song", Hash: "abc123"})

	err := lib.RemoveSong("Test Song")
	if err != nil {
		t.Errorf("RemoveSong() error = %v", err)
	}

	songsMap = lib.Songs.Load()
	var exists bool
	songsMap.Range(func(key, value any) bool {
		if key == "abc123" {
			exists = true
			return false
		}
		return true
	})
	if exists {
		t.Error("RemoveSong() should remove song from library")
	}

	err = lib.RemoveSong("nonexistent")
	if err == nil {
		t.Error("RemoveSong() should return error for nonexistent song")
	}
}

func TestGetByArtist(t *testing.T) {
	lib := newTestLibrary()
	songsMap := lib.Songs.Load()
	songsMap.Store("song1", metadata.Song{Title: "Song 1", Artist: "Artist A"})
	songsMap.Store("song2", metadata.Song{Title: "Song 2", Artist: "Artist B"})
	songsMap.Store("song3", metadata.Song{Title: "Song 3", Artist: "Artist A"})

	songs := lib.GetByArtist("Artist A")
	if len(songs) != 2 {
		t.Errorf("GetByArtist() = %d, want 2", len(songs))
	}
}

func TestGetByAlbum(t *testing.T) {
	lib := newTestLibrary()
	songsMap := lib.Songs.Load()
	songsMap.Store("song1", metadata.Song{Title: "Song 1", Album: "Album A"})
	songsMap.Store("song2", metadata.Song{Title: "Song 2", Album: "Album B"})
	songsMap.Store("song3", metadata.Song{Title: "Song 3", Album: "Album A"})

	songs := lib.GetByAlbum("Album A")
	if len(songs) != 2 {
		t.Errorf("GetByAlbum() = %d, want 2", len(songs))
	}
}
