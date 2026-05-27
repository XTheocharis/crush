package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const defaultLintTimeout = 60 * time.Second

// lintLineRe matches output in the form "file:line:col: message" or
// "file:line: message" (go vet omits the column).
var lintLineRe = regexp.MustCompile(`^[^:]+\.go:\d+(:\d+)?: `)

// GoLinter runs Go linting tools over the specified file paths.
// It prefers golangci-lint and falls back to go vet when golangci-lint
// is not available on the system PATH.
type GoLinter struct {
	WorkingDir string
}

var _ Linter = (*GoLinter)(nil)

func (g *GoLinter) RunLint(ctx context.Context, filePaths []string) ([]string, error) {
	if len(filePaths) == 0 {
		return nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, defaultLintTimeout)
	defer cancel()

	output, err := g.runGolangciLint(timeoutCtx, filePaths)
	if err != nil {
		if isNotFound(err) {
			vetOutput, vetErr := g.runGoVet(timeoutCtx)
			if vetErr != nil {
				return nil, fmt.Errorf("go vet: %w", vetErr)
			}
			return filterLintLines(vetOutput, filePaths), nil
		}
		return filterLintLines(output, filePaths), nil
	}

	return filterLintLines(output, filePaths), nil
}

func (g *GoLinter) runGolangciLint(ctx context.Context, filePaths []string) (string, error) {
	path, err := exec.LookPath("golangci-lint")
	if err != nil {
		return "", fmt.Errorf("golangci-lint not found: %w", err)
	}

	args := []string{"run", "--out-format=line-number"}
	args = append(args, filePaths...)

	cmd := exec.CommandContext(ctx, path, args...)
	if g.WorkingDir != "" {
		cmd.Dir = g.WorkingDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	output := stdout.String()
	if output == "" {
		output = stderr.String()
	}
	return output, err
}

func (g *GoLinter) runGoVet(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "vet", "./...")
	if g.WorkingDir != "" {
		cmd.Dir = g.WorkingDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stderr.String()
	if output == "" {
		output = stdout.String()
	}
	return output, err
}

// GoTester runs Go tests and returns failure output.
type GoTester struct {
	WorkingDir string
}

var _ Tester = (*GoTester)(nil)

func (t *GoTester) RunTests(ctx context.Context) ([]string, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, defaultLintTimeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "go", "test", "./...")
	if t.WorkingDir != "" {
		cmd.Dir = t.WorkingDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if output == "" {
		output = stderr.String()
	} else if stderr.Len() > 0 {
		output = output + "\n" + stderr.String()
	}

	if err != nil {
		return parseTestFailures(output), nil
	}
	return nil, nil
}

func parseTestFailures(output string) []string {
	var failures []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--- FAIL:") {
			failures = append(failures, line)
		}
	}
	return failures
}

func filterLintLines(raw string, filePaths []string) []string {
	fileSet := make(map[string]struct{}, len(filePaths))
	for _, f := range filePaths {
		fileSet[f] = struct{}{}
	}

	var result []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !lintLineRe.MatchString(line) {
			continue
		}
		file := extractFile(line)
		if _, ok := fileSet[file]; ok {
			result = append(result, line)
		}
	}
	return result
}

func extractFile(line string) string {
	file, _, found := strings.Cut(line, ":")
	if !found {
		return line
	}
	return file
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found") ||
		strings.Contains(err.Error(), "executable file not found")
}
