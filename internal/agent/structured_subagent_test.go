package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestStructuredRequestJSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := StructuredRequest{
		Task:     "refactor the auth module",
		Context:  map[string]string{"language": "go", "module": "auth"},
		Tools:    []string{"bash", "edit", "view"},
		MaxSteps: 5,
		Timeout:  30 * time.Second,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded StructuredRequest
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, original.Task, decoded.Task)
	require.Equal(t, original.Context, decoded.Context)
	require.Equal(t, original.Tools, decoded.Tools)
	require.Equal(t, original.MaxSteps, decoded.MaxSteps)
	require.Equal(t, original.Timeout, decoded.Timeout)
}

func TestStructuredResponseJSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := StructuredResponse{
		Result:     "refactored successfully",
		Success:    true,
		StepsTaken: 3,
		Cost:       0.045,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded StructuredResponse
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, original.Result, decoded.Result)
	require.Equal(t, original.Success, decoded.Success)
	require.Equal(t, original.StepsTaken, decoded.StepsTaken)
	require.Equal(t, original.Cost, decoded.Cost)
	require.Empty(t, decoded.Error)
}

func TestStructuredResponseErrorJSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := StructuredResponse{
		Success: false,
		Error:   "task is required",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded StructuredResponse
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.False(t, decoded.Success)
	require.Equal(t, "task is required", decoded.Error)
	require.Empty(t, decoded.Result)
}

func TestStructuredRequestOmitsEmptyFields(t *testing.T) {
	t.Parallel()

	req := StructuredRequest{Task: "do the thing"}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	_, hasContext := raw["context"]
	_, hasTools := raw["tools"]
	require.False(t, hasContext, "context should be omitted when nil")
	require.False(t, hasTools, "tools should be omitted when nil")
}

func TestFilterToolsReturnsAllWhenAllowEmpty(t *testing.T) {
	t.Parallel()

	all := []fantasy.AgentTool{
		&fakeTool{name: "bash"},
		&fakeTool{name: "edit"},
		&fakeTool{name: "view"},
	}

	result := filterTools(all, nil)
	require.Len(t, result, 3)

	result = filterTools(all, []string{})
	require.Len(t, result, 3)
}

func TestFilterToolsFiltersByName(t *testing.T) {
	t.Parallel()

	all := []fantasy.AgentTool{
		&fakeTool{name: "bash"},
		&fakeTool{name: "edit"},
		&fakeTool{name: "view"},
		&fakeTool{name: "grep"},
	}

	result := filterTools(all, []string{"bash", "view"})
	require.Len(t, result, 2)
	require.Equal(t, "bash", result[0].Info().Name)
	require.Equal(t, "view", result[1].Info().Name)
}

func TestFilterToolsReturnsEmptyForNoMatch(t *testing.T) {
	t.Parallel()

	all := []fantasy.AgentTool{
		&fakeTool{name: "bash"},
		&fakeTool{name: "edit"},
	}

	result := filterTools(all, []string{"nonexistent"})
	require.Empty(t, result)
}

func TestBuildStructuredPromptWithoutContext(t *testing.T) {
	t.Parallel()

	req := StructuredRequest{Task: "write a test"}
	p := buildStructuredPrompt(req)
	require.Equal(t, "write a test", p)
}

func TestBuildStructuredPromptWithContext(t *testing.T) {
	t.Parallel()

	req := StructuredRequest{
		Task:    "write a test",
		Context: map[string]string{"lang": "go"},
	}
	p := buildStructuredPrompt(req)
	require.Contains(t, p, "Context:")
	require.Contains(t, p, "lang: go")
	require.Contains(t, p, "Task:\nwrite a test")
}

func TestBuildStructuredPromptWithMaxSteps(t *testing.T) {
	t.Parallel()

	req := StructuredRequest{
		Task:     "refactor",
		MaxSteps: 3,
	}
	p := buildStructuredPrompt(req)
	require.Contains(t, p, "at most 3 steps")
}

func TestBuildStructuredPromptWithContextAndMaxSteps(t *testing.T) {
	t.Parallel()

	req := StructuredRequest{
		Task:     "refactor",
		Context:  map[string]string{"file": "auth.go"},
		MaxSteps: 5,
	}
	p := buildStructuredPrompt(req)
	require.Contains(t, p, "Context:")
	require.Contains(t, p, "file: auth.go")
	require.Contains(t, p, "at most 5 steps")
}

func TestStructuredSubagentExecuteRejectsEmptyTask(t *testing.T) {
	t.Parallel()

	s := &structuredSubagent{
		coordinator:     &coordinator{},
		parentSessionID: "sess-1",
		agent:           nil,
		allTools:        nil,
	}

	resp, err := s.Execute(t.Context(), StructuredRequest{Task: ""})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "task is required", resp.Error)
}

func TestStructuredSubagentCapabilities(t *testing.T) {
	t.Parallel()

	tools := []fantasy.AgentTool{
		&fakeTool{name: "bash"},
		&fakeTool{name: "edit"},
		&fakeTool{name: "view"},
	}

	s := &structuredSubagent{
		coordinator:     &coordinator{},
		parentSessionID: "sess-1",
		allTools:        tools,
	}

	caps := s.Capabilities()
	require.Equal(t, []string{"bash", "edit", "view"}, caps)
}

func TestStructuredSubagentCapabilitiesEmpty(t *testing.T) {
	t.Parallel()

	s := &structuredSubagent{
		coordinator:     &coordinator{},
		parentSessionID: "sess-1",
		allTools:        nil,
	}

	caps := s.Capabilities()
	require.Nil(t, caps)
}

func TestNewStructuredSubagentFactoryRejectsNilCoordinator(t *testing.T) {
	t.Parallel()

	factory := NewStructuredSubagentFactory(nil)
	sub, err := factory.NewStructuredSubagent(t.Context(), "sess-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "coordinator is nil")
	require.Nil(t, sub)
}

func TestNewStructuredSubagentFactoryRejectsEmptySessionID(t *testing.T) {
	t.Parallel()

	factory := NewStructuredSubagentFactory(&coordinator{})
	sub, err := factory.NewStructuredSubagent(t.Context(), "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parent session ID is required")
	require.Nil(t, sub)
}

func TestWithStructuredSubagentFactoryOption(t *testing.T) {
	t.Parallel()

	stub := &stubSessionFactory{}
	c := &coordinator{}
	opt := WithStructuredSubagentFactory(stub)
	require.NotNil(t, opt)
	opt(c)
	require.Equal(t, stub, c.structuredSubagentFactory)
}

func TestWithStructuredSubagentFactoryOptionNil(t *testing.T) {
	t.Parallel()

	c := &coordinator{}
	opt := WithStructuredSubagentFactory(nil)
	require.NotNil(t, opt)
	opt(c)
	require.Nil(t, c.structuredSubagentFactory)
}

type stubSessionFactory struct {
	sub   StructuredSubagent
	err   error
	calls int
}

func (f *stubSessionFactory) NewStructuredSubagent(_ context.Context, parentSessionID string) (StructuredSubagent, error) {
	f.calls++
	return f.sub, f.err
}

func TestNewCoordinatorOptionsApplied(t *testing.T) {
	t.Parallel()

	stub := &stubSessionFactory{}
	c := &coordinator{}
	opts := []CoordinatorOption{
		WithStructuredSubagentFactory(stub),
	}
	for _, opt := range opts {
		opt(c)
	}

	require.Equal(t, stub, c.structuredSubagentFactory)
}

func TestNewCoordinatorNoOptionsNilSafe(t *testing.T) {
	t.Parallel()

	c := &coordinator{}
	var opts []CoordinatorOption
	for _, opt := range opts {
		opt(c)
	}
	require.Nil(t, c.structuredSubagentFactory)
}
