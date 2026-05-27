package agent

import (
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestModelRouterDefaultLimit(t *testing.T) {
	t.Parallel()

	r := NewModelRouter()
	require.Equal(t, DefaultSmallModelTokenLimit, r.limit())
	require.Equal(t, 4000, r.limit())
}

func TestModelRouterRouteByTokenCount_atLimit(t *testing.T) {
	t.Parallel()

	r := NewModelRouter()
	result := r.RouteByTokenCount(DefaultSmallModelTokenLimit)
	require.Equal(t, config.RoleEditor, result)
}

func TestModelRouterRouteByTokenCount_belowLimit(t *testing.T) {
	t.Parallel()

	r := NewModelRouter()
	result := r.RouteByTokenCount(1)
	require.Equal(t, config.RoleEditor, result)
}

func TestModelRouterRouteByTokenCount_aboveLimit(t *testing.T) {
	t.Parallel()

	r := NewModelRouter()
	result := r.RouteByTokenCount(DefaultSmallModelTokenLimit + 1)
	require.Equal(t, config.RoleArchitect, result)
}

func TestModelRouterRouteByTokenCount_zeroTokens(t *testing.T) {
	t.Parallel()

	r := NewModelRouter()
	result := r.RouteByTokenCount(0)
	require.Equal(t, config.RoleEditor, result)
}

func TestModelRouterRouteByTokenCount_largeInput(t *testing.T) {
	t.Parallel()

	r := NewModelRouter()
	result := r.RouteByTokenCount(100000)
	require.Equal(t, config.RoleArchitect, result)
}

func TestModelRouterCustomThreshold(t *testing.T) {
	t.Parallel()

	r := NewModelRouterWithLimit(1000)
	require.Equal(t, 1000, r.limit())

	require.Equal(t, config.RoleEditor, r.RouteByTokenCount(1000))
	require.Equal(t, config.RoleArchitect, r.RouteByTokenCount(1001))
}

func TestModelRouterZeroThresholdFallsBackToDefault(t *testing.T) {
	t.Parallel()

	r := &ModelRouter{SmallModelTokenLimit: 0}
	require.Equal(t, DefaultSmallModelTokenLimit, r.limit())

	require.Equal(t, config.RoleEditor, r.RouteByTokenCount(DefaultSmallModelTokenLimit))
	require.Equal(t, config.RoleArchitect, r.RouteByTokenCount(DefaultSmallModelTokenLimit+1))
}

func TestModelRouterNegativeThresholdFallsBackToDefault(t *testing.T) {
	t.Parallel()

	r := &ModelRouter{SmallModelTokenLimit: -5}
	require.Equal(t, DefaultSmallModelTokenLimit, r.limit())
}

func TestModelRouterRouteByCharCount_belowThreshold(t *testing.T) {
	t.Parallel()

	r := NewModelRouterWithLimit(1000)

	// 3999 chars = 1000 tokens (ceiling division: (3999+3)/4 = 1000).
	result := r.RouteByCharCount(3999)
	require.Equal(t, config.RoleEditor, result)
}

func TestModelRouterRouteByCharCount_aboveThreshold(t *testing.T) {
	t.Parallel()

	r := NewModelRouterWithLimit(1000)

	// 4001 chars = 1001 tokens (ceiling division: (4001+3)/4 = 1001).
	result := r.RouteByCharCount(4001)
	require.Equal(t, config.RoleArchitect, result)
}

func TestModelRouterRouteByCharCount_exactBoundary(t *testing.T) {
	t.Parallel()

	// With default limit 4000 tokens = 16000 chars exactly.
	// 16000 chars = 4000 tokens (ceiling: (16000+3)/4 = 4000) -> editor.
	r := NewModelRouter()
	require.Equal(t, config.RoleEditor, r.RouteByCharCount(16000))

	// 16001 chars = 4001 tokens (ceiling: (16001+3)/4 = 4001) -> architect.
	require.Equal(t, config.RoleArchitect, r.RouteByCharCount(16001))
}

func TestModelRouterRouteByCharCount_zero(t *testing.T) {
	t.Parallel()

	r := NewModelRouter()
	require.Equal(t, config.RoleEditor, r.RouteByCharCount(0))
}

func TestModelRouterRouteByCharCount_consistentWithTokenRouting(t *testing.T) {
	t.Parallel()

	r := NewModelRouter()

	for _, charCount := range []int{0, 100, 1000, 15999, 16000, 16001, 50000} {
		expectedTokens := (charCount + charsPerToken - 1) / charsPerToken
		expected := r.RouteByTokenCount(expectedTokens)
		actual := r.RouteByCharCount(charCount)
		require.Equal(t, expected, actual, "charCount=%d, expectedTokens=%d", charCount, expectedTokens)
	}
}
