package extensions

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/ext"
)

// defaultObservationTokenBudget is the default token budget for observation
// prompt injection. Can be overridden via config.ObservationOptions.TokenBudget.
const defaultObservationTokenBudget int64 = 2000

// PromptAssemblyExtension wraps prompt assembly v2 as a PromptHookProvider.
type PromptAssemblyExtension struct {
	mu      sync.RWMutex
	host    ext.HostContext
	cache   *prompt.ContextCache
	lcm     *LCMExtension
	repomap *RepomapExtension
	active  bool
}

func (e *PromptAssemblyExtension) Name() string { return "prompt-assembly" }

func (e *PromptAssemblyExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host
	e.cache = prompt.NewContextCache()
	e.active = true
	return nil
}

func (e *PromptAssemblyExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cache = nil
	e.active = false
	return nil
}

// SetLCMExtension injects the LCM extension for accessing context files.
func (e *PromptAssemblyExtension) SetLCMExtension(lcm *LCMExtension) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lcm = lcm
}

// SetRepomapExtension injects the repo map extension for cached map injection.
func (e *PromptAssemblyExtension) SetRepomapExtension(ext *RepomapExtension) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.repomap = ext
}

func (e *PromptAssemblyExtension) PromptHook() *ext.PromptHook {
	e.mu.RLock()
	active := e.active
	e.mu.RUnlock()
	if !active {
		return nil
	}
	return &ext.PromptHook{
		Name: "prompt-assembly-v2",
		SystemPromptModifier: func(ctx context.Context, sessionID string, systemPrompt string) (string, error) {
			e.mu.RLock()
			defer e.mu.RUnlock()
			if !e.active {
				return systemPrompt, nil
			}
			return e.systemPromptModifier(ctx, sessionID, systemPrompt)
		},
	}
}

func (e *PromptAssemblyExtension) systemPromptModifier(ctx context.Context, sessionID string, systemPrompt string) (string, error) {
	var sb strings.Builder
	sb.WriteString(systemPrompt)

	if e.lcm != nil {
		mgr := e.lcm.Manager()
		if mgr != nil {
			contextFiles := mgr.GetContextFiles()
			if len(contextFiles) > 0 {
				sb.WriteString("\n\n")
				for _, cf := range contextFiles {
					fmt.Fprintf(&sb, "<context name=%q>\n%s\n</context>\n", cf.Name, cf.Content)
				}
			}

			if sessionID != "" {
				obsPrompt, err := mgr.GetObservationPrompt(ctx, sessionID, defaultObservationTokenBudget)
				if err == nil && obsPrompt != "" {
					fmt.Fprintf(&sb, "\n\n<context name=%q>\n%s\n</context>\n", "observations", obsPrompt)
				}
			}
		}
	}

	if e.repomap != nil && e.repomap.isActive() && e.repomap.ShouldInjectMap(ctx, sessionID) {
		mapString, tokenCount := e.repomap.LoadCachedMap(sessionID)
		if mapString != "" && tokenCount > 0 {
			fmt.Fprintf(&sb, "\n\n<context name=%q>\n%s\n</context>\n", "repo-map", mapString)
			if mgr := TheLCMExtension.Manager(); mgr != nil {
				if err := mgr.SetRepoMapTokens(ctx, sessionID, int64(tokenCount)); err != nil {
					slog.Debug("LCM SetRepoMapTokens failed", "session_id", sessionID, "error", err)
				}
			}
		}
	}

	result := sb.String()
	if result == systemPrompt {
		return systemPrompt, nil
	}
	return result, nil
}

var (
	_ ext.Extension          = (*PromptAssemblyExtension)(nil)
	_ ext.PromptHookProvider = (*PromptAssemblyExtension)(nil)
)
