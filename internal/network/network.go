package network

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/multiformats/go-multiaddr"
	"github.com/p-society/raag/internal/config"
	"github.com/p-society/raag/internal/library"
)

type Network struct {
	host    host.Host
	cfg     *config.Config
	library *library.Library
}

type discoveryNotifee struct {
	PeerChan chan peer.AddrInfo
}

func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	n.PeerChan <- pi
}

func NewNetwork(cfg *config.Config, lib *library.Library) (*Network, error) {
	prvKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", cfg.ListenHost, cfg.ListenPort))

	host, err := libp2p.New(
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.Identity(prvKey),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	return &Network{
		host:    host,
		cfg:     cfg,
		library: lib,
	}, nil
}

func (n *Network) Start(ctx context.Context) error {
	n.host.SetStreamHandler(protocol.ID(n.cfg.ProtocolID), n.handleStream)

	peerChan := n.initMDNS(n.host, n.cfg.RendezvousString)

	fmt.Printf("\n[*] Your Raag Node Multiaddress Is: /ip4/%s/tcp/%v/p2p/%s\n", n.cfg.ListenHost, n.cfg.ListenPort, n.host.ID())

	for {
		select {
		case peer := <-peerChan:
			go n.handlePeer(ctx, peer)
		case <-ctx.Done():
			return nil
		}
	}
}

func (n *Network) handleStream(stream network.Stream) {
	fmt.Println("New peer connected!")
}

func (n *Network) handlePeer(ctx context.Context, peer peer.AddrInfo) {
	if err := n.host.Connect(ctx, peer); err != nil {
		fmt.Printf("Connection failed: %v\n", err)
		return
	}

	stream, err := n.host.NewStream(ctx, peer.ID, protocol.ID(n.cfg.ProtocolID))
	if err != nil {
		fmt.Printf("Stream open failed: %v\n", err)
		return
	}

	fmt.Printf("Connected to: %s\n", peer)
	n.handleStream(stream)
}

func (n *Network) initMDNS(peerhost host.Host, rendezvous string) <-chan peer.AddrInfo {
	notifee := &discoveryNotifee{}
	peerChan := make(chan peer.AddrInfo)
	notifee.PeerChan = peerChan

	service := mdns.NewMdnsService(peerhost, rendezvous, notifee)
	if err := service.Start(); err != nil {
		panic(err)
	}

	return peerChan
}
