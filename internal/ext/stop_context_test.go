package ext

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStopContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	require.False(t, StoppedByCondition(ctx))

	ctx = WithStopCondition(ctx, true)
	require.True(t, StoppedByCondition(ctx))

	ctx = WithStopCondition(ctx, false)
	require.False(t, StoppedByCondition(ctx))
}
