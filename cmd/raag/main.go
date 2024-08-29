package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/p-society/raag/internal/cli"
	"github.com/p-society/raag/internal/config"
	"github.com/p-society/raag/internal/library"
	"github.com/p-society/raag/internal/network"
	"github.com/p-society/raag/internal/player"
)

func main() {
	cfg, err := config.ParseFlags()
	if err != nil {
		log.Fatalf("Error parsing flags: %v", err)
	}

	lib, err := library.NewLibrary(cfg.MusicDir)
	if err != nil {
		log.Fatalf("Error initializing library: %v", err)
	}

	p, err := player.NewPlayer()
	if err != nil {
		log.Fatalf("Error initializing player: %v", err)
	}

	net, err := network.NewNetwork(cfg, lib)
	if err != nil {
		log.Fatalf("Error initializing network: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- net.Start(ctx)
	}()

	cli := cli.NewCLI(lib, p, net)
	go cli.Start()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigChan:
		fmt.Println("\nShutting down...")
	case err := <-errChan:
		log.Printf("Error in network: %v", err)
	}
}
