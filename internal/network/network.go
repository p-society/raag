package network

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/multiformats/go-multiaddr"
	"github.com/p-society/raag/internal/auth"
	"github.com/p-society/raag/internal/config"
	"github.com/p-society/raag/internal/constants"
	"github.com/p-society/raag/internal/discovery"
	"github.com/p-society/raag/internal/library"
	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/internal/metadata"
	"github.com/spf13/viper"
)

func generateSessionNonce() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		nonce := time.Now().UnixNano()
		return fmt.Sprintf("%d", nonce)
	}
	return hex.EncodeToString(bytes)
}

type serializedKey struct {
	Type string `json:"type"`
	Data []byte `json:"data"`
}

func loadOrGenerateIdentity() (crypto.PrivKey, error) {
	keyPath, err := config.IdentityKeyPath()
	if err != nil {
		logger.Warnf("Could not get identity key path, generating new key: %v", err)
		key, _, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
		if err != nil {
			return nil, err
		}
		return key, nil
	}

	data, err := os.ReadFile(keyPath)
	if err == nil {
		var sk serializedKey
		if err := json.Unmarshal(data, &sk); err != nil {
			logger.Warnf("Could not parse identity key, generating new one: %v", err)
			return generateAndSaveKey(keyPath)
		}

		switch sk.Type {
		case "RSA", "Ed25519":
		default:
			logger.Warnf("Unknown key type %s, generating new key", sk.Type)
			return generateAndSaveKey(keyPath)
		}

		key, err := crypto.UnmarshalPrivateKey(sk.Data)
		if err != nil {
			logger.Warnf("Could not unmarshal identity key, generating new one: %v", err)
			return generateAndSaveKey(keyPath)
		}

		logger.Debugf("Loaded existing identity key from %s", keyPath)
		return key, nil
	}
	if !os.IsNotExist(err) {
		logger.Warnf("Error reading identity key: %v", err)
	}
	return generateAndSaveKey(keyPath)
}

func generateAndSaveKey(keyPath string) (crypto.PrivKey, error) {
	key, _, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	keyData, err := crypto.MarshalPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal key: %w", err)
	}

	sk := serializedKey{
		Type: "RSA",
		Data: keyData,
	}

	data, err := json.Marshal(sk)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal key data: %w", err)
	}

	dir := filepath.Dir(keyPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := os.WriteFile(keyPath, data, 0o600); err != nil {
		logger.Warnf("Failed to save identity key: %v", err)
	} else {
		logger.Debugf("Generated and saved new identity key to %s", keyPath)
	}
	return key, nil
}

type idleTimeoutWriter struct {
	stream  network.Stream
	timeout time.Duration
}

func (w *idleTimeoutWriter) Write(p []byte) (int, error) {
	w.stream.SetWriteDeadline(time.Now().Add(w.timeout))
	return w.stream.Write(p)
}

type idleTimeoutReader struct {
	stream  network.Stream
	timeout time.Duration
}

func (r *idleTimeoutReader) Read(p []byte) (int, error) {
	r.stream.SetReadDeadline(time.Now().Add(r.timeout))
	return r.stream.Read(p)
}

type NetworkManager struct {
	host            host.Host
	ctx             context.Context
	cfg             *config.Config
	viper           *viper.Viper
	library         *library.Library
	musicDir        string
	Online          bool
	OnPeerJoin      func(peer.ID)
	OnPeerLeave     func(peer.ID)
	OnStateChange   func(bool)
	discovery       *discovery.Manager
	authEnabled     bool
	authorizedPeers map[string]struct{}
	authorizedMu    sync.RWMutex
	usedNonces      map[string]time.Time
	nonceMu         sync.Mutex
	peerStateMu     sync.RWMutex
	connectedPeers  map[peer.ID]bool
	peerConnectCh   chan struct{}
}

func (n *NetworkManager) getContext() context.Context {
	if n.ctx != nil {
		return n.ctx
	}
	return context.Background()
}

func (n *NetworkManager) Host() host.Host {
	return n.host
}

func (n *NetworkManager) Close() error {
	if n.host == nil {
		return nil
	}
	return n.host.Close()
}

func (n *NetworkManager) SetAuthEnabled(enabled bool) {
	n.authorizedMu.Lock()
	defer n.authorizedMu.Unlock()
	n.authEnabled = enabled
	if enabled {
		n.authorizedPeers = make(map[string]struct{})
		logger.Infof("P2P authentication enabled")
	} else {
		n.authorizedPeers = nil
		logger.Infof("P2P authentication disabled")
	}
}

func (n *NetworkManager) AuthorizePeer(peerID peer.ID) {
	n.authorizedMu.Lock()
	defer n.authorizedMu.Unlock()
	if n.authorizedPeers != nil {
		n.authorizedPeers[peerID.String()] = struct{}{}
		logger.Debugf("Peer authorized: %s", peerID)
	}
}

func (n *NetworkManager) IsPeerAuthorized(peerID peer.ID) bool {
	n.authorizedMu.RLock()
	defer n.authorizedMu.RUnlock()
	if !n.authEnabled {
		return true
	}
	if n.authorizedPeers == nil {
		return false
	}
	_, authorized := n.authorizedPeers[peerID.String()]
	return authorized
}

func (n *NetworkManager) GetAuthData() (string, error) {
	if n.discovery != nil {
		return n.discovery.GetAuthData()
	}
	return "", fmt.Errorf("discovery manager not available")
}

func (n *NetworkManager) IsAuthEnabled() bool {
	n.authorizedMu.RLock()
	defer n.authorizedMu.RUnlock()
	return n.authEnabled
}

func (n *NetworkManager) VerifyAuthToken(tokenData string, expectedPeerID string, sessionNonce string) error {
	if tokenData == "" {
		if n.IsAuthEnabled() {
			return fmt.Errorf("authentication required but no token provided")
		}
		return nil
	}

	token, err := auth.DeserializeToken(tokenData)
	if err != nil {
		return fmt.Errorf("invalid token format: %w", err)
	}

	if expectedPeerID != "" && token.PeerID != expectedPeerID {
		return fmt.Errorf("peer ID mismatch: token is for %s, expected %s", token.PeerID, expectedPeerID)
	}

	if err := n.verifyNonceUsed(sessionNonce); err != nil {
		return err
	}

	valid, err := auth.VerifyToken(token)
	if err != nil {
		return fmt.Errorf("token verification failed: %w", err)
	}
	if !valid {
		return fmt.Errorf("token verification failed")
	}

	return nil
}

func (n *NetworkManager) verifyNonceUsed(nonce string) error {
	if nonce == "" {
		return nil
	}

	n.nonceMu.Lock()
	defer n.nonceMu.Unlock()

	if n.usedNonces == nil {
		n.usedNonces = make(map[string]time.Time)
	}

	if _, exists := n.usedNonces[nonce]; exists {
		return fmt.Errorf("replay attack detected: nonce already used")
	}

	n.usedNonces[nonce] = time.Now()

	go func() {
		time.Sleep(5 * time.Minute)
		n.nonceMu.Lock()
		delete(n.usedNonces, nonce)
		n.nonceMu.Unlock()
	}()

	return nil
}

func NewNetwork(cfg *config.Config, v *viper.Viper, lib *library.Library, musicDir string) (*NetworkManager, error) {
	return newNetworkWithIdentity(cfg, v, lib, musicDir, nil)
}

func newNetworkWithIdentity(cfg *config.Config, v *viper.Viper, lib *library.Library, musicDir string, identity crypto.PrivKey) (*NetworkManager, error) {
	logger.Debugf("Network config network=%v host=%s port=%d rendezvous=%s", cfg.Network, cfg.Host, cfg.Port, cfg.Rendezvous)

	prvKey := identity
	var err error
	if prvKey == nil {
		prvKey, err = loadOrGenerateIdentity()
		if err != nil {
			return nil, fmt.Errorf("failed to load identity: %w", err)
		}
	}

	var opts []libp2p.Option
	tcpListenAddr := fmt.Sprintf("/ip4/%s/tcp/%d", cfg.Host, cfg.Port)
	quicListenAddr := fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", cfg.Host, cfg.Port)
	tcpMultiAddr, _ := multiaddr.NewMultiaddr(tcpListenAddr)
	quicMultiAddr, _ := multiaddr.NewMultiaddr(quicListenAddr)
	opts = append(opts, libp2p.ListenAddrs(tcpMultiAddr, quicMultiAddr), libp2p.Identity(prvKey))
	if cfg.Network {
		logger.Debugf("Using networked mode with NAT traversal")
		opts = append(opts, libp2p.DefaultTransports)
		if resourceManager, err := newResourceManager(); err != nil {
			logger.Warnf("Failed to initialize resource manager error=%v", err)
		} else {
			opts = append(opts, libp2p.ResourceManager(resourceManager))
		}

		opts = append(opts, libp2p.EnableRelay())
		opts = append(opts, libp2p.EnableHolePunching())
		opts = append(opts, libp2p.NATPortMap())
		opts = append(opts, libp2p.EnableNATService())
		logger.Debugf("NAT traversal enabled: circuit relay, hole punching, UPnP, AutoNAT")
	} else {
		logger.Debugf("Using offline mode with limited transports")
		opts = append(opts, libp2p.DefaultTransports)
	}

	host, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	maxPeers := cfg.MaxPeers
	if maxPeers == 0 {
		maxPeers = constants.DefaultMaxPeers
	}

	identityKeyBytes, err := crypto.MarshalPrivateKey(prvKey)
	if err != nil {
		logger.Warnf("Failed to marshal identity key: %v", err)
	}

	discoveryMgr := discovery.NewManager(discovery.ManagerConfig{
		Host:             host,
		IdentityKeyBytes: identityKeyBytes,
		AuthSecret:       cfg.AuthSecret,
		TrackerURL:       cfg.TrackerURL,
		MaxPeers:         maxPeers,
		ListenHost:       cfg.Host,
		Rendezvous:       cfg.Rendezvous,
		DHTEnabled:       cfg.DHTEnabled,
		BootstrapPeers:   cfg.BootstrapPeers,
	})
	nm := &NetworkManager{
		host:          host,
		cfg:           cfg,
		viper:         v,
		library:       lib,
		musicDir:      musicDir,
		discovery:     discoveryMgr,
		peerConnectCh: make(chan struct{}, 1),
	}

	discoveryMgr.SetNetworkManager(nm)
	host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, conn network.Conn) {
			nm.handlePeerConnect(conn.RemotePeer(), conn.RemoteMultiaddr())
		},
		DisconnectedF: func(n network.Network, conn network.Conn) {
			nm.handlePeerDisconnect(conn.RemotePeer(), conn.RemoteMultiaddr())
		},
	})

	return nm, nil
}

func newResourceManager() (network.ResourceManager, error) {
	limiter := rcmgr.DefaultLimits.AutoScale()
	return rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(limiter))
}

func (n *NetworkManager) Start(ctx context.Context) error {
	n.ctx = ctx
	n.connectedPeers = make(map[peer.ID]bool)
	n.host.SetStreamHandler(protocol.ID(constants.ShareProtocolID), n.handleStream)
	n.host.SetStreamHandler(protocol.ID(constants.PresenceProtocolID), n.handlePresence)
	if err := n.discovery.Start(ctx); err != nil {
		logger.Errorf("Discovery failed to start error=%v", err)
	}

	addrs := n.host.Addrs()
	if len(addrs) > 0 {
		logger.Debugf("Your Raag Node Multiaddress address=%s", fmt.Sprintf("%s/p2p/%s", addrs[0], n.host.ID()))
		if len(addrs) > 1 {
			logger.Debugf("Additional addresses")
			for _, addr := range addrs[1:] {
				logger.Debugf("Additional address address=%s", fmt.Sprintf("%s/p2p/%s", addr, n.host.ID()))
			}
		}
		if n.cfg.Network {
			logger.Debugf("TIP: If peers cannot connect, ensure firewall allows incoming connections on port %d (TCP)", n.cfg.Port)
		}
	} else {
		logger.Warnf("No listening addresses found host=%s port=%d", n.cfg.Host, n.cfg.Port)
	}

	<-ctx.Done()
	return ctx.Err()
}

// GetPeers returns information about currently connected peers
func (n *NetworkManager) GetPeers() []peer.AddrInfo {
	peerIDs := n.host.Network().Peers()
	peers := make([]peer.AddrInfo, 0, len(peerIDs))
	for _, peerID := range peerIDs {
		if conns := n.host.Network().ConnsToPeer(peerID); len(conns) > 0 {
			peers = append(peers, peer.AddrInfo{
				ID:    peerID,
				Addrs: []multiaddr.Multiaddr{conns[0].RemoteMultiaddr()},
			})
		}
	}
	return peers
}

// IsOnline returns true if there are any known peers
func (n *NetworkManager) IsOnline() bool {
	return len(n.host.Network().Peers()) > 0
}

// GetPeerCount returns the number of known peers
func (n *NetworkManager) GetPeerCount() int {
	return len(n.host.Network().Peers())
}

func (n *NetworkManager) WaitForPeers(timeout time.Duration) bool {
	if n.IsOnline() {
		return true
	}
	select {
	case <-n.peerConnectCh:
		return n.IsOnline()
	case <-time.After(timeout):
		return n.IsOnline()
	}
}

func (n *NetworkManager) Connect(ctx context.Context, addrInfo peer.AddrInfo) error {
	if err := n.host.Connect(ctx, addrInfo); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	logger.Debugf("Connected to peer peer_id=%s", addrInfo.ID)
	if n.discovery != nil {
		n.discovery.RefreshTrackerRegistration(ctx)
	}
	return nil
}

func (n *NetworkManager) Disconnect(peerID peer.ID) error {
	if err := n.host.Network().ClosePeer(peerID); err != nil {
		return fmt.Errorf("failed to disconnect: %w", err)
	}

	logger.Infof("Disconnected from peer peer_id=%s", peerID)
	return nil
}

func (n *NetworkManager) GetPeerID() peer.ID {
	return n.host.ID()
}

func (n *NetworkManager) GetMultiaddr() string {
	addrs := n.host.Addrs()
	if len(addrs) == 0 {
		return fmt.Sprintf("/p2p/%s", n.host.ID())
	}
	for _, addr := range addrs {
		if isUsableAddr(addr.String()) {
			return fmt.Sprintf("%s/p2p/%s", addr, n.host.ID())
		}
	}

	logger.Debugf("No usable LAN address found, using first address: %s", addrs[0].String())
	return fmt.Sprintf("%s/p2p/%s", addrs[0], n.host.ID())
}

func isUsableAddr(addr string) bool {
	ma, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return false
	}

	protocols := ma.Protocols()
	for _, p := range protocols {
		if p.Code == multiaddr.P_IP4 || p.Code == multiaddr.P_IP6 {
			comp, err := ma.ValueForProtocol(p.Code)
			if err != nil {
				continue
			}

			ip, err := netip.ParseAddr(comp)
			if err != nil {
				continue
			}
			if ip.IsLoopback() || ip.IsUnspecified() {
				return false
			}
			return true
		}
	}
	return false
}

func (n *NetworkManager) GetAllKnownPeers() []peer.AddrInfo {
	if n.discovery == nil {
		return nil
	}
	return n.discovery.GetAllPeers()
}

func (n *NetworkManager) LogNetworkState() {
	if n.discovery != nil {
		n.discovery.LogNetworkState()
	}
}

func (n *NetworkManager) GetNetworkState() (discovery.NetworkState, error) {
	if n.discovery == nil {
		return discovery.NetworkState{}, fmt.Errorf("discovery not initialized")
	}
	return n.discovery.GetNetworkState(), nil
}

func (n *NetworkManager) GetAuthPublicKey() string {
	if n.discovery == nil {
		return ""
	}
	return n.discovery.GetAuthPublicKey()
}

func (n *NetworkManager) ShareSong(peerInfo *peer.AddrInfo, song metadata.Song) error {
	logger.Debugf("ShareSong function called peer_info=%v song=%v", peerInfo, song)
	if song.Size > constants.TransferMaxFileSize {
		return fmt.Errorf("file exceeds max transfer size: %d", song.Size)
	}

	file, err := os.Open(song.Path)
	if err != nil {
		return fmt.Errorf("open file for transfer: %w", err)
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(n.getContext(), 10*time.Second)
	defer cancel()
	stream, err := n.host.NewStream(ctx, peerInfo.ID, protocol.ID(constants.ShareProtocolID))
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}
	defer stream.Close()

	authToken, _ := n.GetAuthData()
	sessionNonce := generateSessionNonce()
	meta := buildTransferMetadata(song, song.Size, song.Hash, authToken, sessionNonce)
	if err := writeTransferMetadata(stream, meta); err != nil {
		stream.Reset()
		return fmt.Errorf("failed to send transfer metadata: %w", err)
	}
	writer := &idleTimeoutWriter{stream: stream, timeout: constants.TransferIdleTimeout}
	if _, err := io.Copy(writer, file); err != nil {
		stream.Reset()
		return fmt.Errorf("failed to send song data: %w", err)
	}

	logger.Debugf("ShareSong function completed successfully")
	return nil
}

func (n *NetworkManager) handleStream(stream network.Stream) {
	peerID := stream.Conn().RemotePeer()
	logger.Debugf("handleStream called peer_id=%s", peerID)

	meta, err := readTransferMetadata(stream)
	if err != nil {
		stream.Reset()
		logger.Errorf("Error reading metadata from peer peer_id=%s error=%v", peerID, err)
		return
	}
	defer stream.Close()

	if err := n.VerifyAuthToken(meta.AuthToken, peerID.String(), meta.SessionNonce); err != nil {
		stream.Reset()
		logger.Warnf("Auth verification failed for peer peer_id=%s error=%v", peerID, err)
		return
	}

	if meta.SizeBytes == 0 {
		logger.Debugf("Received empty file transfer from peer peer_id=%s", peerID)
		return
	}
	if meta.SizeBytes > constants.TransferMaxFileSize {
		stream.Reset()
		logger.Errorf("Transfer rejected: file too large peer_id=%s size=%d", peerID, meta.SizeBytes)
		return
	}

	pID := peerID.String()
	safeTitle := sanitizeTransferName(meta.Title)
	ext := strings.ToLower(meta.Extension)
	if ext == "" {
		ext = ".bin"
	}

	fileName := fmt.Sprintf("%s_%s%s", pID, safeTitle, ext)
	saveDir := n.musicDir
	if saveDir == "" {
		saveDir = "."
	}
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		logger.Errorf("Error creating directory error=%v", err)
		return
	}

	filePath := filepath.Join(saveDir, fileName)
	tmpFile, err := os.CreateTemp(saveDir, fileName+".*.part")
	if err != nil {
		stream.Reset()
		logger.Errorf("Error creating temp file error=%v", err)
		return
	}

	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		if _, statErr := os.Stat(tmpPath); statErr == nil {
			_ = os.Remove(tmpPath)
		}
	}()

	logger.Debugf("Preparing to save file file_path=%s", filePath)
	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)
	reader := &idleTimeoutReader{stream: stream, timeout: constants.TransferIdleTimeout}
	bytesWritten, err := io.CopyN(writer, reader, meta.SizeBytes)
	if err != nil {
		stream.Reset()
		logger.Errorf("Error saving song error=%v", err)
		return
	}
	if bytesWritten != meta.SizeBytes {
		stream.Reset()
		logger.Errorf("Incomplete transfer: wrote=%d expected=%d", bytesWritten, meta.SizeBytes)
		return
	}
	if meta.SHA256 != "" {
		digest := fmt.Sprintf("%x", hasher.Sum(nil))
		if digest != meta.SHA256 {
			stream.Reset()
			logger.Errorf("Hash mismatch for received song peer_id=%s expected=%s got=%s", peerID, meta.SHA256, digest)
			return
		}
	}
	if err := tmpFile.Sync(); err != nil {
		stream.Reset()
		logger.Errorf("Error syncing temp file error=%v", err)
		return
	}
	if err := tmpFile.Close(); err != nil {
		stream.Reset()
		logger.Errorf("Error closing temp file error=%v", err)
		return
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		stream.Reset()
		logger.Errorf("Error finalizing received song error=%v", err)
		return
	}
	if n.library != nil {
		if err := n.library.ScanMusicLibrary(n.musicDir); err != nil {
			logger.Warnf("Failed to rescan library after receiving song error=%v", err)
		}
	}

	logger.Debugf("Song data saved bytes_written=%d", bytesWritten)
	logger.Infof("Successfully received and saved song title=%s peer_id=%s file_path=%s", meta.Title, peerID, filePath)
}

func (n *NetworkManager) handlePeerConnect(peerID peer.ID, addr multiaddr.Multiaddr) {
	n.notifyPeerConnected(peerID, addr.String())
}

// NotifyPeerConnected is called when a peer connection is established externally
func (n *NetworkManager) NotifyPeerConnected(peerID peer.ID, addr string) {
	n.notifyPeerConnected(peerID, addr)
}

func (n *NetworkManager) notifyPeerConnected(peerID peer.ID, addr string) {
	if peerID == n.host.ID() {
		return
	}

	n.peerStateMu.Lock()
	_, alreadyConnected := n.connectedPeers[peerID]
	n.connectedPeers[peerID] = true
	n.peerStateMu.Unlock()

	conns := n.host.Network().ConnsToPeer(peerID)
	if len(conns) == 1 {
		if !alreadyConnected {
			logger.Infof("Peer connected peer_id=%s address=%s", peerID, addr)
			n.broadcastPresence(peerID, "online")
			select {
			case n.peerConnectCh <- struct{}{}:
			default:
			}
		}
		if n.OnPeerJoin != nil {
			n.OnPeerJoin(peerID)
		}
		if !n.Online {
			n.Online = true
			logger.Infof("Network: Online - peers connected")
			if n.OnStateChange != nil {
				n.OnStateChange(true)
			}
		}
	}
}

func (n *NetworkManager) handlePeerDisconnect(peerID peer.ID, addr multiaddr.Multiaddr) {
	conns := n.host.Network().ConnsToPeer(peerID)
	logger.Infof("Peer has disconnected peer_id=%s address=%s", peerID, addr.String())

	n.peerStateMu.Lock()
	if n.connectedPeers[peerID] {
		delete(n.connectedPeers, peerID)
		n.peerStateMu.Unlock()
		n.broadcastPresence(peerID, "offline")
	} else {
		n.peerStateMu.Unlock()
	}

	if len(conns) == 0 {
		if n.OnPeerLeave != nil {
			n.OnPeerLeave(peerID)
		}

		peerCount := len(n.host.Network().Peers())
		if peerCount == 0 {
			n.Online = false
			logger.Infof("Network: Offline - no peers connected")
			if n.OnStateChange != nil {
				n.OnStateChange(false)
			}
		}
	}
}

func (n *NetworkManager) handlePresence(stream network.Stream) {
	peerID := stream.Conn().RemotePeer()
	logger.Debugf("Received presence message from peer_id=%s", peerID)

	buf := make([]byte, 64)
	nRead, err := stream.Read(buf)
	if err != nil || nRead == 0 {
		stream.Close()
		return
	}

	msg := string(buf[:nRead])
	if len(msg) < 2 {
		stream.Close()
		return
	}

	peerIDStr := msg[1:]
	action := msg[:1]
	switch action {
	case "o":
		logger.Infof("Peer is online peer_id=%s", peerIDStr)
	case "x":
		logger.Infof("Peer went offline peer_id=%s", peerIDStr)
	}
	stream.Close()
}

func (n *NetworkManager) broadcastPresence(peerID peer.ID, status string) {
	n.peerStateMu.RLock()
	peers := make([]peer.ID, 0, len(n.connectedPeers))
	for p := range n.connectedPeers {
		if p != n.host.ID() {
			peers = append(peers, p)
		}
	}
	n.peerStateMu.RUnlock()

	if len(peers) == 0 {
		logger.Debugf("No peers to broadcast presence to")
		return
	}

	msg := ""
	if status == "online" {
		msg = "o" + peerID.String()
	} else {
		msg = "x" + peerID.String()
	}

	logger.Debugf("Broadcasting presence: %s to %d peers", status, len(peers))
	for _, p := range peers {
		go func(target peer.ID) {
			ctx, cancel := context.WithTimeout(n.getContext(), 5*time.Second)
			defer cancel()

			stream, err := n.host.NewStream(ctx, target, protocol.ID(constants.PresenceProtocolID))
			if err != nil {
				logger.Debugf("Failed to send presence to peer_id=%s error=%v", target, err)
				return
			}
			defer stream.Close()

			_, err = stream.Write([]byte(msg))
			if err != nil {
				logger.Debugf("Failed to write presence to peer_id=%s error=%v", target, err)
			}
		}(p)
	}
}

// UpdateTrackerURL updates the tracker URL for discovery
func (n *NetworkManager) UpdateTrackerURL(ctx context.Context, newURL string) {
	if n.discovery != nil {
		n.discovery.UpdateTrackerURL(ctx, newURL)
	}
}

// AddBootstrapPeer adds a bootstrap peer to the discovery system
func (n *NetworkManager) AddBootstrapPeer(ctx context.Context, multiaddrStr string) error {
	if n.discovery == nil {
		return fmt.Errorf("discovery not initialized")
	}

	peerAddr, err := peer.AddrInfoFromString(multiaddrStr)
	if err != nil {
		return fmt.Errorf("invalid multiaddress: %w", err)
	}
	return n.discovery.AddBootstrapPeer(ctx, *peerAddr)
}
