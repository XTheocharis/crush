package extensions

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/stretchr/testify/require"
)

type productiveTestFactory struct {
	responses []agent.StructuredResponse
	callCount atomic.Int32
}

func (f *productiveTestFactory) NewStructuredSubagent(_ context.Context, _ string) (agent.StructuredSubagent, error) {
	return &productiveTestSubagent{factory: f}, nil
}

type productiveTestSubagent struct {
	factory *productiveTestFactory
}

func (s *productiveTestSubagent) Execute(_ context.Context, req agent.StructuredRequest) (agent.StructuredResponse, error) {
	idx := int(s.factory.callCount.Add(1)) - 1
	if idx < len(s.factory.responses) {
		return s.factory.responses[idx], nil
	}
	return agent.StructuredResponse{Success: true, Result: "default"}, nil
}

func (s *productiveTestSubagent) Capabilities() []string { return nil }

func newActiveProductiveExtension(t *testing.T) *ProductiveExtension {
	t.Helper()
	e := &ProductiveExtension{}
	err := e.Init(context.Background(), nil)
	require.NoError(t, err)
	return e
}

func TestProductiveToolRegistered(t *testing.T) {
	t.Parallel()

	e := newActiveProductiveExtension(t)

	factory := &productiveTestFactory{
		responses: []agent.StructuredResponse{
			{Success: true, Result: "done"},
		},
	}
	e.SetFactory(factory)
	e.RebuildTools()

	got, err := e.Tools(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1, "should have 1 tool after RebuildTools with factory")

	names := e.ToolNames()
	require.Equal(t, []string{"productive_execute"}, names)
}

func TestProductiveToolRegisteredBeforeFactory(t *testing.T) {
	t.Parallel()

	e := newActiveProductiveExtension(t)

	got, err := e.Tools(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 0, "should have 0 tools before factory is set")
}

func TestProductiveToolExecutes(t *testing.T) {
	t.Parallel()

	e := newActiveProductiveExtension(t)

	factory := &productiveTestFactory{
		responses: []agent.StructuredResponse{
			{Success: true, Result: "iteration-1-done"},
		},
	}
	e.SetFactory(factory)
	e.RebuildTools()

	got, err := e.Tools(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)

	tool := got[0]
	require.Equal(t, "productive_execute", tool.Info().Name)
}

func TestProductiveToolExecutesExpectedIterations(t *testing.T) {
	t.Parallel()

	e := newActiveProductiveExtension(t)

	factory := &productiveTestFactory{
		responses: []agent.StructuredResponse{
			{Success: false, Result: "partial-1"},
			{Success: false, Result: "partial-2"},
			{Success: true, Result: "final-result"},
		},
	}
	e.SetFactory(factory)
	e.RebuildTools()

	got, err := e.Tools(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)

	tool := got[0]
	require.Equal(t, "productive_execute", tool.Info().Name)
}

func TestProductiveToolStallDetection(t *testing.T) {
	t.Parallel()

	e := newActiveProductiveExtension(t)

	factory := &productiveTestFactory{
		responses: []agent.StructuredResponse{
			{Success: false, Result: "same-output"},
			{Success: false, Result: "same-output"},
			{Success: false, Result: "same-output"},
		},
	}
	e.SetFactory(factory)
	e.RebuildTools()

	got, err := e.Tools(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)

	tool := got[0]
	require.Equal(t, "productive_execute", tool.Info().Name)
}

func TestProductiveShutdownClearsTools(t *testing.T) {
	t.Parallel()

	e := newActiveProductiveExtension(t)

	factory := &productiveTestFactory{
		responses: []agent.StructuredResponse{{Success: true, Result: "ok"}},
	}
	e.SetFactory(factory)
	e.RebuildTools()

	got, _ := e.Tools(context.Background())
	require.Len(t, got, 1)

	err := e.Shutdown(context.Background())
	require.NoError(t, err)

	got, err = e.Tools(context.Background())
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestProductiveToolNamesInactive(t *testing.T) {
	t.Parallel()

	e := &ProductiveExtension{}
	_ = e.Init(context.Background(), nil)
	_ = e.Shutdown(context.Background())

	names := e.ToolNames()
	require.Nil(t, names)
}

func TestProductiveRebuildToolsConcurrent(t *testing.T) {
	t.Parallel()

	e := newActiveProductiveExtension(t)
	factory := &productiveTestFactory{
		responses: []agent.StructuredResponse{{Success: true, Result: "ok"}},
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 3 {
			case 0:
				e.SetFactory(factory)
				e.RebuildTools()
			case 1:
				_, _ = e.Tools(context.Background())
			default:
				_ = e.ToolNames()
			}
		}(i)
	}
	wg.Wait()
}

type productiveCreateErrorFactory struct{}

func (f *productiveCreateErrorFactory) NewStructuredSubagent(_ context.Context, _ string) (agent.StructuredSubagent, error) {
	return nil, fmt.Errorf("factory unavailable")
}

func TestProductiveToolCreateSubagentError(t *testing.T) {
	t.Parallel()

	e := newActiveProductiveExtension(t)
	e.SetFactory(&productiveCreateErrorFactory{})
	e.RebuildTools()

	got, err := e.Tools(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "productive_execute", got[0].Info().Name)
}
