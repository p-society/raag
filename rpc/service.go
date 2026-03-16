package rpc

import (
	"context"
	"fmt"
	"net/rpc"
	"slices"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/p-society/raag/app"
	"github.com/p-society/raag/internal/discovery"
	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/internal/metadata"
)

// DaemonService exposes daemon operations via net/rpc.
type DaemonService struct {
	app        *app.App
	ctx        context.Context
	shutdownFn func()
}

// NewDaemonService creates a new RPC service backed by the app.
func NewDaemonService(a *app.App, ctx context.Context, shutdownFn func()) *DaemonService {
	return &DaemonService{app: a, ctx: ctx, shutdownFn: shutdownFn}
}

// Register registers the service with the default RPC server.
func (s *DaemonService) Register() {
	rpc.Register(s)
}

type (
	EmptyArgs   struct{}
	EmptyResult struct{}
)

// PeerInfo is a simplified peer representation for RPC.
type PeerInfo struct {
	ID        string `json:"id"`
	Addr      string `json:"addr,omitempty"`
	Connected bool   `json:"connected,omitempty"`
}

// LibrarySong is a simplified song representation for RPC.
type LibrarySong struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
	Path   string `json:"path"`
}

func peerToInfo(p peer.AddrInfo) PeerInfo {
	return PeerInfo{ID: p.ID.String()}
}

func songToInfo(s metadata.Song) LibrarySong {
	return LibrarySong{
		Title:  s.Title,
		Artist: s.Artist,
		Album:  s.Album,
		Path:   s.Path,
	}
}

type PeerListResult struct {
	Peers []PeerInfo `json:"peers"`
}

func (s *DaemonService) PeersList(_ *EmptyArgs, result *PeerListResult) error {
	peers := s.app.NetMgr.GetPeers()
	result.Peers = make([]PeerInfo, len(peers))
	for i, p := range peers {
		result.Peers[i] = peerToInfo(p)
	}
	return nil
}

type PeersInfoResult struct {
	Self           map[string]string `json:"self"`
	ConnectedPeers []PeerInfo        `json:"connected_peers"`
	ConnectedCount int               `json:"connected_peer_count"`
	KnownPeers     []PeerInfo        `json:"known_peers"`
	KnownCount     int               `json:"known_peer_count"`
}

func (s *DaemonService) PeersInfo(_ *EmptyArgs, result *PeersInfoResult) error {
	peerID := s.app.NetMgr.GetPeerID()
	multiaddr := s.app.NetMgr.GetMultiaddr()
	connectedPeers := s.app.NetMgr.GetPeers()
	allKnownPeers := s.app.NetMgr.GetAllKnownPeers()
	result.Self = map[string]string{
		"peer_id":   peerID.String(),
		"multiaddr": multiaddr,
	}

	result.ConnectedPeers = make([]PeerInfo, len(connectedPeers))
	for i, p := range connectedPeers {
		result.ConnectedPeers[i] = peerToInfo(p)
	}

	result.ConnectedCount = len(connectedPeers)
	result.KnownPeers = make([]PeerInfo, len(allKnownPeers))
	for i, p := range allKnownPeers {
		result.KnownPeers[i] = peerToInfo(p)
	}
	result.KnownCount = len(allKnownPeers)
	return nil
}

type ConnectArgs struct {
	Multiaddr string `json:"multiaddr"`
}

func (s *DaemonService) Connect(args *ConnectArgs, result *EmptyResult) error {
	addrInfo, err := peer.AddrInfoFromString(args.Multiaddr)
	if err != nil {
		return err
	}
	return s.app.NetMgr.Connect(s.ctx, *addrInfo)
}

type DisconnectArgs struct {
	PeerID string `json:"peer_id"`
}

func (s *DaemonService) Disconnect(args *DisconnectArgs, result *EmptyResult) error {
	peerID, err := peer.Decode(args.PeerID)
	if err != nil {
		return err
	}
	return s.app.NetMgr.Disconnect(peerID)
}

type TrackerArgs struct {
	URL string `json:"url"`
}

func (s *DaemonService) UpdateTracker(args *TrackerArgs, result *EmptyResult) error {
	s.app.NetMgr.UpdateTrackerURL(s.ctx, args.URL)
	return nil
}

type BootstrapArgs struct {
	Multiaddr string `json:"multiaddr"`
}

func (s *DaemonService) AddBootstrap(args *BootstrapArgs, result *EmptyResult) error {
	return s.app.NetMgr.AddBootstrapPeer(s.ctx, args.Multiaddr)
}

type LibraryListResult struct {
	Songs []LibrarySong `json:"songs"`
}

func (s *DaemonService) LibraryList(_ *EmptyArgs, result *LibraryListResult) error {
	songs := slices.Collect(s.app.Lib.AllSongs())
	result.Songs = make([]LibrarySong, len(songs))
	for i, song := range songs {
		result.Songs[i] = songToInfo(song)
	}
	return nil
}

type NetworkStatusResult struct {
	State discovery.NetworkState `json:"state"`
}

func (s *DaemonService) NetworkStatus(_ *EmptyArgs, result *NetworkStatusResult) error {
	state, err := s.app.NetMgr.GetNetworkState()
	if err != nil {
		return err
	}
	result.State = state
	return nil
}

type AuthKeyResult struct {
	AuthPublicKey string `json:"auth_public_key"`
}

func (s *DaemonService) AuthKey(_ *EmptyArgs, result *AuthKeyResult) error {
	result.AuthPublicKey = s.app.NetMgr.GetAuthPublicKey()
	return nil
}

type StatusResult struct {
	Running   bool   `json:"running"`
	PeerCount int    `json:"peer_count"`
	Connected bool   `json:"network_online"`
	Uptime    string `json:"uptime"`
	Version   string `json:"version"`
}

func (s *DaemonService) Status(_ *EmptyArgs, result *StatusResult) error {
	result.Running = true
	result.PeerCount = s.app.NetMgr.GetPeerCount()
	result.Connected = s.app.NetMgr.IsOnline()
	result.Uptime = time.Since(s.app.StartTime).String()
	result.Version = "1.0.0"
	return nil
}

func (s *DaemonService) Shutdown() error {
	go s.shutdownFn()
	return nil
}

type PlaylistSongsArgs struct {
	Playlist string `json:"playlist"`
}

type PlaylistSongsResult struct {
	Songs []metadata.Song `json:"songs"`
}

func (s *DaemonService) PlaylistSongs(args *PlaylistSongsArgs, result *PlaylistSongsResult) error {
	songs, err := s.app.PM.GetSongs(args.Playlist)
	if err != nil {
		return err
	}
	result.Songs = songs
	return nil
}

type KnownPeersResult struct {
	Peers []PeerInfo `json:"peers"`
}

func (s *DaemonService) KnownPeers(_ *EmptyArgs, result *KnownPeersResult) error {
	allKnown := s.app.NetMgr.GetAllKnownPeers()
	result.Peers = make([]PeerInfo, len(allKnown))
	for i, p := range allKnown {
		result.Peers[i] = peerToInfo(p)
	}
	return nil
}

type PlayArgs struct {
	Song string `json:"song"`
}

func (s *DaemonService) Play(args *PlayArgs, result *EmptyResult) error {
	song, err := s.app.Lib.FindSong(args.Song)
	if err != nil {
		return err
	}

	s.app.Player.AddToQueue(song)
	if s.app.Player.GetCurrentSong() == nil {
		return s.app.Player.PlayQueue()
	}
	return nil
}

func (s *DaemonService) Pause(_ *EmptyArgs, result *EmptyResult) error {
	s.app.Player.Pause()
	return nil
}

func (s *DaemonService) Resume(_ *EmptyArgs, result *EmptyResult) error {
	s.app.Player.Resume()
	return nil
}

func (s *DaemonService) StopPlayback(_ *EmptyArgs, result *EmptyResult) error {
	s.app.Player.Stop()
	return nil
}

func (s *DaemonService) Next(_ *EmptyArgs, result *EmptyResult) error {
	return s.app.Player.Next()
}

func (s *DaemonService) Previous(_ *EmptyArgs, result *EmptyResult) error {
	return s.app.Player.Previous()
}

type VolumeArgs struct {
	Level float64 `json:"level"`
}

func (s *DaemonService) SetVolume(args *VolumeArgs, result *EmptyResult) error {
	return s.app.Player.SetVolume(args.Level)
}

type SeekArgs struct {
	Seconds int `json:"seconds"`
}

func (s *DaemonService) Seek(args *SeekArgs, result *EmptyResult) error {
	return s.app.Player.Seek(args.Seconds)
}

type QueueResult struct {
	Songs   []LibrarySong `json:"songs"`
	Current int           `json:"current_index"`
}

func (s *DaemonService) GetQueue(_ *EmptyArgs, result *QueueResult) error {
	songs := s.app.Player.GetQueue()
	result.Songs = make([]LibrarySong, len(songs))
	for i, song := range songs {
		result.Songs[i] = songToInfo(song)
	}
	return nil
}

type NowPlayingResult struct {
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Position int     `json:"position"`
	Duration int     `json:"duration"`
	Volume   float64 `json:"volume"`
	Playing  bool    `json:"playing"`
	Paused   bool    `json:"paused"`
}

func (s *DaemonService) NowPlaying(_ *EmptyArgs, result *NowPlayingResult) error {
	song := s.app.Player.GetCurrentSong()
	if song == nil {
		return nil
	}
	result.Title = song.Title
	result.Artist = song.Artist
	result.Album = song.Album
	result.Position = s.app.Player.GetPosition()
	result.Duration = s.app.Player.GetDuration()
	result.Volume = s.app.Player.GetVolume()
	result.Playing = s.app.Player.IsPlaying()
	result.Paused = s.app.Player.IsPaused()
	return nil
}

type SearchArgs struct {
	Query string `json:"query"`
}

type SearchResult struct {
	Songs []LibrarySong `json:"songs"`
}

func (s *DaemonService) LibrarySearch(args *SearchArgs, result *SearchResult) error {
	q := strings.ToLower(args.Query)
	var songs []metadata.Song
	for song := range s.app.Lib.AllSongs() {
		if strings.Contains(strings.ToLower(song.Title), q) ||
			strings.Contains(strings.ToLower(song.Artist), q) ||
			strings.Contains(strings.ToLower(song.Album), q) {
			songs = append(songs, song)
		}
	}

	result.Songs = make([]LibrarySong, len(songs))
	for i, song := range songs {
		result.Songs[i] = songToInfo(song)
	}
	return nil
}

type AddArgs struct {
	Path string `json:"path"`
}

func (s *DaemonService) LibraryAdd(args *AddArgs, result *EmptyResult) error {
	return s.app.Lib.AddSong(args.Path)
}

type RemoveArgs struct {
	Title string `json:"title"`
}

func (s *DaemonService) LibraryRemove(args *RemoveArgs, result *EmptyResult) error {
	return s.app.Lib.RemoveSong(args.Title)
}

type RescanResult struct {
	Count int `json:"count"`
}

func (s *DaemonService) LibraryRescan(_ *EmptyArgs, result *RescanResult) error {
	musicDir := s.app.Lib.GetMusicDir()
	if err := s.app.Lib.ScanMusicLibrary(musicDir); err != nil {
		return err
	}

	count := 0
	for range s.app.Lib.AllSongs() {
		count++
	}

	logger.Infof("Library rescanned: %d songs", count)
	result.Count = count
	return nil
}

type PlaylistCreateArgs struct {
	Name string `json:"name"`
}

func (s *DaemonService) PlaylistCreate(args *PlaylistCreateArgs, result *EmptyResult) error {
	return s.app.PM.Create(args.Name)
}

type PlaylistDeleteArgs struct {
	Name string `json:"name"`
}

func (s *DaemonService) PlaylistDelete(args *PlaylistDeleteArgs, result *EmptyResult) error {
	return s.app.PM.Delete(args.Name)
}

type PlaylistListResult struct {
	Names []string `json:"names"`
}

func (s *DaemonService) PlaylistList(_ *EmptyArgs, result *PlaylistListResult) error {
	result.Names = s.app.PM.List()
	return nil
}

type PlaylistAddArgs struct {
	Playlist string `json:"playlist"`
	Song     string `json:"song"`
}

func (s *DaemonService) PlaylistAdd(args *PlaylistAddArgs, result *EmptyResult) error {
	song, err := s.app.Lib.FindSong(args.Song)
	if err != nil {
		return err
	}
	return s.app.PM.AddSong(args.Playlist, song)
}

type PlaylistRemoveArgs struct {
	Playlist string `json:"playlist"`
	Index    int    `json:"index"`
}

func (s *DaemonService) PlaylistRemove(args *PlaylistRemoveArgs, result *EmptyResult) error {
	return s.app.PM.RemoveSong(args.Playlist, args.Index)
}

type ConfigGetResult struct {
	Config map[string]any `json:"config"`
}

func (s *DaemonService) ConfigGet(_ *EmptyArgs, result *ConfigGetResult) error {
	cfg := s.app.Cfg
	result.Config = map[string]any{
		"music_dir":  cfg.MusicDir,
		"volume":     cfg.Volume,
		"tui":        cfg.TUI,
		"network":    cfg.Network,
		"rendezvous": cfg.Rendezvous,
		"host":       cfg.Host,
		"port":       cfg.Port,
		"log_level":  cfg.LogLevel,
		"tracker":    cfg.TrackerURL,
	}
	return nil
}

type ConfigSetArgs struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (s *DaemonService) ConfigSet(args *ConfigSetArgs, result *EmptyResult) error {
	return fmt.Errorf("config set not yet fully implemented via RPC")
}

type FullStateResult struct {
	NowPlaying *NowPlayingResult
	Queue      *QueueResult
	Network    *NetworkStatusResult
	Status     *StatusResult
	Library    *LibraryListResult
}

func (s *DaemonService) GetFullState(_ *EmptyArgs, result *FullStateResult) error {
	var nowPlaying NowPlayingResult
	if err := s.NowPlaying(&EmptyArgs{}, &nowPlaying); err == nil {
		result.NowPlaying = &nowPlaying
	}

	var queue QueueResult
	if err := s.GetQueue(&EmptyArgs{}, &queue); err == nil {
		result.Queue = &queue
	}

	var network NetworkStatusResult
	if err := s.NetworkStatus(&EmptyArgs{}, &network); err == nil {
		result.Network = &network
	}

	var status StatusResult
	if err := s.Status(&EmptyArgs{}, &status); err == nil {
		result.Status = &status
	}

	var library LibraryListResult
	if err := s.LibraryList(&EmptyArgs{}, &library); err == nil {
		result.Library = &library
	}
	return nil
}
