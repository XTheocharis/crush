package repomap

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepomapExceedProfileAssertions_Exceed(t *testing.T) {
	t.Parallel()

	mode := strings.ToLower(strings.TrimSpace(os.Getenv("PARITY_MODE")))
	if mode != "" {
		require.Equal(t, "false", mode, "Exceed suite must run with PARITY_MODE=false when explicitly set")
	}

	fixtures := loadVerticalSliceFixtures(t)
	require.NotEmpty(t, fixtures)

	for _, fx := range fixtures {
		for _, profile := range fx.Profiles {
			if profile.ParityMode {
				continue
			}
			result := runVerticalSliceHarness(fx, profile)
			require.LessOrEqual(t, result.SafetyTokens, profile.TokenBudget,
				"fixture=%s profile=%s exceed safety budget violated", fx.Name, profile.Name)
			require.NotEmpty(t, result.RawHash,
				"fixture=%s profile=%s exceed raw hash must be populated", fx.Name, profile.Name)
		}
	}
}
