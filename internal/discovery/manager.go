package discovery

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/multiformats/go-multiaddr"
	"github.com/p-society/raag/internal/auth"
	"github.com/p-society/raag/internal/constants"
	"github.com/p-society/raag/internal/logger"
	"golang.org/x/sync/errgroup"
)

type Manager struct {
	host           host.Host
	ctx            context.Context
	dht            *dht.IpfsDHT
	discovery      *routing.RoutingDiscovery
	persistence    *PeerPersistence
	tracker        *TrackerClient
	maxPeers       int
	trackerURL     string
	listenHost     string
	rendezvous     string
	dhtEnabled     bool
	bootstrapPeers []string
	authKeyPair    ed25519.PrivateKey
	authToken      *auth.AuthToken
	networkManager interface {
		NotifyPeerConnected(peerID peer.ID, addr string)
	}
	onPeerSave              func(peers []peer.AddrInfo)
	mdnsPeerCount           uint64
	heartbeatEvery          time.Duration
	refreshEvery            time.Duration
	retryDelay              time.Duration
	maxRetryDelay           time.Duration
	lastBootstrap           time.Time
	bootstrapThrottle       time.Duration
	peerCountSinceBootstrap int
	stateMu                 sync.RWMutex
}

type ManagerConfig struct {
	Host             host.Host
	TrackerURL       string
	IdentityKeyBytes []byte
	AuthSecret       string
	MaxPeers         int
	ListenHost       string
	Rendezvous       string
	DHTEnabled       bool
	BootstrapPeers   []string
}

func NewManager(cfg ManagerConfig) *Manager {
	h := cfg.Host
	var authKeyPair ed25519.PrivateKey
	var err error

	// Priority: AuthSecret > IdentityKey > Random
	if cfg.AuthSecret != "" {
		authKeyPair, err = auth.DeriveKey([]byte(cfg.AuthSecret), "raag-secret-v1")
		if err != nil {
			logger.Warnf("Failed to derive auth key from secret: %v", err)
		} else {
			logger.Infof("Auth enabled via shared secret")
		}
	} else if len(cfg.IdentityKeyBytes) > 0 {
		authKeyPair, err = auth.DeriveKey(cfg.IdentityKeyBytes, "raag-identity-v1")
		if err != nil {
			logger.Warnf("Failed to derive auth key from identity: %v", err)
		} else {
			logger.Infof("Auth enabled via identity key")
		}
	}
	if authKeyPair == nil {
		_, authKeyPair, err = auth.GenerateKeyPair()
		if err != nil {
			logger.Warnf("Failed to generate auth key pair: %v", err)
		} else {
			logger.Warnf("No auth configured, generated random key")
		}
	}
	return &Manager{
		host:                    h,
		persistence:             NewPeerPersistence(),
		tracker:                 NewTrackerClient(cfg.TrackerURL),
		maxPeers:                cfg.MaxPeers,
		trackerURL:              cfg.TrackerURL,
		listenHost:              cfg.ListenHost,
		rendezvous:              cfg.Rendezvous,
		dhtEnabled:              cfg.DHTEnabled,
		bootstrapPeers:          slices.Clone(cfg.BootstrapPeers),
		authKeyPair:             authKeyPair,
		heartbeatEvery:          constants.TrackerHeartbeatInterval,
		refreshEvery:            constants.TrackerRefreshInterval,
		retryDelay:              constants.TrackerRetryInitialDelay,
		maxRetryDelay:           constants.TrackerRetryMaxDelay,
		lastBootstrap:           time.Now(),
		bootstrapThrottle:       constants.DHTBootstrapThrottle,
		peerCountSinceBootstrap: 0,
	}
}

func (m *Manager) SetNetworkManager(nm interface {
	NotifyPeerConnected(peerID peer.ID, addr string)
},
) {
	m.networkManager = nm
}

func (m *Manager) SetOnPeerSave(callback func(peers []peer.AddrInfo)) {
	m.onPeerSave = callback
}

func (m *Manager) SetTestIntervals(heartbeat, refresh, retryDelay, maxRetryDelay time.Duration) {
	if heartbeat > 0 {
		m.heartbeatEvery = heartbeat
	}
	if refresh > 0 {
		m.refreshEvery = refresh
	}
	if retryDelay > 0 {
		m.retryDelay = retryDelay
	}
	if maxRetryDelay > 0 {
		m.maxRetryDelay = maxRetryDelay
	}
}

type PeerInfo struct {
	ID            string    `json:"id"`
	Addr          string    `json:"addr"`
	Connected     bool      `json:"connected"`
	DiscoveredVia string    `json:"discovered_via"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
}

type NetworkState struct {
	SelfID         string     `json:"self_id"`
	ListenAddr     string     `json:"listen_addr"`
	Mode           string     `json:"mode"`
	TrackerURL     string     `json:"tracker_url"`
	TrackerStatus  string     `json:"tracker_status"`
	DHTEnabled     bool       `json:"dht_enabled"`
	DHTPeers       int        `json:"dht_peers"`
	MDNSEnabled    bool       `json:"mdns_enabled"`
	MDNSDiscovered int        `json:"mdns_discovered"`
	ConnectedPeers []PeerInfo `json:"connected_peers"`
	KnownPeers     []PeerInfo `json:"known_peers"`
	AuthPublicKey  string     `json:"auth_public_key,omitempty"`
}

func (m *Manager) GetNetworkState() NetworkState {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()

	state := NetworkState{
		SelfID:         m.host.ID().String(),
		ListenAddr:     "",
		Mode:           "networked",
		TrackerURL:     m.trackerURL,
		TrackerStatus:  "unknown",
		DHTEnabled:     m.dhtEnabled,
		DHTPeers:       0,
		MDNSEnabled:    m.listenHost != "127.0.0.1" && m.listenHost != "localhost",
		MDNSDiscovered: 0,
		ConnectedPeers: []PeerInfo{},
		KnownPeers:     []PeerInfo{},
		AuthPublicKey:  m.GetAuthPublicKey(),
	}

	addrs := m.host.Addrs()
	if len(addrs) > 0 {
		state.ListenAddr = addrs[0].String()
	}
	if m.dht != nil {
		state.DHTPeers = m.dht.RoutingTable().Size()
	}

	connectedPeers := m.host.Network().Peers()
	state.MDNSDiscovered = int(m.mdnsPeerCount)
	knownPeerIDs := m.host.Peerstore().Peers()
	for _, pid := range knownPeerIDs {
		if pid == m.host.ID() {
			continue
		}

		var addrStr string
		addrs := m.host.Peerstore().Addrs(pid)
		if len(addrs) > 0 {
			addrStr = addrs[0].String()
		}

		connected := slices.Contains(connectedPeers, pid)
		state.KnownPeers = append(state.KnownPeers, PeerInfo{
			ID:            pid.String(),
			Addr:          addrStr,
			Connected:     connected,
			DiscoveredVia: "DHT/mDNS",
		})
	}

	for _, pid := range connectedPeers {
		if pid == m.host.ID() {
			continue
		}

		addrs := m.host.Peerstore().Addrs(pid)
		var addrStr string
		if len(addrs) > 0 {
			addrStr = addrs[0].String()
		}
		state.ConnectedPeers = append(state.ConnectedPeers, PeerInfo{
			ID:        pid.String(),
			Addr:      addrStr,
			Connected: true,
		})
	}
	return state
}

func (m *Manager) LogNetworkState() {
	state := m.GetNetworkState()

	logger.Infof("=== P2P Network State ===")
	logger.Infof("Self: %s @ %s", state.SelfID, state.ListenAddr)
	logger.Infof("Mode: %s", state.Mode)
	if state.AuthPublicKey != "" {
		logger.Infof("Auth Key: %s...", state.AuthPublicKey[:32])
	}

	logger.Infof("--- Discovery ---")
	logger.Infof("Tracker: %s", state.TrackerURL)
	logger.Infof("DHT: enabled=%v (peers=%d)", state.DHTEnabled, state.DHTPeers)
	logger.Infof("mDNS: enabled=%v (discovered=%d)", state.MDNSEnabled, state.MDNSDiscovered)

	logger.Infof("--- Connections (%d) ---", len(state.ConnectedPeers))
	if len(state.ConnectedPeers) == 0 {
		logger.Infof("  (no active connections)")
	}
	for _, peer := range state.ConnectedPeers {
		logger.Infof("  ✓ %s @ %s", peer.ID[:12], peer.Addr)
	}

	logger.Infof("--- Known Peers (%d) ---", len(state.KnownPeers))
	if len(state.KnownPeers) == 0 {
		logger.Infof("  (no known peers)")
	}
	for _, peer := range state.KnownPeers {
		status := "○"
		if peer.Connected {
			status = "✓"
		}
		logger.Infof("  %s %s @ %s", status, peer.ID[:12], peer.Addr)
	}
}

func (m *Manager) Start(ctx context.Context) error {
	m.ctx = ctx
	if err := m.loadPersistedPeers(); err != nil {
		logger.Errorf("Failed to load persisted peers error=%v", err)
	}

	logger.Debugf("Starting peer discovery from tracker...")
	if err := m.discoverFromTracker(ctx); err != nil {
		logger.Warnf("Initial tracker discovery failed error=%v", err)
		go func() {
			time.Sleep(2 * time.Second)
			logger.Debugf("Retrying initial tracker discovery...")
			if err := m.discoverFromTracker(context.Background()); err != nil {
				logger.Debugf("Retry tracker discovery failed error=%v", err)
			}
		}()
	}

	if err := m.connectBootstrapPeers(ctx); err != nil {
		logger.Warnf("Failed to connect configured bootstrap peers error=%v", err)
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		m.runTrackerRegistrationLoop(ctx)
		return nil
	})
	g.Go(func() error {
		m.runTrackerRefreshLoop(ctx)
		return nil
	})

	if m.listenHost != "127.0.0.1" && m.listenHost != "localhost" {
		logger.Debugf("Starting mDNS discovery in background...")
		g.Go(func() error {
			m.discoverViaMDNS(ctx)
			return nil
		})
	} else {
		logger.Debugf("Skipping mDNS discovery (localhost mode)")
	}

	if !m.dhtEnabled {
		logger.Infof("DHT discovery disabled")
		if err := g.Wait(); err != nil {
			logger.Warnf("Background service error: %v", err)
		}
		return nil
	}

	logger.Debugf("Initializing DHT with existing peer connections...")
	if err := m.initDHT(ctx); err != nil {
		logger.Errorf("Failed to initialize DHT error=%v", err)
	}
	m.populateDHTFromConnectedPeers()

	g.Go(func() error {
		m.logNetworkStatus(ctx)
		return nil
	})
	g.Go(func() error {
		m.discoverViaDHT(ctx)
		return nil
	})
	g.Go(func() error {
		m.advertisePeriodically(ctx)
		return nil
	})

	if err := g.Wait(); err != nil {
		logger.Warnf("Background service error: %v", err)
	}
	return nil
}

func (m *Manager) refreshRegistration(ctx context.Context) {
	if err := m.tryRefreshRegistration(ctx); err != nil {
		logger.Errorf("Failed to register with tracker error=%v", err)
	}
}

func (m *Manager) selectAdvertisedAddresses(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	filtered := filterReachableAddresses(addrs)
	prioritized := prioritizeAddresses(filtered)
	if len(prioritized) == 0 {
		return nil
	}

	const maxAdvertisedAddrs = 6
	selected := make([]multiaddr.Multiaddr, 0, min(len(prioritized), maxAdvertisedAddrs))
	seen := make(map[string]struct{})
	for _, addr := range prioritized {
		addrStr := addr.String()
		if _, ok := seen[addrStr]; ok {
			continue
		}

		seen[addrStr] = struct{}{}
		selected = append(selected, addr)
		if len(selected) >= maxAdvertisedAddrs {
			break
		}
	}
	if len(selected) == 0 && len(addrs) > 0 {
		return addrs[:1]
	}
	return selected
}

func isReachableAddr(addr multiaddr.Multiaddr) bool {
	ipStr, err := addr.ValueForProtocol(multiaddr.P_IP4)
	if err != nil {
		return true
	}

	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return false
	}
	return !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsMulticast()
}

func filterReachableAddresses(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	var filtered []multiaddr.Multiaddr
	for _, addr := range addrs {
		if isReachableAddr(addr) {
			filtered = append(filtered, addr)
		} else {
			logger.Debugf("Filtered private address: %s", addr.String())
		}
	}
	if len(filtered) == 0 {
		logger.Warnf("No public addresses available, will use observed IP from tracker")
		return nil
	}
	return filtered
}

func prioritizeAddresses(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	var quic, tcp, other []multiaddr.Multiaddr
	for _, addr := range addrs {
		addrStr := strings.ToLower(addr.String())
		if strings.Contains(addrStr, "/quic-v1") {
			quic = append(quic, addr)
		} else if strings.Contains(addrStr, "/tcp/") {
			tcp = append(tcp, addr)
		} else {
			other = append(other, addr)
		}
	}

	result := append(append(quic, tcp...), other...)
	return result
}

func (m *Manager) getAuthData() (string, error) {
	m.regenerateAuthToken()
	if m.authToken == nil {
		return "", fmt.Errorf("no auth key pair available")
	}
	return auth.SerializeToken(m.authToken)
}

func (m *Manager) regenerateAuthToken() {
	if m.authKeyPair == nil {
		return
	}
	if m.authToken != nil {
		timeUntilExpiry := time.Until(time.Unix(m.authToken.ExpiresAt, 0))
		if timeUntilExpiry > constants.TokenRefreshThreshold {
			return
		}
		logger.Debugf("Token expiring soon (%v), regenerating", timeUntilExpiry)
	}

	token, err := auth.GenerateToken(m.host.ID(), m.authKeyPair)
	if err != nil {
		logger.Warnf("Failed to regenerate auth token: %v", err)
		return
	}

	m.authToken = token
	logger.Infof("Auth token regenerated, expires in %v", time.Until(time.Unix(token.ExpiresAt, 0)))
}

func (m *Manager) GetAuthData() (string, error) {
	return m.getAuthData()
}

func (m *Manager) GetAuthPublicKey() string {
	if m.authKeyPair == nil {
		return ""
	}
	return hex.EncodeToString(m.authKeyPair.Public().(ed25519.PublicKey))
}

func (m *Manager) RefreshTrackerRegistration(ctx context.Context) {
	m.refreshRegistration(ctx)
}

func (m *Manager) runTrackerRegistrationLoop(ctx context.Context) {
	if m.trackerURL == "" {
		return
	}
	if err := m.refreshRegistrationWithRetry(ctx); err != nil {
		logger.Warnf("Initial tracker registration failed error=%v", err)
	}
}

func (m *Manager) runTrackerRefreshLoop(ctx context.Context) {
	if m.trackerURL == "" {
		return
	}

	ticker := time.NewTicker(m.refreshEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.discoverFromTrackerWithRetry(ctx); err != nil {
				logger.Warnf("Tracker peer refresh failed error=%v", err)
			}
		}
	}
}

func (m *Manager) refreshRegistrationWithRetry(ctx context.Context) error {
	if m.trackerURL == "" {
		return nil
	}

	retryDelay := m.retryDelay
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		registerCtx, cancel := context.WithTimeout(ctx, constants.HTTPClientTimeout)
		err := m.tryRefreshRegistration(registerCtx)
		cancel()
		if err == nil {
			return nil
		}

		logger.Warnf("Tracker registration failed, retrying error=%v retryDelay=%v", err, retryDelay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay):
		}

		retryDelay *= 2
		if retryDelay > m.maxRetryDelay {
			retryDelay = m.maxRetryDelay
		}
	}
}

func (m *Manager) tryRefreshRegistration(ctx context.Context) error {
	if m.trackerURL == "" {
		return nil
	}

	addrs := m.host.Addrs()
	if len(addrs) == 0 {
		return fmt.Errorf("host has no listening addresses")
	}

	advertisedAddrs := m.selectAdvertisedAddresses(addrs)
	if len(advertisedAddrs) == 0 {
		logger.Debugf("No public addresses, letting tracker use observed IP")
	}

	addrWithPeerID := make([]string, 0, len(advertisedAddrs))
	for _, addr := range advertisedAddrs {
		addrWithPeerID = append(addrWithPeerID, addr.Encapsulate(multiaddr.StringCast("/p2p/"+m.host.ID().String())).String())
	}

	peerID := m.host.ID().String()
	authData, err := m.getAuthData()
	if err != nil {
		logger.Warnf("Failed to generate auth data: %v", err)
		authData = ""
	}
	if err := m.tracker.RegisterPeer(ctx, addrWithPeerID, peerID, authData); err != nil {
		return err
	}
	logger.Infof("Registered with tracker peer_id=%s addrs=%d", peerID, len(addrWithPeerID))
	return nil
}

func (m *Manager) UpdateTrackerURL(ctx context.Context, newURL string) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	if newURL == m.trackerURL {
		return
	}

	logger.Infof("Updating tracker URL from=%s to=%s", m.trackerURL, newURL)
	m.trackerURL = newURL
	m.tracker = NewTrackerClient(newURL)
}

func (m *Manager) AddBootstrapPeer(ctx context.Context, peerAddr peer.AddrInfo) error {
	if !m.dhtEnabled {
		return fmt.Errorf("DHT discovery is disabled")
	}
	if m.dht == nil {
		return fmt.Errorf("DHT not initialized")
	}

	logger.Infof("Adding bootstrap peer peer=%s", peerAddr.ID)
	if err := m.host.Connect(ctx, peerAddr); err != nil {
		return fmt.Errorf("failed to connect to bootstrap peer: %w", err)
	}
	if err := m.dht.Bootstrap(ctx); err != nil {
		return fmt.Errorf("failed to bootstrap DHT: %w", err)
	}

	m.savePeer(peerAddr, false)
	return nil
}

func (m *Manager) loadPersistedPeers() error {
	peers, err := m.persistence.Load()
	if err != nil {
		logger.Warnf("Could not load persisted peers error=%v", err)
		return nil
	}

	for _, p := range peers {
		m.host.Peerstore().AddAddrs(p.ID, p.Addrs, peerstore.PermanentAddrTTL)
	}

	logger.Debugf("Loaded persisted peers count=%d", len(peers))
	return nil
}

func (m *Manager) discoverFromTrackerWithRetry(ctx context.Context) error {
	retryDelay := 1 * time.Second
	maxRetryDelay := m.maxRetryDelay
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := m.discoverFromTracker(ctx); err != nil {
				logger.Warnf("Tracker discovery failed, retrying error=%v retryDelay=%v", err, retryDelay)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(retryDelay):
				}
				retryDelay = min(retryDelay*2, maxRetryDelay)
			} else {
				return nil
			}
		}
	}
}

func (m *Manager) discoverFromTracker(ctx context.Context) error {
	if m.trackerURL == "" {
		return nil // No tracker configured
	}

	peers, err := m.tracker.FetchPeers(ctx)
	if err != nil {
		return err
	}

	logger.Debugf("Tracker returned peers count=%d", len(peers))
	for _, p := range peers {
		if p.ID == m.host.ID() {
			continue
		}

		m.savePeer(p, true)
		if err := m.host.Connect(ctx, p); err != nil {
			logger.Warnf("Failed to connect to tracker peer peer=%s error=%v", p.ID, err)
		} else {
			logger.Infof("Connected to tracker peer peer=%s", p.ID)
			m.addAsBootstrap(ctx)
		}
	}
	return nil
}

func (m *Manager) addAsBootstrap(ctx context.Context) {
	if m.dht == nil {
		return
	}
	m.throttledBootstrap(ctx)
}

func (m *Manager) throttledBootstrap(ctx context.Context) {
	if m.dht == nil {
		return
	}

	m.stateMu.Lock()
	m.peerCountSinceBootstrap++
	timeSinceLastBootstrap := time.Since(m.lastBootstrap)
	if timeSinceLastBootstrap < m.bootstrapThrottle && m.peerCountSinceBootstrap < constants.DHTBootstrapPeerThreshold {
		m.stateMu.Unlock()
		logger.Debugf("Skipping DHT bootstrap: throttled (timeSinceLast=%v, peerCountSince=%d)",
			timeSinceLastBootstrap, m.peerCountSinceBootstrap)
		return
	}

	m.lastBootstrap = time.Now()
	m.peerCountSinceBootstrap = 0
	m.stateMu.Unlock()

	bootstrapCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := m.dht.Bootstrap(bootstrapCtx); err != nil {
		logger.Warnf("DHT bootstrap warning error=%v", err)
	}
	logger.Debugf("DHT routing table size after bootstrap size=%d", m.dht.RoutingTable().Size())
}

func (m *Manager) savePeer(p peer.AddrInfo, skipAutoConnect bool) {
	alreadyConnected := slices.Contains(m.host.Network().Peers(), p.ID)
	knownPeers := m.host.Peerstore().Peers()
	alreadyKnown := slices.Contains(knownPeers, p.ID)
	if !alreadyKnown {
		if len(knownPeers) >= m.maxPeers {
			return
		}

		m.host.Peerstore().AddAddrs(p.ID, p.Addrs, peerstore.PermanentAddrTTL)
		m.persistPeers()
		logger.Debugf("Saved new peer peer=%s", p.ID)
		if !skipAutoConnect && !alreadyConnected {
			m.tryConnectToPeer(p)
		} else if alreadyConnected {
			logger.Debugf("Peer already connected, skipping connection attempt peer=%s", p.ID)
		}
	}
}

func (m *Manager) tryConnectToPeer(p peer.AddrInfo) {
	if p.ID == m.host.ID() {
		return
	}
	go m.connectWithRetry(p)
}

func (m *Manager) connectWithRetry(p peer.AddrInfo) {
	if len(m.host.Network().ConnsToPeer(p.ID)) > 0 {
		logger.Debugf("Already connected to peer, skipping peer=%s", p.ID)
		return
	}
	if len(p.Addrs) > 0 {
		filteredAddrs := filterReachableAddresses(p.Addrs)
		prioritizedAddrs := prioritizeAddresses(filteredAddrs)
		p.Addrs = prioritizedAddrs
		logger.Debugf("Filtered peer addresses from %d to %d peer=%s",
			len(p.Addrs), len(prioritizedAddrs), p.ID)
	}

	maxRetries := 3
	retryDelays := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}
	for attempt := range maxRetries {
		connectCtx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		logger.Debugf("Attempting to connect to peer peer=%s attempt=%d/%d", p.ID, attempt+1, maxRetries)
		if err := m.host.Connect(connectCtx, p); err != nil {
			cancel()
			if attempt < maxRetries-1 {
				logger.Warnf("Connection failed, retrying in %v peer=%s error=%v", retryDelays[attempt], p.ID, err)
				select {
				case <-m.ctx.Done():
					return
				case <-time.After(retryDelays[attempt]):
				}
				continue
			}
			logger.Warnf("Failed to connect to peer after %d attempts peer=%s error=%v", maxRetries, p.ID, err)
			return
		}

		logger.Infof("Connected to discovered peer peer=%s", p.ID)
		m.throttledBootstrap(m.ctx)
		cancel()
		if m.networkManager != nil {
			addr := ""
			if len(p.Addrs) > 0 {
				addr = p.Addrs[0].String()
			}
			m.networkManager.NotifyPeerConnected(p.ID, addr)
		}
		return
	}
}

// GetAllPeers returns all known peers from the peerstore
func (m *Manager) GetAllPeers() []peer.AddrInfo {
	peerIDs := m.host.Peerstore().Peers()
	peersList := make([]peer.AddrInfo, 0, len(peerIDs))
	for _, pid := range peerIDs {
		if pid == m.host.ID() {
			continue
		}
		addrs := m.host.Peerstore().Addrs(pid)
		if len(addrs) > 0 {
			peersList = append(peersList, peer.AddrInfo{ID: pid, Addrs: addrs})
		}
	}
	return peersList
}

func (m *Manager) persistPeers() {
	var peersList []peer.AddrInfo
	peerIDs := m.host.Peerstore().Peers()
	for _, pid := range peerIDs {
		if pid == m.host.ID() {
			continue
		}

		addrs := m.host.Peerstore().Addrs(pid)
		if len(addrs) > 0 {
			peersList = append(peersList, peer.AddrInfo{ID: pid, Addrs: addrs})
		}
	}
	if err := m.persistence.Save(peersList); err != nil {
		logger.Errorf("Failed to save peers to disk error=%v", err)
	} else {
		logger.Debugf("Persisted peers to disk count=%d", len(peersList))
	}

	if m.onPeerSave != nil {
		m.onPeerSave(peersList)
	}
}

func (m *Manager) connectBootstrapPeers(ctx context.Context) error {
	for _, multiaddrStr := range m.bootstrapPeers {
		addrInfo, err := peer.AddrInfoFromString(multiaddrStr)
		if err != nil {
			logger.Warnf("Ignoring invalid bootstrap peer multiaddr=%s error=%v", multiaddrStr, err)
			continue
		}
		if addrInfo.ID == m.host.ID() {
			continue
		}

		m.savePeer(*addrInfo, true)
		if err := m.host.Connect(ctx, *addrInfo); err != nil {
			logger.Warnf("Failed to connect bootstrap peer peer=%s error=%v", addrInfo.ID, err)
			continue
		}
		logger.Infof("Connected configured bootstrap peer peer=%s", addrInfo.ID)
	}
	return nil
}

func (m *Manager) initDHT(ctx context.Context) error {
	connectedPeers := m.host.Network().Peers()
	var bootstrapPeers []peer.AddrInfo
	for _, pid := range connectedPeers {
		if pid == m.host.ID() {
			continue
		}

		addrs := m.host.Peerstore().Addrs(pid)
		if len(addrs) > 0 {
			bootstrapPeers = append(bootstrapPeers, peer.AddrInfo{ID: pid, Addrs: addrs})
		}
	}

	var opts []dht.Option
	opts = append(opts, dht.Mode(dht.ModeServer))
	if len(bootstrapPeers) > 0 {
		opts = append(opts, dht.BootstrapPeers(bootstrapPeers...))
		logger.Debugf("DHT initialized with bootstrap peers count=%d", len(bootstrapPeers))
	}

	kademliaDHT, err := dht.New(ctx, m.host, opts...)
	if err != nil {
		return err
	}

	m.dht = kademliaDHT
	if err := kademliaDHT.Bootstrap(ctx); err != nil {
		logger.Warnf("DHT bootstrap warning error=%v", err)
	}

	m.discovery = routing.NewRoutingDiscovery(kademliaDHT)
	logger.Debugf("DHT initialized successfully")
	logger.Debugf("DHT routing table size (initial) size=%d", m.dht.RoutingTable().Size())
	return nil
}

func (m *Manager) waitForPeers(ctx context.Context, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		connectedPeers := len(m.host.Network().Peers())
		if connectedPeers > 1 {
			logger.Debugf("Found connected peers, proceeding with DHT count=%d", connectedPeers-1)
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(500 * time.Millisecond):
		}
	}
	logger.Debugf("Timeout waiting for peers found=%d", len(m.host.Network().Peers())-1)
	return false
}

func (m *Manager) populateDHTFromConnectedPeers() {
	if m.dht == nil {
		return
	}

	peers := m.host.Network().Peers()
	count := 0
	for _, peerID := range peers {
		if peerID == m.host.ID() {
			continue
		}
		count++
	}
	if count > 0 {
		m.throttledBootstrap(m.ctx)
	}
}

func (m *Manager) advertisePeriodically(ctx context.Context) {
	ticker := time.NewTicker(constants.DHTAdvertisementInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if m.discovery != nil {
				m.discovery.Advertise(ctx, m.rendezvous)
				logger.Infof("DHT re-advertised presence")
			}
		}
	}
}

func (m *Manager) logNetworkStatus(ctx context.Context) {
	ticker := time.NewTicker(constants.DHTLogInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dhtSize := 0
			peerstoreCount := 0
			connectedCount := 0
			if m.dht != nil {
				dhtSize = m.dht.RoutingTable().Size()
			}
			if m.host != nil {
				peerstoreCount = len(m.host.Peerstore().Peers())
				connectedCount = len(m.host.Network().Peers())
			}

			m.stateMu.RLock()
			mdnsPeers := int(m.mdnsPeerCount)
			m.stateMu.RUnlock()
			logger.Debugf("Network status: dht_routing=%d peerstore=%d connected=%d mdns=%d",
				dhtSize, peerstoreCount, connectedCount, mdnsPeers)
		}
	}
}

func (m *Manager) discoverViaDHT(ctx context.Context) {
	if m.discovery == nil {
		return
	}

	logger.Debugf("DHT: Advertising presence...")
	m.discovery.Advertise(ctx, m.rendezvous)
	logger.Debugf("DHT: Advertisement complete")

	logger.Debugf("DHT: Waiting for peer connections...")
	hasPeers := m.waitForPeers(ctx, constants.DHTWaitForPeersTimeout)
	m.populateDHTFromConnectedPeers()
	if !hasPeers {
		logger.Debugf("DHT: No peers connected yet, starting discovery anyway")
	}

	logger.Debugf("DHT: Starting peer discovery...")
	for {
		queryCtx, cancel := context.WithTimeout(ctx, constants.DHTQueryTimeout)
		peerChan, err := m.discovery.FindPeers(queryCtx, m.rendezvous)
		if err != nil {
			cancel()
			logger.Warnf("DHT FindPeers error error=%v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(constants.DHTRetryDelay):
			}
			continue
		}

		discovered := make(map[peer.ID]bool)
		count := 0
		roundDone := false
		for {
			select {
			case <-ctx.Done():
				cancel()
				return
			case p, ok := <-peerChan:
				if !ok {
					logger.Debugf("DHT: Finished discovery round, discovered %d new peers", count)
					roundDone = true
					break
				}
				if p.ID == "" || p.ID == m.host.ID() {
					continue
				}

				knownPeers := m.host.Peerstore().Peers()
				alreadyKnown := slices.Contains(knownPeers, p.ID)
				if alreadyKnown {
					continue
				}
				if discovered[p.ID] {
					continue
				}

				discovered[p.ID] = true
				count++
				logger.Debugf("DHT discovered peer peer=%s", p.ID)
				m.savePeer(p, false)
			}
			if roundDone {
				break
			}
		}
		cancel()

		select {
		case <-ctx.Done():
			return
		case <-time.After(constants.DHTDiscoveryInterval):
		}
	}
}

func (m *Manager) discoverViaMDNS(ctx context.Context) {
	notifee := &mdnsNotifee{manager: m}
	service := mdns.NewMdnsService(m.host, m.rendezvous, notifee)
	if err := service.Start(); err != nil {
		logger.Errorf("mDNS discovery failed error=%v", err)
		return
	}

	logger.Debugf("mDNS service started, running continuously...")
	<-ctx.Done()
	logger.Debugf("mDNS discovery stopped")
}

type mdnsNotifee struct {
	manager *Manager
}

func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	if pi.ID == n.manager.host.ID() {
		return
	}

	knownPeers := n.manager.host.Peerstore().Peers()
	alreadyKnown := slices.Contains(knownPeers, pi.ID)

	if !alreadyKnown {
		n.manager.stateMu.Lock()
		n.manager.mdnsPeerCount++
		n.manager.stateMu.Unlock()
	}

	if alreadyKnown {
		logger.Debugf("mDNS found already known peer, skipping peer=%s", pi.ID)
		return
	}

	logger.Debugf("mDNS discovered peer peer=%s", pi.ID)
	n.manager.savePeer(pi, false)
}
