package tools

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/crush/internal/fsext"
)

const (
	errOldStringNotFound     = "old_string not found in file. Make sure it matches exactly, including whitespace and line breaks"
	errOldStringMultipleHits = "old_string appears multiple times in the file. Please provide more context to ensure a unique match, or set replace_all to true"
)

var (
	errFuzzyNotFound     = errors.New(errOldStringNotFound)
	errFuzzyMultipleHits = errors.New(errOldStringMultipleHits)
)

// fuzzyReplace finds oldString in content using fuzzy matching and replaces it
// with newString. For replaceAll, all occurrences are replaced.
func fuzzyReplace(content, oldString, newString string, replaceAll bool) (string, error) {
	if replaceAll {
		replaced, found := replaceAllWithBestMatch(content, oldString, newString)
		if !found {
			return "", errFuzzyNotFound
		}
		return replaced, nil
	}
	matchedString, found, isMultiple := findBestMatch(content, oldString)
	if !found {
		return "", errFuzzyNotFound
	}
	if isMultiple {
		return "", errFuzzyMultipleHits
	}
	before, after, _ := strings.Cut(content, matchedString)
	return before + newString + after, nil
}

var (
	viewLinePrefixRE     = regexp.MustCompile(`^\s*\d+\|\s?`)
	collapseBlankLinesRE = regexp.MustCompile(`\n{3,}`)
	markdownCodeFenceRE  = regexp.MustCompile("(?s)^\\s*```[^\\n]*\\n(.*)\\n```\\s*$")
)

// findBestMatch attempts to find a match for oldString in content. If an exact
// match is found, it returns the oldString unchanged. Otherwise, it tries
// several normalization strategies to find a fuzzy match.
//
// Returns: (matchedString, found, isMultiple)
//   - matchedString: the actual string found in content that should be used
//   - found: whether any match was found
//   - isMultiple: whether multiple matches were found (ambiguous)
func findBestMatch(content, oldString string) (string, bool, bool) {
	oldString = normalizeOldStringForMatching(oldString)

	// Strategy 1: Exact match.
	index := strings.Index(content, oldString)
	if index != -1 {
		lastIndex := strings.LastIndex(content, oldString)
		return oldString, true, index != lastIndex
	}

	// Strategy 2: Try trimming surrounding blank lines.
	trimmedSurrounding := trimSurroundingBlankLines(oldString)
	if trimmedSurrounding != "" && trimmedSurrounding != oldString {
		index := strings.Index(content, trimmedSurrounding)
		if index != -1 {
			lastIndex := strings.LastIndex(content, trimmedSurrounding)
			return trimmedSurrounding, true, index != lastIndex
		}
	}

	// Strategy 3: Try trimming trailing whitespace from each line of oldString.
	trimmedLines := trimTrailingWhitespacePerLine(oldString)
	if trimmedLines != oldString {
		index := strings.Index(content, trimmedLines)
		if index != -1 {
			lastIndex := strings.LastIndex(content, trimmedLines)
			return trimmedLines, true, index != lastIndex
		}
	}

	// Strategy 4: Try with/without trailing newline.
	if before, ok := strings.CutSuffix(oldString, "\n"); ok {
		withoutTrailing := before
		index := strings.Index(content, withoutTrailing)
		if index != -1 {
			lastIndex := strings.LastIndex(content, withoutTrailing)
			return withoutTrailing, true, index != lastIndex
		}
	} else {
		withTrailing := oldString + "\n"
		index := strings.Index(content, withTrailing)
		if index != -1 {
			lastIndex := strings.LastIndex(content, withTrailing)
			return withTrailing, true, index != lastIndex
		}
	}

	// Strategy 5: Try matching with flexible blank lines (collapse multiple
	// blank lines to single).
	collapsedOld := collapseBlankLines(oldString)
	if collapsedOld != oldString {
		index := strings.Index(content, collapsedOld)
		if index != -1 {
			lastIndex := strings.LastIndex(content, collapsedOld)
			return collapsedOld, true, index != lastIndex
		}
	}

	// Strategy 6: Try normalizing indentation (find content with same structure
	// but different leading whitespace).
	matched, found, isMultiple := tryNormalizeIndentation(content, oldString)
	if found {
		return matched, true, isMultiple
	}

	if collapsedOld != oldString {
		matched, found, isMultiple := tryNormalizeIndentation(content, collapsedOld)
		if found {
			return matched, true, isMultiple
		}
	}

	return "", false, false
}

// FuzzyLookup attempts to find a match for query in the given file content.
// It first tries string-level fuzzy matching via findBestMatch. If that fails
// and tree-sitter is available, it falls back to symbol-level fuzzy lookup.
// Returns the string match if found, or symbol matches as a fallback.
func FuzzyLookup(ctx context.Context, query, filePath string, content []byte) (stringMatch string, found bool, symbols []SymbolMatch) {
	normalized := normalizeOldStringForMatching(query)
	matched, found, _ := findBestMatch(string(content), normalized)
	if found {
		return matched, true, nil
	}

	syms, _ := fuzzySymbolLookup(ctx, query, filePath, content)
	return "", false, syms
}

func normalizeOldStringForMatching(oldString string) string {
	oldString, _ = fsext.ToUnixLineEndings(oldString)
	oldString = stripZeroWidthCharacters(oldString)
	oldString = stripMarkdownCodeFences(oldString)
	oldString = stripViewLineNumbers(oldString)
	oldString = stripAnchorMarkers(oldString)
	return oldString
}

func stripZeroWidthCharacters(s string) string {
	s = strings.ReplaceAll(s, "\ufeff", "")
	s = strings.ReplaceAll(s, "\u200b", "")
	s = strings.ReplaceAll(s, "\u200c", "")
	s = strings.ReplaceAll(s, "\u200d", "")
	s = strings.ReplaceAll(s, "\u2060", "")
	return s
}

func stripMarkdownCodeFences(s string) string {
	m := markdownCodeFenceRE.FindStringSubmatch(s)
	if len(m) != 2 {
		return s
	}
	return m[1]
}

func stripViewLineNumbers(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return s
	}

	var withPrefix int
	for _, line := range lines {
		if viewLinePrefixRE.MatchString(line) {
			withPrefix++
		}
	}

	if withPrefix < (len(lines)+1)/2 {
		return s
	}

	for i, line := range lines {
		lines[i] = viewLinePrefixRE.ReplaceAllString(line, "")
	}

	return strings.Join(lines, "\n")
}

func trimSurroundingBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}

	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}

	return strings.Join(lines[start:end], "\n")
}

// replaceAllWithBestMatch replaces all occurrences of oldString in content
// with newString, using fuzzy matching strategies if an exact match fails.
func replaceAllWithBestMatch(content, oldString, newString string) (string, bool) {
	oldString = normalizeOldStringForMatching(oldString)
	if oldString == "" {
		return "", false
	}

	if strings.Contains(content, oldString) {
		return strings.ReplaceAll(content, oldString, newString), true
	}

	newContent, ok := tryReplaceAllWithFlexibleMultilineRegexp(content, oldString, newString)
	if ok {
		return newContent, true
	}

	collapsedOld := collapseBlankLines(oldString)
	if collapsedOld != oldString {
		newContent, ok := tryReplaceAllWithFlexibleMultilineRegexp(content, collapsedOld, newString)
		if ok {
			return newContent, true
		}
	}

	matchedString, found, _ := findBestMatch(content, oldString)
	if !found || matchedString == "" {
		return "", false
	}
	return strings.ReplaceAll(content, matchedString, newString), true
}

func tryReplaceAllWithFlexibleMultilineRegexp(content, oldString, newString string) (string, bool) {
	re := buildFlexibleMultilineRegexp(oldString)
	if re == nil {
		return "", false
	}

	if !re.MatchString(content) {
		return "", false
	}

	newContent := re.ReplaceAllStringFunc(content, func(string) string {
		return newString
	})
	return newContent, true
}

func buildFlexibleMultilineRegexp(oldString string) *regexp.Regexp {
	oldString = normalizeOldStringForMatching(oldString)
	lines := strings.Split(oldString, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) < 2 {
		return nil
	}

	patternParts := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmedLeft := strings.TrimLeft(line, " \t")
		trimmed := strings.TrimRight(trimmedLeft, " \t")
		if trimmed == "" {
			patternParts = append(patternParts, `^[ \t]*$`)
			continue
		}
		escaped := regexp.QuoteMeta(trimmed)
		patternParts = append(patternParts, `^[ \t]*`+escaped+`[ \t]*$`)
	}

	pattern := "(?m)" + strings.Join(patternParts, "\n")
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return re
}

// trimTrailingWhitespacePerLine removes trailing spaces/tabs from each line.
func trimTrailingWhitespacePerLine(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// collapseBlankLines replaces multiple consecutive blank lines with a single
// blank line.
func collapseBlankLines(s string) string {
	return collapseBlankLinesRE.ReplaceAllString(s, "\n\n")
}

// tryNormalizeIndentation attempts to find a match by adjusting indentation.
// It extracts the "shape" of the code (non-whitespace content per line) and
// looks for that pattern in the content with potentially different
// indentation.
func tryNormalizeIndentation(content, oldString string) (string, bool, bool) {
	re := buildFlexibleMultilineRegexp(oldString)
	if re == nil {
		return "", false, false
	}

	matches := re.FindAllStringIndex(content, 2)
	if len(matches) == 0 {
		return "", false, false
	}
	if len(matches) > 1 {
		return content[matches[0][0]:matches[0][1]], true, true
	}
	return content[matches[0][0]:matches[0][1]], true, false
}

// SymbolMatch represents a symbol found by tree-sitter fuzzy lookup.
type SymbolMatch struct {
	Name     string
	Kind     string // e.g., "function", "type", "method", "struct".
	Line     int    // 1-based line number.
	Score    int    // Higher is better.
	FilePath string
}

// symbolParser is a minimal interface for tree-sitter analysis. This avoids
// importing the treesitter package directly (which requires CGO). The real
// implementation is provided via build-tag-conditional files.
type symbolParser interface {
	Analyze(ctx context.Context, path string, content []byte) (*symbolAnalysis, error)
}

// symbolAnalysis mirrors the subset of treesitter.FileAnalysis needed for
// symbol fuzzy matching.
type symbolAnalysis struct {
	Symbols []symbolDef
}

// symbolDef mirrors the subset of treesitter.SymbolInfo needed for fuzzy
// matching.
type symbolDef struct {
	Name string
	Kind string
	Line int
}

// globalSymbolParser holds the optional tree-sitter parser for symbol lookup.
// Set via SetSymbolParser; nil when tree-sitter is unavailable.
var globalSymbolParser symbolParser

// SetSymbolParser sets the global tree-sitter parser used for symbol-level
// fuzzy matching. Pass nil to disable symbol lookup.
func SetSymbolParser(p symbolParser) {
	globalSymbolParser = p
}

// fuzzySymbolLookup attempts to find symbol definitions in filePath whose
// names fuzzy-match the query. Returns matches sorted by descending score.
// Returns an empty slice (not an error) if tree-sitter is unavailable or the
// file cannot be parsed.
func fuzzySymbolLookup(ctx context.Context, query, filePath string, content []byte) ([]SymbolMatch, error) {
	if globalSymbolParser == nil || query == "" {
		return nil, nil
	}

	analysis, err := globalSymbolParser.Analyze(ctx, filePath, content)
	if err != nil {
		return nil, nil //nolint:nilerr // Graceful fallback on parse errors.
	}
	if analysis == nil {
		return nil, nil
	}

	var matches []SymbolMatch
	for _, sym := range analysis.Symbols {
		score := fuzzyMatchScore(query, sym.Name)
		if score > 0 {
			kind := sym.Kind
			if kind == "" {
				kind = "symbol"
			}
			matches = append(matches, SymbolMatch{
				Name:     sym.Name,
				Kind:     kind,
				Line:     sym.Line,
				Score:    score,
				FilePath: filePath,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Name < matches[j].Name
	})

	return matches, nil
}

// fuzzyMatchScore returns a score indicating how well query matches target
// using subsequence matching. A score of 0 means no match. Higher scores
// indicate better matches. The scoring rewards:
//   - Matching at word boundaries (camelCase, underscore, start of string)
//   - Consecutive character matches
//   - Shorter target names (normalized by length)
func fuzzyMatchScore(query, target string) int {
	if query == "" || target == "" {
		return 0
	}

	qRunes := []rune(strings.ToLower(query))
	tRunes := []rune(strings.ToLower(target))

	qi := 0
	score := 0
	lastMatchIdx := -2

	for ti, tr := range tRunes {
		if qi >= len(qRunes) {
			break
		}

		if tr == qRunes[qi] {
			if ti == 0 || isWordBoundary(tRunes, ti) {
				score += 3
			}
			if lastMatchIdx == ti-1 {
				score += 2
			}
			score += 1
			lastMatchIdx = ti
			qi++
		}
	}

	// Subsequence check: all query chars must be matched.
	if qi < len(qRunes) {
		return 0
	}

	// Prefer shorter names; penalize length.
	score = score * 10 / (len(tRunes) + 1)

	return score
}

// isWordBoundary reports whether position i in runes is the start of a new
// word segment (camelCase transition, underscore boundary, or hyphen
// boundary).
func isWordBoundary(runes []rune, i int) bool {
	if i == 0 {
		return true
	}
	prev := runes[i-1]
	curr := runes[i]
	if prev == '_' || prev == '-' {
		return true
	}
	if unicode.IsLower(prev) && unicode.IsUpper(curr) {
		return true
	}
	return false
}
