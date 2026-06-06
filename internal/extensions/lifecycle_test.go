package extensions

import (
	"context"
	"sync"
	"testing"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/stretchr/testify/require"
)

type trackingExtension struct {
	name           string
	initCalled     bool
	shutdownCalled bool
	mu             sync.Mutex
}

func (e *trackingExtension) Name() string { return e.name }

func (e *trackingExtension) Init(_ context.Context, _ ext.HostContext) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.initCalled = true
	return nil
}

func (e *trackingExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.shutdownCalled = true
	return nil
}

func (e *trackingExtension) wasInitCalled() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.initCalled
}

func (e *trackingExtension) wasShutdownCalled() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.shutdownCalled
}

type shutdownOrderExtension struct {
	name  string
	order *[]string
}

func (e *shutdownOrderExtension) Name() string { return e.name }

func (e *shutdownOrderExtension) Init(_ context.Context, _ ext.HostContext) error {
	return nil
}

func (e *shutdownOrderExtension) Shutdown(_ context.Context) error {
	*e.order = append(*e.order, e.name)
	return nil
}

type lifecycleToolInput struct {
	Value string `json:"value,omitempty"`
}

type toolProvidingExtension struct {
	name     string
	toolName string
	active   bool
	mu       sync.RWMutex
}

func (e *toolProvidingExtension) Name() string { return e.name }

func (e *toolProvidingExtension) Init(_ context.Context, _ ext.HostContext) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.active = true
	return nil
}

func (e *toolProvidingExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.active = false
	return nil
}

func (e *toolProvidingExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil, nil
	}
	tool := fantasy.NewAgentTool(
		e.toolName,
		"Mock tool "+e.toolName,
		func(_ context.Context, _ lifecycleToolInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.ToolResponse{}, nil
		},
	)
	return []fantasy.AgentTool{tool}, nil
}

func (e *toolProvidingExtension) ToolNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil
	}
	return []string{e.toolName}
}

func TestLifecycleInitShutdownAll(t *testing.T) {
	t.Parallel()

	ext.ResetForTesting()

	host := ext.NewExtensionHost(ext.HostDeps{})

	exts := []*trackingExtension{
		{name: "lifecycle-ext-1"},
		{name: "lifecycle-ext-2"},
		{name: "lifecycle-ext-3"},
	}

	for _, e := range exts {
		require.NoError(t, host.Register(e))
	}

	ctx := context.Background()
	require.NoError(t, host.Bootstrap(ctx))

	for i, e := range exts {
		require.True(t, e.wasInitCalled(), "extension %d (%s) should have Init called", i, e.name)
		require.False(t, e.wasShutdownCalled(), "extension %d (%s) should not be shut down yet", i, e.name)
	}

	require.NoError(t, host.Shutdown(ctx))

	for i, e := range exts {
		require.True(t, e.wasShutdownCalled(), "extension %d (%s) should have Shutdown called", i, e.name)
	}
}

func TestLifecycleShutdownReverseOrder(t *testing.T) {
	t.Parallel()

	ext.ResetForTesting()

	var order []string

	host := ext.NewExtensionHost(ext.HostDeps{})

	exts := []*shutdownOrderExtension{
		{name: "alpha", order: &order},
		{name: "beta", order: &order},
		{name: "gamma", order: &order},
	}

	for _, e := range exts {
		require.NoError(t, host.Register(e))
	}

	ctx := context.Background()
	require.NoError(t, host.Bootstrap(ctx))
	require.NoError(t, host.Shutdown(ctx))

	require.Equal(t, []string{"gamma", "beta", "alpha"}, order,
		"Shutdown should be called in reverse registration order")
}

func TestLifecycleShutdownNotBootstrapped(t *testing.T) {
	t.Parallel()

	ext.ResetForTesting()

	host := ext.NewExtensionHost(ext.HostDeps{})
	good := &trackingExtension{name: "good-ext"}
	bad := &trackingExtension{name: "bad-ext"}

	require.NoError(t, host.Register(good))
	require.NoError(t, host.Register(bad))

	err := host.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestConcurrentToolRegistration(t *testing.T) {
	t.Parallel()

	ext.ResetForTesting()

	ctx := context.Background()
	host := ext.NewExtensionHost(ext.HostDeps{})

	const numExtensions = 5
	toolExts := make([]*toolProvidingExtension, numExtensions)
	for i := range numExtensions {
		toolExts[i] = &toolProvidingExtension{
			name:     "tool-ext-" + string(rune('a'+i)),
			toolName: "tool_" + string(rune('a'+i)),
		}
		require.NoError(t, host.Register(toolExts[i]))
	}

	require.NoError(t, host.Bootstrap(ctx))
	t.Cleanup(func() { _ = host.Shutdown(ctx) })

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gotTools := host.ContributedTools()
			require.Len(t, gotTools, numExtensions)

			names := host.ContributedToolNames()
			require.Len(t, names, numExtensions)
		}()
	}
	wg.Wait()

	names := host.ContributedToolNames()
	require.Len(t, names, numExtensions)
}

func TestConcurrentToolRefreshRaceSafety(t *testing.T) {
	t.Parallel()

	ext.ResetForTesting()

	ctx := context.Background()
	host := ext.NewExtensionHost(ext.HostDeps{})

	toolExt := &toolProvidingExtension{name: "refresh-ext", toolName: "refresh_tool"}
	require.NoError(t, host.Register(toolExt))
	require.NoError(t, host.Bootstrap(ctx))
	t.Cleanup(func() { _ = host.Shutdown(ctx) })

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%4 == 0 {
				_ = host.RefreshContributedTools(ctx)
			} else {
				_ = host.ContributedTools()
				_ = host.ContributedToolNames()
			}
		}(i)
	}
	wg.Wait()

	names := host.ContributedToolNames()
	require.Contains(t, names, "refresh_tool")
}

func TestConcurrentExtensionByName(t *testing.T) {
	t.Parallel()

	ext.ResetForTesting()

	ctx := context.Background()
	host := ext.NewExtensionHost(ext.HostDeps{})

	ext1 := &toolProvidingExtension{name: "findable-ext", toolName: "findable_tool"}
	require.NoError(t, host.Register(ext1))
	require.NoError(t, host.Bootstrap(ctx))
	t.Cleanup(func() { _ = host.Shutdown(ctx) })

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			found := host.ExtensionByName("findable-ext")
			require.NotNil(t, found)
			require.Equal(t, "findable-ext", found.Name())

			_ = host.ContributedTools()
			_ = host.IsBootstrapped()
		}()
	}
	wg.Wait()
}

type concurrentSafeMailbox struct {
	mu sync.Mutex
}

func (m *concurrentSafeMailbox) Send(_ tools.MailboxMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return nil
}

func (m *concurrentSafeMailbox) HasInbox(_ string) bool           { return true }
func (m *concurrentSafeMailbox) Broadcast(_ tools.MailboxMessage, _ string) []error { return nil }

func TestConcurrentMailboxAndRegistry(t *testing.T) {
	t.Parallel()

	e := &OrchestrationExtension{}
	require.NoError(t, e.Init(context.Background(), nil))
	t.Cleanup(func() { _ = e.Shutdown(context.Background()) })

	reg := &mockRegistry{agents: map[string]*mockAgentHandle{
		"agent-1": {name: "agent-1", running: true},
		"agent-2": {name: "agent-2", running: true},
	}}
	mb := &concurrentSafeMailbox{}

	e.SetRegistry(reg)
	e.SetMailbox(mb)
	e.RebuildTools()

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 4 {
			case 0:
				e.RebuildTools()
			case 1:
				_, _ = e.Tools(context.Background())
			case 2:
				_ = e.ToolNames()
			case 3:
				_ = mb.Send(tools.MailboxMessage{
					From: "agent-1", To: "agent-2",
					Content: "hello", Type: tools.MailboxMessageDefault,
				})
			}
		}(i)
	}
	wg.Wait()
}

func TestToolSurfaceIncludesRegisteredTools(t *testing.T) {
	t.Parallel()

	surface := agent.NewToolSurface()

	visible := surface.GetVisibleTools()
	require.NotEmpty(t, visible, "default surface should have tools")

	coreTools := []string{"bash", "edit", "view", "grep", "glob"}
	for _, name := range coreTools {
		require.True(t, surface.IsVisible(name),
			"%q should be visible in default surface", name)
	}

	surface.Register("custom_lifecycle_tool", agent.CapabilityExecution)
	require.True(t, surface.IsVisible("custom_lifecycle_tool"))
	require.True(t, surface.HasCapability("custom_lifecycle_tool", agent.CapabilityExecution))

	visible = surface.GetVisibleTools()
	found := false
	for _, name := range visible {
		if name == "custom_lifecycle_tool" {
			found = true
			break
		}
	}
	require.True(t, found, "custom_lifecycle_tool should appear in visible tools")
}

func TestToolSurfaceRegisteredToolsViaHost(t *testing.T) {
	t.Parallel()

	ext.ResetForTesting()

	ctx := context.Background()
	host := ext.NewExtensionHost(ext.HostDeps{})

	ext1 := &toolProvidingExtension{name: "ext-alpha", toolName: "alpha_tool"}
	ext2 := &toolProvidingExtension{name: "ext-beta", toolName: "beta_tool"}

	require.NoError(t, host.Register(ext1))
	require.NoError(t, host.Register(ext2))
	require.NoError(t, host.Bootstrap(ctx))
	t.Cleanup(func() { _ = host.Shutdown(ctx) })

	names := host.ContributedToolNames()
	require.Contains(t, names, "alpha_tool")
	require.Contains(t, names, "beta_tool")

	hostTools := host.ContributedTools()
	require.Len(t, hostTools, 2)
}

func TestToolSurfaceLCMToolsOmittedWhenNoLCM(t *testing.T) {
	t.Parallel()

	surface := agent.NewToolSurface()

	surface.UpdateCapabilities(agent.SurfaceContext{
		HasLSP:     false,
		HasLCM:     false,
		HasMCP:     false,
		HasRepoMap: false,
		BetaTools:  false,
	})

	lcmTools := []string{"lcm_grep", "lcm_describe", "lcm_expand", "llm_map", "agentic_map"}
	for _, name := range lcmTools {
		require.False(t, surface.IsVisible(name),
			"%q should be hidden when HasLCM=false", name)
	}

	lspTools := []string{"lsp_diagnostics", "lsp_references", "lsp_hover"}
	for _, name := range lspTools {
		require.False(t, surface.IsVisible(name),
			"%q should be hidden when HasLSP=false", name)
	}

	require.False(t, surface.IsVisible("list_mcp_resources"))
	require.False(t, surface.IsVisible("read_mcp_resource"))
}

func TestToolSurfaceBetaToolsOmittedWhenNotEnabled(t *testing.T) {
	t.Parallel()

	surface := agent.NewToolSurface()

	surface.UpdateCapabilities(agent.SurfaceContext{
		HasLSP:    true,
		HasLCM:    true,
		HasMCP:    true,
		BetaTools: false,
	})

	require.False(t, surface.IsVisible("synthetic_output"),
		"beta tool should be hidden when BetaTools=false")

	require.True(t, surface.IsVisible("bash"))
	require.True(t, surface.IsVisible("edit"))
}

func TestToolSurfaceBetaToolsShownWhenEnabled(t *testing.T) {
	t.Parallel()

	surface := agent.NewToolSurface()

	surface.UpdateCapabilities(agent.SurfaceContext{
		BetaTools: true,
	})

	require.True(t, surface.IsVisible("synthetic_output"),
		"beta tool should be visible when BetaTools=true")
}

func TestToolSurfaceCodeIntelligenceShownWithLSP(t *testing.T) {
	t.Parallel()

	surface := agent.NewToolSurface()

	surface.UpdateCapabilities(agent.SurfaceContext{
		HasLSP: true,
	})

	lspTools := []string{"lsp_diagnostics", "lsp_references", "lsp_hover",
		"lsp_definition", "lsp_rename", "lsp_symbols"}
	for _, name := range lspTools {
		require.True(t, surface.IsVisible(name),
			"%q should be visible when HasLSP=true", name)
	}
}

func TestToolSurfaceDisabledToolsOmittedFromFiltered(t *testing.T) {
	t.Parallel()

	surface := agent.NewToolSurface()

	require.True(t, surface.IsVisible("bash"))

	surface.Unregister("bash")
	require.False(t, surface.IsVisible("bash"))

	visible := surface.GetVisibleTools()
	for _, name := range visible {
		require.NotEqual(t, "bash", name)
	}
}

func TestToolSurfaceConcurrentRegisterAndRead(t *testing.T) {
	t.Parallel()

	surface := agent.NewToolSurface()

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 4 {
			case 0:
				surface.Register("concurrent-tool", agent.CapabilityExecution)
			case 1:
				surface.Unregister("concurrent-tool")
			case 2:
				_ = surface.IsVisible("concurrent-tool")
			case 3:
				_ = surface.GetVisibleTools()
			}
		}(i)
	}
	wg.Wait()
}

func TestToolSurfaceUpdateCapabilitiesConcurrent(t *testing.T) {
	t.Parallel()

	surface := agent.NewToolSurface()

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 3 {
			case 0:
				surface.UpdateCapabilities(agent.SurfaceContext{
					HasLSP: i%2 == 0,
					HasLCM: i%3 == 0,
				})
			case 1:
				_ = surface.GetVisibleTools()
				_ = surface.GetHiddenTools()
			case 2:
				_ = surface.IsVisible("bash")
				_ = surface.ToolCount()
			}
		}(i)
	}
	wg.Wait()
}

func TestToolSurfaceMemoryToolsShownWithLCM(t *testing.T) {
	t.Parallel()

	surface := agent.NewToolSurface()

	surface.UpdateCapabilities(agent.SurfaceContext{
		HasLCM: true,
	})

	memoryTools := []string{"lcm_grep", "lcm_describe", "lcm_expand",
		"llm_map", "agentic_map", "map_refresh"}
	for _, name := range memoryTools {
		require.True(t, surface.IsVisible(name),
			"%q should be visible when HasLCM=true", name)
	}
}

func TestToolSurfaceMCPToolsShownWithMCP(t *testing.T) {
	t.Parallel()

	surface := agent.NewToolSurface()

	surface.UpdateCapabilities(agent.SurfaceContext{
		HasMCP: true,
	})

	require.True(t, surface.IsVisible("list_mcp_resources"))
	require.True(t, surface.IsVisible("read_mcp_resource"))
}

func TestPhaseFilteredToolsPlanningHidesEdits(t *testing.T) {
	t.Parallel()

	allTools := []string{"bash", "edit", "multiedit", "write", "view", "grep"}

	filtered := agent.PhaseFilteredTools(allTools, agent.PhasePlanning)
	require.NotContains(t, filtered, "edit")
	require.NotContains(t, filtered, "multiedit")
	require.NotContains(t, filtered, "write")
	require.Contains(t, filtered, "bash")
	require.Contains(t, filtered, "view")
	require.Contains(t, filtered, "grep")
}

func TestPhaseFilteredToolsEditingShowsAll(t *testing.T) {
	t.Parallel()

	allTools := []string{"bash", "edit", "multiedit", "write", "view"}

	filtered := agent.PhaseFilteredTools(allTools, agent.PhaseEditing)
	require.Equal(t, allTools, filtered)
}
