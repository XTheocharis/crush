package ext

import (
	"context"
	"fmt"
	"log/slog"

	"charm.land/fantasy"
)

func CollectTools(ctx context.Context, provider ToolProvider) ([]fantasy.AgentTool, []string, error) {
	tools, err := safeCallSlice("Tools:"+provider.Name(), func() ([]fantasy.AgentTool, error) {
		return provider.Tools(ctx)
	})
	if err != nil {
		return nil, nil, err
	}

	names, err := safeCallSlice("ToolNames:"+provider.Name(), func() ([]string, error) {
		return provider.ToolNames(), nil
	})
	if err != nil {
		return nil, nil, err
	}

	return tools, names, nil
}

func safeCallSlice[T any](name string, fn func() ([]T, error)) (result []T, err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Extension hook panicked", "hook", name, "panic", r)
			err = fmt.Errorf("extension hook %s panicked: %v", name, r)
		}
	}()
	return fn()
}
