package lcm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLargeOutputThreshold50K(t *testing.T) {
	t.Parallel()

	require.Equal(t, 50000, LargeOutputThreshold, "LargeOutputThreshold should be 50000 tokens")
}
