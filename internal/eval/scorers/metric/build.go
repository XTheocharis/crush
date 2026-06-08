package metric

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"strings"

	"github.com/charmbracelet/crush/internal/eval"
)

// BuildSuccessScorer scores based on Go syntax validity of files in the eval
// input. It uses go/parser to detect syntax errors rather than shelling out to
// the build toolchain. Score is the ratio of parseable files to total files.
type BuildSuccessScorer struct{}

// NewBuildSuccessScorer creates a build-success scorer that parses Go source.
func NewBuildSuccessScorer() *BuildSuccessScorer {
	return &BuildSuccessScorer{}
}

func (s *BuildSuccessScorer) Name() string          { return "build_success" }
func (s *BuildSuccessScorer) Type() eval.ScorerType { return eval.ScorerMetric }

func (s *BuildSuccessScorer) Score(_ context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	// Collect all Go file content from both Edits and Files.
	contents := collectGoContent(input)

	if len(contents) == 0 {
		return &eval.ScoreResult{
			Score:       0.0,
			Explanation: "No Go source content found to validate.",
			Passed:      false,
			Details:     map[string]any{"files_checked": 0, "parse_errors": 0},
		}, nil
	}

	parsed, errors := parseGoFiles(contents)
	total := len(contents)
	score := float64(parsed) / float64(total)
	passed := parsed == total

	explanation := fmt.Sprintf(
		"%d/%d Go files parsed successfully",
		parsed, total,
	)
	if errors > 0 {
		explanation += fmt.Sprintf(" (%d parse errors)", errors)
	}

	return &eval.ScoreResult{
		Score:       score,
		Explanation: explanation,
		Passed:      passed,
		Details: map[string]any{
			"files_checked": total,
			"parsed_ok":     parsed,
			"parse_errors":  errors,
		},
	}, nil
}

// collectGoContent gathers Go file contents from Edits.After and Files entries
// with .go extension.
func collectGoContent(input *eval.EvalInput) map[string]string {
	files := make(map[string]string)

	// First, collect from Edits — use the After content (the result).
	for _, edit := range input.Edits {
		if strings.HasSuffix(edit.Path, ".go") && len(edit.After) > 0 {
			files[edit.Path] = edit.After
		}
	}

	// Then overlay from Files map.
	for path, content := range input.Files {
		if strings.HasSuffix(path, ".go") && len(content) > 0 {
			files[path] = content
		}
	}

	return files
}

// parseGoFiles attempts to parse each file with go/parser and returns the
// count of successfully parsed files and the count of errors.
func parseGoFiles(files map[string]string) (parsed int, errors int) {
	for name, content := range files {
		fset := token.NewFileSet()
		_, err := parser.ParseFile(fset, name, strings.NewReader(content), parser.AllErrors)
		if err != nil {
			errors++
		} else {
			parsed++
		}
	}
	return parsed, errors
}
