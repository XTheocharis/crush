package agent

import (
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/crush/internal/csync"
	"github.com/stretchr/testify/require"
)

func TestQueueGenerationStartsAtZero(t *testing.T) {
	t.Parallel()

	a := &sessionAgent{queueGenerationBySID: csync.NewMap[string, *atomic.Int64]()}

	require.EqualValues(t, 0, a.currentQueueGeneration("session-1"))
	require.EqualValues(t, 0, a.currentQueueGeneration("session-1"))
}

func TestQueueGenerationIncrementsForQueuedRecursiveRun(t *testing.T) {
	t.Parallel()

	a := &sessionAgent{queueGenerationBySID: csync.NewMap[string, *atomic.Int64]()}

	require.EqualValues(t, 0, a.currentQueueGeneration("session-1"))
	require.EqualValues(t, 1, a.incrementQueueGeneration("session-1"))
	require.EqualValues(t, 1, a.currentQueueGeneration("session-1"))
	require.EqualValues(t, 2, a.incrementQueueGeneration("session-1"))
	require.EqualValues(t, 2, a.currentQueueGeneration("session-1"))
}
