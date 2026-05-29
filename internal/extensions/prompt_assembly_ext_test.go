package extensions

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/lcm"
	"github.com/stretchr/testify/require"
)

func TestObservationInjection(t *testing.T) {
	t.Parallel()

	ext := &PromptAssemblyExtension{active: true}
	lcmExt := &LCMExtension{
		active:  true,
		manager: &mockObservationManager{observations: "switched to PostgreSQL"},
	}
	ext.SetLCMExtension(lcmExt)

	result, err := ext.systemPromptModifier(context.Background(), "session-1", "base prompt")
	require.NoError(t, err)
	require.Contains(t, result, "base prompt")
	require.Contains(t, result, "switched to PostgreSQL")
	require.Contains(t, result, "<context name=\"observations\">")
}

func TestObservationInjectionEmpty(t *testing.T) {
	t.Parallel()

	ext := &PromptAssemblyExtension{active: true}
	lcmExt := &LCMExtension{
		active:  true,
		manager: &mockObservationManager{observations: ""},
	}
	ext.SetLCMExtension(lcmExt)

	result, err := ext.systemPromptModifier(context.Background(), "session-1", "base prompt")
	require.NoError(t, err)
	require.Contains(t, result, "base prompt")
	require.NotContains(t, result, "<context name=\"observations\">")
}

func TestObservationInjectionNoSession(t *testing.T) {
	t.Parallel()

	ext := &PromptAssemblyExtension{active: true}
	lcmExt := &LCMExtension{
		active:  true,
		manager: &mockObservationManager{observations: "should not appear"},
	}
	ext.SetLCMExtension(lcmExt)

	result, err := ext.systemPromptModifier(context.Background(), "", "base prompt")
	require.NoError(t, err)
	require.NotContains(t, result, "should not appear")
	require.NotContains(t, result, "<context name=\"observations\">")
}

type mockObservationManager struct {
	lcm.Manager
	observations string
}

func (m *mockObservationManager) GetObservationPrompt(_ context.Context, _ string, _ int64) (string, error) {
	return m.observations, nil
}

func (m *mockObservationManager) GetContextFiles() []lcm.ContextFile { return nil }
