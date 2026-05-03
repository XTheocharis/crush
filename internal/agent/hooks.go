package agent

import (
	"context"
	"os"
	"strconv"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/providers/bedrock"
	"charm.land/fantasy/providers/vercel"
)

// PrepareStepHook allows coordinator-level extensions to mutate prepared
// messages right before a model step is sent.
type PrepareStepHook func(
	ctx context.Context,
	opts fantasy.PrepareStepFunctionOptions,
	prepared fantasy.PrepareStepResult,
) (context.Context, fantasy.PrepareStepResult, error)

func applyPrepareStepHooks(
	ctx context.Context,
	opts fantasy.PrepareStepFunctionOptions,
	prepared fantasy.PrepareStepResult,
	hooks []PrepareStepHook,
) (context.Context, fantasy.PrepareStepResult, error) {
	if len(hooks) == 0 {
		return ctx, prepared, nil
	}
	var err error
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		ctx, prepared, err = hook(ctx, opts, prepared)
		if err != nil {
			return ctx, prepared, err
		}
	}
	return ctx, prepared, nil
}

func cacheControlOptions() fantasy.ProviderOptions {
	if t, _ := strconv.ParseBool(os.Getenv("CRUSH_DISABLE_ANTHROPIC_CACHE")); t {
		return fantasy.ProviderOptions{}
	}
	return fantasy.ProviderOptions{
		anthropic.Name: &anthropic.ProviderCacheControlOptions{
			CacheControl: anthropic.CacheControl{Type: "ephemeral"},
		},
		bedrock.Name: &anthropic.ProviderCacheControlOptions{
			CacheControl: anthropic.CacheControl{Type: "ephemeral"},
		},
		vercel.Name: &anthropic.ProviderCacheControlOptions{
			CacheControl: anthropic.CacheControl{Type: "ephemeral"},
		},
	}
}
