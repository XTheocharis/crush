package agent

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type xrushMockSubagentFactory struct {
	responses map[string]StructuredResponse
	callCount atomic.Int32
}

func (f *xrushMockSubagentFactory) NewStructuredSubagent(_ context.Context, _ string) (StructuredSubagent, error) {
	return &xrushMockSubagent{factory: f}, nil
}

type xrushMockSubagent struct {
	factory *xrushMockSubagentFactory
}

func (s *xrushMockSubagent) Execute(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
	s.factory.callCount.Add(1)
	if resp, ok := s.factory.responses[req.Task]; ok {
		return resp, nil
	}
	return StructuredResponse{
		Result:     "result:" + req.Task,
		Success:    true,
		StepsTaken: 1,
	}, nil
}

func (s *xrushMockSubagent) Capabilities() []string {
	return []string{"view", "edit", "bash"}
}

func xrushNewTestSwarm(factory StructuredSubagentFactory) *SwarmPattern {
	cache := NewSharedCache()
	par := NewParallelController(ParallelControllerConfig{MaxConcurrent: 5})
	return NewSwarmPattern(SwarmConfig{
		DecomposeFn:  DefaultDecompose,
		SynthesizeFn: DefaultSynthesize,
		MaxSubagents: 5,
		CacheTTL:     time.Minute,
	}, cache, par, factory)
}

func TestXrushSwarmBasicExecution(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	swarm := xrushNewTestSwarm(factory)

	resp, err := swarm.Execute(context.Background(), "session-1", "task-a\ntask-b")
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Equal(t, 2, resp.StepsTaken)
	require.Contains(t, resp.Result, "result:task-a")
	require.Contains(t, resp.Result, "result:task-b")
}

func TestXrushSwarmSingleTask(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	swarm := xrushNewTestSwarm(factory)

	resp, err := swarm.Execute(context.Background(), "session-1", "single-task")
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Contains(t, resp.Result, "result:single-task")
}

func TestXrushSwarmEmptyTask(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	swarm := xrushNewTestSwarm(factory)

	resp, err := swarm.Execute(context.Background(), "session-1", "")
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Contains(t, resp.Error, "empty task")
}

func TestXrushSwarmNoDecomposition(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	swarm := xrushNewTestSwarm(factory)

	swarm.cfg.DecomposeFn = func(_ context.Context, _ string) ([]string, error) {
		return nil, nil
	}

	resp, err := swarm.Execute(context.Background(), "session-1", "some task")
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Contains(t, resp.Error, "no subtasks")
}

func TestXrushSwarmMaxSubagents(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	swarm := xrushNewTestSwarm(factory)
	swarm.cfg.MaxSubagents = 2

	task := "a\nb\nc\nd"
	resp, err := swarm.Execute(context.Background(), "session-1", task)
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.LessOrEqual(t, resp.StepsTaken, 2)
}

func TestXrushSwarmCaching(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	swarm := xrushNewTestSwarm(factory)

	resp1, err := swarm.Execute(context.Background(), "session-1", "task-a\ntask-b")
	require.NoError(t, err)
	require.True(t, resp1.Success)

	callsAfterFirst := factory.callCount.Load()

	resp2, err := swarm.Execute(context.Background(), "session-1", "task-a\ntask-b")
	require.NoError(t, err)
	require.True(t, resp2.Success)

	callsAfterSecond := factory.callCount.Load()
	require.Equal(t, callsAfterFirst, callsAfterSecond, "cache should prevent re-execution")
}

func TestXrushSwarmDecomposeError(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	swarm := xrushNewTestSwarm(factory)
	swarm.cfg.DecomposeFn = func(_ context.Context, _ string) ([]string, error) {
		return nil, fmt.Errorf("decompose failed")
	}

	_, err := swarm.Execute(context.Background(), "session-1", "some task")
	require.Error(t, err)
	require.Contains(t, err.Error(), "decompose failed")
}

func TestXrushSwarmNilFactory(t *testing.T) {
	t.Parallel()
	swarm := xrushNewTestSwarm(nil)

	_, err := swarm.Execute(context.Background(), "session-1", "task")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no structured subagent factory")
}

func TestXrushSwarmSynthesizeError(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	swarm := xrushNewTestSwarm(factory)
	swarm.cfg.SynthesizeFn = func(_ []StructuredResponse) (string, error) {
		return "", fmt.Errorf("synthesize failed")
	}

	_, err := swarm.Execute(context.Background(), "session-1", "task-a\ntask-b")
	require.Error(t, err)
	require.Contains(t, err.Error(), "synthesize failed")
}

func TestXrushSwarmPartialSuccess(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{
		responses: map[string]StructuredResponse{
			"fail": {Success: false, Error: "subtask failed"},
		},
	}
	swarm := xrushNewTestSwarm(factory)

	resp, err := swarm.Execute(context.Background(), "session-1", "fail\nsuccess-task")
	require.NoError(t, err)
	require.True(t, resp.Success, "swarm should succeed when at least one subtask succeeds")
	require.Contains(t, resp.Result, "result:success-task")
}

func xrushNewTestTeammate(factory StructuredSubagentFactory, role TeammateRole) *TeammatePattern {
	cache := NewSharedCache()
	return NewTeammatePattern(TeammateConfig{
		Role:     role,
		CacheTTL: time.Minute,
	}, cache, factory)
}

func TestXrushTeammateBasicExecution(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	teammate := xrushNewTestTeammate(factory, RoleResearcher)

	resp, err := teammate.Execute(context.Background(), "session-1", "investigate the auth module")
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Contains(t, resp.Result, "result:")
}

func TestXrushTeammateEmptyTask(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	teammate := xrushNewTestTeammate(factory, RoleResearcher)

	resp, err := teammate.Execute(context.Background(), "session-1", "")
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Contains(t, resp.Error, "empty task")
}

func TestXrushTeammateNilFactory(t *testing.T) {
	t.Parallel()
	teammate := xrushNewTestTeammate(nil, RoleResearcher)

	_, err := teammate.Execute(context.Background(), "session-1", "task")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no structured subagent factory")
}

func TestXrushTeammateCaching(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	teammate := xrushNewTestTeammate(factory, RoleTester)

	resp1, err := teammate.Execute(context.Background(), "session-1", "test the parser")
	require.NoError(t, err)
	require.True(t, resp1.Success)

	callsAfterFirst := factory.callCount.Load()

	resp2, err := teammate.Execute(context.Background(), "session-1", "test the parser")
	require.NoError(t, err)
	require.True(t, resp2.Success)

	callsAfterSecond := factory.callCount.Load()
	require.Equal(t, callsAfterFirst, callsAfterSecond, "cache should prevent re-execution")
}

func TestXrushTeammateInteractionCount(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	teammate := xrushNewTestTeammate(factory, RoleReviewer)

	require.Equal(t, 0, teammate.InteractionCount())

	_, err := teammate.Execute(context.Background(), "session-1", "review code")
	require.NoError(t, err)
	require.Equal(t, 1, teammate.InteractionCount())

	_, err = teammate.Execute(context.Background(), "session-1", "review code v2")
	require.NoError(t, err)
	require.Equal(t, 2, teammate.InteractionCount())
}

func TestXrushTeammateRole(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}

	roles := []TeammateRole{RoleResearcher, RoleTester, RoleReviewer}
	for _, role := range roles {
		teammate := xrushNewTestTeammate(factory, role)
		require.Equal(t, role, teammate.Role())
	}
}

func TestXrushTeammateSerialization(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	teammate := xrushNewTestTeammate(factory, RoleResearcher)

	for i := range 3 {
		_, err := teammate.Execute(context.Background(), "session-1", fmt.Sprintf("task-%d", i))
		require.NoError(t, err)
	}

	require.Equal(t, 3, teammate.InteractionCount())
	require.Equal(t, int32(3), factory.callCount.Load())
}

func TestXrushTeammateConcurrentSerialization(t *testing.T) {
	t.Parallel()

	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	factory := &xrushSlowFactory{
		concurrent:    &concurrent,
		maxConcurrent: &maxConcurrent,
		duration:      20 * time.Millisecond,
	}
	teammate := xrushNewTestTeammate(factory, RoleResearcher)

	done := make(chan struct{}, 3)
	for i := range 3 {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			_, _ = teammate.Execute(context.Background(), "session-1", fmt.Sprintf("task-%d", n))
		}(i)
	}

	for range 3 {
		<-done
	}

	require.Equal(t, int32(1), maxConcurrent.Load(), "teammate interactions should be serialized")
}

type xrushSlowFactory struct {
	concurrent    *atomic.Int32
	maxConcurrent *atomic.Int32
	duration      time.Duration
}

func (f *xrushSlowFactory) NewStructuredSubagent(_ context.Context, _ string) (StructuredSubagent, error) {
	return &xrushSlowSubagent{factory: f}, nil
}

type xrushSlowSubagent struct {
	factory *xrushSlowFactory
}

func (s *xrushSlowSubagent) Execute(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
	cur := s.factory.concurrent.Add(1)
	for {
		old := s.factory.maxConcurrent.Load()
		if cur <= old || s.factory.maxConcurrent.CompareAndSwap(old, cur) {
			break
		}
	}
	time.Sleep(s.factory.duration)
	s.factory.concurrent.Add(-1)
	return StructuredResponse{Result: "result:" + req.Task, Success: true}, nil
}

func (s *xrushSlowSubagent) Capabilities() []string {
	return []string{"view", "edit", "bash"}
}

func TestXrushDefaultDecompose(t *testing.T) {
	t.Parallel()

	subtasks, err := DefaultDecompose(context.Background(), "a\nb\nc")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, subtasks)

	subtasks, err = DefaultDecompose(context.Background(), "a\n\n  b  \n\nc")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, subtasks)

	subtasks, err = DefaultDecompose(context.Background(), "")
	require.NoError(t, err)
	require.Nil(t, subtasks)
}

func TestXrushDefaultSynthesize(t *testing.T) {
	t.Parallel()

	results := []StructuredResponse{
		{Result: "first", Success: true},
		{Result: "second", Success: true},
		{Result: "failed", Success: false},
	}
	synthesized, err := DefaultSynthesize(results)
	require.NoError(t, err)
	require.Contains(t, synthesized, "first")
	require.Contains(t, synthesized, "second")
	require.NotContains(t, synthesized, "failed")

	synthesized, err = DefaultSynthesize([]StructuredResponse{
		{Success: false, Error: "all failed"},
	})
	require.NoError(t, err)
	require.Equal(t, "no successful results", synthesized)
}

func TestXrushTeammateConfigDefaultPrompts(t *testing.T) {
	t.Parallel()
	require.Contains(t, TeammateConfig{Role: RoleResearcher}.systemPrompt(), "research")
	require.Contains(t, TeammateConfig{Role: RoleTester}.systemPrompt(), "test")
	require.Contains(t, TeammateConfig{Role: RoleReviewer}.systemPrompt(), "review")
	require.Contains(t, TeammateConfig{Role: "custom"}.systemPrompt(), "custom")
}

func TestXrushTeammateConfigCustomPrompt(t *testing.T) {
	t.Parallel()
	cfg := TeammateConfig{Role: RoleResearcher, SystemPrompt: "custom instructions"}
	require.Equal(t, "custom instructions", cfg.systemPrompt())
}

func TestXrushSwarmConfigDefaults(t *testing.T) {
	t.Parallel()
	cfg := SwarmConfig{}
	require.Equal(t, 5, cfg.maxSubagents())
	require.Equal(t, DefaultRepoMapTTL, cfg.cacheTTL())
}

func TestXrushTeammateConfigDefaults(t *testing.T) {
	t.Parallel()
	cfg := TeammateConfig{Role: RoleResearcher}
	require.Equal(t, DefaultDiagnosticsTTL, cfg.cacheTTL())
}

func TestXrushSwarmParallelExecution(t *testing.T) {
	t.Parallel()
	factory := &xrushMockSubagentFactory{}
	swarm := xrushNewTestSwarm(factory)

	task := strings.Repeat("subtask\n", 5)
	resp, err := swarm.Execute(context.Background(), "session-1", task)
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Equal(t, int32(5), factory.callCount.Load())
}
