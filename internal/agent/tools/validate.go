//go:build treesitter

package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/treesitter"
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

// ValidationPipeline runs a configurable sequence of validation stages.
type ValidationPipeline struct {
	stages []ValidationStage
	parser treesitter.Parser
}

// NewValidationPipeline creates a pipeline with default stages 1-12.
func NewValidationPipeline(parser treesitter.Parser) *ValidationPipeline {
	p := &ValidationPipeline{parser: parser}
	p.stages = []ValidationStage{
		// Critical stages (0-3): abort on failure.
		&AnchorVerificationStage{},
		&HashResolutionStage{},
		&PreApplySnapshotStage{},
		&ApplyEditStage{},
		// Non-critical stages (4+): record failure but continue.
		&ParseCheckStage{parser: parser},
		&SymbolConsistencyStage{parser: parser},
		&ImportConsistencyStage{parser: parser},
		&AnchorUpdateStage{},
		&DiagnosticsStage{},
		&FormatStage{},
		&SaveStage{},
		&FinalAnchorSaveStage{},
	}
	return p
}

func (p *ValidationPipeline) AddStage(s ValidationStage) {
	p.stages = append(p.stages, s)
}

func (p *ValidationPipeline) InsertStage(idx int, s ValidationStage) {
	if idx < 0 || idx > len(p.stages) {
		idx = len(p.stages)
	}
	p.stages = append(p.stages[:idx], append([]ValidationStage{s}, p.stages[idx:]...)...)
}

func (p *ValidationPipeline) Stages() []ValidationStage {
	out := make([]ValidationStage, len(p.stages))
	copy(out, p.stages)
	return out
}

// Run executes every stage sequentially. Stages 0-3 are critical and abort on
// failure. Stages 4+ are non-critical: failure is recorded but execution
// continues.
func (p *ValidationPipeline) Run(ctx context.Context, input *ValidationInput) *PipelineResult {
	result := &PipelineResult{OverallStatus: StatusPass}

	for i, stage := range p.stages {
		sr, err := stage.Execute(ctx, input)
		if err != nil {
			sr = &StageResult{
				StageName: stage.Name(),
				Status:    StatusError,
				Message:   fmt.Sprintf("unexpected error: %v", err),
			}
		}
		if sr == nil {
			sr = &StageResult{
				StageName: stage.Name(),
				Status:    StatusError,
				Message:   "stage returned nil result",
			}
		}
		result.StageResults = append(result.StageResults, *sr)

		if sr.Status == StatusFail || sr.Status == StatusError {
			if i < 4 {
				result.OverallStatus = StatusFail
				return result
			}
			result.OverallStatus = StatusFail
		}
	}

	result.Snapshot = input.Snapshot
	result.EditedContent = input.editedContent
	result.UpdatedAnchorMap = input.updatedAnchorMap
	return result
}

// PipelineConfig controls pipeline execution behavior.
type PipelineConfig struct {
	SkipStages   []string
	FailFast     bool
	StageTimeout time.Duration
}

func (c *PipelineConfig) shouldSkip(name string) bool {
	return slices.Contains(c.SkipStages, name)
}

// RunWithConfig executes the pipeline with the given configuration. It supports
// skipping stages by name, fail-fast mode, and per-stage timeouts.
func (p *ValidationPipeline) RunWithConfig(ctx context.Context, input *ValidationInput, cfg PipelineConfig) *PipelineResult {
	result := &PipelineResult{OverallStatus: StatusPass}

	for i, stage := range p.stages {
		if cfg.shouldSkip(stage.Name()) {
			result.StageResults = append(result.StageResults, StageResult{
				StageName: stage.Name(),
				Status:    StatusSkip,
				Message:   "skipped by config",
			})
			continue
		}

		stageCtx := ctx
		if cfg.StageTimeout > 0 {
			var cancel context.CancelFunc
			stageCtx, cancel = context.WithTimeout(ctx, cfg.StageTimeout)
			defer cancel()
		}

		sr, err := stage.Execute(stageCtx, input)
		if err != nil {
			sr = &StageResult{
				StageName: stage.Name(),
				Status:    StatusError,
				Message:   fmt.Sprintf("unexpected error: %v", err),
			}
		}
		if sr == nil {
			sr = &StageResult{
				StageName: stage.Name(),
				Status:    StatusError,
				Message:   "stage returned nil result",
			}
		}
		result.StageResults = append(result.StageResults, *sr)

		if sr.Status == StatusFail || sr.Status == StatusError {
			if i < 4 || cfg.FailFast {
				result.OverallStatus = StatusFail
				result.Snapshot = input.Snapshot
				result.EditedContent = input.editedContent
				result.UpdatedAnchorMap = input.updatedAnchorMap
				return result
			}
			result.OverallStatus = StatusFail
		}
	}

	result.Snapshot = input.Snapshot
	result.EditedContent = input.editedContent
	result.UpdatedAnchorMap = input.updatedAnchorMap
	return result
}

// ---------------------------------------------------------------------------
// Stage 1: AnchorVerification
// ---------------------------------------------------------------------------

// AnchorVerificationStage verifies anchor hashes in PositionHints exist in the
// AnchorMap.
type AnchorVerificationStage struct{}

func (s *AnchorVerificationStage) Name() string  { return "AnchorVerification" }
func (s *AnchorVerificationStage) CanSkip() bool { return false }

func (s *AnchorVerificationStage) Execute(_ context.Context, input *ValidationInput) (*StageResult, error) {
	if len(input.EditSpec.PositionHints) == 0 {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusPass,
			Message:   "no anchor hints provided",
		}, nil
	}

	if input.AnchorMap == nil || len(input.AnchorMap.Lookup) == 0 {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusFail,
			Message:   "anchor map is empty but position hints were provided",
		}, nil
	}

	var missing []string
	for _, hint := range input.EditSpec.PositionHints {
		if _, found := input.AnchorMap.Lookup[parseAnchorHash(hint)]; !found {
			missing = append(missing, hint)
		}
	}

	if len(missing) > 0 {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusFail,
			Message:   fmt.Sprintf("anchors not found: %s", strings.Join(missing, ", ")),
		}, nil
	}

	return &StageResult{
		StageName: s.Name(),
		Status:    StatusPass,
		Message:   fmt.Sprintf("all %d anchor hints verified", len(input.EditSpec.PositionHints)),
	}, nil
}

// parseAnchorHash converts "<hash:a1b2c3d4>" to a uint64. Returns 0 on
// malformed input (which will never match a valid anchor).
func parseAnchorHash(formatted string) uint64 {
	h := strings.TrimPrefix(formatted, "<hash:")
	h = strings.TrimSuffix(h, ">")
	h = strings.TrimSpace(h)
	var val uint64
	fmt.Sscanf(h, "%x", &val)
	return val
}

// ---------------------------------------------------------------------------
// Stage 2: HashResolution
// ---------------------------------------------------------------------------

// HashResolutionStage resolves anchor hashes to line positions via
// ResolveAnchor.
type HashResolutionStage struct{}

func (s *HashResolutionStage) Name() string  { return "HashResolution" }
func (s *HashResolutionStage) CanSkip() bool { return false }

func (s *HashResolutionStage) Execute(_ context.Context, input *ValidationInput) (*StageResult, error) {
	if input.AnchorMap == nil || len(input.AnchorMap.Anchors) == 0 {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusPass,
			Message:   "no anchors to resolve",
		}, nil
	}

	var warnings []string
	for i := range input.AnchorMap.Anchors {
		anchor := &input.AnchorMap.Anchors[i]
		lineNum, err := ResolveAnchor(anchor, input.Content)
		if err != nil {
			return &StageResult{
				StageName: s.Name(),
				Status:    StatusFail,
				Message:   fmt.Sprintf("failed to resolve anchor at original line %d: %v", anchor.LineNum, err),
			}, nil
		}
		if lineNum != anchor.LineNum {
			warnings = append(warnings, fmt.Sprintf("anchor drifted from line %d to %d", anchor.LineNum, lineNum))
		}
	}

	return &StageResult{
		StageName: s.Name(),
		Status:    StatusPass,
		Message:   fmt.Sprintf("all %d anchors resolved", len(input.AnchorMap.Anchors)),
		Warnings:  warnings,
	}, nil
}

// ---------------------------------------------------------------------------
// Stage 3: PreApplySnapshot
// ---------------------------------------------------------------------------

// PreApplySnapshotStage captures file content before the edit is applied.
type PreApplySnapshotStage struct{}

func (s *PreApplySnapshotStage) Name() string  { return "PreApplySnapshot" }
func (s *PreApplySnapshotStage) CanSkip() bool { return false }

func (s *PreApplySnapshotStage) Execute(_ context.Context, input *ValidationInput) (*StageResult, error) {
	snapshot := strings.Clone(input.Content)
	input.Snapshot = snapshot
	return &StageResult{
		StageName: s.Name(),
		Status:    StatusPass,
		Message:   fmt.Sprintf("captured snapshot (%d bytes)", len(snapshot)),
	}, nil
}

// ---------------------------------------------------------------------------
// Stage 4: ApplyEdit
// ---------------------------------------------------------------------------

// ApplyEditStage applies the edit to content and stores the result in
// input.editedContent.
type ApplyEditStage struct{}

func (s *ApplyEditStage) Name() string  { return "ApplyEdit" }
func (s *ApplyEditStage) CanSkip() bool { return false }

func (s *ApplyEditStage) Execute(_ context.Context, input *ValidationInput) (*StageResult, error) {
	old := input.EditSpec.OldString
	new := input.EditSpec.NewString

	if old == "" {
		input.editedContent = new
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusPass,
			Message:   "new content applied (empty old_string)",
		}, nil
	}

	var result string
	if input.EditSpec.ReplaceAll {
		result = strings.ReplaceAll(input.Content, old, new)
		if result == input.Content {
			return &StageResult{
				StageName: s.Name(),
				Status:    StatusFail,
				Message:   "old_string not found in content (replaceAll)",
			}, nil
		}
	} else {
		idx := strings.Index(input.Content, old)
		if idx == -1 {
			return &StageResult{
				StageName: s.Name(),
				Status:    StatusFail,
				Message:   "old_string not found in content",
			}, nil
		}
		if lastIdx := strings.LastIndex(input.Content, old); idx != lastIdx {
			return &StageResult{
				StageName: s.Name(),
				Status:    StatusFail,
				Message:   "old_string appears multiple times in content",
				Warnings:  []string{"use replace_all or provide more context for a unique match"},
			}, nil
		}
		result = input.Content[:idx] + new + input.Content[idx+len(old):]
	}

	if result == input.Content {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusFail,
			Message:   "edit produced no changes",
		}, nil
	}

	input.editedContent = result
	return &StageResult{
		StageName: s.Name(),
		Status:    StatusPass,
		Message:   fmt.Sprintf("edit applied (%d → %d bytes)", len(input.Content), len(result)),
	}, nil
}

// ---------------------------------------------------------------------------
// Stage 5: ParseCheck
// ---------------------------------------------------------------------------

// ParseCheckStage verifies edited content parses with tree-sitter. Unsupported
// languages are skipped.
type ParseCheckStage struct {
	parser treesitter.Parser
}

func (s *ParseCheckStage) Name() string  { return "ParseCheck" }
func (s *ParseCheckStage) CanSkip() bool { return true }

func (s *ParseCheckStage) Execute(ctx context.Context, input *ValidationInput) (*StageResult, error) {
	content := input.editedContent
	if content == "" {
		return &StageResult{StageName: s.Name(), Status: StatusSkip, Message: "no edited content"}, nil
	}
	if s.parser == nil {
		return &StageResult{StageName: s.Name(), Status: StatusSkip, Message: "no parser available"}, nil
	}

	lang := treesitter.MapPath(input.FilePath)
	if lang == "" {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   fmt.Sprintf("unsupported extension %q", filepath.Ext(input.FilePath)),
		}, nil
	}
	if !s.parser.SupportsLanguage(lang) {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   fmt.Sprintf("language %q not supported", lang),
		}, nil
	}

	tree, err := s.parser.ParseTree(ctx, input.FilePath, []byte(content))
	if err != nil {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusFail,
			Message:   fmt.Sprintf("parse error: %v", err),
		}, nil
	}
	if tree != nil {
		tree.Close()
	}
	return &StageResult{
		StageName: s.Name(),
		Status:    StatusPass,
		Message:   fmt.Sprintf("file parses as %q", lang),
	}, nil
}

// ---------------------------------------------------------------------------
// Stage 6: SymbolConsistency
// ---------------------------------------------------------------------------

// SymbolConsistencyStage checks for duplicate symbol definitions in edited
// content using tree-sitter analysis.
type SymbolConsistencyStage struct {
	parser treesitter.Parser
}

func (s *SymbolConsistencyStage) Name() string  { return "SymbolConsistency" }
func (s *SymbolConsistencyStage) CanSkip() bool { return true }

func (s *SymbolConsistencyStage) Execute(ctx context.Context, input *ValidationInput) (*StageResult, error) {
	if input.editedContent == "" {
		return &StageResult{StageName: s.Name(), Status: StatusSkip, Message: "no edited content"}, nil
	}
	if s.parser == nil {
		return &StageResult{StageName: s.Name(), Status: StatusSkip, Message: "no parser available"}, nil
	}

	lang := treesitter.MapPath(input.FilePath)
	if lang == "" {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   fmt.Sprintf("unsupported language for %q", filepath.Base(input.FilePath)),
		}, nil
	}
	if !s.parser.SupportsLanguage(lang) {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   fmt.Sprintf("unsupported language %q", lang),
		}, nil
	}

	analysis, err := s.parser.Analyze(ctx, input.FilePath, []byte(input.editedContent))
	if err != nil {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusFail,
			Message:   fmt.Sprintf("analysis error: %v", err),
		}, nil
	}

	if analysis == nil || len(analysis.Symbols) == 0 {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusPass,
			Message:   "no symbols found",
		}, nil
	}

	seen := make(map[string]int, len(analysis.Symbols))
	var duplicates []string
	for _, sym := range analysis.Symbols {
		key := sym.Name + ":" + sym.Kind
		count := seen[key]
		if count == 1 {
			duplicates = append(duplicates, sym.Name)
		}
		seen[key] = count + 1
	}

	if len(duplicates) > 0 {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusFail,
			Message:   fmt.Sprintf("duplicate symbol definitions: %s", strings.Join(duplicates, ", ")),
		}, nil
	}

	return &StageResult{
		StageName: s.Name(),
		Status:    StatusPass,
		Message:   fmt.Sprintf("all %d symbols unique", len(analysis.Symbols)),
	}, nil
}

// ---------------------------------------------------------------------------
// Stage 7: ImportConsistency
// ---------------------------------------------------------------------------

type ImportConsistencyStage struct {
	parser treesitter.Parser
}

func (s *ImportConsistencyStage) Name() string  { return "ImportConsistency" }
func (s *ImportConsistencyStage) CanSkip() bool { return true }

func (s *ImportConsistencyStage) Execute(ctx context.Context, input *ValidationInput) (*StageResult, error) {
	if input.editedContent == "" {
		return &StageResult{StageName: s.Name(), Status: StatusSkip, Message: "no edited content"}, nil
	}
	if s.parser == nil {
		return &StageResult{StageName: s.Name(), Status: StatusSkip, Message: "no parser available"}, nil
	}

	lang := treesitter.MapPath(input.FilePath)
	if lang == "" {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   fmt.Sprintf("unsupported language for %q", filepath.Base(input.FilePath)),
		}, nil
	}
	if !s.parser.SupportsLanguage(lang) {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   fmt.Sprintf("unsupported language %q", lang),
		}, nil
	}

	analysis, err := s.parser.Analyze(ctx, input.FilePath, []byte(input.editedContent))
	if err != nil {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusFail,
			Message:   fmt.Sprintf("analysis error: %v", err),
		}, nil
	}

	if analysis == nil || len(analysis.Imports) == 0 {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusPass,
			Message:   "no imports found",
		}, nil
	}

	var invalid []string
	for _, imp := range analysis.Imports {
		switch imp.Path {
		case "":
			invalid = append(invalid, "(empty import path)")
		case ".", "..":
			invalid = append(invalid, imp.Path)
		}
	}

	if len(invalid) > 0 {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusFail,
			Message:   fmt.Sprintf("invalid import paths: %s", strings.Join(invalid, ", ")),
		}, nil
	}

	return &StageResult{
		StageName: s.Name(),
		Status:    StatusPass,
		Message:   fmt.Sprintf("all %d imports valid", len(analysis.Imports)),
	}, nil
}

// ---------------------------------------------------------------------------
// Stage 8: AnchorUpdate
// ---------------------------------------------------------------------------

type AnchorUpdateStage struct{}

func (s *AnchorUpdateStage) Name() string  { return "AnchorUpdate" }
func (s *AnchorUpdateStage) CanSkip() bool { return true }

func (s *AnchorUpdateStage) Execute(_ context.Context, input *ValidationInput) (*StageResult, error) {
	if input.editedContent == "" {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   "no edited content to rebuild anchors",
		}, nil
	}

	updated := BuildAnchorMap(input.editedContent, defaultAnchorInterval)
	input.updatedAnchorMap = updated

	return &StageResult{
		StageName: s.Name(),
		Status:    StatusPass,
		Message:   fmt.Sprintf("rebuilt anchor map (%d anchors)", len(updated.Anchors)),
	}, nil
}

// ---------------------------------------------------------------------------
// Stage 9: Diagnostics
// ---------------------------------------------------------------------------

type DiagnosticsStage struct {
	gate *DiagnosticGate
}

func (s *DiagnosticsStage) Name() string  { return "Diagnostics" }
func (s *DiagnosticsStage) CanSkip() bool { return true }

func (s *DiagnosticsStage) Execute(ctx context.Context, input *ValidationInput) (*StageResult, error) {
	if s.gate == nil {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   "no diagnostic gate provided",
		}, nil
	}

	gr := s.gate.Compare(ctx, []string{input.FilePath})
	if gr.NoLSP {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   "no LSP servers available",
		}, nil
	}
	if !gr.Pass {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusFail,
			Message:   gr.Message(),
		}, nil
	}

	return &StageResult{
		StageName: s.Name(),
		Status:    StatusPass,
		Message:   gr.Message(),
	}, nil
}

// ---------------------------------------------------------------------------
// Stage 10: Format
// ---------------------------------------------------------------------------

// FormatStage checks whether the file is formatted according to gofmt.
type FormatStage struct {
	lookPath func(name string) (string, error)
}

func (s *FormatStage) Name() string  { return "Format" }
func (s *FormatStage) CanSkip() bool { return true }

func (s *FormatStage) Execute(ctx context.Context, input *ValidationInput) (*StageResult, error) {
	if input.editedContent == "" {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   "empty content",
		}, nil
	}

	ext := strings.ToLower(filepath.Ext(input.FilePath))
	if ext != ".go" {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   "unsupported file type: " + ext,
		}, nil
	}

	lookPath := exec.LookPath
	if s.lookPath != nil {
		lookPath = s.lookPath
	}
	formatter := "gofmt"
	if _, err := lookPath(formatter); err != nil {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   formatter + " not installed",
		}, nil
	}

	tmp, err := os.CreateTemp("", "format_check_*.go")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.WriteString(input.editedContent); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	tmp.Close()

	cmd := exec.CommandContext(ctx, formatter, "-l", tmpPath)
	output, err := cmd.Output()
	if err != nil {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   "formatter could not parse file",
		}, nil
	}

	if len(strings.TrimSpace(string(output))) > 0 {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusFail,
			Message:   "file needs formatting",
		}, nil
	}

	return &StageResult{
		StageName: s.Name(),
		Status:    StatusPass,
		Message:   "file is well formatted",
	}, nil
}

// ---------------------------------------------------------------------------
// Stage 11: Save
// ---------------------------------------------------------------------------

type SaveStage struct{}

func (s *SaveStage) Name() string  { return "Save" }
func (s *SaveStage) CanSkip() bool { return false }

func (s *SaveStage) Execute(_ context.Context, input *ValidationInput) (*StageResult, error) {
	if input.editedContent == "" {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   "no edited content to save",
		}, nil
	}

	input.Content = input.editedContent
	return &StageResult{
		StageName: s.Name(),
		Status:    StatusPass,
		Message:   fmt.Sprintf("content updated (%d bytes)", len(input.Content)),
	}, nil
}

// ---------------------------------------------------------------------------
// Stage 12: FinalAnchorSave
// ---------------------------------------------------------------------------

type FinalAnchorSaveStage struct{}

func (s *FinalAnchorSaveStage) Name() string  { return "FinalAnchorSave" }
func (s *FinalAnchorSaveStage) CanSkip() bool { return true }

func (s *FinalAnchorSaveStage) Execute(_ context.Context, input *ValidationInput) (*StageResult, error) {
	if input.updatedAnchorMap == nil {
		return &StageResult{
			StageName: s.Name(),
			Status:    StatusSkip,
			Message:   "no updated anchor map to save",
		}, nil
	}

	input.AnchorMap = input.updatedAnchorMap
	return &StageResult{
		StageName: s.Name(),
		Status:    StatusPass,
		Message:   fmt.Sprintf("anchor map persisted (%d anchors)", len(input.AnchorMap.Anchors)),
	}, nil
}
