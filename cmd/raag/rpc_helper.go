package main

import (
	"context"
	"os"

	appconfig "github.com/p-society/raag/internal/config"
	"github.com/p-society/raag/internal/constants"
	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/rpc"
)

func newRPCClient() *rpc.Client {
	socketPath := appconfig.SocketPath()
	return rpc.NewClient(socketPath)
}

func invokeRPC(method string, args, result any) {
	ctx, cancel := context.WithTimeout(context.Background(), constants.RPCTimeout)
	defer cancel()

	client := requireDaemon()
	if err := client.CallWithContext(ctx, method, args, result); err != nil {
		logger.Errorf("RPC call '%s' failed: %v", method, err)
		os.Exit(1)
	}
}
