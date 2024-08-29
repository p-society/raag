package config

import (
	"flag"
	"fmt"
)

type Config struct {
	RendezvousString string
	ProtocolID       string
	ListenHost       string
	ListenPort       int
	MusicDir         string
}

func ParseFlags() (*Config, error) {
	c := &Config{}
	flag.StringVar(&c.RendezvousString, "rendezvous", "raag-music-share", "Unique string to identify Raag nodes on the local network")
	flag.StringVar(&c.ListenHost, "host", "0.0.0.0", "The host address to listen on")
	flag.StringVar(&c.ProtocolID, "pid", "/raag/1.0.0", "Sets a protocol id for stream headers")
	flag.IntVar(&c.ListenPort, "port", 0, "Node listen port (0 to pick a random unused port)")
	flag.StringVar(&c.MusicDir, "musicdir", "./music", "Directory containing music files")
	flag.Parse()

	if c.ListenPort < 0 || c.ListenPort > 65535 {
		return nil, fmt.Errorf("invalid port number: %d", c.ListenPort)
	}

	return c, nil
}
