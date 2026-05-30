package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	// maxIncludeDepth is the maximum recursion depth for @include processing.
	maxIncludeDepth = 5

	// maxContentChars is the character limit per file. Content beyond this
	// limit is discarded and a truncation marker is appended.
	maxContentChars = 40000

	// truncationMarker is appended when content exceeds maxContentChars.
	truncationMarker = "\n<!-- truncated at 40K -->\n"
)

// includePattern matches `@include path/to/file.md` directives.
// The path must not be empty and is trimmed of surrounding whitespace.
var includePattern = regexp.MustCompile(`^@include\s+(\S+)\s*$`)

// conditionalPattern matches `<!-- if: key=value -->` opening tags.
var conditionalPattern = regexp.MustCompile(`^<!--\s*if:\s*(\w+):(\S+)\s*-->$`)

// endifPattern matches `<!-- endif -->` closing tags.
var endifPattern = regexp.MustCompile(`^<!--\s*endif\s*-->$`)

// ConditionEvaluator is a function that evaluates whether a condition holds.
// The key identifies the condition type (e.g. "language", "file", "env") and
// value is the condition argument (e.g. "go", "*.go", "CI").
type ConditionEvaluator func(key, value string) bool

// ProcessIncludes resolves @include directives in content, detects cycles,
// truncates oversized files, and evaluates conditional blocks.
//
// basePath is the directory relative to which include paths are resolved.
// depth tracks the current recursion level (start at 0).
// seen tracks absolute paths already included for cycle detection (pass nil
// for the top-level call — it will be initialised automatically).
// eval evaluates conditional block predicates. If nil, DefaultEvaluator is
// used.
func ProcessIncludes(
	content string,
	basePath string,
	depth int,
	seen map[string]bool,
	eval ConditionEvaluator,
) (string, error) {
	if eval == nil {
		eval = DefaultEvaluator
	}
	if seen == nil {
		seen = make(map[string]bool)
	}

	if depth > maxIncludeDepth {
		return "", fmt.Errorf("@include exceeded maximum depth %d", maxIncludeDepth)
	}

	basePath, err := filepath.Abs(basePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base path: %w", err)
	}

	var out strings.Builder
	lines := strings.Split(content, "\n")

	inConditional := false
	conditionMet := false

	for _, line := range lines {
		if matches := conditionalPattern.FindStringSubmatch(strings.TrimSpace(line)); matches != nil {
			if inConditional {
				return "", fmt.Errorf("nested conditional blocks are not supported: %q", line)
			}
			inConditional = true
			conditionMet = eval(matches[1], matches[2])
			continue
		}

		if endifPattern.MatchString(strings.TrimSpace(line)) {
			if !inConditional {
				return "", fmt.Errorf("unexpected <!-- endif --> without matching <!-- if: ... -->")
			}
			inConditional = false
			conditionMet = false
			continue
		}

		if inConditional && !conditionMet {
			continue
		}

		if matches := includePattern.FindStringSubmatch(line); matches != nil {
			includePath := matches[1]
			resolved, err := resolveIncludePath(includePath, basePath)
			if err != nil {
				return "", err
			}

			absPath, err := filepath.Abs(resolved)
			if err != nil {
				return "", fmt.Errorf("failed to resolve include path %q: %w", includePath, err)
			}

			if !isSubPath(basePath, absPath) {
				return "", fmt.Errorf("@include path %q escapes project directory", includePath)
			}

			if seen[absPath] {
				return "", fmt.Errorf("@include cycle detected: %q already included", absPath)
			}
			seen[absPath] = true

			data, err := os.ReadFile(absPath)
			if err != nil {
				return "", fmt.Errorf("failed to read included file %q: %w", absPath, err)
			}

			included := string(data)
			included = truncate(included)

			processed, err := ProcessIncludes(included, filepath.Dir(absPath), depth+1, seen, eval)
			if err != nil {
				return "", err
			}

			out.WriteString(processed)
			if !strings.HasSuffix(processed, "\n") {
				out.WriteByte('\n')
			}
			continue
		}

		out.WriteString(line)
		out.WriteByte('\n')
	}

	if inConditional {
		return "", fmt.Errorf("unclosed conditional block: missing <!-- endif -->")
	}

	result := out.String()
	if !strings.HasSuffix(content, "\n") && strings.HasSuffix(result, "\n") {
		result = strings.TrimSuffix(result, "\n")
	}
	return result, nil
}

// resolveIncludePath resolves an include path relative to basePath.
// It handles both relative and absolute-like paths, always resolving
// relative to basePath.
func resolveIncludePath(includePath, basePath string) (string, error) {
	if filepath.IsAbs(includePath) {
		return "", fmt.Errorf("@include does not support absolute paths: %q", includePath)
	}
	return filepath.Join(basePath, includePath), nil
}

// truncate truncates content to maxContentChars and appends the truncation
// marker if needed.
func truncate(content string) string {
	if utf8.RuneCountInString(content) <= maxContentChars {
		return content
	}
	return string([]rune(content)[:maxContentChars]) + truncationMarker
}

// isSubPath returns true if sub is within or equal to parent. Both paths
// must be absolute.
func isSubPath(parent, sub string) bool {
	rel, err := filepath.Rel(parent, sub)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..") && rel != ".."
}

// DefaultEvaluator evaluates the built-in condition types.
//
// Supported keys:
//   - "language:<ext>" — true when the file extension matches (e.g.
//     "language:go" matches files ending in ".go").
//   - "file:<glob>" — true when the file path matches the glob pattern.
//   - "env:<var>" — true when the named environment variable is set (non-empty).
//
// This evaluator has limited context (no file path is threaded through), so
// "language" and "file" conditions always return true when used as the
// default. Callers should pass a custom ConditionEvaluator for file-aware
// evaluation.
func DefaultEvaluator(key, value string) bool {
	switch key {
	case "language":
		return true
	case "file":
		return true
	case "env":
		return os.Getenv(value) != ""
	default:
		return false
	}
}

// FileAwareEvaluator returns a ConditionEvaluator that has access to the
// current file path for "language" and "file" condition evaluation.
func FileAwareEvaluator(filePath string) ConditionEvaluator {
	absPath, _ := filepath.Abs(filePath)
	return func(key, value string) bool {
		switch key {
		case "language":
			ext := "." + value
			return strings.HasSuffix(absPath, ext)
		case "file":
			matched, err := filepath.Match(value, filepath.Base(absPath))
			if err != nil {
				return false
			}
			return matched
		case "env":
			return os.Getenv(value) != ""
		default:
			return false
		}
	}
}

// processJSONIncludes processes @include directives in JSON config data.
// It walks the parsed JSON tree and replaces objects of the form
// {"@include": "path/to/file.json"} with the contents of the referenced
// file. Paths must be relative, contain no ".." components, and resolve
// within baseDir.
func processJSONIncludes(data []byte, baseDir string) ([]byte, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base directory: %w", err)
	}

	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	processed, err := processJSONIncludeValue(parsed, absBase, 0, nil)
	if err != nil {
		return nil, err
	}

	result, err := json.Marshal(processed)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal processed config: %w", err)
	}
	return result, nil
}

// processJSONIncludeValue recursively walks a parsed JSON value and replaces
// {"@include": "path"} objects with the loaded file's contents.
func processJSONIncludeValue(val any, baseDir string, depth int, seen map[string]bool) (any, error) {
	if depth > maxIncludeDepth {
		return nil, fmt.Errorf("@include exceeded maximum depth %d", maxIncludeDepth)
	}
	if seen == nil {
		seen = make(map[string]bool)
	}

	switch v := val.(type) {
	case map[string]any:
		// Detect {"@include": "path"} with exactly one key.
		if includePath, ok := v["@include"].(string); ok && len(v) == 1 {
			if err := validateIncludePath(includePath); err != nil {
				return nil, err
			}

			resolved := filepath.Join(baseDir, includePath)
			absPath, err := filepath.Abs(resolved)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve include path %q: %w", includePath, err)
			}

			if !isSubPath(baseDir, absPath) {
				return nil, fmt.Errorf("@include path %q escapes project directory", includePath)
			}

			if seen[absPath] {
				return nil, fmt.Errorf("@include cycle detected: %q already included", absPath)
			}
			seen[absPath] = true

			raw, err := os.ReadFile(absPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read included file %q: %w", absPath, err)
			}

			if !json.Valid(raw) {
				return nil, fmt.Errorf("included file %q is not valid JSON", absPath)
			}

			var included any
			if err := json.Unmarshal(raw, &included); err != nil {
				return nil, fmt.Errorf("failed to parse included file %q: %w", absPath, err)
			}

			return processJSONIncludeValue(included, filepath.Dir(absPath), depth+1, seen)
		}

		// Recurse into each map value.
		result := make(map[string]any, len(v))
		for key, child := range v {
			processed, err := processJSONIncludeValue(child, baseDir, depth, seen)
			if err != nil {
				return nil, fmt.Errorf(".%s: %w", key, err)
			}
			result[key] = processed
		}
		return result, nil

	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			processed, err := processJSONIncludeValue(item, baseDir, depth, seen)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			result[i] = processed
		}
		return result, nil

	default:
		return val, nil
	}
}

// validateIncludePath rejects absolute paths and paths containing "..".
func validateIncludePath(p string) error {
	if filepath.IsAbs(p) {
		return fmt.Errorf("@include does not support absolute paths: %q", p)
	}
	// Check each path component for ".." to prevent directory traversal.
	for _, part := range strings.Split(p, string(filepath.Separator)) {
		if part == ".." {
			return fmt.Errorf("@include path must not contain '..': %q", p)
		}
	}
	return nil
}
