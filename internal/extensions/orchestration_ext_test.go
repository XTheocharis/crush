package extensions

import (
	"context"
	"sync"
	"testing"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/stretchr/testify/require"
)

type mockAgentHandle struct {
	name    string
	running bool
	stopped bool
	closed  bool
}

func (m *mockAgentHandle) Name() string    { return m.name }
func (m *mockAgentHandle) IsRunning() bool { return m.running }
func (m *mockAgentHandle) Stop()           { m.stopped = true }
func (m *mockAgentHandle) Close()          { m.closed = true }

type mockRegistry struct {
	agents map[string]*mockAgentHandle
}

func (m *mockRegistry) Get(name string) (tools.AgentHandle, bool) {
	h, ok := m.agents[name]
	if !ok {
		return nil, false
	}
	return h, true
}

func (m *mockRegistry) HasAgent(name string) bool {
	_, ok := m.agents[name]
	return ok
}

func (m *mockRegistry) List() []string {
	names := make([]string, 0, len(m.agents))
	for n := range m.agents {
		names = append(names, n)
	}
	return names
}

type mockMailbox struct {
	messages []tools.MailboxMessage
}

func (m *mockMailbox) Send(msg tools.MailboxMessage) error {
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockMailbox) HasInbox(_ string) bool { return true }

func newActiveExtension(t *testing.T) *OrchestrationExtension {
	t.Helper()
	e := &OrchestrationExtension{}
	err := e.Init(context.Background(), nil)
	require.NoError(t, err)
	return e
}

func TestOrchestrationRebuildTools(t *testing.T) {
	t.Parallel()

	e := newActiveExtension(t)

	toolsBefore, err := e.Tools(context.Background())
	require.NoError(t, err)
	require.Len(t, toolsBefore, 4, "Init should create 4 tools even with nil deps")

	reg := &mockRegistry{agents: map[string]*mockAgentHandle{
		"agent-1": {name: "agent-1", running: true},
	}}
	mb := &mockMailbox{}

	e.SetRegistry(reg)
	e.SetMailbox(mb)
	e.RebuildTools()

	got, err := e.Tools(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 4, "RebuildTools should produce 4 tools")

	names := e.ToolNames()
	require.Equal(t, []string{"send_message", "team_create", "team_delete", "task_stop"}, names)
}

func TestOrchestrationRebuildNilSafe(t *testing.T) {
	t.Parallel()

	e := newActiveExtension(t)

	require.NotPanics(t, func() {
		e.RebuildTools()
	}, "RebuildTools with nil deps should not panic")

	got, err := e.Tools(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 4, "should still produce 4 tool shells")
}

func TestOrchestrationToolNames(t *testing.T) {
	t.Parallel()

	e := newActiveExtension(t)
	e.SetRegistry(&mockRegistry{})
	e.SetMailbox(&mockMailbox{})
	e.RebuildTools()

	names := e.ToolNames()
	expected := []string{"send_message", "team_create", "team_delete", "task_stop"}
	require.Equal(t, expected, names)

	for _, name := range names {
		require.NotEmpty(t, name)
	}
}

func TestOrchestrationToolNamesInactive(t *testing.T) {
	t.Parallel()

	e := &OrchestrationExtension{}
	_ = e.Init(context.Background(), nil)
	_ = e.Shutdown(context.Background())

	names := e.ToolNames()
	require.Nil(t, names)
}

func TestOrchestrationRebuildToolsConcurrent(t *testing.T) {
	t.Parallel()

	e := newActiveExtension(t)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%3 == 0 {
				e.RebuildTools()
			} else if i%3 == 1 {
				_, _ = e.Tools(context.Background())
			} else {
				_ = e.ToolNames()
			}
		}(i)
	}
	wg.Wait()

	got, err := e.Tools(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 4)
}

func TestOrchestrationRebuildClearsOldTools(t *testing.T) {
	t.Parallel()

	e := newActiveExtension(t)

	before, _ := e.Tools(context.Background())
	require.Len(t, before, 4)

	e.RebuildTools()

	after, _ := e.Tools(context.Background())
	require.Len(t, after, 4)

	for i := range before {
		require.NotEqual(t, fantasy.AgentTool(nil), after[i])
	}
}
