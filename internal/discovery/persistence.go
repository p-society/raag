package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	appconfig "github.com/p-society/raag/internal/config"
	"github.com/p-society/raag/internal/storage"
)

type PeerPersistence struct {
	peersFile string
}

func NewPeerPersistence() *PeerPersistence {
	peersFile, err := appconfig.PeerPersistencePath()
	if err != nil {
		peersFile = filepath.Join(os.TempDir(), "raag-peers.json")
	}
	return &PeerPersistence{peersFile: peersFile}
}

func (p *PeerPersistence) Load() ([]peer.AddrInfo, error) {
	data, err := os.ReadFile(p.peersFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var multiaddrs []string
	if err := json.Unmarshal(data, &multiaddrs); err != nil {
		return nil, err
	}

	var peers []peer.AddrInfo
	for _, addrStr := range multiaddrs {
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			continue
		}

		addrInfo, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			continue
		}
		peers = append(peers, *addrInfo)
	}
	return peers, nil
}

func (p *PeerPersistence) Save(peers []peer.AddrInfo) error {
	multiaddrs := AddrInfoStrings(peers)
	dir := filepath.Dir(p.peersFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return storage.WriteJSONAtomic(p.peersFile, multiaddrs)
}
