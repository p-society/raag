package storage

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/p-society/raag/internal/config"
	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/internal/metadata"
	"github.com/p-society/raag/internal/playlist"
)

func WriteJSONAtomic(filePath string, data any) error {
	tmpPath := filePath + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("error creating temp file: %w", err)
	}

	defer func() {
		file.Close()
		os.Remove(tmpPath)
	}()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("error syncing to disk: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("error closing file: %w", err)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("error atomically saving file: %w", err)
	}
	return nil
}

type PlaylistData struct {
	Name  string          `json:"name"`
	Songs []metadata.Song `json:"songs"`
}

type StorageData struct {
	Playlists []PlaylistData `json:"playlists"`
}

func Init() error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}
	return nil
}

func SavePlaylists(pm *playlist.Manager) error {
	playlistNames := pm.List()
	playlists := make([]PlaylistData, 0, len(playlistNames))
	for _, name := range playlistNames {
		songs, err := pm.GetSongs(name)
		if err != nil {
			continue
		}

		playlists = append(playlists, PlaylistData{
			Name:  name,
			Songs: songs,
		})
	}

	data := StorageData{Playlists: playlists}
	filePath, err := config.PlaylistsPath()
	if err != nil {
		return err
	}
	return WriteJSONAtomic(filePath, data)
}

func LoadPlaylists(pm *playlist.Manager) error {
	filePath, err := config.PlaylistsPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("error reading playlists file: %w", err)
	}
	if len(data) == 0 {
		logger.Warnf("playlists file is empty, starting with no playlists")
		return nil
	}

	var storageData StorageData
	if err := json.Unmarshal(data, &storageData); err != nil {
		logger.Warnf("corrupted playlists file, resetting error=%v", err)
		return nil
	}
	for _, pl := range storageData.Playlists {
		if err := pm.Create(pl.Name); err != nil {
			continue
		}
		for _, song := range pl.Songs {
			pm.AddSong(pl.Name, song)
		}
	}
	return nil
}
