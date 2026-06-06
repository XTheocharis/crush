package tools

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

type mockTeamManager struct {
	mu    sync.RWMutex
	teams map[string][]string
}

func newMockTeamManager() *mockTeamManager {
	return &mockTeamManager{teams: make(map[string][]string)}
}

func (m *mockTeamManager) CreateTeam(name string, agentIDs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, len(agentIDs))
	copy(ids, agentIDs)
	m.teams[name] = ids
}

func (m *mockTeamManager) DeleteTeam(name string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids, ok := m.teams[name]
	if !ok {
		return nil
	}
	delete(m.teams, name)
	return ids
}

func (m *mockTeamManager) GetTeamAgents(name string) ([]string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids, ok := m.teams[name]
	if !ok {
		return nil, false
	}
	out := make([]string, len(ids))
	copy(out, ids)
	return out, true
}

type mockHandleForTeam struct {
	name    string
	stopped bool
	closed  bool
}

func (m *mockHandleForTeam) Name() string    { return m.name }
func (m *mockHandleForTeam) IsRunning() bool { return true }
func (m *mockHandleForTeam) Stop()           { m.stopped = true }
func (m *mockHandleForTeam) Close()          { m.closed = true }

type mockRegistryForTeam struct {
	mu     sync.RWMutex
	agents map[string]*mockHandleForTeam
}

func newMockRegistryForTeam() *mockRegistryForTeam {
	return &mockRegistryForTeam{agents: make(map[string]*mockHandleForTeam)}
}

func (r *mockRegistryForTeam) Get(name string) (AgentHandle, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.agents[name]
	return h, ok
}

func (r *mockRegistryForTeam) HasAgent(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.agents[name]
	return ok
}

func (r *mockRegistryForTeam) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.agents))
	for n := range r.agents {
		names = append(names, n)
	}
	return names
}

func (r *mockRegistryForTeam) addAgent(name string) *mockHandleForTeam {
	h := &mockHandleForTeam{name: name}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[name] = h
	return h
}

func TestTeamCreateDelete(t *testing.T) {
	t.Parallel()

	reg := newMockRegistryForTeam()
	h1 := reg.addAgent("agent-1")
	h2 := reg.addAgent("agent-2")
	tm := newMockTeamManager()

	createTool := NewTeamCreateTool(reg, nil, tm)
	deleteTool := NewTeamDeleteTool(reg, tm)

	input, err := json.Marshal(TeamCreateParams{
		TeamName: "alpha",
		Agents:   []string{"agent-1", "agent-2"},
	})
	require.NoError(t, err)

	resp, err := createTool.Run(context.Background(), fantasy.ToolCall{
		ID:    "tc1",
		Name:  TeamCreateToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "created with 2 agent(s)")

	agents, ok := tm.GetTeamAgents("alpha")
	require.True(t, ok)
	require.Equal(t, []string{"agent-1", "agent-2"}, agents)

	delInput, err := json.Marshal(TeamDeleteParams{TeamName: "alpha"})
	require.NoError(t, err)

	resp, err = deleteTool.Run(context.Background(), fantasy.ToolCall{
		ID:    "td1",
		Name:  TeamDeleteToolName,
		Input: string(delInput),
	})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "deleted")

	require.True(t, h1.stopped, "agent-1 should be stopped")
	require.True(t, h1.closed, "agent-1 should be closed")
	require.True(t, h2.stopped, "agent-2 should be stopped")
	require.True(t, h2.closed, "agent-2 should be closed")

	_, ok = tm.GetTeamAgents("alpha")
	require.False(t, ok, "team should be removed after deletion")
}

func TestTeamDeleteIdempotent(t *testing.T) {
	t.Parallel()

	reg := newMockRegistryForTeam()
	tm := newMockTeamManager()

	deleteTool := NewTeamDeleteTool(reg, tm)

	delInput, err := json.Marshal(TeamDeleteParams{TeamName: "nonexistent"})
	require.NoError(t, err)

	resp, err := deleteTool.Run(context.Background(), fantasy.ToolCall{
		ID:    "td1",
		Name:  TeamDeleteToolName,
		Input: string(delInput),
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "not found")
}
