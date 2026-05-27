package processor

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPIIDetector_ID(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	require.Equal(t, "pii_detector", d.ID())
}

func TestPIIDetector_SSNDetection(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("My SSN is 123-45-6789 please help")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Equal(t, "My SSN is [REDACTED_SSN] please help", result.Messages[0].Content)

	require.Equal(t, true, result.State["pii_found"])
	types := result.State["pii_types"].([]string)
	require.Contains(t, types, "ssn")
	require.Equal(t, 1, result.State["redacted_count"])
}

func TestPIIDetector_EmailDetection(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Contact me at alice@example.com thanks")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Equal(t, "Contact me at [REDACTED_EMAIL] thanks", result.Messages[0].Content)
}

func TestPIIDetector_PhoneDetection(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Call me at 555-012-3456 today")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Contains(t, result.Messages[0].Content, "[REDACTED_PHONE]")
}

func TestPIIDetector_CreditCardDetection(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Card number 4111-1111-1111-1111 on file")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Contains(t, result.Messages[0].Content, "[REDACTED_CREDIT_CARD]")
	types := result.State["pii_types"].([]string)
	require.Contains(t, types, "credit_card")
}

func TestPIIDetector_CleanInputPassthrough(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Please help me refactor the auth module")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, "Please help me refactor the auth module", result.Messages[0].Content)
	require.Equal(t, false, result.State["pii_found"])
}

func TestPIIDetector_EmptyInput(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Empty(t, result.Messages)
	require.Equal(t, false, result.State["pii_found"])
}

func TestPIIDetector_EmptyContent(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, "", result.Messages[0].Content)
}

func TestPIIDetector_MultiplePIITypes(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{UserMessage(
			"SSN: 123-45-6789, email: alice@example.com, phone: 555-012-3456, card: 4111111111111111",
		)},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Equal(t, true, result.State["pii_found"])
	require.Equal(t, 4, result.State["redacted_count"])

	types := result.State["pii_types"].([]string)
	require.Contains(t, types, "ssn")
	require.Contains(t, types, "email")
	require.Contains(t, types, "phone")
	require.Contains(t, types, "credit_card")

	content := result.Messages[0].Content
	require.Contains(t, content, "[REDACTED_SSN]")
	require.Contains(t, content, "[REDACTED_EMAIL]")
	require.Contains(t, content, "[REDACTED_PHONE]")
	require.Contains(t, content, "[REDACTED_CREDIT_CARD]")
}

func TestPIIDetector_SensitivityLow_SSNOnly(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityLow, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{UserMessage(
			"My email is bob@example.com and SSN is 987-65-4321",
		)},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)

	content := result.Messages[0].Content
	require.Contains(t, content, "[REDACTED_SSN]")
	require.Contains(t, content, "bob@example.com",
		"low sensitivity should NOT redact email")
	require.NotContains(t, content, "[REDACTED_EMAIL]",
		"low sensitivity should NOT redact email")
}

func TestPIIDetector_SensitivityHigh_WithLLM(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"redacted": "My name is [REDACTED_NAME] and I live at [REDACTED_ADDRESS]",
		"types":    []string{"name", "address"},
		"count":    2,
	})

	mock := &MockLLMClient{Response: string(llmResp)}
	d := NewPIIDetector(SensitivityHigh, mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{UserMessage(
			"My name is John Doe and I live at 123 Main St",
		)},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Equal(t, true, result.State["pii_found"])

	require.Len(t, mock.Calls, 1)
	require.Equal(t, "My name is John Doe and I live at 123 Main St", mock.Calls[0].Input)
}

func TestPIIDetector_SensitivityHigh_LLMError(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{Err: context.DeadlineExceeded}
	d := NewPIIDetector(SensitivityHigh, mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Clean content here")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, "Clean content here", result.Messages[0].Content)
}

func TestPIIDetector_SensitivityHigh_LLMInvalidJSON(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{Response: "not valid json"}
	d := NewPIIDetector(SensitivityHigh, mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Clean content here")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, "Clean content here", result.Messages[0].Content)
}

func TestPIIDetector_ProcessOutputStream(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{AssistantMessage("User email is alice@example.com")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Contains(t, result.Messages[0].Content, "[REDACTED_EMAIL]")
}

func TestPIIDetector_ProcessOutputResult(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputResultPhase,
		Messages: []Message{AssistantMessage("SSN on file: 111-22-3333")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessOutputResult(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Contains(t, result.Messages[0].Content, "[REDACTED_SSN]")
}

func TestPIIDetector_ProcessAPIError_PassThrough(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	msgs := []Message{UserMessage("My SSN is 123-45-6789")}
	pctx := ProcessorContext{
		Phase:    APIErrorPhase,
		Messages: msgs,
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessAPIError(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
}

func TestPIIDetector_MultipleMessages(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			UserMessage("My email is alice@example.com"),
			AssistantMessage("I see your email."),
			UserMessage("SSN: 123-45-6789"),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Len(t, result.Messages, 3)
	require.Contains(t, result.Messages[0].Content, "[REDACTED_EMAIL]")
	require.Equal(t, "I see your email.", result.Messages[1].Content)
	require.Contains(t, result.Messages[2].Content, "[REDACTED_SSN]")
	require.Equal(t, 2, result.State["redacted_count"])
}

func TestPIIDetector_RunAllPhases(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			UserMessage("My SSN is 123-45-6789"),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	final := RunAllPhases(t, d, pctx)
	require.Len(t, final.Messages, 1)
	require.Contains(t, final.Messages[0].Content, "[REDACTED_SSN]")
}

func TestPIIDetector_SensitivityLow_IgnoresEmailAndPhone(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityLow, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{UserMessage(
			"Call 555-123-4567 or email test@test.com",
		)},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action,
		"low sensitivity should not detect email or phone")
	require.Equal(t, false, result.State["pii_found"])
}

func TestPIIDetector_StateContainsPIIFields(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("SSN: 111-22-3333")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)

	_, hasFound := result.State["pii_found"]
	_, hasTypes := result.State["pii_types"]
	_, hasCount := result.State["redacted_count"]
	require.True(t, hasFound, "state should contain pii_found")
	require.True(t, hasTypes, "state should contain pii_types")
	require.True(t, hasCount, "state should contain redacted_count")
}

func TestPIIDetector_CreditCardWithSpaces(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Card: 4111 1111 1111 1111")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Contains(t, result.Messages[0].Content, "[REDACTED_CREDIT_CARD]")
}

func TestPIIDetector_PhoneWithParens(t *testing.T) {
	t.Parallel()
	d := NewPIIDetector(SensitivityMedium, nil)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Call (555) 123-4567 now")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Contains(t, result.Messages[0].Content, "[REDACTED_PHONE]")
}
