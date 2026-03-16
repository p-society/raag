package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/multiformats/go-multiaddr"
	"github.com/p-society/raag/internal/auth"
	"github.com/p-society/raag/internal/constants"
	"github.com/p-society/raag/internal/logger"
)

const (
	maxPeers         = 10000 // Hard cap on peer map size
	maxPeersResponse = 50    // Max peers returned per /peers request
	maxRegisterBody  = 16384 // 16KB max payload for registration

	rateLimitWindow           = 1 * time.Minute
	rateLimitMaxRegistrations = 10 // Max registrations per IP per minute
)

// RateLimiter tracks registration attempts per IP
type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
	}
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rateLimitWindow)

	var valid []time.Time
	for _, t := range rl.requests[ip] {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}
	if len(valid) >= rateLimitMaxRegistrations {
		return false
	}

	rl.requests[ip] = append(valid, now)
	return true
}

type Tracker struct {
	mu              sync.RWMutex
	peers           map[string]registeredPeer
	httpPort        int
	libp2pPort      int
	host            host.Host
	relayEnabled    bool
	startedAt       time.Time
	trustedKeyMgr   *auth.TrustedKeyManager
	usedTokenNonces map[string]time.Time
	nonceMu         sync.Mutex
	rateLimiter     *RateLimiter
	tlsEnabled      bool
	tlsCertFile     string
	tlsKeyFile      string
}

type registeredPeer struct {
	PeerID   string
	Addrs    []string
	LastSeen time.Time
}

type TrackerConfig struct {
	HTTPPort       int
	Libp2pPort     int
	RelayEnabled   bool
	AuthPublicKeys []string
	TLSEnabled     bool
	TLSCertFile    string
	TLSKeyFile     string
}

func NewTracker(cfg TrackerConfig) *Tracker {
	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = constants.DefaultHTTPPort
	}
	if cfg.Libp2pPort == 0 {
		cfg.Libp2pPort = constants.DefaultPort
	}

	t := &Tracker{
		peers:           make(map[string]registeredPeer),
		httpPort:        cfg.HTTPPort,
		libp2pPort:      cfg.Libp2pPort,
		relayEnabled:    cfg.RelayEnabled,
		startedAt:       time.Now(),
		usedTokenNonces: make(map[string]time.Time),
		rateLimiter:     NewRateLimiter(),
		tlsEnabled:      cfg.TLSEnabled,
		tlsCertFile:     cfg.TLSCertFile,
		tlsKeyFile:      cfg.TLSKeyFile,
	}
	if len(cfg.AuthPublicKeys) > 0 {
		t.trustedKeyMgr = auth.NewTrustedKeyManager()
		for _, key := range cfg.AuthPublicKeys {
			t.trustedKeyMgr.AddKey(key)
		}
		logger.Infof("Auth enabled with %d trusted key(s)", len(cfg.AuthPublicKeys))
	}
	return t
}

func (t *Tracker) Start() error {
	if t.relayEnabled {
		if err := t.initLibp2p(); err != nil {
			logger.Warnf("Failed to init libp2p (relay disabled): %v", err)
			t.relayEnabled = false
		}
	}

	go t.cleanupOldPeers()
	if t.tlsEnabled {
		logger.Infof("TLS enabled, loading or generating certificates...")
		certFile, keyFile, err := LoadOrGenerateCerts(t.tlsCertFile, t.tlsKeyFile)
		if err != nil {
			logger.Warnf("Failed to load/generate certs: %v, falling back to HTTP", err)
			t.tlsEnabled = false
		} else {
			t.tlsCertFile = certFile
			t.tlsKeyFile = keyFile
			logger.Infof("Using TLS certificate: %s", certFile)
		}
	}

	addr := fmt.Sprintf(":%d", t.httpPort)
	server := &http.Server{
		Addr:              addr,
		Handler:           t,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		var err error
		if t.tlsEnabled {
			tlsConfig, tlsErr := GetTLSConfig(t.tlsCertFile, t.tlsKeyFile)
			if tlsErr != nil {
				logger.Warnf("Failed to load TLS config: %v, falling back to HTTP", tlsErr)
				err = server.ListenAndServe()
			} else {
				server.TLSConfig = tlsConfig
				logger.Infof("HTTPS enabled with TLS 1.3, cert=%s", t.tlsCertFile)
				err = server.ListenAndServeTLS(t.tlsCertFile, t.tlsKeyFile)
			}
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		logger.Errorf("HTTP server failed: %v", err)
	default:
	}

	logger.Infof("Tracker HTTP server started on port %d", t.httpPort)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Infof("Tracker shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Warnf("HTTP server shutdown error: %v", err)
	}
	if t.host != nil {
		t.host.Close()
	}
	return nil
}

func (t *Tracker) initLibp2p() error {
	listenAddr := fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", t.libp2pPort)
	sourceMultiAddr, _ := multiaddr.NewMultiaddr(listenAddr)
	relayOpts := []relay.Option{
		relay.WithLimit(&relay.RelayLimit{
			Duration: 2 * time.Minute, // Max 2 minutes proxy time
			Data:     1 << 20,         // Max 1MB data (forces hole-punching for large files)
		}),
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.EnableRelayService(relayOpts...),
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return fmt.Errorf("failed to create libp2p host: %w", err)
	}

	t.host = h
	logger.Infof("Tracker libp2p initialized, ID: %s", t.host.ID())
	logger.Infof("Tracker listening on: %s", t.multiaddr())
	return nil
}

func (t *Tracker) multiaddr() string {
	if t.host == nil {
		return ""
	}

	addrs := t.host.Addrs()
	if len(addrs) == 0 {
		return ""
	}
	return fmt.Sprintf("%s/p2p/%s", addrs[0], t.host.ID())
}

func (t *Tracker) HostID() string {
	if t.host == nil {
		return ""
	}
	return t.host.ID().String()
}

func (t *Tracker) RelayAddr() string {
	if t.host == nil || !t.relayEnabled {
		return ""
	}
	return fmt.Sprintf("/p2p/%s/p2p-circuit", t.host.ID())
}

func (t *Tracker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger.Debugf("tracker request method=%s path=%s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	switch r.URL.Path {
	case "/":
		t.handleRoot(w)
	case "/peers":
		t.handleGetPeers(w)
	case "/register":
		t.handleRegisterPeer(w, r)
	case "/addr":
		t.handleGetAddr(w)
	case "/health":
		t.handleHealth(w)
	default:
		http.NotFound(w, r)
	}
}

func encodeJSON(w http.ResponseWriter, data any) {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Warnf("failed to encode JSON response: %v", err)
	}
}

func (t *Tracker) handleRoot(w http.ResponseWriter) {
	encodeJSON(w, map[string]any{
		"name":        "Raag Tracker",
		"description": "Peer registry with relay support for Raag P2P network",
		"version":     "1.0.0",
		"endpoints":   []string{"/peers", "/register", "/addr", "/health"},
	})
}

func (t *Tracker) handleGetAddr(w http.ResponseWriter) {
	resp := map[string]any{
		"relay_enabled": t.relayEnabled,
		"host_id":       t.HostID(),
	}
	encodeJSON(w, resp)
}

func (t *Tracker) handleRegisterPeer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitedReader := io.LimitReader(r.Body, maxRegisterBody)
	var req struct {
		Addrs    []string `json:"addrs"`
		PeerID   string   `json:"peer_id"`
		AuthData string   `json:"auth_data,omitempty"`
	}
	if err := json.NewDecoder(limitedReader).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.PeerID == "" {
		http.Error(w, "Missing peer_id", http.StatusBadRequest)
		return
	}

	remoteIP := extractRemoteIP(r)
	if !t.rateLimiter.Allow(remoteIP) {
		logger.Warnf("Rate limit exceeded for IP: %s", remoteIP)
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}
	if len(req.Addrs) == 0 {
		logger.Debugf("No addresses provided, using observed IP: %s", remoteIP)
		req.Addrs = []string{
			fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", remoteIP, constants.DefaultPort, req.PeerID),
			fmt.Sprintf("/ip4/%s/udp/%d/quic-v1/p2p/%s", remoteIP, constants.DefaultPort, req.PeerID),
		}
	}

	validatedAddrs := make([]string, 0, len(req.Addrs))
	for _, addr := range req.Addrs {
		addrInfo, err := peer.AddrInfoFromString(addr)
		if err != nil {
			logger.Warnf("Registration denied: invalid peer addr for peer %s: %v", req.PeerID, err)
			http.Error(w, "Invalid peer address", http.StatusBadRequest)
			return
		}
		if addrInfo.ID.String() != req.PeerID {
			logger.Warnf("Registration denied: peer_id mismatch request=%s addr=%s", req.PeerID, addrInfo.ID)
			http.Error(w, "peer_id does not match addr", http.StatusBadRequest)
			return
		}
		if !isValidRegistrationAddr(addr, remoteIP) {
			logger.Warnf("Registration denied: suspicious IP in multiaddr remote=%s addr=%s", remoteIP, addr)
			http.Error(w, "Address does not match origin IP", http.StatusBadRequest)
			return
		}
		validatedAddrs = append(validatedAddrs, addr)
	}
	if t.trustedKeyMgr != nil {
		if req.AuthData == "" {
			logger.Warnf("Registration denied: auth required but not provided for peer %s", req.PeerID)
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		token, err := auth.DeserializeToken(req.AuthData)
		if err != nil {
			logger.Warnf("Registration denied: invalid auth token format for peer %s: %v", req.PeerID, err)
			http.Error(w, "Invalid auth token format", http.StatusUnauthorized)
			return
		}
		if token.PeerID != req.PeerID {
			logger.Warnf("Registration denied: token peer mismatch request=%s token=%s", req.PeerID, token.PeerID)
			http.Error(w, "Authentication failed", http.StatusUnauthorized)
			return
		}
		if !t.isNonceValid(token.Nonce) {
			logger.Warnf("Registration denied: invalid or reused nonce for peer %s", req.PeerID)
			http.Error(w, "Authentication failed", http.StatusUnauthorized)
			return
		}

		valid, err := t.trustedKeyMgr.VerifyAndCheckTrust(token)
		if err != nil || !valid {
			logger.Warnf("Registration denied: auth verification failed for peer %s: %v", req.PeerID, err)
			http.Error(w, "Authentication failed", http.StatusUnauthorized)
			return
		}
		logger.Infof("Peer authenticated: %s", req.PeerID)
	} else if t.trustedKeyMgr == nil && req.AuthData != "" {
		logger.Debugf("Auth data provided but tracker has no trusted keys - allowing (auth disabled)")
	}

	t.mu.Lock()
	_, isNewPeer := t.peers[req.PeerID]
	if !isNewPeer && len(t.peers) >= maxPeers {
		t.mu.Unlock()
		http.Error(w, "Tracker is full", http.StatusServiceUnavailable)
		return
	}

	t.peers[req.PeerID] = registeredPeer{
		PeerID:   req.PeerID,
		Addrs:    validatedAddrs,
		LastSeen: time.Now(),
	}
	t.mu.Unlock()

	if isNewPeer {
		logger.Infof("Peer registered: %s (%d addrs)", req.PeerID, len(validatedAddrs))
	} else {
		logger.Debugf("Peer updated: %s (%d addrs)", req.PeerID, len(validatedAddrs))
	}
	encodeJSON(w, map[string]any{
		"success": true,
		"message": "Peer registered successfully",
	})
}

func (t *Tracker) handleGetPeers(w http.ResponseWriter) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	peerList := make([]map[string]any, 0, maxPeersResponse)
	for _, record := range t.peers {
		if len(peerList) >= maxPeersResponse {
			break
		}
		peerInfo := map[string]any{
			"peer_id":   record.PeerID,
			"addrs":     record.Addrs,
			"last_seen": record.LastSeen.Unix(),
		}
		peerList = append(peerList, peerInfo)
	}
	encodeJSON(w, peerList)
}

func (t *Tracker) handleHealth(w http.ResponseWriter) {
	t.mu.RLock()
	peerCount := len(t.peers)
	t.mu.RUnlock()

	encodeJSON(w, map[string]any{
		"status":         "healthy",
		"uptime_seconds": int(time.Since(t.startedAt).Seconds()),
		"peers_count":    peerCount,
		"relay_enabled":  t.relayEnabled,
	})
}

func extractRemoteIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// isValidRegistrationAddr checks if the multiaddr IP matches the remote or is acceptable for NAT scenarios.
func isValidRegistrationAddr(maddr, remoteIP string) bool {
	addrLower := strings.ToLower(maddr)
	if strings.Contains(addrLower, "/127.0.0.1/") || strings.Contains(addrLower, "/localhost/") {
		return true
	}
	if strings.Contains(addrLower, "/"+remoteIP+"/") {
		return true
	}

	remoteIPParsed, err := netip.ParseAddr(remoteIP)
	if err != nil {
		return false
	}

	maddrParsed, err := multiaddr.NewMultiaddr(maddr)
	if err != nil {
		return false
	}

	ipStr, err := maddrParsed.ValueForProtocol(multiaddr.P_IP4)
	if err != nil {
		return false
	}

	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return false
	}
	if ip.IsPrivate() && remoteIPParsed.IsPrivate() {
		return false
	}
	if ip.IsPrivate() && !remoteIPParsed.IsPrivate() {
		return true
	}
	return false
}

func (t *Tracker) cleanupOldPeers() {
	ticker := time.NewTicker(constants.PeerCleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		t.mu.Lock()
		for peerID, record := range t.peers {
			if now.Sub(record.LastSeen) > constants.PeerTimeout {
				delete(t.peers, peerID)
				logger.Debugf("Removed stale peer: %s", peerID)
			}
		}

		t.mu.Unlock()
		t.cleanupOldNonces(now)
	}
}

func (t *Tracker) isNonceValid(nonce string) bool {
	if nonce == "" {
		return false
	}

	t.nonceMu.Lock()
	defer t.nonceMu.Unlock()

	if _, exists := t.usedTokenNonces[nonce]; exists {
		return false
	}

	t.usedTokenNonces[nonce] = time.Now()
	return true
}

func (t *Tracker) cleanupOldNonces(now time.Time) {
	t.nonceMu.Lock()
	defer t.nonceMu.Unlock()

	for nonce, usedAt := range t.usedTokenNonces {
		if now.Sub(usedAt) > constants.NonceExpiry {
			delete(t.usedTokenNonces, nonce)
		}
	}
}
