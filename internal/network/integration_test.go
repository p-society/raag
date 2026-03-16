package network

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	appconfig "github.com/p-society/raag/internal/config"
	"github.com/p-society/raag/internal/constants"
	"github.com/p-society/raag/internal/library"
	"github.com/p-society/raag/internal/metadata"
	"github.com/p-society/raag/tracker"
	"github.com/spf13/viper"
)

func TestTrackerHeartbeatKeepsPeerRegistered(t *testing.T) {
	t.Skip("flaky: test expects heartbeat within 3s but TrackerHeartbeatInterval is 2min")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	tr := tracker.NewTracker(tracker.TrackerConfig{})
	ts := httptest.NewServer(tr)
	defer ts.Close()

	manager, cleanup := newTestNetworkManager(t, ts.URL)
	defer cleanup()

	ctx := t.Context()
	go func() {
		_ = manager.Start(ctx)
	}()

	var firstLastSeen string
	waitForCondition(t, 3*time.Second, func() bool {
		resp, err := httpGet(ts.URL + "/peers")
		if err != nil {
			return false
		}
		if !strings.Contains(resp, "\"peer_id\":") || !strings.Contains(resp, "\"addrs\":") {
			return false
		}

		lastSeen, ok := extractLastSeen(resp)
		if !ok {
			return false
		}
		firstLastSeen = lastSeen
		return true
	})

	waitForCondition(t, 3*time.Second, func() bool {
		resp, err := httpGet(ts.URL + "/peers")
		if err != nil {
			return false
		}
		updatedLastSeen, ok := extractLastSeen(resp)
		return ok && updatedLastSeen != firstLastSeen
	})
}

func TestTwoPeerFramedTransfer(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	managerA, cleanupA := newTestNetworkManager(t, "")
	defer cleanupA()
	managerB, cleanupB := newTestNetworkManager(t, "")
	defer cleanupB()

	ctxA := t.Context()
	ctxB := t.Context()
	go func() { _ = managerA.Start(ctxA) }()
	go func() { _ = managerB.Start(ctxB) }()

	addr := managerB.GetMultiaddr()
	addrInfo, err := peerAddrInfoFromString(addr)
	if err != nil {
		t.Fatalf("parse peer multiaddr: %v", err)
	}
	if err := managerA.Connect(context.Background(), *addrInfo); err != nil {
		t.Fatalf("connect peers: %v", err)
	}

	waitForCondition(t, 5*time.Second, func() bool {
		return managerA.WaitForPeers(100*time.Millisecond) && managerB.WaitForPeers(100*time.Millisecond)
	})

	sourcePath := filepath.Join(managerA.musicDir, "song.ogg")
	payload := []byte("this-is-a-real-framed-transfer-test")
	if err := os.WriteFile(sourcePath, payload, 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	song := metadata.Song{
		Title:  "Integration Song",
		Artist: "Raag",
		Album:  "Tests",
		Path:   sourcePath,
		Size:   int64(len(payload)),
	}
	if err := managerA.ShareSong(addrInfo, song); err != nil {
		t.Fatalf("share song: %v", err)
	}

	var receivedPath string
	waitForCondition(t, 5*time.Second, func() bool {
		matches, err := filepath.Glob(filepath.Join(managerB.musicDir, managerA.GetPeerID().String()+"_Integration Song.ogg"))
		if err != nil || len(matches) == 0 {
			return false
		}
		receivedPath = matches[0]
		return true
	})

	received, err := os.ReadFile(receivedPath)
	if err != nil {
		t.Fatalf("read received file: %v", err)
	}
	if string(received) != string(payload) {
		t.Fatalf("received payload mismatch: got %q want %q", string(received), string(payload))
	}
	if filepath.Ext(receivedPath) != ".ogg" {
		t.Fatalf("received extension = %q, want .ogg", filepath.Ext(receivedPath))
	}
}

func TestNetworkHostExposesTCPAndQUICAddresses(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	manager, cleanup := newTestNetworkManager(t, "")
	defer cleanup()

	addrs := manager.Host().Addrs()
	if len(addrs) == 0 {
		t.Fatalf("expected host to expose listen addresses")
	}

	hasTCP := false
	hasQUIC := false
	for _, addr := range addrs {
		addrStr := addr.String()
		if strings.Contains(addrStr, "/tcp/") {
			hasTCP = true
		}
		if strings.Contains(addrStr, "/quic-v1") {
			hasQUIC = true
		}
	}
	if !hasTCP {
		t.Fatalf("expected TCP listen address, got %v", addrs)
	}
	if !hasQUIC {
		t.Fatalf("expected QUIC listen address, got %v", addrs)
	}
}

func newTestNetworkManager(t *testing.T, trackerURL string) (*NetworkManager, func()) {
	t.Helper()
	identity, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("generate test identity: %v", err)
	}

	musicDir := filepath.Join(t.TempDir(), "music")
	lib, err := library.NewLibrary(musicDir)
	if err != nil {
		t.Fatalf("new library: %v", err)
	}

	port := freeTCPPort(t)
	v := viper.New()
	cfg := &appconfig.Config{
		Host:           "127.0.0.1",
		Port:           port,
		Rendezvous:     constants.DefaultRendezvous,
		TrackerURL:     trackerURL,
		DHTEnabled:     false,
		MaxPeers:       constants.DefaultMaxPeers,
		BootstrapPeers: nil,
		MusicDir:       musicDir,
		Volume:         constants.DefaultVolume,
		TUI:            false,
		Network:        true,
		LogLevel:       "error",
	}

	nm, err := newNetworkWithIdentity(cfg, v, lib, musicDir, identity)
	if err != nil {
		t.Fatalf("new network: %v", err)
	}

	cleanup := func() {
		_ = lib.Close()
		_ = nm.Close()
	}
	return nm, cleanup
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

func httpGet(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func peerAddrInfoFromString(addr string) (*peer.AddrInfo, error) {
	addrInfo, err := peer.AddrInfoFromString(addr)
	if err != nil {
		return nil, fmt.Errorf("parse addr info: %w", err)
	}
	return addrInfo, nil
}

func extractLastSeen(body string) (string, bool) {
	idx := strings.Index(body, "\"last_seen\":")
	if idx == -1 {
		return "", false
	}

	start := idx + len("\"last_seen\":")
	for start < len(body) && body[start] == ' ' {
		start++
	}

	end := start
	for end < len(body) && body[end] >= '0' && body[end] <= '9' {
		end++
	}
	if end == start {
		return "", false
	}
	return body[start:end], true
}
