package collector

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// timedCache is a thread-safe byte cache with a configurable TTL.
// Designed for expensive Slurm commands (e.g. scontrol show nodes -o)
// that are called by multiple collectors on every scrape but whose
// output changes much more slowly than the scrape interval.
type timedCache struct {
	mu      sync.RWMutex
	data    []byte
	fetchAt time.Time
	ttl     time.Duration
}

// GetOrFetch returns cached data if still fresh, otherwise calls fetch(),
// stores the result and returns it. Concurrent callers wait on a single
// write lock — no thundering herd, no stale-while-revalidate complexity.
func (c *timedCache) GetOrFetch(fetch func() ([]byte, error)) ([]byte, error) {
	// Fast path: read lock, return cache if still valid.
	c.mu.RLock()
	if !c.fetchAt.IsZero() && time.Since(c.fetchAt) < c.ttl {
		data := make([]byte, len(c.data))
		copy(data, c.data)
		c.mu.RUnlock()
		return data, nil
	}
	c.mu.RUnlock()

	// Slow path: write lock, double-check, then fetch.
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.fetchAt.IsZero() && time.Since(c.fetchAt) < c.ttl {
		data := make([]byte, len(c.data))
		copy(data, c.data)
		return data, nil
	}
	data, err := fetch()
	if err != nil {
		return nil, err
	}
	c.data = data
	c.fetchAt = time.Now()
	return data, nil
}

// AgeSeconds returns how many seconds ago the cache was last refreshed.
// Returns -1 if the cache has never been populated.
func (c *timedCache) AgeSeconds() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.fetchAt.IsZero() {
		return -1
	}
	return time.Since(c.fetchAt).Seconds()
}

// ── Shared caches ─────────────────────────────────────────────────────────────

// scontrolNodesCache is shared between NodesCollector (SlurmGetTotal) and
// ReservationNodesCollector (ReservationNodesData). Both need the full
// scontrol show nodes -o output but there is no reason to fetch it twice.
// TTL is set just below the scrape interval (default 30s) so a single
// scrape always gets fresh data without double-fetching.
var scontrolNodesCache = &timedCache{ttl: 25 * time.Second}

// ── Cache age metric ──────────────────────────────────────────────────────────

var cacheAgeGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "slurm_exporter_cache_age_seconds",
		Help: "Age in seconds of the last refresh for each internal cache. " +
			"-1 means the cache has never been populated.",
	},
	[]string{"cache"},
)

// RegisterCacheMetrics registers the cache age gauge with the given registry.
// Must be called once at startup alongside RegisterExecMetrics.
func RegisterCacheMetrics(reg prometheus.Registerer) {
	reg.MustRegister(cacheAgeGauge)
}

// updateCacheAge publishes the current age of all known caches.
// Called from collectors that use caches so the metric stays up to date.
func updateCacheAge() {
	cacheAgeGauge.WithLabelValues("scontrol_nodes").Set(scontrolNodesCache.AgeSeconds())
}
