# Raag

Raag is a production-ready P2P music streaming application with terminal-first UI, featuring local playback, playlist management, and secure peer-to-peer music sharing over libp2p.

## Features

- **Local Playback** - Play music from a local directory with full playback controls
- **Playlist Management** - Create, manage, and play playlists from CLI or TUI
- **Secure P2P Sharing** - Share music with peers using encrypted libp2p connections
- **Multi-NAT Traversal** - Hole punching, UPnP, and AutoNAT for direct peer connections
- **Peer Discovery** - mDNS (LAN), DHT (decentralized), and optional tracker (cross-network)
- **Daemon Mode** - Persistent peer connections for continuous availability
- **End-to-End Auth** - Token-based authentication with replay protection

## Quick Start

### Build

```bash
make build          # Production binaries
make build-dev      # Debug builds
make build-race     # Race detector build
```

Or manually:
```bash
go build -o bin/raag ./cmd/raag
go build -o bin/tracker ./tracker/cmd/tracker
```

### Run Client

```bash
# Local playback with TUI
./bin/raag --tui

# Network mode (peer discovery enabled)
./bin/raag --network

# Daemon mode (persistent connections)
./bin/raag daemon --network
```

## Authentication

### Option 1: Shared Secret (Recommended - Easy)

Use a shared password for all peers:

**Tracker:**
```bash
# Set environment variable
AUTH_SECRET=mysecretpassword

# Deploy
railway up
# OR locally with docker
docker-compose up -d
```

**Client:**
```bash
./bin/raag daemon --network --auth-secret mysecretpassword
```

The system automatically derives a cryptographic key from the password using PBKDF2.

### Option 2: Per-Peer Keys (More Secure)

Each peer has a unique key derived from their identity:

**Get your auth key:**
```bash
./bin/raag --network network auth-key
```

**Tracker:**
```bash
# Single key
AUTH_KEY=23eb477a60833ed094e456110928584093d349b35f6f793f5057cc781aee586d railway up

# Multiple keys (comma-separated)
AUTH_KEY=key1,key2,key3 railway up
```

**Client:**
```bash
# No extra flag needed - uses identity.key automatically
./bin/raag daemon --network
```

### How Auth Works

```
Shared Secret / Identity Key
         ↓
    PBKDF2 (50k iterations)
         ↓
    Ed25519 Key Pair
         ↓
    Sign registration token
         ↓
    Tracker verifies & accepts
```

Security features:
- Password never sent over network
- PBKDF2 key derivation (50k iterations)
- Token expires after 24 hours
- Automatic token refresh before expiration (1 hour threshold)
- Clock skew tolerance: 60 seconds
- Signature verification prevents tampering
- Nonce tracking prevents replay attacks
- Rate limiting on tracker registration (10 req/min/IP)
- HTTPS support for production deployments

## Commands

### Playback

```bash
./bin/raag play <song>
./bin/raag pause
./bin/raag resume
./bin/raag stop
./bin/raag next
./bin/raag previous
./bin/raag seek [seconds]
./bin/raag volume [0-100]
./bin/raag queue
./bin/raag nowplaying
```

### Library

```bash
./bin/raag library list
./bin/raag library search [query]
./bin/raag library rescan
./bin/raag library add <path>
./bin/raag library remove <title>
```

### Playlists

```bash
./bin/raag playlist create <name>
./bin/raag playlist delete <name>
./bin/raag playlist list
./bin/raag playlist add <playlist> <song>
./bin/raag playlist remove <playlist> <index>
./bin/raag playlist songs <playlist>
./bin/raag playlist play <playlist>
```

### Networking

```bash
./bin/raag peers list
./bin/raag peers info
./bin/raag peers ping <peerID>
./bin/raag peers connect <multiaddr>
./bin/raag peers disconnect <peerID>
./bin/raag peers tracker <url>
./bin/raag peers bootstrap <multiaddr>
./bin/raag network status
./bin/raag network auth-key
```

### Daemon & Config

```bash
./bin/raag daemon              # Start daemon
./bin/raag daemon --network    # With networking
./bin/raag status              # Check status
./bin/raag config show        # Show config
./bin/raag config set <key> <value>
```

**Daemon RPC:** Commands communicate with the daemon via Unix socket (`~/.config/raag/daemon.sock`). RPC timeout is 10 seconds.

## Flags

### Client

| Flag | Default | Description |
|------|---------|-------------|
| `--network` | `false` | Enable peer discovery |
| `--host` | `0.0.0.0` | Bind address |
| `--port` | `45678` | Peer listen port |
| `--tracker` | (none) | Tracker URL |
| `--auth-secret` | (none) | Shared secret for auth |
| `--dht` | `true` | Enable DHT discovery |
| `--bootstrap` | - | Bootstrap peers (comma-separated) |
| `--max-peers` | `100` | Maximum peers |
| `--music-dir` | `~/.config/raag/music` | Music directory |
| `--tui` | `false` | Start TUI |
| `--json` | `false` | JSON logging |
| `--log-level` | `info` | Log level (debug, info, warn, error) |

### Tracker

| Flag | Default | Description |
|------|---------|-------------|
| `--http-port` | `8080` | HTTP API port |
| `--libp2p-port` | `45678` | Libp2p port |
| `--relay` | `true` | Enable circuit relay |
| `--auth-key` | - | Trusted auth key (repeatable) |
| `--auth-secret` | - | Shared secret for auth |
| `--tls` | `false` | Enable HTTPS |
| `--tls-cert` | - | TLS certificate file path |
| `--tls-key` | - | TLS key file path |

### Tracker Environment Variables

| Variable | Description |
|----------|-------------|
| `AUTH_SECRET` | Shared secret for auth |
| `AUTH_KEY` | Comma-separated trusted keys |
| `TLS_ENABLED` | Enable HTTPS (`true`/`false`) |
| `TLS_CERT_FILE` | TLS certificate file path |
| `TLS_KEY_FILE` | TLS key file path |

**Auto TLS**: If `TLS_ENABLED=true` is set but no cert/key files provided, the tracker automatically generates a self-signed certificate.

## Deployment

### Railway (Recommended)

1. Set environment variables in Railway Dashboard:
   - `AUTH_SECRET=yourpassword` (shared secret) OR
   - `AUTH_KEY=key1,key2` (per-peer keys)

2. Deploy:
```bash
railway up
```

3. Enable networking in Railway:
   - Go to Service Settings → Networking
   - Add TCP port 45678
   - Add UDP port 45678

### Local Docker

```bash
docker-compose up -d
```

The tracker will start with `AUTH_SECRET=raag-secret` from docker-compose.yml.

### Manual

```bash
# Start tracker
./bin/tracker --http-port 8080 --relay

# Or with shared secret auth
./bin/tracker --http-port 8080 --relay --auth-secret mypassword

# Or with per-peer keys
./bin/tracker --http-port 8080 --relay --auth-key <key1> --auth-key <key2>
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `PORT` | HTTP API port (default: 8080) |
| `AUTH_SECRET` | Shared secret for authentication |
| `AUTH_KEY` | Comma-separated trusted keys (tracker) |
| `TRACKER_URL` | Default tracker URL (client) |
| `XDG_CONFIG_HOME` | Config directory (default: ~/.config/raag) |

## Architecture

### Components

```
raag CLI/TUI
    ├── config + storage
    ├── library + playlists + player
    └── network manager
            ├── identity.key (persistent)
            ├── libp2p host (TCP + QUIC)
            ├── mDNS discovery (LAN)
            ├── DHT discovery (decentralized)
            ├── tracker client (registration)
            ├── NAT traversal (hole punch, UPnP, AutoNAT)
            ├── presence protocol (peer online/offline)
            └── file transfer protocol
```

### Discovery Mechanisms

- **mDNS** - Local network peer discovery
- **DHT** - Decentralized peer discovery via Kademlia
- **Tracker** - Centralized peer registration (optional)

### File Transfer

- Framed libp2p stream protocol
- Metadata: title, artist, album, size, SHA-256
- Encrypted via libp2p's Noise protocol
- Idle timeout protection (30s)
- Atomic write with temp files
- Max file size: 256MB

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make build` | Production build |
| `make build-dev` | Debug build |
| `make build-race` | Race detector build |
| `make test` | Run tests |
| `make test-race` | Tests with race detector |
| `make ci` | Full CI pipeline |
| `make run` | Build and run raag |
| `make run-tracker` | Build and run tracker |
| `make lint` | Format + vet + staticcheck |
| `make clean` | Remove artifacts |

## Testing

The project includes comprehensive tests:

```bash
# Run all tests
make test

# Run with race detector
make test-race

# Run specific package tests
go test ./internal/auth/...     # Auth & crypto
go test ./tracker/...           # Tracker server
go test ./internal/network/...  # Network & transfer
go test ./internal/playlist/... # Playlist manager
go test ./internal/library/...   # Music library
```

### Test Coverage

- **Authentication**: Key generation, PBKDF2 derivation, token signing/verification, nonce uniqueness
- **Tracker**: Rate limiting, peer registration, auth validation, replay protection
- **Network**: Framed transfer protocol, metadata serialization, address handling
- **Playlist**: CRUD operations, song management
- **Library**: Song scanning, searching, removal

## Files & Storage

Config stored in `~/.config/raag/`:

- `config.yaml` - Configuration
- `state.json` - Playback state
- `identity.key` - Libp2p identity (critical!)
- `daemon.sock` - Daemon socket
- `playlists.json` - Playlist data
- `peers.json` - Discovered peers cache
- `music/` - Music files directory

**Important:** `identity.key` determines your peer ID. Keep it safe! If lost, you'll get a new peer ID.

## Troubleshooting

### No peers discovered

```bash
# Check network status
./bin/raag network status

# Enable debug logging
./bin/raag --log-level debug daemon --network
```

### Tracker registration fails

- Verify auth secret matches between tracker and client
- Check tracker URL is reachable
- For per-peer keys: verify your key is in tracker's trusted list

### Peer connection issues

- Both peers need to be authenticated with the same tracker
- NAT/firewall may block connections
- Use relay mode (`--relay`) for NAT traversal

## License

Apache License 2.0
