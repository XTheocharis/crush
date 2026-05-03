package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/agent/tools/mcp"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/lcm"
	"github.com/charmbracelet/crush/internal/message"
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
	ClearInjection(sessionID string, runKey repomap.RunInjectionKey)
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

// WithTokenCounterProvider wires a tokenizer provider into the coordinator
// so that repo-map generation can use tokenizer-backed token counting.
func WithTokenCounterProvider(p repomap.TokenCounterProvider) CoordinatorOption {
	return func(c *coordinator) {
		c.tokenCounterProvider = p
	}
}

// WithExtraTools appends additional tools (e.g., LCM tools) to the
// coordinator's tool set.
func WithExtraTools(tools []fantasy.AgentTool) CoordinatorOption {
	return func(c *coordinator) {
		c.extraTools = tools
	}
}

// WithLCMOverheadTracking registers a postUpdateModels hook that
// computes system prompt and tool token overhead and pushes it to LCM.
func WithLCMOverheadTracking() CoordinatorOption {
	return func(c *coordinator) {
		c.postUpdateModels = c.computeAndSetOverhead
	}
}

func (c *coordinator) computeAndSetOverhead(_ context.Context, large Model, tools []fantasy.AgentTool) {
	if c.lcm == nil {
		return
	}

	// Estimate tool token overhead from the resolved tool slice.
	var toolTokens int64
	for _, t := range tools {
		toolTokens += estimateToolTokens(t)
	}

	// Start from base system prompt tokens (set during buildAgent).
	spTokens := c.systemPromptTokens

	// Account for SystemPromptPrefix injected as a separate system
	// message in agent.go:313-315.
	largeProviderCfg, _ := c.cfg.Config().Providers.Get(large.ModelCfg.Provider)
	if largeProviderCfg.SystemPromptPrefix != "" {
		spTokens += lcm.EstimateTokens(largeProviderCfg.SystemPromptPrefix)
	}

	// Account for MCP instructions appended at runtime (agent.go:184-196).
	// The actual code concatenates all instructions with "\n\n" separators
	// and wraps them in a single <mcp-instructions> tag.
	var mcpInstructions strings.Builder
	for _, server := range mcp.GetStates() {
		if server.State == mcp.StateConnected {
			if s := server.Client.InitializeResult().Instructions; s != "" {
				mcpInstructions.WriteString(s)
				mcpInstructions.WriteString("\n\n")
			}
		}
	}
	if s := mcpInstructions.String(); s != "" {
		spTokens += lcm.EstimateTokens(s) + 11 // +11 for <mcp-instructions> wrapper (~41 chars)
	}

	c.lcm.SetOverheadTokens(spTokens, toolTokens)
}

func estimateToolTokens(t fantasy.AgentTool) int64 {
	info := t.Info()
	schema, _ := json.Marshal(info.Parameters)
	required, _ := json.Marshal(info.Required)
	chars := int64(len(info.Name) + len(info.Description) + len(schema) + len(required))
	chars += 50 // framing overhead (role, type fields, etc.)
	return (chars + lcm.CharsPerToken - 1) / lcm.CharsPerToken
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

		profile := c.repoMapProfile()
		opts := buildRepoMapGenerateOpts(
			sessionID,
			chatFiles,
			mentionText,
			allRepoFiles,
			addableRepoFiles,
			inChatOrReadOnlyFiles,
			profile,
			true,
		)

		// Merge file paths from LCM summaries into repo map ranking hints.
		if c.lcm != nil {
			summaryPaths, err := c.lcm.GetSummaryMentionedPaths(callCtx, sessionID)
			if err == nil && len(summaryPaths) > 0 {
				for i, p := range summaryPaths {
					summaryPaths[i] = filepath.ToSlash(p)
				}
				opts.MentionedFnames = unionRepoPaths(opts.MentionedFnames, summaryPaths)
			}
		}

		var repoMapText string
		if profile.ParityMode {
			// Parity mode: 3-attempt fallback chain using Refresh()
			// return values directly.
			// Attempt 1 — full (hinted with chat files).
			mapText, tokenCount, err := c.repoMapSvc.Refresh(callCtx, sessionID, opts)
			if err == nil && mapText != "" {
				repoMapText = mapText
				slog.Debug("Repo map: parity attempt 1 (full) succeeded")
			} else {
				if err := callCtx.Err(); err != nil {
					return callCtx, prepared, err
				}
				// Attempt 2 — disjoint (no chat files, expanded budget).
				slog.Debug("Repo map: parity attempt 1 failed, trying attempt 2 (disjoint)")
				opts2 := opts
				opts2.ChatFiles = nil
				opts2.TokenBudget = 0
				opts2.ForceRefresh = false
				mapText, tokenCount, err = c.repoMapSvc.Refresh(callCtx, sessionID, opts2)
				if err == nil && mapText != "" {
					repoMapText = mapText
					slog.Debug("Repo map: parity attempt 2 (disjoint) succeeded")
				} else {
					if err := callCtx.Err(); err != nil {
						return callCtx, prepared, err
					}
					// Attempt 3 — unhinted (no files, no mentions).
					slog.Debug("Repo map: parity attempt 2 failed, trying attempt 3 (unhinted)")
					opts3 := opts
					opts3.ChatFiles = nil
					opts3.MentionedFnames = nil
					opts3.MentionedIdents = nil
					opts3.TokenBudget = 0
					opts3.ForceRefresh = false
					mapText, tokenCount, err = c.repoMapSvc.Refresh(callCtx, sessionID, opts3)
					if err == nil && mapText != "" {
						repoMapText = mapText
						slog.Debug("Repo map: parity attempt 3 (unhinted) succeeded")
					}
				}
			}
			if repoMapText == "" {
				return callCtx, prepared, nil
			}
			if c.lcm != nil {
				_ = c.lcm.SetRepoMapTokens(callCtx, sessionID, int64(tokenCount))
			}
		} else {
			// Non-parity mode: preserve existing behavior with mentions
			// guard and LastGoodMap/LastTokenCount fallback.
			if len(opts.MentionedFnames) > 0 || len(opts.MentionedIdents) > 0 {
				_, _, _ = c.repoMapSvc.Refresh(callCtx, sessionID, opts)
			}
			repoMapText = c.repoMapSvc.LastGoodMap(sessionID)
			if repoMapText == "" {
				return callCtx, prepared, nil
			}
			if c.lcm != nil {
				_ = c.lcm.SetRepoMapTokens(callCtx, sessionID, int64(c.repoMapSvc.LastTokenCount(sessionID)))
			}
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
	TokenCounter         repomap.TokenCounter
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
		TokenCounter:         profile.TokenCounter,
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

// recentFileReader is a duck-typed interface for recency-filtered file
// listing. The concrete filetracker.service satisfies this via the method
// added in filetracker/service_lcm.go, without modifying the upstream
// filetracker.Service interface.
type recentFileReader interface {
	ListRecentReadFiles(ctx context.Context, sessionID string, sinceUnix int64) ([]string, error)
}

func (c *coordinator) sessionChatFiles(ctx context.Context, sessionID string) []string {
	if c == nil || c.filetracker == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}

	sessionIDs := []string{sessionID}

	// Sub-agent sessions (parent_session_id set) should also include
	// files tracked by the parent so that edits made in the parent
	// session boost repo-map ranking in the child.
	if c.sessions != nil {
		if sess, err := c.sessions.Get(ctx, sessionID); err == nil && sess.ParentSessionID != "" {
			sessionIDs = append(sessionIDs, sess.ParentSessionID)
		}
	}

	wd := ""
	if c.cfg != nil {
		wd = c.cfg.WorkingDir()
	}

	var allFiles []string
	for _, sid := range sessionIDs {
		readPaths, err := c.listSessionReadFiles(ctx, sid)
		if err != nil {
			continue
		}
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
			allFiles = append(allFiles, rel)
		}
	}

	sort.Strings(allFiles)
	uniq := allFiles[:0]
	for _, p := range allFiles {
		if len(uniq) == 0 || uniq[len(uniq)-1] != p {
			uniq = append(uniq, p)
		}
	}
	return uniq
}

func (c *coordinator) listSessionReadFiles(ctx context.Context, sessionID string) ([]string, error) {
	if c.lcm != nil {
		if rfr, ok := c.filetracker.(recentFileReader); ok {
			since := time.Now().Add(-30 * time.Minute).Unix()
			return rfr.ListRecentReadFiles(ctx, sessionID, since)
		}
	}
	return c.filetracker.ListReadFiles(ctx, sessionID)
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
	cfg := c.cfg.Config()
	if model := cfg.GetModelByType(config.SelectedModelTypeLarge); model != nil {
		ctxWindow := int(model.ContextWindow)
		profile.MaxContextWindow = ctxWindow
		if c.lcm != nil {
			profile.TokenBudget = config.DefaultRepoMapMaxTokensLCM(ctxWindow)
		} else {
			profile.TokenBudget = config.DefaultRepoMapMaxTokens(ctxWindow)
		}
		profile.Model = model.ID
		if cfg.Options != nil && cfg.Options.RepoMap != nil && cfg.Options.RepoMap.MaxTokens > 0 {
			profile.TokenBudget = cfg.Options.RepoMap.MaxTokens
		}
	}
	if cfg.Options != nil && cfg.Options.LCM != nil {
		profile.ParityMode = strings.EqualFold(strings.TrimSpace(cfg.Options.LCM.ExplorerOutputProfile), "parity")
	}
	if profile.ParityMode {
		profile.PromptCachingEnabled = true
		profile.EnhancementTiers = "none"
		profile.DeterministicMode = true
		profile.TokenCounterMode = "tokenizer_backed"
	}
	// Resolve a tokenizer-backed counter from the provider when available.
	if c.tokenCounterProvider != nil && profile.Model != "" {
		if counter, ok := c.tokenCounterProvider.CounterForModel(profile.Model); ok {
			profile.TokenCounter = counter
		}
	}
	return profile
}

// buildMapRefreshFns returns sync and async repo-map refresh callbacks for the
// tool layer. Both are nil when svc is nil.
func buildMapRefreshFns(svc RepoMapService, profile repoMapProfileOptions) (tools.MapRefreshFn, tools.MapRefreshFn) {
	if svc == nil {
		return nil, nil
	}
	syncFn := func(ctx context.Context, sessionID string) error {
		opts := buildRepoMapGenerateOpts(sessionID, nil, "", nil, nil, nil, profile, true)
		if _, _, err := svc.Refresh(ctx, sessionID, opts); err != nil {
			return err
		}
		if runKey, ok := repomap.RunInjectionKeyFromContext(ctx); ok {
			svc.ClearInjection(sessionID, runKey)
		}
		return nil
	}
	asyncFn := func(ctx context.Context, sessionID string) error {
		opts := buildRepoMapGenerateOpts(sessionID, nil, "", nil, nil, nil, profile, true)
		svc.RefreshAsync(sessionID, opts)
		if runKey, ok := repomap.RunInjectionKeyFromContext(ctx); ok {
			svc.ClearInjection(sessionID, runKey)
		}
		return nil
	}
	return syncFn, asyncFn
}

// lcmContextFiles converts lcm.ContextFile values to prompt.ContextFile values
// for injection into the system prompt.
func lcmContextFiles(mgr lcm.Manager) []prompt.ContextFile {
	lcmFiles := mgr.GetContextFiles()
	promptFiles := make([]prompt.ContextFile, len(lcmFiles))
	for i, f := range lcmFiles {
		promptFiles[i] = prompt.ContextFile{Path: f.Name, Content: f.Content}
	}
	return promptFiles
}

func (c *coordinator) RecoverSession(ctx context.Context, sessionID string) error {
	if c.currentAgent != nil && c.currentAgent.IsSessionBusy(sessionID) {
		return nil
	}

	msgs, err := c.messages.List(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}

	for _, msg := range msgs {
		if msg.IsFinished() {
			continue
		}

		msg.FinishThinking()
		for _, tc := range msg.ToolCalls() {
			if !tc.Finished {
				msg.FinishToolCall(tc.ID)
			}
		}

		msg.AddFinish(message.FinishReasonError, "Session interrupted", "The session was previously interrupted")
		if updateErr := c.messages.Update(ctx, msg); updateErr != nil {
			slog.Error("Failed to recover message", "message_id", msg.ID, "error", updateErr)
		}
	}

	return nil
}
