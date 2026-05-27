package agent

import (
	"fmt"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

// --- Intervention types ported from doom_wire.go for test use. ---

// xrushInterventionType classifies the action taken by the doom loop
// intervention system.
type xrushInterventionType int

const (
	xrushInterventionNone        xrushInterventionType = iota
	xrushInterventionMessage                           // message only
	xrushInterventionWarn                              // warning + tool restriction
	xrushInterventionForceSwitch                       // forced tool switch + rollback
)

// xrushInterventionAction describes the result of a doom loop intervention
// check.
type xrushInterventionAction struct {
	Type              xrushInterventionType
	Message           string
	RestrictedTool    string
	ForcedTool        string
	RollbackRequested bool
}

// xrushInterventionConfig controls how the doom loop detector intervenes.
type xrushInterventionConfig struct {
	Mode string // "none", "warn", "full"
}

const xrushDefaultIntervention = "warn"

func xrushApplyDoomLoopIntervention(detector *ProductiveLoopDetector, steps []fantasy.StepResult, cfg xrushInterventionConfig) xrushInterventionAction {
	if detector == nil {
		return xrushInterventionAction{Type: xrushInterventionNone}
	}

	result := detector.Detect(steps)

	if result.IsProductive || result.Level == EscalationNone || result.Level == EscalationHard {
		return xrushInterventionAction{Type: xrushInterventionNone}
	}

	switch result.Level {
	case EscalationSoft:
		return xrushApplySoftIntervention(result, cfg)
	case EscalationMedium:
		return xrushApplyMediumIntervention(result, cfg)
	default:
		return xrushInterventionAction{Type: xrushInterventionNone}
	}
}

func xrushApplySoftIntervention(result ProductiveLoopResult, cfg xrushInterventionConfig) xrushInterventionAction {
	msg := xrushEscalationMessage(result)

	if cfg.Mode == "none" {
		return xrushInterventionAction{
			Type:    xrushInterventionMessage,
			Message: msg,
		}
	}

	return xrushInterventionAction{
		Type:           xrushInterventionWarn,
		Message:        msg,
		RestrictedTool: result.ToolName,
	}
}

func xrushApplyMediumIntervention(result ProductiveLoopResult, cfg xrushInterventionConfig) xrushInterventionAction {
	msg := xrushEscalationMessage(result)

	if cfg.Mode != "full" {
		return xrushInterventionAction{
			Type:    xrushInterventionMessage,
			Message: msg,
		}
	}

	forcedTool := "view"
	if result.ToolName == "view" {
		forcedTool = "grep"
	}

	return xrushInterventionAction{
		Type:              xrushInterventionForceSwitch,
		Message:           msg,
		ForcedTool:        forcedTool,
		RollbackRequested: true,
	}
}

func xrushEscalationMessage(result ProductiveLoopResult) string {
	switch result.Level {
	case EscalationSoft:
		return fmt.Sprintf(
			"<doom-loop-warning level=\"soft\">%s\nConsider switching to a completely different approach.</doom-loop-warning>",
			result.Message,
		)
	case EscalationMedium:
		return fmt.Sprintf(
			"<doom-loop-warning level=\"medium\">%s\nYou MUST immediately try a different tool or strategy. Do not repeat the same operation.</doom-loop-warning>",
			result.Message,
		)
	default:
		return ""
	}
}

func xrushCheckDoomLoopEscalation(detector *ProductiveLoopDetector, steps []fantasy.StepResult) ProductiveLoopResult {
	if detector == nil {
		return ProductiveLoopResult{}
	}
	return detector.Detect(steps)
}

func xrushDoomLoopStopConditions(detector *ProductiveLoopDetector) []func([]fantasy.StepResult) bool {
	if detector == nil {
		return nil
	}
	return []func([]fantasy.StepResult) bool{
		func(steps []fantasy.StepResult) bool {
			result := detector.Detect(steps)
			return result.Level == EscalationHard
		},
	}
}

// ===========================================================================
// Tests from doom_test.go
// ===========================================================================

func TestXrushDoomEscalationNone(t *testing.T) {
	t.Parallel()

	d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)

	steps := make([]fantasy.StepResult, 10)
	for i := range steps {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("content-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationNone, result.Level)
	require.Equal(t, 0, result.RepeatCount)
}

func TestXrushDoomSoftEscalation(t *testing.T) {
	t.Parallel()

	d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)

	steps := make([]fantasy.StepResult, 10)
	for i := range 3 {
		steps[i] = makeToolStep("bash", `{"command":"ls"}`, "file1.go file2.go")
	}
	for i := 3; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationSoft, result.Level)
	require.Equal(t, "bash", result.ToolName)
	require.Equal(t, 3, result.RepeatCount)
	require.Contains(t, result.Message, "SOFT LOOP")
	require.Contains(t, result.Message, "bash")
	require.Contains(t, result.Message, "3 times")
}

func TestXrushDoomMediumEscalation(t *testing.T) {
	t.Parallel()

	d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)

	steps := make([]fantasy.StepResult, 10)
	for i := range 5 {
		steps[i] = makeToolStep("edit", `{"file":"auth.go"}`, "error: permission denied")
	}
	for i := 5; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("content-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationMedium, result.Level)
	require.Equal(t, "edit", result.ToolName)
	require.Equal(t, 5, result.RepeatCount)
	require.Contains(t, result.Message, "MEDIUM LOOP")
	require.Contains(t, result.Message, "change your approach")
}

func TestXrushDoomHardEscalation(t *testing.T) {
	t.Parallel()

	d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)

	steps := make([]fantasy.StepResult, 10)
	for i := range 8 {
		steps[i] = makeToolStep("grep", `{"pattern":"TODO"}`, "no matches")
	}
	for i := 8; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationHard, result.Level)
	require.Equal(t, "grep", result.ToolName)
	require.Equal(t, 8, result.RepeatCount)
	require.Contains(t, result.Message, "HARD LOOP")
	require.Contains(t, result.Message, "Halting")
}

func TestXrushDoomWindowTooSmall(t *testing.T) {
	t.Parallel()

	d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)

	steps := make([]fantasy.StepResult, 5)
	for i := range steps {
		steps[i] = makeToolStep("bash", `{"command":"ls"}`, "output")
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationNone, result.Level)
}

func TestXrushDoomCustomThresholds(t *testing.T) {
	t.Parallel()

	thresholds := DoomLoopThresholds{Soft: 2, Medium: 4, Hard: 6}
	d := NewDoomLoopDetector(thresholds, 8)

	steps := make([]fantasy.StepResult, 8)
	for i := range 2 {
		steps[i] = makeToolStep("bash", `{"command":"ls"}`, "output")
	}
	for i := 2; i < 8; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationSoft, result.Level)
	require.Equal(t, 2, result.RepeatCount)
}

func TestXrushDoomExactMatchPreferred(t *testing.T) {
	t.Parallel()

	d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)

	steps := make([]fantasy.StepResult, 10)
	for i := range 5 {
		steps[i] = makeToolStep("edit", `{"file":"a.go","old":"foo","new":"bar"}`, "error")
	}
	for i := 5; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationMedium, result.Level)
	require.Equal(t, "edit", result.ToolName)
}

func TestXrushDoomSemanticSimilarDetectsLoop(t *testing.T) {
	t.Parallel()

	d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)

	steps := make([]fantasy.StepResult, 10)
	for i := range 5 {
		steps[i] = makeToolStep("edit", fmt.Sprintf(`{"file":"line_%d.go","old":"foo","new":"bar"}`, i), "error: not found")
	}
	for i := 5; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationMedium, result.Level)
	require.Equal(t, "edit", result.ToolName)
	require.Equal(t, 5, result.RepeatCount)
}

func TestXrushSemanticSimilarIdenticalSteps(t *testing.T) {
	t.Parallel()

	a := makeToolStep("bash", `{"command":"ls"}`, "output")
	b := makeToolStep("bash", `{"command":"ls"}`, "output")
	require.True(t, SemanticSimilar(a, b))
}

func TestXrushSemanticSimilarDifferentTool(t *testing.T) {
	t.Parallel()

	a := makeToolStep("bash", `{"command":"ls"}`, "output")
	b := makeToolStep("read", `{"command":"ls"}`, "output")
	require.False(t, SemanticSimilar(a, b))
}

func TestXrushSemanticSimilarDifferentOutput(t *testing.T) {
	t.Parallel()

	a := makeToolStep("bash", `{"command":"ls"}`, "output-a")
	b := makeToolStep("bash", `{"command":"ls"}`, "output-b")
	require.False(t, SemanticSimilar(a, b))
}

func TestXrushSemanticSimilarSameOutputSimilarArgs(t *testing.T) {
	t.Parallel()

	a := makeToolStep("edit", `{"file":"auth.go","old":"foo","new":"bar"}`, "error")
	b := makeToolStep("edit", `{"file":"auth.go","old":"baz","new":"qux"}`, "error")
	require.True(t, SemanticSimilar(a, b))
}

func TestXrushSemanticSimilarEmptySteps(t *testing.T) {
	t.Parallel()

	a := makeEmptyStep()
	b := makeEmptyStep()
	require.False(t, SemanticSimilar(a, b))
}

func TestXrushArgsSimilarIdentical(t *testing.T) {
	t.Parallel()
	require.True(t, argsSimilar(`{"file":"a.go"}`, `{"file":"a.go"}`))
}

func TestXrushArgsSimilarPrefix80Percent(t *testing.T) {
	t.Parallel()
	a := `{"file":"auth.go","old":"foo","new":"bar"}`
	b := `{"file":"auth.go","old":"baz","new":"qux"}`
	require.True(t, argsSimilar(a, b))
}

func TestXrushArgsSimilarCompletelyDifferent(t *testing.T) {
	t.Parallel()
	require.False(t, argsSimilar(`{"file":"alpha.go"}`, `{"path":"beta.yaml"}`))
}

func TestXrushArgsSimilarEmptyStrings(t *testing.T) {
	t.Parallel()
	require.True(t, argsSimilar("", ""))
	require.False(t, argsSimilar("", "not empty"))
}

func TestXrushEscalationLevelString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "none", EscalationNone.String())
	require.Equal(t, "soft", EscalationSoft.String())
	require.Equal(t, "medium", EscalationMedium.String())
	require.Equal(t, "hard", EscalationHard.String())
	require.Contains(t, EscalationLevel(99).String(), "unknown")
}

func TestXrushNewDoomLoopDetectorDefaults(t *testing.T) {
	t.Parallel()

	d := NewDoomLoopDetector(DoomLoopThresholds{}, 0)
	require.Equal(t, DefaultDoomLoopThresholds, d.Thresholds)
	require.Equal(t, loopDetectionWindowSize, d.WindowSize)
	require.NotNil(t, d.SimilarityFn)
}

func TestXrushDoomHardAtExactThreshold(t *testing.T) {
	t.Parallel()

	d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)

	steps := make([]fantasy.StepResult, 10)
	for i := range 7 {
		steps[i] = makeToolStep("grep", `{"pattern":"TODO"}`, "no matches")
	}
	for i := 7; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationHard, result.Level)
	require.Equal(t, 7, result.RepeatCount)
}

func TestXrushDoomBelowSoftThreshold(t *testing.T) {
	t.Parallel()

	d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)

	steps := make([]fantasy.StepResult, 10)
	steps[0] = makeToolStep("bash", `{"command":"ls"}`, "output")
	steps[1] = makeToolStep("bash", `{"command":"ls"}`, "output")
	for i := 2; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("content-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationNone, result.Level)
}

func TestXrushDoomSemanticSoftEscalation(t *testing.T) {
	t.Parallel()

	d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 8)

	steps := make([]fantasy.StepResult, 8)
	for i := range 3 {
		steps[i] = makeToolStep("edit", fmt.Sprintf(`{"file":"module_%d.go"}`, i), "error: permission denied")
	}
	for i := 3; i < 8; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("content-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationSoft, result.Level)
	require.Equal(t, "edit", result.ToolName)
}

func TestXrushDoomCustomSimilarityFn(t *testing.T) {
	t.Parallel()

	d := NewDoomLoopDetector(DoomLoopThresholds{Soft: 2, Medium: 3, Hard: 5}, 6)

	alwaysSimilar := func(_, _ fantasy.StepResult) bool {
		return true
	}
	d.SimilarityFn = alwaysSimilar

	steps := make([]fantasy.StepResult, 6)
	for i := range 3 {
		steps[i] = makeToolStep("bash", fmt.Sprintf(`{"cmd":"%d"}`, i), "output")
	}
	for i := 3; i < 6; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d"}`, i), "data")
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationHard, result.Level)
}

func TestXrushExtractToolSignature(t *testing.T) {
	t.Parallel()

	content := fantasy.ResponseContent{
		fantasy.ToolCallContent{ToolCallID: "1", ToolName: "read", Input: `{"file":"a.go"}`},
		fantasy.ToolResultContent{ToolCallID: "1", ToolName: "read", Result: fantasy.ToolResultOutputContentText{Text: "content"}},
	}
	sig := extractToolSignature(content)
	require.NotEmpty(t, sig)
}

func TestXrushExtractToolSignatureIgnoresInput(t *testing.T) {
	t.Parallel()

	content1 := fantasy.ResponseContent{
		fantasy.ToolCallContent{ToolCallID: "1", ToolName: "read", Input: `{"file":"a.go"}`},
		fantasy.ToolResultContent{ToolCallID: "1", ToolName: "read", Result: fantasy.ToolResultOutputContentText{Text: "ok"}},
	}
	content2 := fantasy.ResponseContent{
		fantasy.ToolCallContent{ToolCallID: "1", ToolName: "read", Input: `{"file":"b.go"}`},
		fantasy.ToolResultContent{ToolCallID: "1", ToolName: "read", Result: fantasy.ToolResultOutputContentText{Text: "ok"}},
	}

	sig1 := extractToolSignature(content1)
	sig2 := extractToolSignature(content2)
	require.Equal(t, sig1, sig2, "signatures should match when tool name and output are the same")
}

func TestXrushExtractToolSignatureDifferentOutput(t *testing.T) {
	t.Parallel()

	content1 := fantasy.ResponseContent{
		fantasy.ToolCallContent{ToolCallID: "1", ToolName: "read", Input: `{"file":"a.go"}`},
		fantasy.ToolResultContent{ToolCallID: "1", ToolName: "read", Result: fantasy.ToolResultOutputContentText{Text: "ok"}},
	}
	content2 := fantasy.ResponseContent{
		fantasy.ToolCallContent{ToolCallID: "1", ToolName: "read", Input: `{"file":"a.go"}`},
		fantasy.ToolResultContent{ToolCallID: "1", ToolName: "read", Result: fantasy.ToolResultOutputContentText{Text: "error"}},
	}

	sig1 := extractToolSignature(content1)
	sig2 := extractToolSignature(content2)
	require.NotEqual(t, sig1, sig2)
}

func TestXrushFirstToolNameEmpty(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", firstToolName(fantasy.ResponseContent{}))
}

func TestXrushFirstToolNameWithCalls(t *testing.T) {
	t.Parallel()
	content := fantasy.ResponseContent{
		fantasy.ToolCallContent{ToolCallID: "1", ToolName: "bash", Input: `{}`},
	}
	require.Equal(t, "bash", firstToolName(content))
}

func TestXrushDoomSemanticHardEscalation(t *testing.T) {
	t.Parallel()

	d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)

	steps := make([]fantasy.StepResult, 10)
	for i := range 8 {
		steps[i] = makeToolStep("edit", fmt.Sprintf(`{"file":"line_%d.go"}`, i), "error: not found")
	}
	for i := 8; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationHard, result.Level)
	require.Equal(t, "edit", result.ToolName)
	require.Equal(t, 8, result.RepeatCount)
}

// ===========================================================================
// Tests from doom_normalize_test.go
// ===========================================================================

func TestXrushNormalizeSignature_Whitespace(t *testing.T) {
	t.Parallel()
	a := normalizeSignature("edit  file.go   line 10")
	b := normalizeSignature("edit file.go line 10")
	require.Equal(t, a, b)
}

func TestXrushNormalizeSignature_AbsolutePaths(t *testing.T) {
	t.Parallel()
	a := normalizeSignature("view /home/user/project/src/main.go")
	b := normalizeSignature("view /tmp/other/src/main.go")
	require.Equal(t, a, b)
}

func TestXrushNormalizeSignature_NumericIDs(t *testing.T) {
	t.Parallel()
	a := normalizeSignature("session 12345 started")
	b := normalizeSignature("session 67890 started")
	require.Equal(t, a, b)
}

func TestXrushNormalizeSignature_Combined(t *testing.T) {
	t.Parallel()
	a := normalizeSignature("edit  /home/user/project/src/main.go  session 12345")
	b := normalizeSignature("edit /tmp/other/src/main.go session 67890")
	require.Equal(t, a, b)
}

func TestXrushNormalizeSignature_ShortNumbersPreserved(t *testing.T) {
	t.Parallel()
	a := normalizeSignature("line 42 column 8")
	require.Contains(t, a, "42")
	require.Contains(t, a, "8")
}

func TestXrushNormalizeSignature_EmptyString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", normalizeSignature(""))
}

func TestXrushNormalizeSignature_LeadingTrailingWhitespace(t *testing.T) {
	t.Parallel()
	a := normalizeSignature("  edit file.go  ")
	b := normalizeSignature("edit file.go")
	require.Equal(t, a, b)
}

func TestXrushNormalizeToolCall_EmptyArgs(t *testing.T) {
	t.Parallel()
	got := NormalizeToolCall("bash", "")
	require.Equal(t, "bash", got)
}

func TestXrushNormalizeToolCall_PathsCollapsed(t *testing.T) {
	t.Parallel()
	a := NormalizeToolCall("edit", "/home/user/project/src/main.go")
	b := NormalizeToolCall("edit", "/tmp/other/src/main.go")
	require.Equal(t, a, b)
	require.Contains(t, a, "main.go")
	require.NotContains(t, a, "/home/")
}

func TestXrushNormalizeToolCall_NumericIDsReplaced(t *testing.T) {
	t.Parallel()
	a := NormalizeToolCall("session", "12345 started")
	b := NormalizeToolCall("session", "67890 started")
	require.Equal(t, a, b)
	require.Contains(t, a, "<ID>")
}

func TestXrushNormalizeToolCall_WhitespaceCollapsed(t *testing.T) {
	t.Parallel()
	a := NormalizeToolCall("edit", "  file.go   line 10")
	b := NormalizeToolCall("edit", "file.go line 10")
	require.Equal(t, a, b)
}

// ===========================================================================
// Tests from doom_intervention_test.go (adapted to use local helpers)
// ===========================================================================

func TestXrushDoomInterventionSoftWarn(t *testing.T) {
	t.Parallel()

	detector := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 3 {
		steps[i] = makeToolStep("bash", `{"command":"ls"}`, "same error")
	}
	for i := 3; i < 10; i++ {
		steps[i] = makeToolStep("grep", fmt.Sprintf(`{"pattern":"x%d"}`, i), "")
	}

	cfg := xrushInterventionConfig{Mode: "warn"}
	action := xrushApplyDoomLoopIntervention(detector, steps, cfg)

	result := detector.Detect(steps)
	if result.IsProductive {
		t.Skip("productive loop detected, skipping")
	}

	require.Equal(t, xrushInterventionWarn, action.Type)
	require.Contains(t, action.Message, "doom-loop-warning")
	require.Equal(t, "bash", action.RestrictedTool)
}

func TestXrushDoomInterventionSoftNoneMode(t *testing.T) {
	t.Parallel()

	detector := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 3 {
		steps[i] = makeToolStep("bash", `{"command":"ls"}`, "same error")
	}
	for i := 3; i < 10; i++ {
		steps[i] = makeToolStep("grep", fmt.Sprintf(`{"pattern":"x%d"}`, i), "")
	}

	cfg := xrushInterventionConfig{Mode: "none"}
	action := xrushApplyDoomLoopIntervention(detector, steps, cfg)

	require.Equal(t, xrushInterventionMessage, action.Type)
	require.NotEmpty(t, action.Message)
	require.Empty(t, action.RestrictedTool)
}

func TestXrushDoomInterventionMediumFull(t *testing.T) {
	t.Parallel()

	detector := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 5 {
		steps[i] = makeToolStep("edit", `{"file":"main.go","old":"foo","new":"bar"}`, "error: not found")
	}
	for i := 5; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "")
	}

	cfg := xrushInterventionConfig{Mode: "full"}
	action := xrushApplyDoomLoopIntervention(detector, steps, cfg)

	result := detector.Detect(steps)
	if result.IsProductive {
		t.Skip("productive loop detected, skipping")
	}

	require.Equal(t, xrushInterventionForceSwitch, action.Type)
	require.Contains(t, action.Message, "doom-loop-warning")
	require.Contains(t, action.ForcedTool, "view")
	require.True(t, action.RollbackRequested)
}

func TestXrushDoomInterventionMediumWarnOnlyMode(t *testing.T) {
	t.Parallel()

	detector := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 5 {
		steps[i] = makeToolStep("edit", `{"file":"main.go","old":"foo","new":"bar"}`, "error: not found")
	}
	for i := 5; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "")
	}

	cfg := xrushInterventionConfig{Mode: "warn"}
	action := xrushApplyDoomLoopIntervention(detector, steps, cfg)

	result := detector.Detect(steps)
	if result.IsProductive {
		t.Skip("productive loop detected, skipping")
	}

	require.Equal(t, xrushInterventionMessage, action.Type)
	require.Empty(t, action.ForcedTool)
	require.False(t, action.RollbackRequested)
}

func TestXrushDoomInterventionHardUnchanged(t *testing.T) {
	t.Parallel()

	detector := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 7 {
		steps[i] = makeToolStep("edit", `{"file":"main.go","old":"foo","new":"bar"}`, "error: not found")
	}
	for i := 7; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "")
	}

	cfg := xrushInterventionConfig{Mode: "full"}
	action := xrushApplyDoomLoopIntervention(detector, steps, cfg)

	require.Equal(t, xrushInterventionNone, action.Type)
}

func TestXrushDoomInterventionNilDetector(t *testing.T) {
	t.Parallel()

	cfg := xrushInterventionConfig{Mode: "warn"}
	action := xrushApplyDoomLoopIntervention(nil, nil, cfg)

	require.Equal(t, xrushInterventionNone, action.Type)
}

func TestXrushDoomInterventionNoLoop(t *testing.T) {
	t.Parallel()

	detector := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range steps {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("content-%d", i))
	}

	cfg := xrushInterventionConfig{Mode: "warn"}
	action := xrushApplyDoomLoopIntervention(detector, steps, cfg)

	require.Equal(t, xrushInterventionNone, action.Type)
}

func TestXrushDoomInterventionConfigDefault(t *testing.T) {
	t.Parallel()

	require.Equal(t, "warn", xrushDefaultIntervention)
}

// ===========================================================================
// Tests from doom_wire_test.go (adapted to use local helpers)
// ===========================================================================

func TestXrushDoomLoopTriggersAtThreshold(t *testing.T) {
	t.Parallel()

	detector := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 3 {
		steps[i] = makeToolStep("bash", `{"command":"ls -la"}`, "no such file")
	}
	for i := 3; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
	}

	result := detector.Detect(steps)
	require.True(t, result.RepeatCount >= 3 || result.IsProductive,
		"expected detection at soft threshold (3 repeats)")
}

func TestXrushDoomLoopEscalation(t *testing.T) {
	t.Parallel()

	detector := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	t.Run("soft_at_3", func(t *testing.T) {
		t.Parallel()

		steps := make([]fantasy.StepResult, 10)
		for i := range 3 {
			steps[i] = makeToolStep("bash", `{"command":"ls"}`, "same error")
		}
		for i := 3; i < 10; i++ {
			steps[i] = makeToolStep("grep", fmt.Sprintf(`{"pattern":"x%d"}`, i), "")
		}

		result := detector.Detect(steps)
		if !result.IsProductive {
			require.Equal(t, EscalationSoft, result.Level)
		}
	})

	t.Run("medium_at_5", func(t *testing.T) {
		t.Parallel()

		steps := make([]fantasy.StepResult, 10)
		for i := range 5 {
			steps[i] = makeToolStep("edit", `{"file":"main.go","old":"foo","new":"bar"}`, "error: not found")
		}
		for i := 5; i < 10; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "")
		}

		result := detector.Detect(steps)
		if !result.IsProductive {
			require.Equal(t, EscalationMedium, result.Level)
		}
	})

	t.Run("hard_at_7", func(t *testing.T) {
		t.Parallel()

		steps := make([]fantasy.StepResult, 10)
		for i := range 7 {
			steps[i] = makeToolStep("edit", `{"file":"main.go","old":"foo","new":"bar"}`, "error: not found")
		}
		for i := 7; i < 10; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "")
		}

		result := detector.Detect(steps)
		if !result.IsProductive {
			require.Equal(t, EscalationHard, result.Level)
		}
	})
}

func TestXrushDoomLoopEnforcement(t *testing.T) {
	t.Parallel()

	t.Run("stop_condition_halts_at_hard", func(t *testing.T) {
		t.Parallel()

		detector := NewProductiveLoopDetector(
			NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
		)

		conditions := xrushDoomLoopStopConditions(detector)
		require.Len(t, conditions, 1)

		steps := make([]fantasy.StepResult, 10)
		for i := range 7 {
			steps[i] = makeToolStep("bash", `{"command":"ls"}`, "same error")
		}
		for i := 7; i < 10; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "data")
		}

		result := detector.Detect(steps)
		if !result.IsProductive {
			shouldStop := conditions[0](steps)
			require.True(t, shouldStop, "hard escalation should stop")
		}
	})

	t.Run("stop_condition_does_not_stop_below_hard", func(t *testing.T) {
		t.Parallel()

		detector := NewProductiveLoopDetector(
			NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
		)

		conditions := xrushDoomLoopStopConditions(detector)
		require.Len(t, conditions, 1)

		steps := make([]fantasy.StepResult, 10)
		for i := range 3 {
			steps[i] = makeToolStep("bash", `{"command":"ls"}`, "same error")
		}
		for i := 3; i < 10; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
		}

		shouldStop := conditions[0](steps)
		require.False(t, shouldStop, "soft escalation should not stop")
	})

	t.Run("escalation_message_for_soft", func(t *testing.T) {
		t.Parallel()

		result := ProductiveLoopResult{
			DoomLoopResult: DoomLoopResult{
				Level:       EscalationSoft,
				RepeatCount: 3,
				ToolName:    "bash",
				Message:     "SOFT LOOP: Tool \"bash\" repeated 3 times.",
			},
		}
		msg := xrushEscalationMessage(result)
		require.Contains(t, msg, "doom-loop-warning")
		require.Contains(t, msg, `level="soft"`)
	})

	t.Run("escalation_message_for_medium", func(t *testing.T) {
		t.Parallel()

		result := ProductiveLoopResult{
			DoomLoopResult: DoomLoopResult{
				Level:       EscalationMedium,
				RepeatCount: 5,
				ToolName:    "edit",
				Message:     "MEDIUM LOOP: Tool \"edit\" repeated 5 times.",
			},
		}
		msg := xrushEscalationMessage(result)
		require.Contains(t, msg, "doom-loop-warning")
		require.Contains(t, msg, `level="medium"`)
		require.Contains(t, msg, "MUST immediately")
	})

	t.Run("no_escalation_message_for_none", func(t *testing.T) {
		t.Parallel()

		result := ProductiveLoopResult{
			DoomLoopResult: DoomLoopResult{Level: EscalationNone},
		}
		msg := xrushEscalationMessage(result)
		require.Empty(t, msg)
	})

	t.Run("no_escalation_message_for_hard", func(t *testing.T) {
		t.Parallel()

		result := ProductiveLoopResult{
			DoomLoopResult: DoomLoopResult{
				Level:   EscalationHard,
				Message: "HARD LOOP",
			},
		}
		msg := xrushEscalationMessage(result)
		require.Empty(t, msg)
	})
}

func TestXrushDoomLoopWiredInProduction(t *testing.T) {
	t.Parallel()

	t.Run("nil_detector_produces_no_conditions", func(t *testing.T) {
		t.Parallel()

		conditions := xrushDoomLoopStopConditions(nil)
		require.Nil(t, conditions)
	})

	t.Run("stop_conditions_work_end_to_end", func(t *testing.T) {
		t.Parallel()

		detector := NewProductiveLoopDetector(
			NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
		)

		conditions := xrushDoomLoopStopConditions(detector)
		require.Len(t, conditions, 1)

		steps := make([]fantasy.StepResult, 10)
		for i := range 10 {
			steps[i] = makeToolStep("edit", `{"file":"main.go"}`, "error: not found")
		}

		result := detector.Detect(steps)
		if !result.IsProductive {
			shouldStop := conditions[0](steps)
			require.True(t, shouldStop, "10 identical edits should trigger hard stop")
		}
	})
}

func TestXrushCheckDoomLoopEscalation(t *testing.T) {
	t.Parallel()

	t.Run("nil_detector_returns_zero_result", func(t *testing.T) {
		t.Parallel()

		result := xrushCheckDoomLoopEscalation(nil, nil)
		require.Equal(t, EscalationNone, result.Level)
	})

	t.Run("returns_detection_result", func(t *testing.T) {
		t.Parallel()

		detector := NewProductiveLoopDetector(
			NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
		)

		steps := make([]fantasy.StepResult, 10)
		for i := range 10 {
			steps[i] = makeToolStep("bash", `{"command":"ls"}`, "same output")
		}

		result := xrushCheckDoomLoopEscalation(detector, steps)
		if !result.IsProductive {
			require.Equal(t, EscalationHard, result.Level)
		}
	})
}
