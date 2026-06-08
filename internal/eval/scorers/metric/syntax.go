package metric

import (
	"context"
	"fmt"

	"github.com/charmbracelet/crush/internal/eval"
)

// SyntaxValidityScorer checks bracket balance, unterminated strings, and
// unclosed block comments in Files. Perfectly valid files yield 1.0.
type SyntaxValidityScorer struct {
	PassThreshold float64
}

// NewSyntaxValidityScorer creates a scorer that passes when the syntax
// validity meets the threshold (0.0-1.0).
func NewSyntaxValidityScorer(threshold float64) *SyntaxValidityScorer {
	return &SyntaxValidityScorer{PassThreshold: threshold}
}

func (s *SyntaxValidityScorer) Name() string          { return "syntax_validity" }
func (s *SyntaxValidityScorer) Type() eval.ScorerType { return eval.ScorerMetric }

func (s *SyntaxValidityScorer) Score(_ context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	if len(input.Files) == 0 {
		return &eval.ScoreResult{
			Score:       1.0,
			Explanation: "No files to validate.",
			Passed:      true,
			Details:     map[string]any{"files": 0, "balanced": true},
		}, nil
	}

	balanced := 0
	unbalanced := 0
	issues := make(map[string][]string)
	for name, content := range input.Files {
		fileIssues := checkSyntax(content)
		if len(fileIssues) == 0 {
			balanced++
		} else {
			unbalanced++
			issues[name] = fileIssues
		}
	}

	total := balanced + unbalanced
	score := float64(balanced) / float64(total)
	passed := score >= s.PassThreshold
	explanation := fmt.Sprintf(
		"%d/%d files have valid syntax",
		balanced, total,
	)

	return &eval.ScoreResult{
		Score:       score,
		Explanation: explanation,
		Passed:      passed,
		Details: map[string]any{
			"files":      total,
			"balanced":   balanced,
			"unbalanced": unbalanced,
			"issues":     issues,
		},
	}, nil
}

func checkSyntax(content string) []string {
	var issues []string

	if err := checkBracketBalance(content); err != "" {
		issues = append(issues, err)
	}

	if unterminated := checkUnterminatedStrings(content); unterminated != "" {
		issues = append(issues, unterminated)
	}

	if unclosed := checkBlockComments(content); unclosed != "" {
		issues = append(issues, unclosed)
	}

	return issues
}

func checkBracketBalance(content string) string {
	stack := make([]rune, 0, 32)
	inString := false
	inRawString := false
	var stringDelim rune
	inLineComment := false
	inBlockComment := false

	runes := []rune(content)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}

		if inBlockComment {
			if ch == '*' && i+1 < len(runes) && runes[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if inString {
			if ch == '\\' {
				i++
				continue
			}
			if ch == stringDelim {
				inString = false
			}
			continue
		}

		if inRawString {
			if ch == stringDelim {
				inRawString = false
			}
			continue
		}

		switch ch {
		case '/':
			if i+1 < len(runes) {
				switch runes[i+1] {
				case '/':
					inLineComment = true
					i++
					continue
				case '*':
					inBlockComment = true
					i++
					continue
				}
			}
		case '"', '\'':
			inString = true
			stringDelim = ch
		case '`':
			inRawString = true
			stringDelim = ch
		case '(', '[', '{':
			stack = append(stack, ch)
		case ')':
			if len(stack) == 0 || stack[len(stack)-1] != '(' {
				return "unmatched closing ')'"
			}
			stack = stack[:len(stack)-1]
		case ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				return "unmatched closing ']'"
			}
			stack = stack[:len(stack)-1]
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				return "unmatched closing '}'"
			}
			stack = stack[:len(stack)-1]
		}
	}

	if inBlockComment {
		return "unclosed block comment"
	}
	if inString {
		return "unterminated string literal"
	}
	if inRawString {
		return "unterminated raw string literal"
	}
	if len(stack) > 0 {
		return "unclosed brackets remain"
	}
	return ""
}

func checkUnterminatedStrings(content string) string {
	inDouble := false
	inRaw := false
	inLineComment := false
	inBlockComment := false

	runes := []rune(content)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && i+1 < len(runes) && runes[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		switch {
		case ch == '/' && i+1 < len(runes) && runes[i+1] == '/':
			inLineComment = true
			i++
		case ch == '/' && i+1 < len(runes) && runes[i+1] == '*':
			inBlockComment = true
			i++
		case inDouble:
			if ch == '\\' {
				i++
			} else if ch == '"' {
				inDouble = false
			}
		case inRaw:
			if ch == '`' {
				inRaw = false
			}
		case ch == '"':
			inDouble = true
		case ch == '`':
			inRaw = true
		}
	}

	if inDouble {
		return "unterminated double-quoted string"
	}
	if inRaw {
		return "unterminated raw string"
	}
	return ""
}

func checkBlockComments(content string) string {
	inBlockComment := false
	runes := []rune(content)
	for i := 0; i < len(runes); i++ {
		if inBlockComment {
			if runes[i] == '*' && i+1 < len(runes) && runes[i+1] == '/' {
				inBlockComment = false
				i++
			}
		} else {
			if runes[i] == '/' && i+1 < len(runes) && runes[i+1] == '*' {
				inBlockComment = true
				i++
			}
		}
	}
	if inBlockComment {
		return "unclosed block comment /*"
	}
	return ""
}

// isBracketBalanced is kept for backward compatibility with tests.
func isBracketBalanced(content string) bool {
	return checkBracketBalance(content) == ""
}
