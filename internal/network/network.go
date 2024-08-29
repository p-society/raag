package network

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

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
	host         host.Host
	cfg          *config.Config
	library      *library.Library
	peers        map[peer.ID]struct{}
	peersLock    sync.RWMutex
	isNewNetwork bool
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
		peers:   make(map[peer.ID]struct{}),
	}, nil
}

func (n *Network) Start(ctx context.Context) error {
	n.host.SetStreamHandler(protocol.ID(n.cfg.ProtocolID), n.handleStream)
	peerChan := n.initMDNS(n.host, n.cfg.RendezvousString)

	log.Printf("Your Raag Node Multiaddress Is: /ip4/%s/tcp/%v/p2p/%s\n", n.cfg.ListenHost, n.cfg.ListenPort, n.host.ID())

	maxWait := 30 * time.Second
	waitInterval := 1 * time.Second
	waited := 0 * time.Second
	extendedWait := false

	for waited < maxWait || extendedWait {
		n.peersLock.RLock()
		peerCount := len(n.peers)
		n.peersLock.RUnlock()

		if peerCount > 0 {
			log.Printf("Found %d peers with the current protocol ID.\n", peerCount)
			break
		}

		select {
		case peer := <-peerChan:
			go n.handlePeer(ctx, peer)
			extendedWait = true
			waited = 0
		default:
			if !extendedWait {
				log.Println("Waiting for peers...")
			}
			extendedWait = false
			time.Sleep(waitInterval)
			waited += waitInterval
		}
	}

	if !extendedWait && waited >= maxWait {
		log.Println("No peers found with the current protocol ID. Starting a new network.")
		n.isNewNetwork = true
	}

	for {
		select {
		case peer := <-peerChan:
			go n.handlePeer(ctx, peer)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (n *Network) handlePeer(ctx context.Context, peerInfo peer.AddrInfo) {
	if err := n.host.Connect(ctx, peerInfo); err != nil {
		log.Printf("Connection failed: %v\n", err)
		return
	}

	stream, err := n.host.NewStream(ctx, peerInfo.ID, protocol.ID(n.cfg.ProtocolID))
	if err != nil {
		log.Printf("Stream open failed: %v\n", err)
		return
	}
	defer stream.Close()

	n.peersLock.Lock()
	n.peers[peerInfo.ID] = struct{}{}
	n.peersLock.Unlock()

	log.Printf("Connected to peer: %s\n", peerInfo.ID)
}

func (n *Network) handleStream(stream network.Stream) {
	defer stream.Close()

	remotePeer := stream.Conn().RemotePeer().String()
	log.Printf("New stream from: %s\n", remotePeer)

	buf := make([]byte, 1024)
	amt, err := stream.Read(buf)
	if err != nil {
		if err != io.EOF {
			log.Printf("Error reading from stream: %v\n", err)
		}
		return
	}

	message := string(buf[:amt])
	log.Printf("Received message from %s: %s\n", remotePeer, message)

	response := []byte("Hello from Raag!")
	_, err = stream.Write(response)
	if err != nil {
		log.Printf("Error writing to stream: %v\n", err)
		return
	}

	log.Printf("Sent response to %s: %s\n", remotePeer, string(response))
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

func (n *Network) IsNewNetwork() bool {
	return n.isNewNetwork
}
