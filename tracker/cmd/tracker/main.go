package main

import (
	"os"
	"strings"

	"github.com/p-society/raag/internal/auth"
	"github.com/p-society/raag/internal/constants"
	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/tracker"
	"github.com/spf13/cobra"
)

func main() {
	logger.NewLogger("raag::tracker")

	var cfg struct {
		httpPort       int
		libp2pPort     int
		relayEnabled   bool
		authPublicKeys []string
		tlsEnabled     bool
		tlsCertFile    string
		tlsKeyFile     string
	}

	rootCmd := &cobra.Command{
		Use:   "tracker",
		Short: "Raag tracker server with optional relay support",
		Long: `Raag Tracker - Peer registry with optional relay for P2P network.

This tracker handles peer registration and peer list distribution.
Optionally provides circuit relay for NAT traversal between peers.

Example:
  ./tracker --http-port 8080 --libp2p-port 45678 --relay`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// If AUTH_SECRET is set, derive trust from it
			if authSecret := os.Getenv(constants.EnvAuthSecret); authSecret != "" {
				derivedKeyPair, err := auth.DeriveKey([]byte(authSecret), "raag-secret-v1")
				if err == nil {
					derivedKey := auth.GetPublicKeyHex(derivedKeyPair)
					cfg.authPublicKeys = append(cfg.authPublicKeys, derivedKey)
					logger.Infof("Auth enabled via shared secret")
				}
			}
			// If AUTH_KEY env is set AND --auth-key flags are used, prefer flags
			if authKeyEnv := os.Getenv(constants.EnvAuthKey); authKeyEnv != "" && len(cfg.authPublicKeys) == 0 {
				envKeys := strings.SplitSeq(authKeyEnv, ",")
				for k := range envKeys {
					k = strings.TrimSpace(k)
					if k != "" {
						cfg.authPublicKeys = append(cfg.authPublicKeys, k)
					}
				}
			}

			seen := make(map[string]bool)
			uniqueKeys := []string{}
			for _, k := range cfg.authPublicKeys {
				if !seen[k] {
					seen[k] = true
					uniqueKeys = append(uniqueKeys, k)
				}
			}

			cfg.authPublicKeys = uniqueKeys
			if tlsEnabled := os.Getenv("TLS_ENABLED"); tlsEnabled == "true" {
				cfg.tlsEnabled = true
			}
			if certFile := os.Getenv("TLS_CERT_FILE"); certFile != "" {
				cfg.tlsCertFile = certFile
			}
			if keyFile := os.Getenv("TLS_KEY_FILE"); keyFile != "" {
				cfg.tlsKeyFile = keyFile
			}

			config := tracker.TrackerConfig{
				HTTPPort:       cfg.httpPort,
				Libp2pPort:     cfg.libp2pPort,
				RelayEnabled:   cfg.relayEnabled,
				AuthPublicKeys: cfg.authPublicKeys,
				TLSEnabled:     cfg.tlsEnabled,
				TLSCertFile:    cfg.tlsCertFile,
				TLSKeyFile:     cfg.tlsKeyFile,
			}

			t := tracker.NewTracker(config)
			return t.Start()
		},
	}

	rootCmd.Flags().IntVar(&cfg.httpPort, "http-port", constants.DefaultHTTPPort, "HTTP API listen port")
	rootCmd.Flags().IntVar(&cfg.libp2pPort, "libp2p-port", constants.DefaultPort, "libp2p listen port (for relay)")
	rootCmd.Flags().BoolVar(&cfg.relayEnabled, "relay", true, "Enable circuit relay for NAT traversal")
	rootCmd.Flags().StringArrayVar(&cfg.authPublicKeys, "auth-key", nil, "Trusted ed25519 public key(s) for peer authentication (hex encoded, can be specified multiple times)")
	rootCmd.Flags().BoolVar(&cfg.tlsEnabled, "tls", false, "Enable HTTPS")
	rootCmd.Flags().StringVar(&cfg.tlsCertFile, "tls-cert", "", "TLS certificate file path")
	rootCmd.Flags().StringVar(&cfg.tlsKeyFile, "tls-key", "", "TLS key file path")
	if err := rootCmd.Execute(); err != nil {
		logger.Errorf("failed to execute command error=%v", err)
		os.Exit(1)
	}
}
