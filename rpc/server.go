package rpc

import (
	"context"
	"net"
	"net/rpc"
	"os"
	"sync"

	"github.com/p-society/raag/app"
	"github.com/p-society/raag/internal/logger"
)

// Server is a Unix socket RPC server using standard net/rpc.
type Server struct {
	socketPath string
	listener   net.Listener
	ctx        context.Context
	cancel     context.CancelFunc
	shutdownCh chan struct{}
	stopOnce   sync.Once
}

// NewServer creates a new RPC server using standard net/rpc.
func NewServer(socketPath string, a *app.App, shutdownFn func()) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	svc := NewDaemonService(a, ctx, shutdownFn)
	svc.Register()

	return &Server{
		socketPath: socketPath,
		ctx:        ctx,
		cancel:     cancel,
		shutdownCh: make(chan struct{}),
	}
}

// Start begins listening for connections.
func (s *Server) Start() error {
	os.Remove(s.socketPath)
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}

	s.listener = listener
	os.Chmod(s.socketPath, 0o600)
	logger.Infof("RPC server listening socket_path=%s", s.socketPath)
	go s.acceptLoop()
	return nil
}

// Stop shuts down the server.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		s.cancel()
		close(s.shutdownCh)
		if s.listener != nil {
			s.listener.Close()
		}
		os.Remove(s.socketPath)
	})
}

// ShutdownRequested returns a channel that closes when shutdown is requested via RPC.
func (s *Server) ShutdownRequested() <-chan struct{} {
	return s.shutdownCh
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.shutdownCh:
				return
			default:
			}
			return
		}
		go rpc.ServeConn(conn)
	}
}
