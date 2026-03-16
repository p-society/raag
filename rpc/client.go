package rpc

import (
	"context"
	"net"
	"net/rpc"
	"time"
)

// Client is an RPC client that connects to the daemon via Unix socket.
type Client struct {
	socketPath string
}

// NewClient creates a new RPC client.
func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

// Call executes an RPC method on the daemon.
func (c *Client) Call(method string, args, result any) error {
	conn, err := net.DialTimeout("unix", c.socketPath, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := rpc.NewClient(conn)
	defer client.Close()
	return client.Call(method, args, result)
}

// CallWithContext executes an RPC method with context support for cancellation.
// If the context is cancelled before the RPC completes, an error is returned.
func (c *Client) CallWithContext(ctx context.Context, method string, args, result any) error {
	conn, err := net.DialTimeout("unix", c.socketPath, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := rpc.NewClient(conn)
	defer client.Close()

	call := client.Go(method, args, result, nil)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case reply := <-call.Done:
		return reply.Error
	}
}

// IsAvailable checks if the daemon is reachable.
func (c *Client) IsAvailable() bool {
	conn, err := net.DialTimeout("unix", c.socketPath, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
