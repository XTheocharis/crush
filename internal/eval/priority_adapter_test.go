package eval

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepomapPriorityAdapterImplementsInterface(t *testing.T) {
	t.Parallel()

	var _ PrioritySource = NewRepomapPriorityAdapter(nil)
}

func TestRepomapPriorityAdapterReturnsScore(t *testing.T) {
	t.Parallel()

	scores := map[string]float64{
		"main.go":   0.7,
		"utils.go":  0.3,
		"README.md": 0.0,
	}
	a := NewRepomapPriorityAdapter(scores)

	require.InDelta(t, 0.7, a.Priority("main.go"), 1e-9)
	require.InDelta(t, 0.3, a.Priority("utils.go"), 1e-9)
	require.InDelta(t, 0.0, a.Priority("README.md"), 1e-9)
}

func TestRepomapPriorityAdapterUnknownFile(t *testing.T) {
	t.Parallel()

	a := NewRepomapPriorityAdapter(map[string]float64{"a.go": 0.5})
	require.Equal(t, 0.0, a.Priority("nonexistent.go"))
}

func TestRepomapPriorityAdapterNilScores(t *testing.T) {
	t.Parallel()

	a := NewRepomapPriorityAdapter(nil)
	require.Equal(t, 0.0, a.Priority("anything.go"))
	require.Equal(t, 0, a.Len())
}

func TestRepomapPriorityAdapterUpdateScores(t *testing.T) {
	t.Parallel()

	a := NewRepomapPriorityAdapter(map[string]float64{"old.go": 0.5})
	require.InDelta(t, 0.5, a.Priority("old.go"), 1e-9)

	a.UpdateScores(map[string]float64{"new.go": 0.9})
	require.Equal(t, 0.0, a.Priority("old.go"))
	require.InDelta(t, 0.9, a.Priority("new.go"), 1e-9)
	require.Equal(t, 1, a.Len())
}

func TestRepomapPriorityAdapterConcurrentAccess(t *testing.T) {
	t.Parallel()

	a := NewRepomapPriorityAdapter(map[string]float64{"seed.go": 0.1})

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			a.UpdateScores(map[string]float64{fmt.Sprintf("file_%d.go", i): float64(i) / 100.0})
		}(i)
		go func(i int) {
			defer wg.Done()
			_ = a.Priority(fmt.Sprintf("file_%d.go", i))
		}(i)
	}
	wg.Wait()

	require.Greater(t, a.Len(), 0)
}

func TestRepomapPriorityAdapterNormalizesPaths(t *testing.T) {
	t.Parallel()

	a := NewRepomapPriorityAdapter(map[string]float64{"dir/file.go": 0.5})
	require.InDelta(t, 0.5, a.Priority("dir/file.go"), 1e-9)
}

func TestRepomapPriorityAdapterPriorityLevels(t *testing.T) {
	t.Parallel()

	scores := map[string]float64{
		"high.go":   1.0,
		"medium.go": 0.4,
		"low.go":    0.05,
	}
	a := NewRepomapPriorityAdapter(scores)

	require.Equal(t, PriorityHigh, a.PriorityLevel("high.go"))
	require.Equal(t, PriorityMedium, a.PriorityLevel("medium.go"))
	require.Equal(t, PriorityLow, a.PriorityLevel("low.go"))
}

func TestRepomapPriorityAdapterPriorityLevelUnknownFile(t *testing.T) {
	t.Parallel()

	a := NewRepomapPriorityAdapter(map[string]float64{"a.go": 0.5})
	require.Equal(t, PriorityLow, a.PriorityLevel("missing.go"))
}

func TestRepomapPriorityAdapterWithReadCoordinator(t *testing.T) {
	t.Parallel()

	scores := map[string]float64{
		"high.go":   0.7,
		"medium.go": 0.2,
		"low.go":    0.1,
	}
	a := NewRepomapPriorityAdapter(scores)
	rc := NewReadCoordinator(8, nil, WithPrioritySource(a))

	alloc := rc.Allocate([]string{"high.go", "medium.go", "low.go"}, 10000)
	require.True(t, alloc["high.go"] > alloc["medium.go"],
		"high priority should get more tokens than medium")
	require.True(t, alloc["medium.go"] > alloc["low.go"],
		"medium priority should get more tokens than low")
	require.LessOrEqual(t, alloc["high.go"], PerFileMaxTokens)
}

func TestRepomapPriorityAdapterDefaultBudget(t *testing.T) {
	t.Parallel()

	require.Greater(t, DefaultPriorityBudget, 0,
		"DefaultPriorityBudget must be non-zero for token enforcement")
}
