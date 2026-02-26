package explorer

import (
	"context"
	"errors"
	"testing"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

func TestNewRuntimeAdapter_WithoutParser_UsesDefaultRegistry(t *testing.T) {
	t.Parallel()

	adapter := NewRuntimeAdapter(nil)
	require.NotNil(t, adapter)
	require.NotNil(t, adapter.registry)

	summary, explorerUsed, err := adapter.Explore(
		context.Background(),
		"session-1",
		"main.go",
		[]byte("package main\n\nfunc main() {}\n"),
	)
	require.NoError(t, err)
	require.NotEmpty(t, summary)
	require.Equal(t, "go", explorerUsed)
}

func TestNewRuntimeAdapter_WithParser_UsesTreeSitter(t *testing.T) {
	t.Parallel()

	adapter := NewRuntimeAdapter(&mockParser{})
	require.NotNil(t, adapter)
	require.NotNil(t, adapter.registry)

	summary, explorerUsed, err := adapter.Explore(
		context.Background(),
		"session-2",
		"main.go",
		[]byte("package main\n\nfunc main() {}\n"),
	)
	require.NoError(t, err)
	require.NotEmpty(t, summary)
	require.Equal(t, "treesitter", explorerUsed)
}

func TestRuntimeAdapter_Explore_TrimmedOutputs(t *testing.T) {
	t.Parallel()

	adapter := &RuntimeAdapter{
		registry: NewRegistryWithLLM(&mockLLM{response: "\n  LLM summary  \n"}, nil),
	}

	summary, explorerUsed, err := adapter.Explore(
		context.Background(),
		"session-3",
		"main.go",
		[]byte("package main\n\nfunc main() {}\n"),
	)
	require.NoError(t, err)
	require.Contains(t, summary, "LLM summary")
	require.Equal(t, "go+llm", explorerUsed)
}

func TestRuntimeAdapter_Explore_NilAdapter(t *testing.T) {
	t.Parallel()

	var adapter *RuntimeAdapter
	_, _, err := adapter.Explore(context.Background(), "session", "main.go", []byte("package main"))
	require.ErrorIs(t, err, errNilRuntimeAdapter)
}

func TestRuntimeAdapter_Explore_TreeSitterErrorFallsBack(t *testing.T) {
	t.Parallel()

	adapter := NewRuntimeAdapter(&mockParser{
		analyzeFn: func(ctx context.Context, path string, content []byte) (*treesitter.FileAnalysis, error) {
			return nil, errors.New("analyze failed")
		},
	})

	summary, explorerUsed, err := adapter.Explore(context.Background(), "session", "main.go", []byte("package main"))
	require.NoError(t, err)
	require.NotEmpty(t, summary)
	require.Equal(t, "go", explorerUsed)
}
