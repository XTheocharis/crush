package agent

import (
	"context"
	"sort"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/repomap"
)

// RepoMapService is the coordinator-facing repo-map contract.
type RepoMapService interface {
	Available() bool
	Generate(ctx context.Context, opts repomap.GenerateOpts) (string, int, error)
	LastGoodMap(sessionID string) string
	LastTokenCount(sessionID string) int
	ShouldInject(sessionID string, runKey repomap.RunInjectionKey) bool
	RefreshAsync(sessionID string, opts repomap.GenerateOpts)
	Refresh(ctx context.Context, sessionID string, opts repomap.GenerateOpts) (string, int, error)
	Reset(ctx context.Context, sessionID string) error
	AllFiles(ctx context.Context) []string
	Close() error
}

// CoordinatorOption mutates coordinator construction.
type CoordinatorOption func(*coordinator)

// WithRepoMap wires a repo-map service into coordinator hook plumbing.
func WithRepoMap(svc RepoMapService) CoordinatorOption {
	return func(c *coordinator) {
		c.repoMapSvc = svc
	}
}

func (c *coordinator) buildRepoMapHook() PrepareStepHook {
	if c == nil || c.repoMapSvc == nil || !c.repoMapSvc.Available() {
		return nil
	}

	return func(
		callCtx context.Context,
		_ fantasy.PrepareStepFunctionOptions,
		prepared fantasy.PrepareStepResult,
	) (context.Context, fantasy.PrepareStepResult, error) {
		sessionID := tools.GetSessionFromContext(callCtx)
		if sessionID == "" {
			return callCtx, prepared, nil
		}
		runKey, ok := repomap.RunInjectionKeyFromContext(callCtx)
		if !ok || !c.repoMapSvc.ShouldInject(sessionID, runKey) {
			return callCtx, prepared, nil
		}

		currentRunMessages := repomap.ExtractCurrentRunMessages(prepared.Messages)
		mentionText := repomap.ExtractCurrentMessageText(currentRunMessages)
		allRepoFiles := c.repoMapSvc.AllFiles(callCtx)
		inChatOrReadOnlyFiles := make([]string, 0, len(prepared.Messages))
		for _, msg := range prepared.Messages {
			for _, part := range msg.Content {
				if textPart, ok := part.(fantasy.TextPart); ok {
					inChatOrReadOnlyFiles = append(inChatOrReadOnlyFiles, repomap.ExtractMentionedFnames(textPart.Text, allRepoFiles, nil)...)
				}
			}
		}
		opts := buildRepoMapGenerateOpts(sessionID, nil, mentionText, allRepoFiles, inChatOrReadOnlyFiles, 0, 0, true)
		if len(opts.MentionedFnames) > 0 || len(opts.MentionedIdents) > 0 {
			_, _, _ = c.repoMapSvc.Refresh(callCtx, sessionID, opts)
		}

		repoMapText := c.repoMapSvc.LastGoodMap(sessionID)
		if repoMapText == "" {
			return callCtx, prepared, nil
		}
		if c.lcm != nil {
			_ = c.lcm.SetRepoMapTokens(callCtx, sessionID, int64(c.repoMapSvc.LastTokenCount(sessionID)))
		}

		userMsg := fantasy.NewUserMessage(
			"Below is a map of the repository showing the most relevant files and their key definitions.\n" +
				"Use this to understand the codebase structure. These files are read-only context — use tools to read full contents when needed.\n\n" +
				"<repo-map>\n" + repoMapText + "\n</repo-map>",
		)
		assistantMsg := fantasy.Message{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "Ok, I won't try and edit those files without asking first."},
			},
			ProviderOptions: cacheControlOptions(),
		}
		insertAt := 0
		for insertAt < len(prepared.Messages) && prepared.Messages[insertAt].Role == fantasy.MessageRoleSystem {
			insertAt++
		}
		withRepoMap := make([]fantasy.Message, 0, len(prepared.Messages)+2)
		withRepoMap = append(withRepoMap, prepared.Messages[:insertAt]...)
		withRepoMap = append(withRepoMap, userMsg, assistantMsg)
		withRepoMap = append(withRepoMap, prepared.Messages[insertAt:]...)
		prepared.Messages = withRepoMap
		return callCtx, prepared, nil
	}
}

func buildRepoMapGenerateOpts(
	sessionID string,
	chatFiles []string,
	mentionText string,
	allRepoFiles []string,
	inChatOrReadOnlyFiles []string,
	budget int,
	maxContextWindow int,
	forceRefresh bool,
) repomap.GenerateOpts {
	opts := repomap.GenerateOpts{
		SessionID:        sessionID,
		ChatFiles:        chatFiles,
		TokenBudget:      budget,
		MaxContextWindow: maxContextWindow,
		ForceRefresh:     forceRefresh,
	}
	if mentionText == "" {
		return opts
	}

	opts.MentionedFnames = repomap.ExtractMentionedFnames(mentionText, allRepoFiles, inChatOrReadOnlyFiles)
	opts.MentionedIdents = repomap.ExtractIdents(mentionText)

	if identMatches := repomap.IdentFilenameMatches(opts.MentionedIdents, allRepoFiles); len(identMatches) > 0 {
		combined := append([]string{}, opts.MentionedFnames...)
		combined = append(combined, identMatches...)
		sort.Strings(combined)
		uniq := combined[:0]
		for _, p := range combined {
			if len(uniq) == 0 || uniq[len(uniq)-1] != p {
				uniq = append(uniq, p)
			}
		}
		opts.MentionedFnames = uniq
	}

	return opts
}
