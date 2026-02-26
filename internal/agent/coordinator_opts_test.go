package agent

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/repomap"
	"github.com/stretchr/testify/require"
)

type fakeRepoMapService struct {
	available         bool
	lastMap           string
	shouldInject      bool
	allFiles          []string
	refreshSyncCalls  int
	refreshAsyncCalls int
	lastRefreshOpts   repomap.GenerateOpts
	lastSessionID     string
}

func (f *fakeRepoMapService) Available() bool { return f.available }

func (f *fakeRepoMapService) Generate(_ context.Context, _ repomap.GenerateOpts) (string, int, error) {
	return "", 0, nil
}

func (f *fakeRepoMapService) LastGoodMap(_ string) string { return f.lastMap }

func (f *fakeRepoMapService) LastTokenCount(_ string) int { return 0 }

func (f *fakeRepoMapService) ShouldInject(_ string, _ repomap.RunInjectionKey) bool {
	return f.shouldInject
}

func (f *fakeRepoMapService) RefreshAsync(sessionID string, opts repomap.GenerateOpts) {
	f.refreshAsyncCalls++
	f.lastSessionID = sessionID
	f.lastRefreshOpts = opts
}

func (f *fakeRepoMapService) Refresh(_ context.Context, sessionID string, opts repomap.GenerateOpts) (string, int, error) {
	f.refreshSyncCalls++
	f.lastSessionID = sessionID
	f.lastRefreshOpts = opts
	return "", 0, nil
}

func (f *fakeRepoMapService) Reset(_ context.Context, _ string) error { return nil }

func (f *fakeRepoMapService) AllFiles(_ context.Context) []string {
	return append([]string(nil), f.allFiles...)
}

func (f *fakeRepoMapService) Close() error { return nil }

func TestBuildRepoMapHookInjectsAfterSystemMessages(t *testing.T) {
	t.Parallel()

	svc := &fakeRepoMapService{
		available:    true,
		lastMap:      "path/to/file.go",
		shouldInject: true,
		allFiles: []string{
			"path/to/file.go",
			"internal/repomap/mentions.go",
		},
	}
	c := &coordinator{repoMapSvc: svc}

	hook := c.buildRepoMapHook()
	require.NotNil(t, hook)

	ctx := context.Background()
	ctx = context.WithValue(ctx, tools.SessionIDContextKey, "sess-1")
	ctx = repomap.WithRunInjectionKey(ctx, repomap.RunInjectionKey{RootUserMessageID: "msg-1", QueueGeneration: 0})

	prepared := fantasy.PrepareStepResult{Messages: []fantasy.Message{
		fantasy.NewSystemMessage("system-1"),
		fantasy.NewSystemMessage("system-2"),
		fantasy.NewUserMessage("please check path/to/file.go"),
	}}

	_, out, err := hook(ctx, fantasy.PrepareStepFunctionOptions{}, prepared)
	require.NoError(t, err)
	require.Len(t, out.Messages, 5)

	require.Equal(t, fantasy.MessageRoleSystem, out.Messages[0].Role)
	require.Equal(t, fantasy.MessageRoleSystem, out.Messages[1].Role)
	require.Equal(t, fantasy.MessageRoleUser, out.Messages[2].Role)
	require.Equal(t, fantasy.MessageRoleAssistant, out.Messages[3].Role)
	require.Equal(t, fantasy.MessageRoleUser, out.Messages[4].Role)

	require.Contains(t, out.Messages[2].Content[0].(fantasy.TextPart).Text, "<repo-map>")
	require.Contains(t, out.Messages[2].Content[0].(fantasy.TextPart).Text, "path/to/file.go")
	require.Equal(t, "Ok, I won't try and edit those files without asking first.", out.Messages[3].Content[0].(fantasy.TextPart).Text)
	require.NotNil(t, out.Messages[3].ProviderOptions)
	require.NotEmpty(t, out.Messages[3].ProviderOptions)

	require.Equal(t, 1, svc.refreshSyncCalls)
	require.Equal(t, []string{"path/to/file.go"}, svc.lastRefreshOpts.MentionedFnames)
	require.Contains(t, svc.lastRefreshOpts.MentionedIdents, "path")
	require.Contains(t, svc.lastRefreshOpts.MentionedIdents, "to")
	require.Contains(t, svc.lastRefreshOpts.MentionedIdents, "file")
	require.Contains(t, svc.lastRefreshOpts.MentionedIdents, "go")
}

func TestBuildRepoMapHookNoopWhenRunKeyMissing(t *testing.T) {
	t.Parallel()

	svc := &fakeRepoMapService{available: true, lastMap: "map", shouldInject: true}
	c := &coordinator{repoMapSvc: svc}
	hook := c.buildRepoMapHook()
	require.NotNil(t, hook)

	ctx := context.WithValue(context.Background(), tools.SessionIDContextKey, "sess-1")
	prepared := fantasy.PrepareStepResult{Messages: []fantasy.Message{fantasy.NewUserMessage("user-1")}}

	_, out, err := hook(ctx, fantasy.PrepareStepFunctionOptions{}, prepared)
	require.NoError(t, err)
	require.Equal(t, prepared.Messages, out.Messages)
}

func TestBuildRepoMapHookNoopWhenMapMissing(t *testing.T) {
	t.Parallel()

	svc := &fakeRepoMapService{available: true, lastMap: "", shouldInject: true}
	c := &coordinator{repoMapSvc: svc}
	hook := c.buildRepoMapHook()
	require.NotNil(t, hook)

	ctx := context.Background()
	ctx = context.WithValue(ctx, tools.SessionIDContextKey, "sess-1")
	ctx = repomap.WithRunInjectionKey(ctx, repomap.RunInjectionKey{RootUserMessageID: "msg-1", QueueGeneration: 0})
	prepared := fantasy.PrepareStepResult{Messages: []fantasy.Message{fantasy.NewUserMessage("user-1")}}

	_, out, err := hook(ctx, fantasy.PrepareStepFunctionOptions{}, prepared)
	require.NoError(t, err)
	require.Equal(t, prepared.Messages, out.Messages)
}

func TestBuildRepoMapGenerateOptsIncludesMentions(t *testing.T) {
	t.Parallel()

	opts := buildRepoMapGenerateOpts(
		"sess-1",
		[]string{"chat/a.go"},
		"Please inspect internal/repomap/mentions.go and MentionHelper",
		[]string{"internal/repomap/mentions.go", "internal/repomap/other.go", "mentionhelper.go"},
		[]string{"internal/repomap/other.go"},
		256,
		8192,
		true,
	)

	require.Equal(t, "sess-1", opts.SessionID)
	require.Equal(t, []string{"chat/a.go"}, opts.ChatFiles)
	require.Equal(t, 256, opts.TokenBudget)
	require.Equal(t, 8192, opts.MaxContextWindow)
	require.True(t, opts.ForceRefresh)
	require.Equal(t, []string{"internal/repomap/mentions.go", "mentionhelper.go"}, opts.MentionedFnames)
	require.Contains(t, opts.MentionedIdents, "MentionHelper")
}

func TestBuildRepoMapGenerateOptsWithoutMentionText(t *testing.T) {
	t.Parallel()

	opts := buildRepoMapGenerateOpts("sess-1", nil, "", []string{"a.go"}, nil, 0, 0, false)
	require.Nil(t, opts.MentionedFnames)
	require.Nil(t, opts.MentionedIdents)
}

func TestBuildToolsMapRefreshWiresRepoMapService(t *testing.T) {
	env := testEnv(t)
	cfg, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)
	cfg.SetupAgents()

	svc := &fakeRepoMapService{available: true}
	c := &coordinator{
		cfg:         cfg,
		sessions:    env.sessions,
		messages:    env.messages,
		permissions: env.permissions,
		history:     env.history,
		filetracker: *env.filetracker,
		repoMapSvc:  svc,
	}

	coderTools, err := c.buildTools(t.Context(), cfg.Agents[config.AgentCoder])
	require.NoError(t, err)

	var mapRefresh fantasy.AgentTool
	for _, tool := range coderTools {
		if tool.Info().Name == tools.MapRefreshToolName {
			mapRefresh = tool
			break
		}
	}
	require.NotNil(t, mapRefresh)

	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "sess-1")
	resp, err := mapRefresh.Run(ctx, fantasy.ToolCall{ID: "sync", Input: `{"sync":true}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "refreshed")
	require.Equal(t, 1, svc.refreshSyncCalls)
	require.Equal(t, "sess-1", svc.lastSessionID)
	require.True(t, svc.lastRefreshOpts.ForceRefresh)

	resp, err = mapRefresh.Run(ctx, fantasy.ToolCall{ID: "async", Input: `{}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "scheduled")
	require.Equal(t, 1, svc.refreshAsyncCalls)
	require.Equal(t, "sess-1", svc.lastSessionID)
	require.True(t, svc.lastRefreshOpts.ForceRefresh)
}
