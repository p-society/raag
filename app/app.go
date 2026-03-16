package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/p-society/raag/internal/config"
	"github.com/p-society/raag/internal/library"
	"github.com/p-society/raag/internal/network"
	"github.com/p-society/raag/internal/player"
	"github.com/p-society/raag/internal/playlist"
	"github.com/p-society/raag/internal/storage"
	"github.com/spf13/viper"
)

// App is the application container. All services access components through App.
type App struct {
	V      *viper.Viper
	Cfg    *config.Config
	Lib    *library.Library
	Player *player.Player
	NetMgr *network.NetworkManager
	PM     *playlist.Manager

	StartTime time.Time
	cancel    context.CancelFunc
}

// NewApp creates and initializes all application components and services.
func NewApp(v *viper.Viper, cfg *config.Config) (*App, error) {
	if err := storage.Init(); err != nil {
		return nil, fmt.Errorf("init storage: %w", err)
	}

	lib, err := library.NewLibrary(cfg.MusicDir)
	if err != nil {
		return nil, fmt.Errorf("init library: %w", err)
	}

	p, err := player.NewPlayer()
	if err != nil {
		return nil, fmt.Errorf("init player: %w", err)
	}
	if err := p.SetVolume(float64(cfg.Volume)); err != nil {
		return nil, fmt.Errorf("set volume: %w", err)
	}
	p.SetNextCallback(func() error {
		return p.Next()
	})

	netMgr, err := network.NewNetwork(cfg, v, lib, cfg.MusicDir)
	if err != nil {
		return nil, fmt.Errorf("init network: %w", err)
	}

	pm := playlist.NewManager()
	storage.LoadPlaylists(pm)
	startTime := time.Now()
	app := &App{
		V:         v,
		Cfg:       cfg,
		Lib:       lib,
		Player:    p,
		NetMgr:    netMgr,
		PM:        pm,
		StartTime: startTime,
	}
	return app, nil
}

// StartNetwork begins network discovery in the background.
func (a *App) StartNetwork(ctx context.Context) {
	go func() {
		if err := a.NetMgr.Start(ctx); err != nil {
			if err != context.Canceled {
				fmt.Fprintf(os.Stderr, "network error: %v\n", err)
			}
		}
	}()
}

// SaveState persists player state and playlists.
func (a *App) SaveState() {
	state := &storage.PlayerState{
		Volume: int(a.Player.GetVolume()),
	}
	if song := a.Player.GetCurrentSong(); song != nil {
		state.LastSong = song.Title
		state.Position = a.Player.GetPosition()
	}

	storage.SaveState(state)
	storage.SavePlaylists(a.PM)
	config.SaveConfig(a.V, a.Cfg)
}

// Close shuts down the application cleanly.
func (a *App) Close() {
	if a.cancel != nil {
		a.cancel()
	}
	if a.NetMgr != nil {
		a.NetMgr.Close()
	}
	a.SaveState()
}
