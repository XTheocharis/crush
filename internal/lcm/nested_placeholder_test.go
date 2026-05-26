package lcm

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveNestedPlaceholders_NoPlaceholders(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	resolved, err := ResolveNestedPlaceholders(ctx, "plain text without placeholders", stubResolver(nil))
	require.NoError(t, err)
	require.Equal(t, "plain text without placeholders", resolved)
}

func TestResolveNestedPlaceholders_SinglePlaceholder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	blocks := map[string]string{
		"b1": "resolved block 1 content",
	}
	resolved, err := ResolveNestedPlaceholders(ctx, "before (b1) after", stubResolver(blocks))
	require.NoError(t, err)
	require.Equal(t, "before resolved block 1 content after", resolved)
}

func TestResolveNestedPlaceholders_MultiplePlaceholders(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	blocks := map[string]string{
		"b1":  "content A",
		"b22": "content B",
	}
	resolved, err := ResolveNestedPlaceholders(ctx, "start (b1) middle (b22) end", stubResolver(blocks))
	require.NoError(t, err)
	require.Equal(t, "start content A middle content B end", resolved)
}

func TestResolveNestedPlaceholders_NestedResolution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	blocks := map[string]string{
		"b1": "outer (b2) tail",
		"b2": "inner content",
	}
	resolved, err := ResolveNestedPlaceholders(ctx, "prefix (b1) suffix", stubResolver(blocks))
	require.NoError(t, err)
	require.Equal(t, "prefix outer inner content tail suffix", resolved)
}

func TestResolveNestedPlaceholders_DeepNesting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	blocks := map[string]string{
		"b1": "L1 (b2)",
		"b2": "L2 (b3)",
		"b3": "L3 (b4)",
		"b4": "L4 deepest",
	}
	resolved, err := ResolveNestedPlaceholders(ctx, "start (b1) end", stubResolver(blocks))
	require.NoError(t, err)
	require.Equal(t, "start L1 L2 L3 L4 deepest end", resolved)
}

func TestResolveNestedPlaceholders_MaxDepthExceeded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	blocks := map[string]string{
		"b1": "L1 (b2)",
		"b2": "L2 (b3)",
		"b3": "L3 (b4)",
		"b4": "L4 (b5)",
		"b5": "L5 (b6)",
		"b6": "L6 should not resolve",
	}
	resolved, err := ResolveNestedPlaceholders(ctx, "start (b1) end", stubResolver(blocks))
	require.NoError(t, err)
	require.Contains(t, resolved, "L5")
	require.Contains(t, resolved, "(b6)")
	require.NotContains(t, resolved, "should not resolve")
}

func TestResolveNestedPlaceholders_CycleDetection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	blocks := map[string]string{
		"b1": "A references (b2)",
		"b2": "B references (b1)",
	}
	resolved, err := ResolveNestedPlaceholders(ctx, "start (b1) end", stubResolver(blocks))
	require.NoError(t, err)
	require.Contains(t, resolved, "A references")
	require.Contains(t, resolved, "B references")
}

func TestResolveNestedPlaceholders_SelfReferencing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	blocks := map[string]string{
		"b1": "self (b1) loop",
	}
	resolved, err := ResolveNestedPlaceholders(ctx, "(b1)", stubResolver(blocks))
	require.NoError(t, err)
	require.Contains(t, resolved, "self")
	require.Contains(t, resolved, "(b1)")
}

func TestResolveNestedPlaceholders_MissingBlock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	resolved, err := ResolveNestedPlaceholders(ctx, "before (b99) after", stubResolver(nil))
	require.NoError(t, err)
	require.Equal(t, "before (b99) after", resolved)
}

func TestResolveNestedPlaceholders_ResolverError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	resolver := func(_ context.Context, blockID string) (string, bool, error) {
		return "", false, fmt.Errorf("db error for %s", blockID)
	}
	_, err := ResolveNestedPlaceholders(ctx, "(b1)", resolver)
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolving block b1")
}

func TestResolveNestedPlaceholders_EmptyContent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	resolved, err := ResolveNestedPlaceholders(ctx, "", stubResolver(nil))
	require.NoError(t, err)
	require.Equal(t, "", resolved)
}

func TestBlockPlaceholderPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		match bool
		nums  []string
	}{
		{"(b1)", true, []string{"1"}},
		{"(b42)", true, []string{"42"}},
		{"(b123)", true, []string{"123"}},
		{"text (b1) more (b2)", true, []string{"1", "2"}},
		{"(b0)", true, []string{"0"}},
		{"(B1)", false, nil},
		{"b1", false, nil},
		{"(b)", false, nil},
		{"(bx)", false, nil},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			matches := blockPlaceholderPattern.FindAllStringSubmatch(tc.input, -1)
			if !tc.match {
				require.Empty(t, matches)
				return
			}
			var nums []string
			for _, m := range matches {
				nums = append(nums, m[1])
			}
			require.Equal(t, tc.nums, nums)
		})
	}
}

// stubResolver returns a BlockResolver backed by a map.
func stubResolver(blocks map[string]string) BlockResolver {
	return func(_ context.Context, blockID string) (string, bool, error) {
		if blocks == nil {
			return "", false, nil
		}
		content, ok := blocks[blockID]
		return content, ok, nil
	}
}
