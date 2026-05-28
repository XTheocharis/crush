package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
)

// ModelRouterExtension provides token-based model routing as a StepHook
// provider. On each step it estimates the token count of the pending
// messages and selects the appropriate model type (large or small) via the
// configured tier router, falling back to the binary ModelRouter when no
// tiers are configured.
type ModelRouterExtension struct {
	mu            sync.RWMutex
	host          ext.HostContext
	tier          *agent.TierRouter
	binary        *agent.ModelRouter
	active        bool
	hooks         []ext.StepHook
	lastModelType config.SelectedModelType
}

func (e *ModelRouterExtension) Name() string { return "model_router" }

func (e *ModelRouterExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host

	cfg := host.Config()
	var tiers []config.RoutingTier
	if cfg.Options != nil {
		tiers = cfg.Options.RouterTiers
	}

	if len(tiers) > 0 {
		e.tier = agent.NewTierRouter(tiers)
	} else {
		e.binary = agent.NewModelRouter()
	}

	e.hooks = []ext.StepHook{
		{
			Name:          "model_router:select_model",
			OnPrepareStep: e.selectModel,
		},
	}
	e.active = true
	return nil
}

func (e *ModelRouterExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tier = nil
	e.binary = nil
	e.hooks = nil
	e.active = false
	e.lastModelType = ""
	return nil
}

// StepHooks returns a defensive copy of the step hooks for model routing.
func (e *ModelRouterExtension) StepHooks() []ext.StepHook {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil
	}
	return append([]ext.StepHook{}, e.hooks...)
}

// LastRoutedModel returns the model type selected by the most recent
// selectModel call. Returns the zero value if no routing has occurred.
func (e *ModelRouterExtension) LastRoutedModel() config.SelectedModelType {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastModelType
}

func (e *ModelRouterExtension) selectModel(_ context.Context, _ string, messages []fantasy.Message) ([]fantasy.Message, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.active {
		return messages, nil
	}

	charCount := estimateCharCount(messages)

	var modelType config.SelectedModelType
	if e.tier != nil {
		modelType = e.tier.ResolveByCharCount(charCount)
	} else {
		role := e.binary.RouteByCharCount(charCount)
		if role == config.RoleEditor {
			modelType = config.SelectedModelTypeSmall
		} else {
			modelType = config.SelectedModelTypeLarge
		}
	}

	e.lastModelType = modelType
	return messages, nil
}

func estimateCharCount(messages []fantasy.Message) int {
	total := 0
	for _, msg := range messages {
		for _, part := range msg.Content {
			if tp, ok := fantasy.AsContentType[fantasy.TextPart](part); ok {
				total += len(tp.Text)
			}
		}
	}
	return total
}

var (
	_ ext.Extension        = (*ModelRouterExtension)(nil)
	_ ext.StepHookProvider = (*ModelRouterExtension)(nil)
)
