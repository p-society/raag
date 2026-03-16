package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/p-society/raag/internal/constants"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config holds persistent configuration values only.
// Runtime state (volume, playback, etc.) is stored in state.json.
type Config struct {
	// Network - persistent network settings
	Host       string `json:"network.host"`
	Port       int    `json:"network.port"`
	Rendezvous string `json:"network.rendezvous"`

	// Discovery - persistent discovery settings
	TrackerURL     string   `json:"discovery.tracker_url"`
	DHTEnabled     bool     `json:"discovery.dht_enabled"`
	MaxPeers       int      `json:"discovery.max_peers"`
	BootstrapPeers []string `json:"discovery.bootstrap_peers"`
	AuthSecret     string   `mapstructure:"-" json:"-"`

	// Playback - persistent playback settings
	MusicDir string `json:"playback.music_dir"`
	// Runtime - these are set at startup
	Network  bool   `mapstructure:"-" json:"-"`
	Volume   int    `mapstructure:"-" json:"-"`
	TUI      bool   `mapstructure:"-" json:"-"`
	LogLevel string `mapstructure:"-" json:"-"`
}

func DefaultConfig() Config {
	musicDir := "./music"
	if defaultMusicDir, err := MusicDir(); err == nil {
		musicDir = defaultMusicDir
	}
	return Config{
		Host:           constants.DefaultHost,
		Port:           constants.DefaultPort,
		Rendezvous:     constants.DefaultRendezvous,
		TrackerURL:     "",
		DHTEnabled:     true,
		MaxPeers:       constants.DefaultMaxPeers,
		BootstrapPeers: []string{},
		MusicDir:       musicDir,
		Volume:         constants.DefaultVolume,
		Network:        false,
		LogLevel:       "info",
	}
}

// InitViper initializes Viper with defaults and binds flags
func InitViper(cmd *cobra.Command) (*viper.Viper, error) {
	v := viper.New()
	configDir, err := Dir()
	if err != nil {
		return nil, fmt.Errorf("could not get user config dir: %w", err)
	}

	configFile, err := FilePath()
	if err != nil {
		return nil, fmt.Errorf("could not determine config file: %w", err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, fmt.Errorf("could not create config dir: %w", err)
	}

	v.SetConfigType("yaml")
	v.SetConfigFile(configFile)
	setDefaults(v)

	if _, err := os.Stat(configFile); err == nil {
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("could not read config file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("could not check config file: %w", err)
	} else {
		if err := v.WriteConfigAs(configFile); err != nil {
			return nil, fmt.Errorf("could not write default config: %w", err)
		}
	}

	bindFlags(v, cmd)
	return v, nil
}

func setDefaults(v *viper.Viper) {
	defaults := DefaultConfig()
	v.SetDefault("network.host", defaults.Host)
	v.SetDefault("network.port", defaults.Port)
	v.SetDefault("network.rendezvous", defaults.Rendezvous)
	v.SetDefault("discovery.tracker_url", defaults.TrackerURL)
	v.SetDefault("discovery.dht_enabled", defaults.DHTEnabled)
	v.SetDefault("discovery.max_peers", defaults.MaxPeers)
	v.SetDefault("discovery.bootstrap_peers", defaults.BootstrapPeers)
	v.SetDefault("discovery.auth_secret", os.Getenv("AUTH_SECRET"))
	v.SetDefault("playback.music_dir", defaults.MusicDir)
}

func bindFlags(v *viper.Viper, cmd *cobra.Command) {
	root := cmd.Root()
	v.BindPFlag("network.host", root.PersistentFlags().Lookup("host"))
	v.BindPFlag("network.port", root.PersistentFlags().Lookup("port"))
	v.BindPFlag("network.rendezvous", root.PersistentFlags().Lookup("rendezvous"))
	v.BindPFlag("discovery.tracker_url", root.PersistentFlags().Lookup("tracker"))
	v.BindPFlag("discovery.dht_enabled", root.PersistentFlags().Lookup("dht"))
	v.BindPFlag("discovery.max_peers", root.PersistentFlags().Lookup("max-peers"))
	v.BindPFlag("discovery.bootstrap_peers", root.PersistentFlags().Lookup("bootstrap"))
	v.BindPFlag("discovery.auth_secret", root.PersistentFlags().Lookup("auth-secret"))
	v.BindPFlag("playback.music_dir", root.PersistentFlags().Lookup("music-dir"))
}

// LoadConfig loads configuration from Viper instance.
func LoadConfig(v *viper.Viper) (*Config, error) {
	cfg := &Config{
		Host:           v.GetString("network.host"),
		Port:           v.GetInt("network.port"),
		Rendezvous:     v.GetString("network.rendezvous"),
		TrackerURL:     v.GetString("discovery.tracker_url"),
		DHTEnabled:     v.GetBool("discovery.dht_enabled"),
		MaxPeers:       v.GetInt("discovery.max_peers"),
		BootstrapPeers: v.GetStringSlice("discovery.bootstrap_peers"),
		AuthSecret:     v.GetString("discovery.auth_secret"),
		MusicDir:       v.GetString("playback.music_dir"),
	}

	if cfg.TrackerURL == "" {
		if url := os.Getenv(constants.EnvTrackerURL); url != "" {
			cfg.TrackerURL = url
		} else {
			cfg.TrackerURL = constants.DefaultTrackerURL
		}
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid port number: %d", cfg.Port)
	}
	return cfg, nil
}

// SaveConfig saves persistent configuration values to file.
func SaveConfig(v *viper.Viper, cfg *Config) error {
	jsonBytes, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	var configMap map[string]any
	decoder := json.NewDecoder(bytes.NewReader(jsonBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&configMap); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}
	for key, value := range configMap {
		if value == nil {
			continue
		}
		v.Set(key, value)
	}
	return v.WriteConfig()
}
