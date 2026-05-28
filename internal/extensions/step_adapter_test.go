package extensions

import (
	"context"
	"testing"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestStepAdapter_AddMutatorAndInvoke(t *testing.T) {
	t.Parallel()

	e := &StepAdapter{}
	host := &mockHostContext{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Shutdown(context.Background()) })

	e.AddMutator(func(_ context.Context, _ string, messages []fantasy.Message) ([]fantasy.Message, error) {
		return append(messages, fantasy.NewUserMessage("mutated")), nil
	})

	hooks := e.StepHooks()
	require.Len(t, hooks, 1)
	require.Equal(t, "step-adapter-mutator", hooks[0].Name)
	require.NotNil(t, hooks[0].OnPrepareStep)
	require.Nil(t, hooks[0].OnStepFinish)
	require.Nil(t, hooks[0].StopCondition)

	result, err := hooks[0].OnPrepareStep(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage("original"),
	})
	require.NoError(t, err)
	require.Len(t, result, 2)
}

func TestStepAdapter_NoMutatorsReturnsNil(t *testing.T) {
	t.Parallel()

	e := &StepAdapter{}
	host := &mockHostContext{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Shutdown(context.Background()) })

	// No mutators registered — StepHooks should return nil.
	hooks := e.StepHooks()
	require.Nil(t, hooks)
}

func TestStepAdapter_NotActiveReturnsNil(t *testing.T) {
	t.Parallel()

	e := &StepAdapter{}
	// Never call Init, so active stays false.
	e.AddMutator(func(_ context.Context, _ string, messages []fantasy.Message) ([]fantasy.Message, error) {
		return messages, nil
	})

	hooks := e.StepHooks()
	require.Nil(t, hooks)
}
