package agent

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/repomap"
)

// RepoMapService is the coordinator-facing repo-map contract.
type RepoMapService interface {
	Available() bool
	Generate(ctx context.Context, opts repomap.GenerateOpts) (string, int, error)
	LastGoodMap(sessionID string) string
	LastTokenCount(sessionID string) int
	SessionReadOnlyFiles(ctx context.Context, sessionID string) []string
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
		chatFiles := c.sessionChatFiles(callCtx, sessionID)
		readOnlyFiles := c.repoMapSvc.SessionReadOnlyFiles(callCtx, sessionID)
		inChatOrReadOnlyFiles := unionRepoPaths(chatFiles, readOnlyFiles)
		addableRepoFiles := subtractRepoPaths(allRepoFiles, inChatOrReadOnlyFiles)

		opts := buildRepoMapGenerateOpts(
			sessionID,
			chatFiles,
			mentionText,
			allRepoFiles,
			addableRepoFiles,
			inChatOrReadOnlyFiles,
			c.repoMapProfile(),
			true,
		)
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

type repoMapProfileOptions struct {
	TokenBudget          int
	MaxContextWindow     int
	Model                string
	ParityMode           bool
	PromptCachingEnabled bool
	EnhancementTiers     string
	DeterministicMode    bool
	TokenCounterMode     string
}

func buildRepoMapGenerateOpts(
	sessionID string,
	chatFiles []string,
	mentionText string,
	allRepoFiles []string,
	addableRepoFiles []string,
	inChatOrReadOnlyFiles []string,
	profile repoMapProfileOptions,
	forceRefresh bool,
) repomap.GenerateOpts {
	opts := repomap.GenerateOpts{
		SessionID:            sessionID,
		ChatFiles:            chatFiles,
		TokenBudget:          profile.TokenBudget,
		MaxContextWindow:     profile.MaxContextWindow,
		Model:                profile.Model,
		ParityMode:           profile.ParityMode,
		PromptCachingEnabled: profile.PromptCachingEnabled,
		EnhancementTiers:     profile.EnhancementTiers,
		DeterministicMode:    profile.DeterministicMode,
		TokenCounterMode:     profile.TokenCounterMode,
		ForceRefresh:         forceRefresh,
	}
	if mentionText == "" {
		return opts
	}

	opts.MentionedFnames = repomap.ExtractMentionedFnames(mentionText, addableRepoFiles, inChatOrReadOnlyFiles)
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

func (c *coordinator) sessionChatFiles(ctx context.Context, sessionID string) []string {
	if c == nil || c.filetracker == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	readPaths, err := c.filetracker.ListReadFiles(ctx, sessionID)
	if err != nil {
		return nil
	}
	wd := ""
	if c.cfg != nil {
		wd = c.cfg.WorkingDir()
	}
	files := make([]string, 0, len(readPaths))
	for _, p := range readPaths {
		rel := p
		if wd != "" {
			if pathRel, relErr := filepath.Rel(wd, p); relErr == nil {
				rel = pathRel
			}
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || strings.HasPrefix(rel, "../") {
			continue
		}
		files = append(files, rel)
	}
	sort.Strings(files)
	uniq := files[:0]
	for _, p := range files {
		if len(uniq) == 0 || uniq[len(uniq)-1] != p {
			uniq = append(uniq, p)
		}
	}
	return uniq
}

func unionRepoPaths(a, b []string) []string {
	combined := make([]string, 0, len(a)+len(b))
	combined = append(combined, a...)
	combined = append(combined, b...)
	sort.Strings(combined)
	uniq := combined[:0]
	for _, p := range combined {
		if p == "" {
			continue
		}
		if len(uniq) == 0 || uniq[len(uniq)-1] != p {
			uniq = append(uniq, p)
		}
	}
	return uniq
}

func subtractRepoPaths(all, excluded []string) []string {
	if len(all) == 0 {
		return nil
	}
	excludeSet := make(map[string]struct{}, len(excluded))
	for _, p := range excluded {
		excludeSet[p] = struct{}{}
	}
	out := make([]string, 0, len(all))
	for _, p := range all {
		if _, skip := excludeSet[p]; skip {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (c *coordinator) repoMapProfile() repoMapProfileOptions {
	profile := repoMapProfileOptions{}
	if c == nil || c.cfg == nil {
		return profile
	}
	if model := c.cfg.GetModelByType(config.SelectedModelTypeLarge); model != nil {
		ctxWindow := int(model.ContextWindow)
		profile.MaxContextWindow = ctxWindow
		profile.TokenBudget = config.DefaultRepoMapMaxTokens(ctxWindow)
		profile.Model = model.ID
		if c.cfg.Options != nil && c.cfg.Options.RepoMap != nil && c.cfg.Options.RepoMap.MaxTokens > 0 {
			profile.TokenBudget = c.cfg.Options.RepoMap.MaxTokens
		}
	}
	if c.cfg.Options != nil && c.cfg.Options.LCM != nil {
		profile.ParityMode = strings.EqualFold(strings.TrimSpace(c.cfg.Options.LCM.ExplorerOutputProfile), "parity")
	}
	if profile.ParityMode {
		profile.PromptCachingEnabled = true
		profile.EnhancementTiers = "none"
		profile.DeterministicMode = true
		profile.TokenCounterMode = "tokenizer_backed"
	}
	return profile
}
