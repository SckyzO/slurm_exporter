package collector

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimedCache_FreshCacheReturnsCachedData(t *testing.T) {
	c := &timedCache{ttl: 10 * time.Second}
	calls := atomic.Int32{}

	fetch := func() ([]byte, error) {
		calls.Add(1)
		return []byte("data"), nil
	}

	// First call fetches
	data, err := c.GetOrFetch(fetch)
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), data)
	assert.Equal(t, int32(1), calls.Load())

	// Second call within TTL returns cache (no fetch)
	data, err = c.GetOrFetch(fetch)
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), data)
	assert.Equal(t, int32(1), calls.Load(), "second call must not re-fetch")
}

func TestTimedCache_ExpiredCacheRefetches(t *testing.T) {
	c := &timedCache{ttl: 1 * time.Millisecond}
	calls := atomic.Int32{}

	fetch := func() ([]byte, error) {
		calls.Add(1)
		return []byte("fresh"), nil
	}

	_, _ = c.GetOrFetch(fetch)
	time.Sleep(5 * time.Millisecond) // wait for TTL to expire

	_, err := c.GetOrFetch(fetch)
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load(), "expired cache must trigger a re-fetch")
}

func TestTimedCache_FetchErrorDoesNotPopulateCache(t *testing.T) {
	c := &timedCache{ttl: 10 * time.Second}
	fetchErr := errors.New("fetch failed")

	_, err := c.GetOrFetch(func() ([]byte, error) { return nil, fetchErr })
	assert.ErrorIs(t, err, fetchErr)

	// AgeSeconds must return -1 (cache still empty)
	assert.Equal(t, float64(-1), c.AgeSeconds())
}

func TestTimedCache_AgeSeconds(t *testing.T) {
	c := &timedCache{ttl: 10 * time.Second}
	assert.Equal(t, float64(-1), c.AgeSeconds(), "unpopulated cache age should be -1")

	_, _ = c.GetOrFetch(func() ([]byte, error) { return []byte("x"), nil })
	age := c.AgeSeconds()
	assert.GreaterOrEqual(t, age, float64(0))
	assert.Less(t, age, float64(1), "age should be < 1s right after fetch")
}

func TestTimedCache_ConcurrentAccess(t *testing.T) {
	c := &timedCache{ttl: 100 * time.Millisecond}
	calls := atomic.Int32{}

	fetch := func() ([]byte, error) {
		calls.Add(1)
		time.Sleep(5 * time.Millisecond) // simulate slow command
		return []byte("result"), nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := c.GetOrFetch(fetch)
			assert.NoError(t, err)
			assert.Equal(t, []byte("result"), data)
		}()
	}
	wg.Wait()

	// All 20 goroutines should have gotten the result but fetch called ≤ twice
	// (once for the initial fetch, potentially once more if TTL expired mid-test)
	assert.LessOrEqual(t, calls.Load(), int32(3), "concurrent callers must not all re-fetch")
}
