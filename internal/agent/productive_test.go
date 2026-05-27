package agent

import (
	"fmt"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestProductiveLoopNoLoopDetected(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	toolNames := []string{"read", "edit", "bash", "grep", "glob"}
	steps := make([]fantasy.StepResult, 10)
	for i := range 10 {
		steps[i] = makeToolStep(toolNames[i%len(toolNames)], fmt.Sprintf(`{"arg":"%d"}`, i), fmt.Sprintf("content-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationNone, result.Level)
	require.False(t, result.IsProductive)
}

func TestProductiveLoopDoomLoopSameOutput(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 5 {
		steps[i] = makeToolStep("edit", `{"file":"auth.go"}`, "error: permission denied")
	}
	for i := 5; i < 10; i++ {
		steps[i] = makeToolStep("bash", fmt.Sprintf(`{"command":"cmd%d"}`, i), "ok")
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationMedium, result.Level, "should escalate when outputs are identical")
	require.False(t, result.IsProductive)
	require.Equal(t, "edit", result.ToolName)
	require.Equal(t, 5, result.RepeatCount)
}

func TestProductiveLoopProductiveDifferentOutputs(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 5 {
		steps[i] = makeToolStep("edit", fmt.Sprintf(`{"file":"file%d.go"}`, i), fmt.Sprintf("fixed-bug-%d", i))
	}
	for i := 5; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("content-%d", i))
	}

	result := d.Detect(steps)
	require.True(t, result.IsProductive, "should detect productivity when outputs differ")
	require.Equal(t, EscalationNone, result.Level, "should not escalate productive loops")
	require.Equal(t, "edit", result.ToolName)
	require.True(t, result.UniqueOutputCnt > 1, "should have multiple unique outputs")
}

func TestProductiveLoopProductiveSameToolVaryingResults(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 7 {
		steps[i] = makeToolStep("bash", `{"command":"ls"}`, fmt.Sprintf("file-%d.txt file-%d.txt", i, i+1))
	}
	for i := 7; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "data")
	}

	result := d.Detect(steps)
	require.True(t, result.IsProductive)
	require.Equal(t, EscalationNone, result.Level)
	require.Equal(t, "bash", result.ToolName)
	require.Equal(t, 7, result.UniqueOutputCnt)
}

func TestProductiveLoopHardLoopSameOutput(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 8 {
		steps[i] = makeToolStep("grep", `{"pattern":"TODO"}`, "no matches")
	}
	for i := 8; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "data")
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationHard, result.Level, "hard escalation for identical outputs")
	require.False(t, result.IsProductive)
}

func TestProductiveLoopProductiveSuppressesHard(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 8 {
		steps[i] = makeToolStep("grep", fmt.Sprintf(`{"pattern":"TODO-%d"}`, i), fmt.Sprintf("found %d matches", i+1))
	}
	for i := 8; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "data")
	}

	result := d.Detect(steps)
	require.True(t, result.IsProductive)
	require.Equal(t, EscalationNone, result.Level, "productive loop should suppress hard escalation")
}

func TestProductiveLoopBelowThresholdNotProductive(t *testing.T) {
	t.Parallel()

	thresholds := DoomLoopThresholds{Soft: 4, Medium: 6, Hard: 8}
	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(thresholds, 6),
	)

	steps := make([]fantasy.StepResult, 6)
	steps[0] = makeToolStep("edit", `{"file":"a.go"}`, "output-0")
	steps[1] = makeToolStep("edit", `{"file":"a.go"}`, "output-1")
	steps[2] = makeToolStep("edit", `{"file":"a.go"}`, "output-2")
	steps[3] = makeToolStep("bash", `{"command":"ls"}`, "files")
	steps[4] = makeToolStep("bash", `{"command":"ls"}`, "files")
	steps[5] = makeToolStep("bash", `{"command":"ls"}`, "files")

	result := d.Detect(steps)
	require.Equal(t, EscalationNone, result.Level)
	require.False(t, result.IsProductive, "below threshold should not trigger productivity check")
}

func TestProductiveLoopMixedCallsSameToolDifferentOutput(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 4 {
		steps[i] = makeToolStep("bash", `{"command":"go test"}`, fmt.Sprintf("PASS %d/10 tests", i+1))
	}
	for i := 4; i < 10; i++ {
		steps[i] = makeToolStep("bash", fmt.Sprintf(`{"command":"build %d"}`, i), "ok")
	}

	result := d.Detect(steps)
	require.True(t, result.IsProductive)
	require.Equal(t, EscalationNone, result.Level)
}

func TestProductiveLoopFewerThanWindowSize(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 5)
	for i := range 5 {
		steps[i] = makeToolStep("edit", `{"file":"a.go"}`, fmt.Sprintf("result-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationNone, result.Level)
	require.False(t, result.IsProductive)
}

func TestProductiveLoopExactOutputRatio50Percent(t *testing.T) {
	t.Parallel()

	thresholds := DoomLoopThresholds{Soft: 3, Medium: 5, Hard: 7}
	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(thresholds, 6),
	)

	steps := make([]fantasy.StepResult, 6)
	steps[0] = makeToolStep("bash", `{"command":"run"}`, "output-a")
	steps[1] = makeToolStep("bash", `{"command":"run"}`, "output-b")
	steps[2] = makeToolStep("bash", `{"command":"run"}`, "output-a")
	steps[3] = makeToolStep("bash", `{"command":"run"}`, "output-c")
	steps[4] = makeToolStep("read", `{"file":"x.go"}`, "data")
	steps[5] = makeToolStep("read", `{"file":"y.go"}`, "data")

	result := d.Detect(steps)
	require.True(t, result.IsProductive, "50% unique ratio should be productive")
	require.Equal(t, EscalationNone, result.Level)
	require.Equal(t, "bash", result.ToolName)
}

func TestProductiveLoopBelowOutputRatioNotProductive(t *testing.T) {
	t.Parallel()

	thresholds := DoomLoopThresholds{Soft: 3, Medium: 5, Hard: 7}
	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(thresholds, 6),
	)

	steps := make([]fantasy.StepResult, 6)
	steps[0] = makeToolStep("bash", `{"command":"run"}`, "same-output")
	steps[1] = makeToolStep("bash", `{"command":"run"}`, "same-output")
	steps[2] = makeToolStep("bash", `{"command":"run"}`, "same-output")
	steps[3] = makeToolStep("bash", `{"command":"run"}`, "same-output")
	steps[4] = makeToolStep("read", `{"file":"x.go"}`, "data")
	steps[5] = makeToolStep("read", `{"file":"y.go"}`, "data")

	result := d.Detect(steps)
	require.False(t, result.IsProductive, "1 unique output out of 4 calls should not be productive")
}

func TestProductiveLoopMultipleToolsInWindow(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 5 {
		steps[i] = makeToolStep("edit", fmt.Sprintf(`{"file":"file%d.go"}`, i), fmt.Sprintf("result-%d", i))
	}
	for i := 5; i < 10; i++ {
		steps[i] = makeToolStep("bash", fmt.Sprintf(`{"command":"cmd%d"}`, i), "done")
	}

	result := d.Detect(steps)
	require.True(t, result.IsProductive)
}

func TestProductiveLoopResultPreservesDoomInfoWhenNotProductive(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 3 {
		steps[i] = makeToolStep("bash", `{"command":"ls"}`, "file1.go file2.go")
	}
	for i := 3; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "data")
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationSoft, result.Level)
	require.False(t, result.IsProductive)
	require.Equal(t, "bash", result.ToolName)
	require.Contains(t, result.Message, "SOFT LOOP")
}

func TestProductiveLoopEmptySteps(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	result := d.Detect(nil)
	require.Equal(t, EscalationNone, result.Level)
	require.False(t, result.IsProductive)
}

func TestProductiveLoopAllEmptySteps(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range steps {
		steps[i] = makeEmptyStep()
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationNone, result.Level)
	require.False(t, result.IsProductive)
}

func TestHashOutputDeterministic(t *testing.T) {
	t.Parallel()

	h1 := hashOutput("bash", "output-1")
	h2 := hashOutput("bash", "output-1")
	h3 := hashOutput("bash", "output-2")

	require.Equal(t, h1, h2, "same inputs should produce same hash")
	require.NotEqual(t, h1, h3, "different outputs should produce different hashes")
}

func TestHashOutputDifferentTools(t *testing.T) {
	t.Parallel()

	h1 := hashOutput("bash", "output")
	h2 := hashOutput("edit", "output")

	require.NotEqual(t, h1, h2, "different tools with same output should produce different hashes")
}

func TestProductiveLoopMultipleStepsPerCall(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 6),
	)

	steps := make([]fantasy.StepResult, 6)
	for i := range 4 {
		callID1 := fmt.Sprintf("call_edit_%d_1", i)
		callID2 := fmt.Sprintf("call_edit_%d_2", i)
		steps[i] = makeStep(
			[]fantasy.ToolCallContent{
				{ToolCallID: callID1, ToolName: "edit", Input: fmt.Sprintf(`{"file":"a%d.go"}`, i)},
				{ToolCallID: callID2, ToolName: "edit", Input: fmt.Sprintf(`{"file":"b%d.go"}`, i)},
			},
			[]fantasy.ToolResultContent{
				{ToolCallID: callID1, ToolName: "edit", Result: fantasy.ToolResultOutputContentText{Text: fmt.Sprintf("ok-%d-a", i)}},
				{ToolCallID: callID2, ToolName: "edit", Result: fantasy.ToolResultOutputContentText{Text: fmt.Sprintf("ok-%d-b", i)}},
			},
		)
	}
	steps[4] = makeToolStep("read", `{"file":"x.go"}`, "data")
	steps[5] = makeToolStep("read", `{"file":"y.go"}`, "data")

	result := d.Detect(steps)
	require.True(t, result.IsProductive)
	require.Equal(t, EscalationNone, result.Level)
	require.Equal(t, "edit", result.ToolName)
}
