package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	pathpkg "path"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/p-society/raag/internal/constants"
)

var sharedHTTPClient = &http.Client{
	Timeout: constants.HTTPClientTimeout,
	Transport: &http.Transport{
		MaxIdleConns:      10,
		IdleConnTimeout:   30 * time.Second,
		DisableKeepAlives: false,
	},
}

type TrackerClient struct {
	trackerURL string
	client     *http.Client
}

type trackerPeerResponse struct {
	PeerID   string   `json:"peer_id"`
	Addrs    []string `json:"addrs"`
	LastSeen int64    `json:"last_seen"`
}

type trackerRegisterRequest struct {
	Addrs    []string `json:"addrs"`
	PeerID   string   `json:"peer_id"`
	AuthData string   `json:"auth_data,omitempty"`
}

type trackerAddrResponse struct {
	RelayEnabled bool   `json:"relay_enabled"`
	RelayAddr    string `json:"relay_addr,omitempty"`
	HostID       string `json:"host_id,omitempty"`
}

func NewTrackerClient(trackerURL string) *TrackerClient {
	return &TrackerClient{
		trackerURL: trackerURL,
		client:     sharedHTTPClient,
	}
}

func (t *TrackerClient) FetchPeers(ctx context.Context) ([]peer.AddrInfo, error) {
	if t.trackerURL == "" {
		return nil, fmt.Errorf("no tracker URL configured")
	}

	peersURL, err := joinTrackerPath(t.trackerURL, "peers")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, peersURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tracker request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker returned status %d", resp.StatusCode)
	}

	var peerResponses []trackerPeerResponse
	if err := json.NewDecoder(resp.Body).Decode(&peerResponses); err != nil {
		return nil, fmt.Errorf("failed to decode tracker response: %w", err)
	}

	var peers []peer.AddrInfo
	for _, p := range peerResponses {
		if p.PeerID == "" || len(p.Addrs) == 0 {
			continue
		}

		addrSeen := make(map[string]struct{})
		var addrInfo *peer.AddrInfo
		for _, addr := range p.Addrs {
			if addr == "" {
				continue
			}
			if _, ok := addrSeen[addr]; ok {
				continue
			}
			addrSeen[addr] = struct{}{}

			ma, err := multiaddr.NewMultiaddr(addr)
			if err != nil {
				continue
			}

			parsedAddrInfo, err := peer.AddrInfoFromP2pAddr(ma)
			if err != nil {
				continue
			}
			if addrInfo == nil {
				addrInfo = parsedAddrInfo
				continue
			}
			addrInfo.Addrs = append(addrInfo.Addrs, parsedAddrInfo.Addrs...)
		}
		if addrInfo == nil {
			continue
		}
		peers = append(peers, *addrInfo)
	}
	return peers, nil
}

func (t *TrackerClient) RegisterPeer(ctx context.Context, addrs []string, peerID string, authData string) error {
	if t.trackerURL == "" {
		return nil
	}

	registerURL, err := joinTrackerPath(t.trackerURL, "register")
	if err != nil {
		return err
	}

	data := trackerRegisterRequest{
		Addrs:    addrs,
		PeerID:   peerID,
		AuthData: authData,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registerURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized: tracker requires authentication")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tracker registration failed: %d", resp.StatusCode)
	}
	return nil
}

func (t *TrackerClient) FetchTrackerAddr(ctx context.Context) (string, string, error) {
	if t.trackerURL == "" {
		return "", "", fmt.Errorf("no tracker URL configured")
	}

	addrURL, err := joinTrackerPath(t.trackerURL, "addr")
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addrURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("tracker request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("tracker returned status %d", resp.StatusCode)
	}

	var result trackerAddrResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("failed to decode tracker response: %w", err)
	}
	return "", result.RelayAddr, nil
}

func joinTrackerPath(baseURL, path string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid tracker URL: %w", err)
	}

	cleanPath := pathpkg.Clean(parsed.Path)
	if cleanPath == "/peers" || cleanPath == "/register" || cleanPath == "/addr" {
		parsed.Path = pathpkg.Dir(cleanPath)
	}

	joined, err := url.JoinPath(parsed.String(), path)
	if err != nil {
		return "", fmt.Errorf("invalid tracker path: %w", err)
	}
	return joined, nil
}
