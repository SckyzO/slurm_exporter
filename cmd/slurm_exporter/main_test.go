package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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

// TestNewMux_RouteStatuses is the regression test for the landing page claiming
// every unmatched path. The page used to be registered with http.HandleFunc("/"),
// and "/" in an http.ServeMux matches everything nothing else claims, so a
// Prometheus server pointed at a mistyped /metric was answered 200 with an HTML
// page. The target scraped as up and the mistake surfaced as a parse error in
// Prometheus rather than as a 404 from the exporter.
func TestNewMux_RouteStatuses(t *testing.T) {
	mux, err := newMux(prometheus.NewRegistry())
	require.NoError(t, err)

	for _, tc := range []struct {
		path string
		want int
	}{
		{"/", http.StatusOK},
		{"/metrics", http.StatusOK},
		{"/healthz", http.StatusOK},
		{"/metric", http.StatusNotFound},           // the typo that motivated this
		{"/metrics/", http.StatusNotFound},         // trailing slash is a different route
		{"/debug/pprof/heap", http.StatusNotFound}, // never served; see newMux
		{"/anything-else", http.StatusNotFound},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, nil))
			require.Equal(t, tc.want, rec.Code)
		})
	}
}

// TestNewMux_LandingPage checks the two things the hand-written page got wrong
// besides its status code: it never set a Content-Type, and it linked only to
// /metrics.
func TestNewMux_LandingPage(t *testing.T) {
	mux, err := newMux(prometheus.NewRegistry())
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/html; charset=UTF-8", rec.Header().Get("Content-Type"))

	body := rec.Body.String()
	require.Contains(t, body, "Slurm Exporter")
	require.Contains(t, body, "metrics")
	require.Contains(t, body, "healthz")

	// LandingConfig.Profiling defaults to "true", which renders links to
	// /debug/pprof/*. The exporter does not import net/http/pprof, so leaving
	// the default would advertise a profiler that answers 404. The assertion is
	// on the links rather than on the word: the template emits a "#pprof" CSS
	// rule whatever the flag says, so matching "pprof" alone fails on styling
	// that has no route behind it.
	require.NotContains(t, body, "debug/pprof")
}

// TestNewMux_HealthzBody pins the payload orchestrators match on.
func TestNewMux_HealthzBody(t *testing.T) {
	mux, err := newMux(prometheus.NewRegistry())
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "ok", rec.Body.String())
}
