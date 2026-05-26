package extensions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/ext"
)

// PromptAssemblyExtension wraps prompt assembly v2 as a PromptHookProvider.
type PromptAssemblyExtension struct {
	mu     sync.RWMutex
	host   ext.HostContext
	cache  *prompt.ContextCache
	lcm    *LCMExtension
	active bool
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

func (e *PromptAssemblyExtension) PromptHook() *ext.PromptHook {
	e.mu.RLock()
	active := e.active
	e.mu.RUnlock()
	if !active {
		return nil
	}
	return &ext.PromptHook{
		Name: "prompt-assembly-v2",
		OnPreparePrompt: func(ctx context.Context, _ string, messages []fantasy.Message) ([]fantasy.Message, error) {
			e.mu.RLock()
			defer e.mu.RUnlock()
			if !e.active {
				return defensiveCopyMessages(messages), nil
			}
			return e.onPreparePrompt(ctx, messages)
		},
		SystemPromptModifier: func(_ context.Context, _ string, systemPrompt string) (string, error) {
			e.mu.RLock()
			defer e.mu.RUnlock()
			if !e.active {
				return systemPrompt, nil
			}
			return e.systemPromptModifier(systemPrompt)
		},
	}
}

func (e *PromptAssemblyExtension) onPreparePrompt(_ context.Context, messages []fantasy.Message) ([]fantasy.Message, error) {
	cfg := e.host.Config()
	if cfg == nil || cfg.Options == nil {
		return defensiveCopyMessages(messages), nil
	}

	contextPaths := cfg.Options.ContextPaths
	if len(contextPaths) == 0 {
		return defensiveCopyMessages(messages), nil
	}

	workingDir := e.host.WorkingDir()
	var files []prompt.ContextFile

	for _, p := range contextPaths {
		resolved := resolveContextPath(p, workingDir)
		cf := e.readContextPath(resolved)
		files = append(files, cf...)
	}

	if len(files) == 0 {
		return defensiveCopyMessages(messages), nil
	}

	var sb strings.Builder
	sb.WriteString("<memory>\n")
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("<file path=%q>\n%s\n</file>\n", f.Path, f.Content))
	}
	sb.WriteString("</memory>")

	contextMsg := fantasy.Message{
		Role: fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{
			&fantasy.TextPart{Text: sb.String()},
		},
	}

	result := make([]fantasy.Message, len(messages)+1)
	copy(result, messages)
	result[len(messages)] = contextMsg
	return result, nil
}

func (e *PromptAssemblyExtension) systemPromptModifier(systemPrompt string) (string, error) {
	if e.lcm == nil {
		return systemPrompt, nil
	}

	mgr := e.lcm.Manager()
	if mgr == nil {
		return systemPrompt, nil
	}

	contextFiles := mgr.GetContextFiles()
	if len(contextFiles) == 0 {
		return systemPrompt, nil
	}

	var sb strings.Builder
	sb.WriteString(systemPrompt)
	sb.WriteString("\n\n")
	for _, cf := range contextFiles {
		sb.WriteString(fmt.Sprintf("<context name=%q>\n%s\n</context>\n", cf.Name, cf.Content))
	}

	return sb.String(), nil
}

func (e *PromptAssemblyExtension) readContextPath(path string) []prompt.ContextFile {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}

	if !info.IsDir() {
		cf := e.cache.Get(path)
		if cf == nil {
			return nil
		}
		return []prompt.ContextFile{*cf}
	}

	var files []prompt.ContextFile
	filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if cf := e.cache.Get(p); cf != nil {
			files = append(files, *cf)
		}
		return nil
	})
	return files
}

func resolveContextPath(p, workingDir string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(workingDir, p)
}

func defensiveCopyMessages(messages []fantasy.Message) []fantasy.Message {
	if messages == nil {
		return nil
	}
	out := make([]fantasy.Message, len(messages))
	copy(out, messages)
	return out
}

var (
	_ ext.Extension          = (*PromptAssemblyExtension)(nil)
	_ ext.PromptHookProvider = (*PromptAssemblyExtension)(nil)
)
