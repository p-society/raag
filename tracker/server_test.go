package tracker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/p-society/raag/internal/auth"
)

func TestHandleRegisterPeerRejectsMismatchedIdentityClaims(t *testing.T) {
	_, authPriv, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate auth key: %v", err)
	}

	trustedAuthKey := auth.GetPublicKeyHex(authPriv)
	requestPeerID := mustPeerID(t)
	otherPeerID := mustPeerID(t)
	tests := []struct {
		name           string
		requestPeerID  string
		addrPeerID     peer.ID
		tokenPeerID    peer.ID
		wantStatusCode int
		wantBody       string
	}{
		{
			name:           "rejects request peer id mismatch with multiaddr",
			requestPeerID:  requestPeerID.String(),
			addrPeerID:     otherPeerID,
			tokenPeerID:    requestPeerID,
			wantStatusCode: http.StatusBadRequest,
			wantBody:       "peer_id does not match addr",
		},
		{
			name:           "rejects token peer id mismatch with request",
			requestPeerID:  requestPeerID.String(),
			addrPeerID:     requestPeerID,
			tokenPeerID:    otherPeerID,
			wantStatusCode: http.StatusUnauthorized,
			wantBody:       "Authentication failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTracker(TrackerConfig{AuthPublicKeys: []string{trustedAuthKey}})
			token, err := auth.GenerateToken(tc.tokenPeerID, authPriv)
			if err != nil {
				t.Fatalf("generate token: %v", err)
			}

			authData, err := auth.SerializeToken(token)
			if err != nil {
				t.Fatalf("serialize token: %v", err)
			}

			body, err := json.Marshal(map[string]string{
				"peer_id": tc.requestPeerID,
			})
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}

			var request map[string]any
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatalf("unmarshal request: %v", err)
			}

			request["auth_data"] = authData
			request["addrs"] = []string{addrForPeer(tc.addrPeerID)}
			body, err = json.Marshal(request)
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			tr.handleRegisterPeer(rec, req)
			if rec.Code != tc.wantStatusCode {
				t.Fatalf("status = %d, want %d, body=%q", rec.Code, tc.wantStatusCode, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("body = %q, want substring %q", rec.Body.String(), tc.wantBody)
			}
		})
	}
}

func TestHandleRegisterPeerAcceptsMatchingIdentityClaims(t *testing.T) {
	_, authPriv, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate auth key: %v", err)
	}

	trustedAuthKey := auth.GetPublicKeyHex(authPriv)
	peerID := mustPeerID(t)
	token, err := auth.GenerateToken(peerID, authPriv)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	authData, err := auth.SerializeToken(token)
	if err != nil {
		t.Fatalf("serialize token: %v", err)
	}

	body, err := json.Marshal(map[string]any{
		"peer_id":   peerID.String(),
		"auth_data": authData,
		"addrs":     []string{addrForPeer(peerID), quicAddrForPeer(peerID)},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	tr := NewTracker(TrackerConfig{AuthPublicKeys: []string{trustedAuthKey}})
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	tr.handleRegisterPeer(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	record, ok := tr.peers[peerID.String()]
	if !ok {
		t.Fatalf("peer %q was not registered", peerID)
	}
	if len(record.Addrs) != 2 {
		t.Fatalf("registered addr count = %d, want 2", len(record.Addrs))
	}
}

func mustPeerID(t *testing.T) peer.ID {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate identity key: %v", err)
	}

	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("derive peer id: %v", err)
	}
	return id
}

func addrForPeer(id peer.ID) string {
	return "/ip4/127.0.0.1/tcp/45678/p2p/" + id.String()
}

func quicAddrForPeer(id peer.ID) string {
	return "/ip4/127.0.0.1/udp/45678/quic-v1/p2p/" + id.String()
}

func TestRateLimiterAllowsWithinLimit(t *testing.T) {
	rl := NewRateLimiter()
	ip := "192.168.1.1"
	for i := range 10 {
		if !rl.Allow(ip) {
			t.Errorf("Allow() should return true for requests 1-10, failed at %d", i+1)
		}
	}
}

func TestRateLimiterBlocksAfterLimit(t *testing.T) {
	rl := NewRateLimiter()
	ip := "192.168.1.1"
	for range 10 {
		rl.Allow(ip)
	}
	if rl.Allow(ip) {
		t.Error("Allow() should return false after rate limit exceeded")
	}
}

func TestRateLimiterDifferentIPsIndependent(t *testing.T) {
	rl := NewRateLimiter()
	for i := range 10 {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("Allow() should return true for IP1 requests 1-10, failed at %d", i+1)
		}
	}
	if !rl.Allow("192.168.1.2") {
		t.Error("Allow() should return true for different IP after other IP hit limit")
	}
}

func TestHandleRegisterPeerMethodNotAllowed(t *testing.T) {
	tr := NewTracker(TrackerConfig{})
	req := httptest.NewRequest(http.MethodGet, "/register", nil)
	rec := httptest.NewRecorder()

	tr.handleRegisterPeer(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRegisterPeerMissingPeerID(t *testing.T) {
	tr := NewTracker(TrackerConfig{})
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	tr.handleRegisterPeer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleRegisterPeerInvalidJSON(t *testing.T) {
	tr := NewTracker(TrackerConfig{})
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()

	tr.handleRegisterPeer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
