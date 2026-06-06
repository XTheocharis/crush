//go:build !treesitter

package tools

import (
	"context"
	"fmt"
	"slices"
)

type StageStatus string

const (
	StatusPass  StageStatus = "pass"
	StatusFail  StageStatus = "fail"
	StatusSkip  StageStatus = "skip"
	StatusError StageStatus = "error"
)

// ValidationInput carries all data needed by the validation pipeline.
type ValidationInput struct {
	FilePath         string
	Content          string
	AnchorMap        *AnchorMap
	EditSpec         EditSpec
	Snapshot         string
	editedContent    string
	updatedAnchorMap *AnchorMap
}

// EditSpec describes a single edit operation for validation.
type EditSpec struct {
	OldString     string
	NewString     string
	ReplaceAll    bool
	PositionHints []string
}

// StageResult is the outcome of a single validation stage.
type StageResult struct {
	StageName string
	Status    StageStatus
	Message   string
	Warnings  []string
}

// PipelineResult is the aggregate result of running the full pipeline.
type PipelineResult struct {
	OverallStatus    StageStatus
	StageResults     []StageResult
	Snapshot         string
	EditedContent    string
	UpdatedAnchorMap *AnchorMap
}

// ValidationStage is the interface every pipeline stage must implement.
type ValidationStage interface {
	Name() string
	Execute(ctx context.Context, input *ValidationInput) (*StageResult, error)
	CanSkip() bool
}

// PipelineConfig controls pipeline execution behavior.
type PipelineConfig struct {
	SkipStages   []string
	FailFast     bool
	StageTimeout any
}

//nolint:unused
func (c *PipelineConfig) shouldSkip(name string) bool {
	return slices.Contains(c.SkipStages, name)
}

// stubPipeline is returned by NewValidationPipeline when treesitter is not
// available. It records a single "unavailable" stage result.
type stubPipeline struct{}

// NewValidationPipeline returns a stub pipeline when treesitter is not
// available.
func NewValidationPipeline(_ any) *stubPipeline {
	return &stubPipeline{}
}

func (p *stubPipeline) AddStage(_ ValidationStage) {}

func (p *stubPipeline) InsertStage(_ int, _ ValidationStage) {}

func (p *stubPipeline) Stages() []ValidationStage { return nil }

func (p *stubPipeline) Run(_ context.Context, _ *ValidationInput) *PipelineResult {
	return &PipelineResult{
		OverallStatus: StatusSkip,
		StageResults: []StageResult{{
			StageName: "ValidationPipeline",
			Status:    StatusSkip,
			Message:   "validation pipeline not available: treesitter not enabled",
		}},
	}
}

func (p *stubPipeline) RunWithConfig(_ context.Context, _ *ValidationInput, _ PipelineConfig) *PipelineResult {
	return p.Run(context.TODO(), nil)
}

// ValidationHandler stub — returns "not available" errors when treesitter is
// not enabled.

// ValidationHandlerConfig controls ValidationHandler behavior.
type ValidationHandlerConfig struct {
	Enabled        bool
	AutoFix        bool
	PipelineConfig PipelineConfig
}

// ValidationHandlerResult captures the full outcome of the edit validation
// flow.
type ValidationHandlerResult struct {
	PipelineResult *PipelineResult
	AutoFixResult  *AutoFixResult
	RolledBack     bool
	FinalContent   string
}

// ValidationHandler orchestrates the full edit validation flow.
// When treesitter is not available, all methods return "not available" errors.
type ValidationHandler struct {
	config   ValidationHandlerConfig
	rollback *RollbackManager
}

// NewValidationHandler creates a stub ValidationHandler when treesitter is not
// available.
func NewValidationHandler(
	_ any,
	_ *DiagnosticGate,
	cfg ValidationHandlerConfig,
) *ValidationHandler {
	return &ValidationHandler{
		config:   cfg,
		rollback: NewRollbackManager(),
	}
}

func (vh *ValidationHandler) Enabled() bool { return false }

func (vh *ValidationHandler) CaptureBaseline(_ context.Context, _ []string) {}

func (vh *ValidationHandler) CaptureSnapshot(filePaths []string) (*Snapshot, error) {
	return nil, fmt.Errorf("validation handler not available: treesitter not enabled")
}

func (vh *ValidationHandler) ValidateEdit(
	_ context.Context,
	_ *Snapshot,
	_ string,
	_ string,
	_ string,
	_ EditSpec,
) (*ValidationHandlerResult, error) {
	return nil, fmt.Errorf("validation handler not available: treesitter not enabled")
}

func (vh *ValidationHandler) ReplaceStages(_ []ValidationStage) {}
