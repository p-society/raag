package network

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
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
	"github.com/p-society/raag/internal/metadata"
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

func (n *Network) ShareSong(peerInfo *peer.AddrInfo, song metadata.Song) error {
	log.Printf("ShareSong function called with peerInfo: %+v and song: %+v\n", peerInfo, song)

	log.Printf("Creating new stream to peer %s\n", peerInfo.ID)
	stream, err := n.host.NewStream(context.Background(), peerInfo.ID, protocol.ID(n.cfg.ProtocolID))
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}
	defer stream.Close()
	log.Printf("Stream created successfully\n")

	mdata := metadata.FormatMetadata(song)
	log.Printf("Sending metadata: %s\n", mdata)
	if _, err = stream.Write([]byte(mdata)); err != nil {
		return fmt.Errorf("failed to send song metadata: %w", err)
	}
	log.Printf("Metadata sent successfully\n")

	log.Printf("Opening file: %s\n", song.Path)
	file, err := os.Open(song.Path)
	if err != nil {
		return fmt.Errorf("failed to open song file: %w", err)
	}
	defer file.Close()

	log.Printf("Sending file data...\n")
	bytesWritten, err := io.Copy(stream, file)
	if err != nil {
		return fmt.Errorf("failed to send song data: %w", err)
	}
	log.Printf("File data sent. Bytes written: %d\n", bytesWritten)

	log.Printf("ShareSong function completed successfully\n")
	return nil
}

func (n *Network) handleStream(stream network.Stream) {
	defer stream.Close()

	peerID := stream.Conn().RemotePeer()
	log.Printf("handleStream called for peer: %s\n", peerID)
	if err := stream.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		log.Printf("Error setting read deadline: %s\n", err)
		return
	}

	buf := make([]byte, 1024)
	log.Printf("Reading metadata from stream...\n")
	size, err := stream.Read(buf)
	if err != nil {
		if err == io.EOF {
			log.Printf("Stream closed by peer %s before sending data\n", peerID)
		} else {
			log.Printf("Error reading metadata from peer %s: %s\n", peerID, err)
		}
		return
	}

	if err := stream.SetReadDeadline(time.Time{}); err != nil {
		log.Printf("Error clearing read deadline: %s\n", err)
		return
	}

	log.Printf("Read %d bytes of metadata from peer %s\n", size, peerID)

	if size == 0 {
		log.Printf("Received empty stream from peer %s, ignoring\n", peerID)
		return
	}

	mdata := string(buf[:size])
	log.Printf("Received metadata: %s\n", mdata)
	songInfo := strings.Split(mdata, "|")
	if len(songInfo) < 3 {
		log.Printf("Invalid song metadata, fields may be missing or corrupted")
		return
	}

	title := songInfo[0]
	peerId := peerID.String()
	fileName := fmt.Sprintf("%s_%s.mp3", peerId, title)
	log.Printf("Preparing to save file as: %s\n", fileName)

	file, err := os.Create(fileName)
	if err != nil {
		log.Printf("Error creating file: %s\n", err)
		return
	}
	defer file.Close()

	log.Printf("Copying song data from stream to file...\n")
	bytesWritten, err := io.Copy(file, stream)
	if err != nil {
		log.Printf("Error saving song: %s\n", err)
		return
	}
	log.Printf("Song data saved. Bytes written: %d\n", bytesWritten)

	log.Printf("Successfully received and saved '%s' from peer '%s' as '%s'\n", title, peerID, fileName)
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
