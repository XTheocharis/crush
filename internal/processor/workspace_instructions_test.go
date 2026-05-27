package processor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkspaceInstructionsID(t *testing.T) {
	t.Parallel()
	p := &WorkspaceInstructions{}
	require.Equal(t, "workspace_instructions", p.ID())
}

func TestWorkspaceInstructionsPrependSystemMessage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := &WorkspaceInstructions{
		Instructions: "Use Go 1.22. Prefer table-driven tests.",
	}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("refactor this")},
		State:    make(map[string]any),
	}

	result, err := p.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)

	// System message prepended.
	require.Len(t, result.Messages, 2)
	require.Equal(t, "system", result.Messages[0].Role)
	require.Equal(t, "Use Go 1.22. Prefer table-driven tests.", result.Messages[0].Content)

	// Original messages preserved after injection.
	require.Equal(t, "user", result.Messages[1].Role)
	require.Equal(t, "refactor this", result.Messages[1].Content)
}

func TestWorkspaceInstructionsEmptyNoop(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := &WorkspaceInstructions{Instructions: ""}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("hello")},
		State:    make(map[string]any),
	}

	result, err := p.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Len(t, result.Messages, 1)
	require.Equal(t, "user", result.Messages[0].Role)

	// State reports no injection.
	require.Equal(t, false, result.State["injected"])
	require.Equal(t, 0, result.State["instructions_length"])
}

func TestWorkspaceInstructionsStateContainsInfo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	instructions := "Project uses Go modules."
	p := &WorkspaceInstructions{Instructions: instructions}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{},
		State:    make(map[string]any),
	}

	result, err := p.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, true, result.State["injected"])
	require.Equal(t, len(instructions), result.State["instructions_length"])
}

func TestWorkspaceInstructionsPreservesExistingMessages(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := &WorkspaceInstructions{Instructions: "workspace rules"}
	original := []Message{
		UserMessage("first"),
		AssistantMessage("second"),
		UserMessage("third"),
	}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: original,
		State:    make(map[string]any),
	}

	result, err := p.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Len(t, result.Messages, 4)
	require.Equal(t, "system", result.Messages[0].Role)
	require.Equal(t, "user", result.Messages[1].Role)
	require.Equal(t, "first", result.Messages[1].Content)
	require.Equal(t, "assistant", result.Messages[2].Role)
	require.Equal(t, "second", result.Messages[2].Content)
	require.Equal(t, "user", result.Messages[3].Role)
	require.Equal(t, "third", result.Messages[3].Content)
}

func TestWorkspaceInstructionsSystemMessageCorrect(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	content := "Always format with gofumpt."
	p := &WorkspaceInstructions{Instructions: content}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("fix formatting")},
		State:    make(map[string]any),
	}

	result, err := p.ProcessInput(ctx, pctx)
	require.NoError(t, err)

	sysMsg := result.Messages[0]
	require.Equal(t, "system", sysMsg.Role)
	require.Equal(t, content, sysMsg.Content)
	require.Nil(t, sysMsg.Meta)
}

func TestWorkspaceInstructionsPassThroughPhases(t *testing.T) {
	t.Parallel()
	p := &WorkspaceInstructions{Instructions: "instructions"}
	pctx := NewTestContext()
	ctx := context.Background()

	// OutputStream phase passes through.
	pctx.Phase = OutputStreamPhase
	result, err := p.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, pctx.Messages, result.Messages)

	// OutputResult phase passes through.
	pctx.Phase = OutputResultPhase
	result, err = p.ProcessOutputResult(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, pctx.Messages, result.Messages)

	// APIError phase passes through.
	pctx.Phase = APIErrorPhase
	result, err = p.ProcessAPIError(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, pctx.Messages, result.Messages)
}

func TestWorkspaceInstructionsRunAllPhases(t *testing.T) {
	t.Parallel()
	p := &WorkspaceInstructions{
		Instructions: "Use semantic commits.",
	}
	pctx := NewTestContext()

	final := RunAllPhases(t, p, pctx)

	// After all phases, the system message should be prepended by
	// ProcessInput.
	require.Len(t, final.Messages, 3)
	require.Equal(t, "system", final.Messages[0].Role)
	require.Equal(t, "Use semantic commits.", final.Messages[0].Content)
	require.Equal(t, "user", final.Messages[1].Role)
	require.Equal(t, "hello", final.Messages[1].Content)
}
