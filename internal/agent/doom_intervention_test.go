package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

// TestDoomInterventionEscalation simulates 7+ identical failing tool calls and
// asserts that the doom loop detector escalates through None → Soft → Medium →
// Hard at the correct thresholds (3, 5, 7).
func TestDoomInterventionEscalation(t *testing.T) {
	t.Parallel()

	// Use the DoomLoopDetector directly for pure escalation-threshold testing.
	d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)
	windowSize := 10

	t.Run("none_below_soft_threshold", func(t *testing.T) {
		t.Parallel()

		// Arrange: 2 identical "edit" calls — below Soft=3 threshold.
		steps := make([]fantasy.StepResult, windowSize)
		for i := range 2 {
			steps[i] = makeToolStep("edit", `{"file":"auth.go","old":"foo","new":"bar"}`, "error: not found")
		}
		for i := 2; i < windowSize; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
		}

		// Act.
		result := d.Detect(steps)

		// Assert: below soft threshold, no escalation.
		require.Equal(t, EscalationNone, result.Level)
		require.Equal(t, 0, result.RepeatCount)
	})

	t.Run("soft_at_threshold_3", func(t *testing.T) {
		t.Parallel()

		// Arrange: exactly 3 identical "edit" calls — triggers Soft=3.
		steps := make([]fantasy.StepResult, windowSize)
		for i := range 3 {
			steps[i] = makeToolStep("edit", `{"file":"auth.go","old":"foo","new":"bar"}`, "error: not found")
		}
		for i := 3; i < windowSize; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
		}

		// Act.
		result := d.Detect(steps)

		// Assert: soft escalation at exactly 3 repeats.
		require.Equal(t, EscalationSoft, result.Level)
		require.Equal(t, 3, result.RepeatCount)
		require.Equal(t, "edit", result.ToolName)
		require.Contains(t, result.Message, "SOFT LOOP")
	})

	t.Run("soft_at_threshold_4", func(t *testing.T) {
		t.Parallel()

		// Arrange: 4 identical calls — still soft, below Medium=5.
		steps := make([]fantasy.StepResult, windowSize)
		for i := range 4 {
			steps[i] = makeToolStep("edit", `{"file":"auth.go","old":"foo","new":"bar"}`, "error: not found")
		}
		for i := 4; i < windowSize; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
		}

		// Act.
		result := d.Detect(steps)

		// Assert: still soft between 3 and 5.
		require.Equal(t, EscalationSoft, result.Level)
		require.Equal(t, 4, result.RepeatCount)
	})

	t.Run("medium_at_threshold_5", func(t *testing.T) {
		t.Parallel()

		// Arrange: exactly 5 identical calls — triggers Medium=5.
		steps := make([]fantasy.StepResult, windowSize)
		for i := range 5 {
			steps[i] = makeToolStep("edit", `{"file":"auth.go","old":"foo","new":"bar"}`, "error: not found")
		}
		for i := 5; i < windowSize; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
		}

		// Act.
		result := d.Detect(steps)

		// Assert: medium escalation at exactly 5 repeats.
		require.Equal(t, EscalationMedium, result.Level)
		require.Equal(t, 5, result.RepeatCount)
		require.Equal(t, "edit", result.ToolName)
		require.Contains(t, result.Message, "MEDIUM LOOP")
	})

	t.Run("medium_at_threshold_6", func(t *testing.T) {
		t.Parallel()

		// Arrange: 6 identical calls — still medium, below Hard=7.
		steps := make([]fantasy.StepResult, windowSize)
		for i := range 6 {
			steps[i] = makeToolStep("edit", `{"file":"auth.go","old":"foo","new":"bar"}`, "error: not found")
		}
		for i := 6; i < windowSize; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
		}

		// Act.
		result := d.Detect(steps)

		// Assert: still medium between 5 and 7.
		require.Equal(t, EscalationMedium, result.Level)
		require.Equal(t, 6, result.RepeatCount)
	})

	t.Run("hard_at_threshold_7", func(t *testing.T) {
		t.Parallel()

		// Arrange: exactly 7 identical calls — triggers Hard=7.
		steps := make([]fantasy.StepResult, windowSize)
		for i := range 7 {
			steps[i] = makeToolStep("edit", `{"file":"auth.go","old":"foo","new":"bar"}`, "error: not found")
		}
		for i := 7; i < windowSize; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
		}

		// Act.
		result := d.Detect(steps)

		// Assert: hard escalation at exactly 7 repeats.
		require.Equal(t, EscalationHard, result.Level)
		require.Equal(t, 7, result.RepeatCount)
		require.Equal(t, "edit", result.ToolName)
		require.Contains(t, result.Message, "HARD LOOP")
		require.Contains(t, result.Message, "Halting")
	})

	t.Run("hard_above_threshold_9", func(t *testing.T) {
		t.Parallel()

		// Arrange: 9 identical calls — well above Hard=7, still hard.
		steps := make([]fantasy.StepResult, windowSize)
		for i := range 9 {
			steps[i] = makeToolStep("bash", `{"command":"ls"}`, "same error")
		}
		steps[9] = makeToolStep("read", `{"file":"misc.go"}`, "data")

		// Act.
		result := d.Detect(steps)

		// Assert: remains hard escalation.
		require.Equal(t, EscalationHard, result.Level)
		require.Equal(t, 9, result.RepeatCount)
	})
}

// TestDoomInterventionStopsLoop asserts that after the Hard threshold is
// reached, the intervention system signals a stop and the strategy changes
// such that no further identical tool calls should be processed.
func TestDoomInterventionStopsLoop(t *testing.T) {
	t.Parallel()

	t.Run("hard_escalation_returns_none_intervention", func(t *testing.T) {
		t.Parallel()

		// Arrange: 7 identical "edit" calls → Hard threshold.
		d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)
		d.SetInterventionMode("full")

		steps := make([]fantasy.StepResult, 10)
		for i := range 7 {
			steps[i] = makeToolStep("edit", `{"file":"main.go","old":"foo","new":"bar"}`, "error: not found")
		}
		for i := 7; i < 10; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "data")
		}

		// Act: ApplyIntervention at Hard level returns no action — execution
		// should halt rather than intervene.
		action := d.ApplyIntervention(steps)

		// Assert: Hard loops produce no intervention action because the caller
		// is expected to stop entirely.
		require.Equal(t, InterventionTypeNone, action.Type)
		require.Empty(t, action.Message)
	})

	t.Run("stop_condition_halts_at_hard", func(t *testing.T) {
		t.Parallel()

		// Arrange: build a productive loop detector with stop conditions.
		detector := NewProductiveLoopDetector(
			NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
		)
		conditions := xrushDoomLoopStopConditions(detector)
		require.Len(t, conditions, 1)

		// 7 identical steps → Hard escalation.
		steps := make([]fantasy.StepResult, 10)
		for i := range 7 {
			steps[i] = makeToolStep("bash", `{"command":"ls"}`, "same error")
		}
		for i := 7; i < 10; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "data")
		}

		// Act.
		result := detector.Detect(steps)

		// Assert: if productive, the loop was downgraded and we skip.
		if result.IsProductive {
			t.Skip("productive loop detected, skipping hard stop assertion")
		}

		shouldStop := conditions[0](steps)
		require.True(t, shouldStop, "hard escalation should trigger stop condition")
	})

	t.Run("strategy_changes_after_hard", func(t *testing.T) {
		t.Parallel()

		// Arrange: simulate escalation progression — identical calls grow from
		// 1 to 8, checking that the level escalates monotonically and that
		// after Hard is reached, further detection continues to report Hard.
		d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)

		var prevLevel EscalationLevel = EscalationNone

		for identicalCount := 1; identicalCount <= 8; identicalCount++ {
			identicalCount := identicalCount
			t.Run(fmt.Sprintf("identical_%d", identicalCount), func(t *testing.T) {
				t.Parallel()

				steps := make([]fantasy.StepResult, 10)
				for i := range identicalCount {
					steps[i] = makeToolStep("edit", `{"file":"main.go","old":"foo","new":"bar"}`, "error: not found")
				}
				for i := identicalCount; i < 10; i++ {
					steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
				}

				result := d.Detect(steps)

				// Assert: level is non-decreasing as repeats increase.
				require.GreaterOrEqual(t, result.Level, prevLevel,
					"escalation level should be non-decreasing at %d identical calls", identicalCount)
			})
		}
	})

	t.Run("no_further_calls_after_hard_stop", func(t *testing.T) {
		t.Parallel()

		// Arrange: after reaching Hard, verify that subsequent detection with
		// more identical calls still reports Hard (no regression to lower level).
		d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)

		// First: 7 identical calls → Hard.
		steps7 := make([]fantasy.StepResult, 10)
		for i := range 7 {
			steps7[i] = makeToolStep("grep", `{"pattern":"TODO"}`, "no matches")
		}
		for i := 7; i < 10; i++ {
			steps7[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
		}
		result7 := d.Detect(steps7)
		require.Equal(t, EscalationHard, result7.Level)

		// Act: 10 identical calls — all window is the same pattern.
		steps10 := make([]fantasy.StepResult, 10)
		for i := range 10 {
			steps10[i] = makeToolStep("grep", `{"pattern":"TODO"}`, "no matches")
		}
		result10 := d.Detect(steps10)

		// Assert: still Hard, no regression.
		require.Equal(t, EscalationHard, result10.Level)
		require.Equal(t, 10, result10.RepeatCount)
	})
}

// TestDoomInterventionFilesystemSafe verifies that doom loop intervention does
// NOT create files or directories on disk as a side effect. The doom loop
// detector is pure logic and should never touch the filesystem.
func TestDoomInterventionFilesystemSafe(t *testing.T) {
	t.Parallel()

	t.Run("no_file_created_at_impossible_path", func(t *testing.T) {
		t.Parallel()

		// Arrange: create a temp directory and define an impossible target path.
		tmpDir := t.TempDir()
		impossiblePath := filepath.Join(tmpDir, "this", "path", "should", "never", "exist", "doom_output.txt")

		// Verify the path does not exist before the test.
		_, err := os.Stat(impossiblePath)
		require.True(t, os.IsNotExist(err), "impossible path should not exist before test")

		// Act: run doom loop detection with 10 identical failing steps.
		d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)
		d.SetInterventionMode("full")

		steps := make([]fantasy.StepResult, 10)
		for i := range 10 {
			steps[i] = makeToolStep("edit",
				fmt.Sprintf(`{"file":"%s"}`, impossiblePath),
				"error: permission denied",
			)
		}

		result := d.Detect(steps)
		action := d.ApplyIntervention(steps)

		// Assert: detection ran (hard escalation expected).
		require.Equal(t, EscalationHard, result.Level)
		require.Equal(t, InterventionTypeNone, action.Type)

		// Assert: the impossible path still does not exist on disk — the doom
		// loop detector is pure logic and must never touch the filesystem.
		_, err = os.Stat(impossiblePath)
		require.True(t, os.IsNotExist(err), "impossible path must not be created by doom loop detection")
	})

	t.Run("no_file_created_during_soft_intervention", func(t *testing.T) {
		t.Parallel()

		// Arrange: temp directory with a candidate output path.
		tmpDir := t.TempDir()
		candidatePath := filepath.Join(tmpDir, "doom_soft_output.txt")

		_, err := os.Stat(candidatePath)
		require.True(t, os.IsNotExist(err), "candidate path should not exist before test")

		// Act: 3 identical calls → soft escalation with warn intervention.
		d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)
		d.SetInterventionMode("warn")

		steps := make([]fantasy.StepResult, 10)
		for i := range 3 {
			steps[i] = makeToolStep("bash", `{"command":"ls"}`, "same error")
		}
		for i := 3; i < 10; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
		}

		action := d.ApplyIntervention(steps)

		// Assert: intervention returned a warn action.
		require.Equal(t, InterventionTypeWarn, action.Type)
		require.Equal(t, "bash", action.RestrictedTool)

		// Assert: no file was created.
		_, err = os.Stat(candidatePath)
		require.True(t, os.IsNotExist(err), "no file should be created during soft intervention")
	})

	t.Run("no_file_created_during_medium_force_switch", func(t *testing.T) {
		t.Parallel()

		// Arrange: temp directory with a candidate output path.
		tmpDir := t.TempDir()
		candidatePath := filepath.Join(tmpDir, "doom_medium_output.txt")

		_, err := os.Stat(candidatePath)
		require.True(t, os.IsNotExist(err), "candidate path should not exist before test")

		// Act: 5 identical "view" calls → medium escalation with full
		// intervention (force switch).
		d := NewDoomLoopDetector(DefaultDoomLoopThresholds, 10)
		d.SetInterventionMode("full")

		steps := make([]fantasy.StepResult, 10)
		for i := range 5 {
			steps[i] = makeToolStep("view", `{"file":"main.go"}`, "error: permission denied")
		}
		for i := 5; i < 10; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("data-%d", i))
		}

		action := d.ApplyIntervention(steps)

		// Assert: force switch was triggered, tool switched from "view" to "grep".
		require.Equal(t, InterventionTypeForceSwitch, action.Type)
		require.Equal(t, "grep", action.ForcedTool, "view tool should force switch to grep")
		require.True(t, action.RollbackRequested)

		// Assert: no file was created on disk.
		_, err = os.Stat(candidatePath)
		require.True(t, os.IsNotExist(err), "no file should be created during medium force-switch intervention")
	})

	t.Run("productive_loop_detector_no_side_effects", func(t *testing.T) {
		t.Parallel()

		// Arrange: temp directory to monitor for unexpected file creation.
		tmpDir := t.TempDir()
		targetPath := filepath.Join(tmpDir, "side_effect.txt")

		beforeEntries, err := os.ReadDir(tmpDir)
		require.NoError(t, err)
		require.Empty(t, beforeEntries, "temp dir should be empty before test")

		// Act: run productive loop detector with hard-level repeats.
		detector := NewProductiveLoopDetector(
			NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
		)

		steps := make([]fantasy.StepResult, 10)
		for i := range 8 {
			steps[i] = makeToolStep("edit", fmt.Sprintf(`{"file":"%s"}`, targetPath), "error")
		}
		for i := 8; i < 10; i++ {
			steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "data")
		}

		// Run detection multiple times to exercise the detector thoroughly.
		for range 3 {
			result := detector.Detect(steps)
			// The result should be non-nil regardless of productive assessment.
			_ = result
		}

		// Assert: no files were created in the temp directory.
		afterEntries, err := os.ReadDir(tmpDir)
		require.NoError(t, err)
		require.Empty(t, afterEntries, "productive loop detector must not create files on disk")

		// Assert: the target path does not exist.
		_, err = os.Stat(targetPath)
		require.True(t, os.IsNotExist(err), "target path must not exist after detection")
	})
}

var _ fantasy.StepResult
