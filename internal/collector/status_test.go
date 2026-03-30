package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// mockCollector is a simple collector for testing StatusTracker
type mockCollector struct {
	desc  *prometheus.Desc
	value float64
	panic bool
}

func newMockCollector(name string, value float64) *mockCollector {
	return &mockCollector{
		desc:  prometheus.NewDesc(name, name, nil, nil),
		value: value,
	}
}

func (m *mockCollector) Describe(ch chan<- *prometheus.Desc) { ch <- m.desc }
func (m *mockCollector) Collect(ch chan<- prometheus.Metric) {
	if m.panic {
		panic("simulated collector panic")
	}
	ch <- prometheus.MustNewConstMetric(m.desc, prometheus.GaugeValue, m.value)
}

func TestStatusTracker_BasicCollect(t *testing.T) {
	log := logger.NewLogger("error")
	st := NewStatusTracker(log)
	st.Add("test_collector", newMockCollector("slurm_test_metric", 42))

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(st))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	assert.True(t, names["slurm_test_metric"])
	assert.True(t, names["slurm_exporter_collector_success"])
	assert.True(t, names["slurm_exporter_collector_duration_seconds"])
}

func TestStatusTracker_SuccessMetric(t *testing.T) {
	log := logger.NewLogger("error")
	st := NewStatusTracker(log)
	st.Add("healthy", newMockCollector("slurm_healthy_metric", 1))

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(st))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "slurm_exporter_collector_success" {
			require.Len(t, mf.Metric, 1)
			assert.Equal(t, float64(1), mf.Metric[0].Gauge.GetValue())
		}
	}
}

func TestStatusTracker_PanicRecovery(t *testing.T) {
	log := logger.NewLogger("error")
	st := NewStatusTracker(log)

	panicCollector := newMockCollector("slurm_panic_metric", 0)
	panicCollector.panic = true
	st.Add("panicking", panicCollector)

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(st))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	// success metric must be 0 for panicking collector
	for _, mf := range mfs {
		if mf.GetName() == "slurm_exporter_collector_success" {
			require.Len(t, mf.Metric, 1)
			assert.Equal(t, float64(0), mf.Metric[0].Gauge.GetValue())
		}
	}
	// panicking collector's metric must NOT be present
	for _, mf := range mfs {
		assert.NotEqual(t, "slurm_panic_metric", mf.GetName())
	}
}

func TestStatusTracker_MultipleCollectors(t *testing.T) {
	log := logger.NewLogger("error")
	st := NewStatusTracker(log)
	st.Add("col_a", newMockCollector("slurm_col_a", 1))
	st.Add("col_b", newMockCollector("slurm_col_b", 2))

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(st))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	assert.True(t, names["slurm_col_a"])
	assert.True(t, names["slurm_col_b"])

	// Both collectors should report success=1
	for _, mf := range mfs {
		if mf.GetName() == "slurm_exporter_collector_success" {
			assert.Len(t, mf.Metric, 2)
			for _, m := range mf.Metric {
				assert.Equal(t, float64(1), m.Gauge.GetValue())
			}
		}
	}
}

func TestStatusTracker_Describe(t *testing.T) {
	log := logger.NewLogger("error")
	st := NewStatusTracker(log)
	st.Add("col_a", newMockCollector("slurm_desc_a", 1))
	st.Add("col_b", newMockCollector("slurm_desc_b", 2))

	ch := make(chan *prometheus.Desc, 20)
	st.Describe(ch)
	close(ch)

	count := 0
	for range ch {
		count++
	}
	// 2 inner descriptors + success + duration = 4
	assert.Equal(t, 4, count)
}

func TestStatusTracker_Add(t *testing.T) {
	log := logger.NewLogger("error")
	st := NewStatusTracker(log)
	assert.Len(t, st.entries, 0)
	st.Add("a", newMockCollector("slurm_add_a", 1))
	st.Add("b", newMockCollector("slurm_add_b", 1))
	assert.Len(t, st.entries, 2)
}
