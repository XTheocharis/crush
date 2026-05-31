//go:build treesitter

package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/charmbracelet/crush/internal/treesitter"
)

// ValidationHandlerConfig controls ValidationHandler behavior.
type ValidationHandlerConfig struct {
	Enabled bool

	AutoFix bool

	PipelineConfig PipelineConfig
}

// ValidationHandlerResult captures the full outcome of the edit validation
// flow.
type ValidationHandlerResult struct {
	PipelineResult *PipelineResult

	AutoFixResult *AutoFixResult

	RolledBack bool

	FinalContent string
}

// ValidationHandler orchestrates the full edit validation flow:
//  1. Capture pre-edit snapshot via RollbackManager
//  2. Apply edit (handled by caller)
//  3. Run validation pipeline (12 stages)
//  4. If pipeline fails and auto-fix is enabled, run AutoFixLoop
//  5. If auto-fix fails or is disabled, rollback via RollbackManager
//  6. Post-rollback: verify diagnostics against pre-edit baseline
//  7. Return comprehensive result
//
// It is optional — when config.Enabled is false, ValidateEdit is a no-op.
type ValidationHandler struct {
	config    ValidationHandlerConfig
	pipeline  *ValidationPipeline
	rollback  *RollbackManager
	autofixer *AutoFixer
	diagGate  *DiagnosticGate
}

// NewValidationHandler creates a new ValidationHandler. The parser may be nil
// (ParseCheck stage will be skipped). When cfg.Enabled is false, the handler
// is inert.
func NewValidationHandler(
	parser interface{},
	diagGate *DiagnosticGate,
	cfg ValidationHandlerConfig,
) *ValidationHandler {
	vh := &ValidationHandler{
		config:   cfg,
		rollback: NewRollbackManager(),
		diagGate: diagGate,
	}

	if !cfg.Enabled {
		return vh
	}

	tsParser, _ := parser.(treesitter.Parser)
	vh.pipeline = NewValidationPipeline(tsParser)

	if diagGate != nil {
		for i, stage := range vh.pipeline.stages {
			if _, ok := stage.(*DiagnosticsStage); ok {
				vh.pipeline.stages[i] = &DiagnosticsStage{gate: diagGate}
				break
			}
		}
	}

	if cfg.AutoFix {
		for i, stage := range vh.pipeline.stages {
			if _, ok := stage.(*FormatStage); ok {
				vh.pipeline.stages[i] = &FormatStageAutoFix{AutoFix: true}
				break
			}
		}
	}

	return vh
}

func (vh *ValidationHandler) Enabled() bool {
	return vh.config.Enabled
}

// CaptureSnapshot captures the pre-edit state of the given file paths via the
// RollbackManager.
func (vh *ValidationHandler) CaptureSnapshot(filePaths []string) (*Snapshot, error) {
	if !vh.config.Enabled {
		return nil, nil
	}
	return vh.rollback.Capture(filePaths)
}

// ValidateEdit runs the full validation flow for an edit that has already been
// applied to disk.
func (vh *ValidationHandler) ValidateEdit(
	ctx context.Context,
	snapshot *Snapshot,
	filePath string,
	originalContent string,
	newContent string,
	editSpec EditSpec,
) (*ValidationHandlerResult, error) {
	if !vh.config.Enabled {
		return nil, nil
	}

	result := &ValidationHandlerResult{}

	anchorMap := loadAnchorMap(filePath)

	input := &ValidationInput{
		FilePath:  filePath,
		Content:   originalContent,
		AnchorMap: anchorMap,
		EditSpec:  editSpec,
	}

	pipelineResult := vh.pipeline.RunWithConfig(ctx, input, vh.config.PipelineConfig)
	result.PipelineResult = pipelineResult

	if input.editedContent != "" {
		result.FinalContent = input.editedContent
	} else {
		result.FinalContent = newContent
	}

	if pipelineResult.OverallStatus == StatusPass {
		return result, nil
	}

	if vh.config.AutoFix && vh.autofixer != nil {
		slog.Debug("ValidationHandler: pipeline failed, attempting auto-fix",
			"file", filePath,
			"failedStages", countFailedStages(pipelineResult),
		)

		if err := os.WriteFile(filePath, []byte(result.FinalContent), 0o644); err != nil {
			return nil, fmt.Errorf("validation handler: write for auto-fix: %w", err)
		}

		autoFixResult, err := vh.autofixer.Run(ctx, filePath)
		if err != nil {
			slog.Debug("ValidationHandler: auto-fix error", "error", err)
		}
		result.AutoFixResult = &autoFixResult

		if fixed, err := os.ReadFile(filePath); err == nil {
			result.FinalContent = string(fixed)
		}

		if len(autoFixResult.RemainingErrors) == 0 && err == nil {
			slog.Debug("ValidationHandler: auto-fix resolved all errors")
			return result, nil
		}

		slog.Debug("ValidationHandler: auto-fix did not resolve all errors",
			"remaining", len(autoFixResult.RemainingErrors),
		)
	}

	if snapshot != nil {
		slog.Debug("ValidationHandler: rolling back", "file", filePath)
		if err := vh.rollback.Restore(snapshot); err != nil {
			return nil, fmt.Errorf("validation handler: rollback failed: %w", err)
		}
		result.RolledBack = true
		result.FinalContent = originalContent

		vh.verifyPostRollbackDiagnostics(ctx, snapshot)
	}

	return result, nil
}

func (vh *ValidationHandler) SetAutoFixer(af *AutoFixer) {
	vh.autofixer = af
}

func (vh *ValidationHandler) ReplaceStages(stages []ValidationStage) {
	if vh.pipeline != nil {
		vh.pipeline.stages = stages
	}
}

func countFailedStages(pr *PipelineResult) int {
	n := 0
	for _, sr := range pr.StageResults {
		if sr.Status == StatusFail || sr.Status == StatusError {
			n++
		}
	}
	return n
}

func (vh *ValidationHandler) verifyPostRollbackDiagnostics(ctx context.Context, snapshot *Snapshot) {
	if vh.diagGate == nil || snapshot == nil {
		return
	}

	filePaths := make([]string, len(snapshot.Files))
	for i, f := range snapshot.Files {
		filePaths[i] = f.FilePath
	}

	vh.diagGate.CaptureBaseline(ctx, filePaths)
	gateResult := vh.diagGate.Compare(ctx, filePaths)

	if len(gateResult.NewErrors) > 0 {
		slog.Warn("Post-rollback diagnostics show new errors",
			"newErrors", len(gateResult.NewErrors),
		)
		for _, ne := range gateResult.NewErrors {
			slog.Debug("Post-rollback new error",
				"file", ne.FilePath,
				"line", ne.Line,
				"message", ne.Message,
			)
		}
	} else {
		slog.Info("Post-rollback diagnostics clean")
	}
}
