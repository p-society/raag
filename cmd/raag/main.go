package main

import (
	"context"
	"fmt"
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
	cfg := config.ParseFlags()

	lib, err := library.NewLibrary(cfg.MusicDir)
	if err != nil {
		fmt.Printf("Error initializing library: %v\n", err)
		os.Exit(1)
	}

	p := player.NewPlayer()

	net, err := network.NewNetwork(cfg, lib)
	if err != nil {
		fmt.Printf("Error initializing network: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := net.Start(ctx); err != nil {
			fmt.Printf("Error starting network: %v\n", err)
			cancel()
		}
	}()

	cli := cli.NewCLI(lib, p, net)
	go cli.Start()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
}
