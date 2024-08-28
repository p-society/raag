package p2p

import (
	"bufio"
	"context"
	"fmt"
	"os"

	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	discovery "github.com/libp2p/go-libp2p/core/discovery"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

func StartP2PNode() error {
	ctx := context.Background()
	host, err := libp2p.New()
	if err != nil {
		return fmt.Errorf("failed to create host: %w", err)
	}

	kad, err := dht.New(ctx, host)
	if err != nil {
		return fmt.Errorf("failed to initialize DHT: %w", err)
	}

	if err = kad.Bootstrap(ctx); err != nil {
		return fmt.Errorf("failed to bootstrap kademlia DHT: %w", err)
	}

	ps, err := pubsub.NewGossipSub(ctx, host)
	if err != nil {
		return fmt.Errorf("failed to create pubsub connection: %w", err)
	}

	topic, err := ps.Join("raag-test")
	if err != nil {
		return fmt.Errorf("failed to join topic")
	}

	sub, err := topic.Subscribe()
	if err != nil {
		return fmt.Errorf("failed to subscribe to topic: %w", err)
	}

	go func() {
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				fmt.Printf("failed to receive message: %s\n", err)
				continue
			}
			fmt.Printf("%s: %s\n", msg.ReceivedFrom, string(msg.Data))
		}
	}()

	rd := drouting.NewRoutingDiscovery(kad)
	_, err = discovery.Discovery.Advertise(rd, ctx, "raag")
	if err != nil {
		return fmt.Errorf("failed to advertise service: %w", err)
	}

	peerChan, err := rd.FindPeers(ctx, "raag")
	if err != nil {
		return fmt.Errorf("failed to find peers: %w", err)
	}

	go func() {
		for peer := range peerChan {
			if peer.ID == host.ID() {
				continue
			}

			host.Connect(ctx, peer)
		}
	}()

	fmt.Println("Node addresses:")
	for _, addr := range host.Addrs() {
		fmt.Printf("%s ---> %s\n", addr, host.ID())
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		msg, _ := reader.ReadString('\n')
		if err := topic.Publish(ctx, []byte(msg)); err != nil {
			fmt.Printf("failed to publish message: %s\n", err)
		}
	}
}
