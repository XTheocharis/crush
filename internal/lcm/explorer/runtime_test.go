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

	adapter := NewRuntimeAdapter()
	require.NotNil(t, adapter)
	require.NotNil(t, adapter.registry)

	summary, explorerUsed, persist, err := adapter.Explore(
		context.Background(),
		"session-1",
		"main.go",
		[]byte("package main\n\nfunc main() {}\n"),
	)
	require.NoError(t, err)
	require.NotEmpty(t, summary)
	require.Equal(t, "go", explorerUsed)
	require.True(t, persist)
}

func TestNewRuntimeAdapter_WithParser_UsesTreeSitter(t *testing.T) {
	t.Parallel()

	adapter := NewRuntimeAdapter(WithRuntimeTreeSitter(&mockParser{}))
	require.NotNil(t, adapter)
	require.NotNil(t, adapter.registry)

	summary, explorerUsed, persist, err := adapter.Explore(
		context.Background(),
		"session-2",
		"main.go",
		[]byte("package main\n\nfunc main() {}\n"),
	)
	require.NoError(t, err)
	require.NotEmpty(t, summary)
	require.Equal(t, "treesitter", explorerUsed)
	require.True(t, persist)
}

func TestRuntimeAdapter_Explore_TrimmedOutputs(t *testing.T) {
	t.Parallel()

	adapter := &RuntimeAdapter{
		registry: NewRegistryWithLLM(&mockLLM{response: "\n  LLM summary  \n"}, nil),
	}

	summary, explorerUsed, persist, err := adapter.Explore(
		context.Background(),
		"session-3",
		"main.go",
		[]byte("package main\n\nfunc main() {}\n"),
	)
	require.NoError(t, err)
	require.Contains(t, summary, "LLM summary")
	require.Equal(t, "go+llm", explorerUsed)
	require.True(t, persist)
}

func TestNewRuntimeAdapter_WithParityProfile(t *testing.T) {
	t.Parallel()

	adapter := NewRuntimeAdapter(WithRuntimeOutputProfile(OutputProfileParity))
	require.NotNil(t, adapter)
	require.NotNil(t, adapter.registry)

	summary, explorerUsed, persist, err := adapter.Explore(
		context.Background(),
		"session-4",
		"main.go",
		[]byte("package main\n\nfunc main() {}\n"),
	)
	require.NoError(t, err)
	require.NotEmpty(t, summary)
	require.Contains(t, summary, "##")
	require.Equal(t, "go", explorerUsed)
	require.False(t, persist)
}

func TestRuntimeAdapter_Explore_NilAdapter(t *testing.T) {
	t.Parallel()

	var adapter *RuntimeAdapter
	_, _, _, err := adapter.Explore(context.Background(), "session", "main.go", []byte("package main"))
	require.ErrorIs(t, err, errNilRuntimeAdapter)
}

func TestRuntimeAdapter_Explore_TreeSitterErrorFallsBack(t *testing.T) {
	t.Parallel()

	adapter := NewRuntimeAdapter(WithRuntimeTreeSitter(&mockParser{
		analyzeFn: func(ctx context.Context, path string, content []byte) (*treesitter.FileAnalysis, error) {
			return nil, errors.New("analyze failed")
		},
	}))

	summary, explorerUsed, persist, err := adapter.Explore(context.Background(), "session", "main.go", []byte("package main"))
	require.NoError(t, err)
	require.NotEmpty(t, summary)
	require.Equal(t, "go", explorerUsed)
	require.True(t, persist)
}
