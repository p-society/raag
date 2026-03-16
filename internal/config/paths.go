package config

import (
	"fmt"
	"os"
	"path/filepath"
)

func Dir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not get user config dir: %w", err)
	}
	return filepath.Join(configDir, "raag"), nil
}

func FilePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func SocketPath() string {
	dir, err := Dir()
	if err != nil {
		return "/tmp/raag-daemon.sock"
	}
	return filepath.Join(dir, "daemon.sock")
}

func PeerPersistencePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "peers.json"), nil
}

func StatePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

func PlaylistsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "playlists.json"), nil
}

func IdentityKeyPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "identity.key"), nil
}

func MusicDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "music"), nil
}
