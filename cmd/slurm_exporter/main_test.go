package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// TestRunServer_ShutsDownOnContextCancel is the regression test for issue #139:
// on SIGTERM/SIGINT the signal context is cancelled, and runServer must shut the
// HTTP server down and return within a bounded time. Before the fix runServer
// blocked in web.ListenAndServe and never observed ctx.Done(), so a shutdown
// signal was ignored and the process had to be SIGKILLed.
func TestRunServer_ShutsDownOnContextCancel(t *testing.T) {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	serve := func() error { return server.Serve(ln) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runServer(ctx, server, serve, nil, logger.NewTextLogger("error")) }()

	// Let the server start serving, then send the shutdown signal.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err, "a clean shutdown returns nil")
	case <-time.After(3 * time.Second):
		t.Fatal("runServer did not return within 3s of context cancellation (issue #139)")
	}
}

// TestRunServer_ReturnsStartupError verifies a genuine listen failure is
// surfaced so main can exit non-zero, rather than swallowed.
func TestRunServer_ReturnsStartupError(t *testing.T) {
	serve := func() error { return errors.New("listen tcp :9341: bind: address already in use") }
	err := runServer(context.Background(), &http.Server{}, serve, nil, logger.NewTextLogger("error"))
	require.Error(t, err)
}

// TestRunServer_IgnoresServerClosed verifies http.ErrServerClosed, which
// ListenAndServe returns after a clean Shutdown, is not treated as a startup
// failure (which would exit 1 on every normal stop).
func TestRunServer_IgnoresServerClosed(t *testing.T) {
	serve := func() error { return http.ErrServerClosed }
	err := runServer(context.Background(), &http.Server{}, serve, nil, logger.NewTextLogger("error"))
	require.NoError(t, err)
}
