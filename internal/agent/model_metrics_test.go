package agent

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestModelMetrics_RecordAndGet(t *testing.T) {
	t.Parallel()

	store := NewMetricsStore()
	store.Record("gpt-4", 100*time.Millisecond, true, 500, 200, 0.03)
	store.Record("gpt-4", 200*time.Millisecond, true, 300, 100, 0.02)
	store.Record("gpt-4", 50*time.Millisecond, false, 100, 0, 0.001)

	m := store.Get("gpt-4")
	require.NotNil(t, m)
	require.Equal(t, int64(3), m.RequestCount())
	require.Equal(t, int64(2), m.SuccessCount())
	require.Equal(t, int64(1), m.FailureCount())
	require.Equal(t, 350*time.Millisecond, m.TotalLatency())
	require.Equal(t, int64(900), m.TotalInputTokens())
	require.Equal(t, int64(300), m.TotalOutputTokens())
	require.InDelta(t, 0.051, m.TotalCost(), 0.0001)
}

func TestModelMetrics_SuccessRate(t *testing.T) {
	t.Parallel()

	store := NewMetricsStore()
	require.Nil(t, store.Get("unknown"))

	store.Record("model-a", time.Second, true, 0, 0, 0)
	store.Record("model-a", time.Second, true, 0, 0, 0)
	store.Record("model-a", time.Second, false, 0, 0, 0)

	m := store.Get("model-a")
	require.InDelta(t, 2.0/3.0, m.SuccessRate(), 0.001)
}

func TestModelMetrics_AvgLatency(t *testing.T) {
	t.Parallel()

	store := NewMetricsStore()
	store.Record("model-b", 100*time.Millisecond, true, 0, 0, 0)
	store.Record("model-b", 300*time.Millisecond, true, 0, 0, 0)

	m := store.Get("model-b")
	require.Equal(t, 200*time.Millisecond, m.AvgLatency())
}

func TestModelMetrics_TokenEfficiency(t *testing.T) {
	t.Parallel()

	store := NewMetricsStore()
	store.Record("model-c", time.Millisecond, true, 800, 200, 0)

	m := store.Get("model-c")
	require.InDelta(t, 0.2, m.TokenEfficiency(), 0.001)
}

func TestModelMetrics_EmptyModel(t *testing.T) {
	t.Parallel()

	store := NewMetricsStore()
	store.Record("", time.Millisecond, true, 100, 50, 0)
	require.Nil(t, store.Get(""))
	require.Empty(t, store.GetAll())
}

func TestModelMetrics_ZeroRequests(t *testing.T) {
	t.Parallel()

	m := &ModelMetrics{ModelName: "unused"}
	require.Equal(t, float64(0), m.SuccessRate())
	require.Equal(t, time.Duration(0), m.AvgLatency())
	require.Equal(t, float64(0), m.TokenEfficiency())
	require.Equal(t, int64(0), m.RequestCount())
}

func TestModelMetrics_GetAll(t *testing.T) {
	t.Parallel()

	store := NewMetricsStore()
	store.Record("model-x", time.Millisecond, true, 10, 5, 0)
	store.Record("model-y", time.Millisecond, true, 20, 10, 0)

	all := store.GetAll()
	require.Len(t, all, 2)
	require.Contains(t, all, "model-x")
	require.Contains(t, all, "model-y")
	require.Equal(t, int64(10), all["model-x"].TotalInputTokens())
	require.Equal(t, int64(20), all["model-y"].TotalInputTokens())
}

func TestModelMetrics_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	store := NewMetricsStore()
	const workers = 50
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			store.Record("concurrent-model", time.Millisecond, true, 10, 5, 0.01)
		}()
	}
	wg.Wait()

	m := store.Get("concurrent-model")
	require.Equal(t, int64(workers), m.RequestCount())
	require.Equal(t, int64(workers), m.SuccessCount())
	require.Equal(t, int64(0), m.FailureCount())
	require.Equal(t, int64(workers*10), m.TotalInputTokens())
	require.Equal(t, int64(workers*5), m.TotalOutputTokens())
}

func TestModelMetrics_MultipleModels(t *testing.T) {
	t.Parallel()

	store := NewMetricsStore()
	store.Record("a", 10*time.Millisecond, true, 100, 50, 0.01)
	store.Record("b", 20*time.Millisecond, false, 200, 0, 0)

	ma := store.Get("a")
	mb := store.Get("b")
	require.Equal(t, int64(1), ma.RequestCount())
	require.Equal(t, int64(1), mb.RequestCount())
	require.InDelta(t, 1.0, ma.SuccessRate(), 0.001)
	require.InDelta(t, 0.0, mb.SuccessRate(), 0.001)
}

func TestModelMetrics_TierRouterSetMetricsStore(t *testing.T) {
	t.Parallel()

	store := NewMetricsStore()
	r := NewTierRouter(nil)
	r.SetMetricsStore(store)
	require.Equal(t, store, r.metrics)
}
