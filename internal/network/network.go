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
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/multiformats/go-multiaddr"
	"github.com/p-society/raag/internal/config"
	"github.com/p-society/raag/internal/library"
	"github.com/p-society/raag/internal/metadata"
)

//TODO :- offline and online detection by the application

type NetworkManager struct {
	host      host.Host
	cfg       *config.Config
	library   *library.Library
	peers     map[peer.ID]struct{}
	peersLock sync.RWMutex
}

type discoveryNotifee struct {
	PeerChan chan peer.AddrInfo
}

func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	n.PeerChan <- pi
}

func NewNetwork(cfg *config.Config, lib *library.Library) (*NetworkManager, error) {
	prvKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	var opts []libp2p.Option

	sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", cfg.ListenHost, cfg.ListenPort))

	opts = append(opts, libp2p.ListenAddrs(sourceMultiAddr), libp2p.Identity(prvKey))
	if cfg.Offline {
		opts = append(opts, libp2p.NoTransports, libp2p.Transport(tcp.NewTCPTransport))
		opts = append(opts, libp2p.ConnectionManager(NewConnectionManager(10, 15, time.Minute)))

	} else if cfg.Wifi {
		opts = append(opts, libp2p.DefaultTransports)
	}

	host, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	nm := &NetworkManager{
		host:    host,
		cfg:     cfg,
		library: lib,
		peers:   make(map[peer.ID]struct{}),
	}

	host.Network().Notify(&network.NotifyBundle{
		DisconnectedF: func(n network.Network, conn network.Conn) {
			nm.handlePeerDisconnect(conn.RemotePeer(), conn.RemoteMultiaddr())
		},
	})

	return nm, nil
}

func NewConnectionManager(low, high int, gracePeriod time.Duration) *connmgr.BasicConnMgr {
	cm, _ := connmgr.NewConnManager(low, high, connmgr.WithGracePeriod(gracePeriod))
	return cm
}

func (n *NetworkManager) Start(ctx context.Context) error {
	n.host.SetStreamHandler(protocol.ID(n.cfg.ProtocolID), n.handleStream)

	if n.cfg.Offline {
		peerChan := n.initMDNS(n.host, n.cfg.RendezvousString)
		go n.discoverPeers(ctx, peerChan)
	} else if n.cfg.Wifi {
		peerChan := n.initMDNS(n.host, n.cfg.RendezvousString)
		go n.discoverPeers(ctx, peerChan)
	}

	//TODO :- if wifi enabled, should we use kademlia along with mDNS for peer discovery?

	log.Printf("Your Raag Node Multiaddress Is: /ip4/%s/tcp/%v/p2p/%s\n", n.cfg.ListenHost, n.cfg.ListenPort, n.host.ID())

	<-ctx.Done()
	return ctx.Err()

}

func (n *NetworkManager) discoverPeers(ctx context.Context, peerChan <-chan peer.AddrInfo) {
	for {
		select {
		case peer := <-peerChan:
			go n.handlePeer(ctx, peer)
		case <-ctx.Done():
			return
		}
	}
}

func (n *NetworkManager) handlePeer(ctx context.Context, peerInfo peer.AddrInfo) {
	if err := n.host.Connect(ctx, peerInfo); err != nil {
		log.Printf("Connection failed: %v\n", err)
		return
	}

	n.peersLock.Lock()
	n.peers[peerInfo.ID] = struct{}{}
	n.peersLock.Unlock()

	log.Printf("Connected to peer: %s\n", peerInfo.ID)
}

func (n *NetworkManager) ShareSong(peerInfo *peer.AddrInfo, song metadata.Song) error {
	log.Printf("ShareSong function called with peerInfo: %+v and song: %+v\n", peerInfo, song)
	dataChan := make(chan []byte)
	go func() {
		defer close(dataChan)
		file, err := os.Open(song.Path)
		if err != nil {
			log.Printf("Error opening file: %s\n", err)
			return
		}
		defer file.Close()

		buffer := make([]byte, 1024)
		for {
			n, err := file.Read(buffer)
			if err != nil && err != io.EOF {
				log.Printf("Error reading file: %s\n", err)
				return
			}
			if n == 0 {
				break
			}
			dataChan <- buffer[:n]
		}
	}()

	stream, err := n.host.NewStream(context.Background(), peerInfo.ID, protocol.ID(n.cfg.ProtocolID))
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}
	defer stream.Close()

	mdata := metadata.FormatMetadata(song)
	if _, err = stream.Write([]byte(mdata)); err != nil {
		return fmt.Errorf("failed to send song metadata: %w", err)
	}

	for data := range dataChan {
		if _, err = stream.Write(data); err != nil {
			return fmt.Errorf("failed to send song data: %w", err)
		}
	}

	log.Printf("ShareSong function completed successfully\n")
	return nil
}

func (n *NetworkManager) handleStream(stream network.Stream) {
	defer stream.Close()

	peerID := stream.Conn().RemotePeer()
	log.Printf("handleStream called for peer: %s\n", peerID)

	buf := make([]byte, 1024)
	size, err := stream.Read(buf)
	if err != nil {
		if err == io.EOF {
			log.Printf("Stream closed by peer %s before sending data\n", peerID)
		} else {
			log.Printf("Error reading metadata from peer %s: %s\n", peerID, err)
		}
		return
	}

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

func (n *NetworkManager) initMDNS(peerhost host.Host, rendezvous string) <-chan peer.AddrInfo {
	notifee := &discoveryNotifee{}
	peerChan := make(chan peer.AddrInfo)
	notifee.PeerChan = peerChan

	service := mdns.NewMdnsService(peerhost, rendezvous, notifee)
	if err := service.Start(); err != nil {
		panic(err)
	}

	return peerChan
}

func (n *NetworkManager) handlePeerDisconnect(peerId peer.ID, addr multiaddr.Multiaddr) {
	n.peersLock.Lock()
	defer n.peersLock.Unlock()

	if _, ok := n.peers[peerId]; ok {
		delete(n.peers, peerId)
		log.Printf("Peer %s has disconnected: %s", peerId, addr.String())
	}
}
