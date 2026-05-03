package explorer

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// ShellExplorer explores shell script files.
type ShellExplorer struct{}

func (e *ShellExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "sh" || ext == "bash" || ext == "zsh" || ext == "fish" {
		return true
	}
	// Also detect via shebang
	return detectShebang(content) == "shell"
}

func (e *ShellExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("Shell script too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "shell", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	var summary strings.Builder
	fmt.Fprintf(&summary, "Shell script: %s\n", filepath.Base(input.Path))

	// Shebang
	if strings.HasPrefix(content, "#!") {
		lines := strings.SplitN(content, "\n", 2)
		fmt.Fprintf(&summary, "Shebang: %s\n", lines[0])
	}

	// Source/dot commands
	sourceRe := regexp.MustCompile(`(?m)^(?:source|\.)\s+(.+)`)
	sources := sourceRe.FindAllStringSubmatch(content, -1)
	if len(sources) > 0 {
		summary.WriteString("\nSources:\n")
		for _, match := range sources {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Functions
	funcRe := regexp.MustCompile(`(?m)^(?:function\s+)?(\w+)\s*\(\s*\)\s*\{`)
	functions := funcRe.FindAllStringSubmatch(content, -1)
	if len(functions) > 0 {
		summary.WriteString("\nFunctions:\n")
		for _, match := range functions {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Environment variables set
	varRe := regexp.MustCompile(`(?m)^(?:export\s+)?([A-Z_][A-Z0-9_]*)\s*=`)
	vars := varRe.FindAllStringSubmatch(content, -1)
	if len(vars) > 0 {
		summary.WriteString("\nEnvironment variables:\n")
		seen := make(map[string]bool)
		for _, match := range vars {
			v := match[1]
			if !seen[v] {
				fmt.Fprintf(&summary, "  - %s\n", v)
				seen[v] = true
			}
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "shell",
		TokenEstimate: estimateTokens(result),
	}, nil
}
