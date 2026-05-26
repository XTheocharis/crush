package explorer

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// MarkdownExplorer explores Markdown files.
type MarkdownExplorer struct{}

func (e *MarkdownExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	return ext == "md" || ext == "markdown"
}

func (e *MarkdownExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("Markdown file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "markdown", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	result := e.analyzeMarkdown(content, filepath.Base(input.Path))

	return ExploreResult{
		Summary:       result.summary,
		ExplorerUsed:  "markdown",
		TokenEstimate: estimateTokens(result.summary),
	}, nil
}

type markdownAnalysis struct {
	summary string
}

func (e *MarkdownExplorer) analyzeMarkdown(content, filename string) markdownAnalysis {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Markdown file: %s\n", filename)
	fmt.Fprintf(&sb, "Size: %d bytes\n", len(content))

	// Frontmatter analysis and extraction
	hasFrontmatter, frontmatterContent, frontmatterEndOffset := e.extractFrontmatterWithOffset(content)
	keyCount := 0
	if hasFrontmatter {
		// Count top-level keys in YAML
		var parsed any
		if err := yaml.Unmarshal(frontmatterContent, &parsed); err == nil {
			if m, ok := parsed.(map[string]any); ok {
				keyCount = len(m)
			}
		}
	}
	fmt.Fprintf(&sb, "Frontmatter: %v\n", hasFrontmatter)
	if hasFrontmatter {
		fmt.Fprintf(&sb, "Frontmatter keys: %d\n", keyCount)
	}

	// Content to analyze for headings, code blocks, links (exclude frontmatter)
	var contentToAnalyze string
	if hasFrontmatter && frontmatterEndOffset > 0 {
		contentToAnalyze = content[frontmatterEndOffset:]
	} else {
		contentToAnalyze = content
	}

	// Heading hierarchy analysis
	headings := e.extractHeadings(contentToAnalyze)
	hCounts := [6]int{0, 0, 0, 0, 0, 0} // h1..h6
	for _, h := range headings {
		if h.level >= 1 && h.level <= 6 {
			hCounts[h.level-1]++
		}
	}
	totalHeadings := len(headings)
	sb.WriteString("\nHeading hierarchy:\n")
	fmt.Fprintf(&sb, "  H1: %d\n", hCounts[0])
	fmt.Fprintf(&sb, "  H2: %d\n", hCounts[1])
	fmt.Fprintf(&sb, "  H3: %d\n", hCounts[2])
	fmt.Fprintf(&sb, "  H4: %d\n", hCounts[3])
	fmt.Fprintf(&sb, "  H5: %d\n", hCounts[4])
	fmt.Fprintf(&sb, "  H6: %d\n", hCounts[5])
	fmt.Fprintf(&sb, "  Total: %d\n", totalHeadings)

	// Fenced code block language histogram
	codeBlocks := e.extractFencedCodeBlocks(contentToAnalyze)
	langHist := make(map[string]int)
	for _, cb := range codeBlocks {
		lang := cb.lang
		if lang == "" {
			lang = "unknown/plain"
		}
		langHist[lang]++
	}
	if len(langHist) > 0 {
		sb.WriteString("\nFenced code blocks:\n")
		langs := make([]string, 0, len(langHist))
		for lang := range langHist {
			langs = append(langs, lang)
		}
		sort.Strings(langs)
		for _, lang := range langs {
			fmt.Fprintf(&sb, "  %s: %d\n", lang, langHist[lang])
		}
	}

	// Link/reference counts
	inlineLinks := e.countInlineLinks(contentToAnalyze)
	refLinks := e.countReferenceLinks(contentToAnalyze)
	autolinks := e.countAutolinks(contentToAnalyze)
	refDefs := e.countReferenceDefinitions(contentToAnalyze)

	sb.WriteString("\nLinks:\n")
	fmt.Fprintf(&sb, "  Inline links (markdown style): %d\n", inlineLinks)
	fmt.Fprintf(&sb, "  Reference-style links: %d\n", refLinks)
	fmt.Fprintf(&sb, "  Autolinks (http/https URLs): %d\n", autolinks)
	fmt.Fprintf(&sb, "  Reference definitions: %d\n", refDefs)

	return markdownAnalysis{summary: sb.String()}
}

func (e *MarkdownExplorer) extractFrontmatter(content string) (found bool, frontmatter []byte) {
	found, fm, _ := e.extractFrontmatterWithOffset(content)
	return found, fm
}

func (e *MarkdownExplorer) extractFrontmatterWithOffset(content string) (found bool, frontmatter []byte, endOffset int) {
	// Frontmatter must start at line 0, not after leading whitespace
	// Check for any leading whitespace (space, tab, carriage return) but NOT newline
	leadingWhitespaceCount := 0
outer:
	for i := 0; i < len(content); i++ {
		switch content[i] {
		case ' ', '\t', '\r':
			leadingWhitespaceCount++
		case '\n':
			// Newline before content means no frontmatter possible at this position
			return false, nil, 0
		default:
			break outer
		}
	}

	// If there's leading whitespace, frontmatter is not valid
	if leadingWhitespaceCount > 0 {
		return false, nil, 0
	}

	// Content must start with ---
	if !strings.HasPrefix(content, "---") {
		return false, nil, 0
	}
	// Find first newline after opening "---"
	firstNewline := strings.Index(content[3:], "\n")
	if firstNewline == -1 {
		// Only "---" on first line, rest of content is on same line - not valid frontmatter
		return false, nil, 0
	}
	// Content starts after first newline
	contentStart := 4 + firstNewline
	remaining := content[contentStart:]

	// Check if remaining starts with "---" (possible empty inline frontmatter)
	// Pattern: ---\n---\n where content between is empty
	if strings.HasPrefix(remaining, "---\n") {
		endOffset = contentStart + 4
		return true, []byte{}, endOffset
	}

	// Look for closing "---" delimiter
	// Must be on its own line (possibly preceded by whitespace)
	// Pattern: \s*---\s*\n or \s*---$ (end of string)
	re := regexp.MustCompile(`(?m)^\s*---\s*$`)
	matches := re.FindAllStringIndex(remaining, -1)

	if len(matches) == 0 {
		// No closing delimiter found - check for implicit closing via blank lines
		// Pattern: ---\n\ncontent where blank lines before content act as implicit close
		// Skip leading blank lines in remaining
		remainingTrimmed := strings.TrimLeft(remaining, "\n\r\t ")
		if remaining != remainingTrimmed && remainingTrimmed != "" {
			// We had blank lines followed by content - treat as empty frontmatter
			blankLinesLen := len(remaining) - len(remainingTrimmed)
			endOffset = contentStart + blankLinesLen
			return true, []byte{}, endOffset
		}
		return false, nil, 0
	}

	// First match is the closing delimiter
	// The frontmatter content is everything before the closing delimiter's line
	closingIdx := matches[0][0]
	fmContent := remaining[:closingIdx]

	// Calculate end offset: position after closing "---" delimiter line in original content
	// The closing delimiter in content starts at: contentStart + closingIdx
	lineEndIdx := contentStart + matches[0][1]

	// Find the end of the line containing the closing delimiter
	if lineEndIdx < len(content) {
		nextNewline := strings.Index(content[lineEndIdx:], "\n")
		if nextNewline == -1 {
			// No newline after closing delimiter, end at end of content
			endOffset = len(content)
		} else {
			endOffset = lineEndIdx + nextNewline + 1 // +1 to include the newline
		}
	} else {
		endOffset = len(content)
	}

	return true, []byte(fmContent), endOffset
}

type heading struct {
	level int
	text  string
	line  int
}

func (e *MarkdownExplorer) extractHeadings(content string) []heading {
	var headings []heading
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		lineTrimmed := strings.TrimRight(line, " \t")
		// ATX-style headings: # heading, ## heading, etc.
		if strings.HasPrefix(lineTrimmed, "#") {
			level := 0
			for j := 0; j < len(lineTrimmed); j++ {
				if lineTrimmed[j] == '#' {
					level++
				} else {
					break
				}
			}
			// Valid ATX heading: 1-6 # signs at start
			isValid := false
			if level >= 1 && level <= 6 {
				if level >= len(lineTrimmed) {
					// Just # signs (like "###") - not valid heading
					isValid = false
				} else if lineTrimmed[level] == ' ' || lineTrimmed[level] == '\t' {
					// Standard ATX: "## Text" (CommonMark spec)
					isValid = true
				} else {
					// No space after #: Check if it looks like a heading vs hashtag
					// Heuristic: Check if there's a space in what comes next AND
					// the first character is capital letter or there's other heading-like content
					rest := lineTrimmed[level:]
					if !strings.Contains(rest, " ") {
						// Single word without any space - definitely a hashtag like "#hashtag"
						isValid = false
					} else {
						// Has space - check if first word looks hashtag-like
						firstWord := ""
						for j := 0; j < len(rest); j++ {
							if rest[j] == ' ' {
								break
							}
							firstWord += string(rest[j])
						}
						// If first word is all lowercase and simple, treat as hashtag
						// But if it starts with capital letter or contains special chars, treat as heading
						looksLikeHeading := false
						if len(firstWord) == 0 {
							looksLikeHeading = false
						} else {
							// Check if first character is uppercase
							if firstWord[0] >= 'A' && firstWord[0] <= 'Z' {
								looksLikeHeading = true
							} else {
								// Check if the line looks more like a heading (more complexity)
								// For now, be conservative: reject single-word lowercase patterns
								// even if they have trailing words
								looksLikeHeading = false
							}
						}
						isValid = looksLikeHeading
					}
				}
			}
			if isValid {
				// Extract text and trim space
				text := strings.TrimSpace(lineTrimmed[level:])
				// Remove trailing closing #'s
				// Pattern: # Text ###, ## Text #### etc.
				trailingHashes := 0
				for j := len(text) - 1; j >= 0 && text[j] == '#'; j-- {
					trailingHashes++
				}
				if trailingHashes > 0 {
					// Remove the trailing hashes and any spaces before them
					text = strings.TrimRight(text[:len(text)-trailingHashes], " \t")
				}
				headings = append(headings, heading{level: level, text: text, line: i + 1})
				continue
			}
		}

		// Setext-style headings: ---- or ==== on next line
		if i > 0 {
			prevLine := strings.TrimRight(lines[i-1], " \t")
			trimmed := strings.TrimLeft(line, " \t")
			if len(trimmed) == 0 {
				continue
			}
			if allSameChar(trimmed, '=') {
				headings = append(headings, heading{level: 1, text: prevLine, line: i})
				continue
			}
			if allSameChar(trimmed, '-') {
				headings = append(headings, heading{level: 2, text: prevLine, line: i})
				continue
			}
		}
	}
	return headings
}

func allSameChar(s string, c rune) bool {
	for _, r := range s {
		if r != c {
			return false
		}
	}
	return len(s) > 0
}

type codeBlock struct {
	lang string
}

func (e *MarkdownExplorer) extractFencedCodeBlocks(content string) []codeBlock {
	var blocks []codeBlock
	lines := strings.Split(content, "\n")
	inBlock := false
	var currentLang string

	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if after, ok := strings.CutPrefix(trimmed, "```"); ok {
			if !inBlock {
				// Opening fence - extract language spec
				inBlock = true
				langSpec := after
				currentLang = strings.TrimSpace(langSpec)
				// Language can be empty or contain multiple words
				if currentLang != "" {
					// Take the first word as the language tag
					parts := strings.Fields(currentLang)
					if len(parts) > 0 {
						currentLang = parts[0]
					}
				}
			} else {
				// Closing fence - record the block
				blocks = append(blocks, codeBlock{lang: currentLang})
				inBlock = false
				currentLang = ""
			}
		}
	}
	return blocks
}

func (e *MarkdownExplorer) countInlineLinks(content string) int {
	// Pattern for inline links: [text](url "title")
	re := regexp.MustCompile(`\[[^\]]*\]\([^\)]+\)`)
	return len(re.FindAllString(content, -1))
}

func (e *MarkdownExplorer) countReferenceLinks(content string) int {
	// Pattern for reference-style links: [text][id] where id is non-empty
	// This excludes [text][] and also shouldn't match [id]: definitions
	re := regexp.MustCompile(`\[[^\]]+\]\[[^\]]+\]`)
	matches := re.FindAllString(content, -1)
	count := 0

	// Find all reference definition positions
	refDefRe := regexp.MustCompile(`(?m)^\s{0,3}\[[^\]]+\]:`)
	refDefIndices := refDefRe.FindAllStringIndex(content, -1)

	for _, match := range matches {
		// Check if this match is actually part of a reference definition
		// [id]: could be partially matched by [text][id] pattern
		isDefinition := false
		for _, defRange := range refDefIndices {
			// If the match overlaps significantly with a definition, skip it
			matchStart := strings.Index(content, match)
			overlap := matchStart+len(match) > defRange[0] && matchStart < defRange[1]
			if overlap {
				isDefinition = true
				break
			}
		}
		if !isDefinition {
			count++
		}
	}
	return count
}

func (e *MarkdownExplorer) countAutolinks(content string) int {
	// Pattern for autolinks: <http://...> or <https://...>
	re := regexp.MustCompile(`<https?://[^>]+>`)
	return len(re.FindAllString(content, -1))
}

func (e *MarkdownExplorer) countReferenceDefinitions(content string) int {
	// Pattern for reference definitions: [id]: url "title"
	// Must be at start of line (with optional leading whitespace)
	re := regexp.MustCompile(`(?m)^\s{0,3}\[[^\]]+\]:\s+\S+`)
	return len(re.FindAllString(content, -1))
}
