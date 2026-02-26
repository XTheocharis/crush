package repomap

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"charm.land/fantasy"
)

var mentionPathPattern = regexp.MustCompile(`[^\s"'\(\)\[\]\{\}<>]+`)

// isWordChar returns true if c is a letter, digit, or underscore.
func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// ExtractCurrentRunMessages extracts messages from the prepared array for the
// current LLM run. It finds the last system message and returns all messages
// after it, which represent the current conversation turn.
func ExtractCurrentRunMessages(prepared []fantasy.Message) []fantasy.Message {
	if len(prepared) == 0 {
		return nil
	}

	lastSystemIdx := -1
	for i, msg := range prepared {
		if msg.Role == fantasy.MessageRoleSystem {
			lastSystemIdx = i
		}
	}

	// If there are no system messages, return all messages
	if lastSystemIdx < 0 {
		return prepared
	}

	// If the last system message is the last in the array, return nil (no current run messages)
	if lastSystemIdx >= len(prepared)-1 {
		return nil
	}

	// Return messages after the last system message
	return prepared[lastSystemIdx+1:]
}

// ExtractCurrentMessageText extracts all text content from a slice of messages.
// It concatenates content from all TextPart parts across all messages.
func ExtractCurrentMessageText(messages []fantasy.Message) string {
	if len(messages) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, msg := range messages {
		for _, part := range msg.Content {
			if textPart, ok := part.(fantasy.TextPart); ok {
				sb.WriteString(textPart.Text)
			}
		}
	}
	return sb.String()
}

// ExtractMentionedFnames extracts filenames mentioned in the given text.
// It supports two mention styles:
//  1. Exact relative path mention (e.g., "path/to/file.go")
//  2. Unique basename mention (e.g., "file.go") - only added if the basename
//     doesn't already exist in inChatOrReadOnlyFiles
//
// Results are returned as deterministic sorted normalized paths.
func ExtractMentionedFnames(text string, addableRepoFiles []string, inChatOrReadOnlyFiles []string) []string {
	if text == "" {
		return nil
	}

	// Normalize and build set of already-present files
	inChatSet := make(map[string]struct{}, len(inChatOrReadOnlyFiles))
	basenameInChatSet := make(map[string]struct{}, len(inChatOrReadOnlyFiles))

	for _, f := range normalizeUniqueGraphPaths(inChatOrReadOnlyFiles) {
		inChatSet[f] = struct{}{}
		basename := path.Base(f)
		if basename != "" {
			basenameInChatSet[basename] = struct{}{}
		}
	}

	// Build normalized addable file set and basename map for uniqueness check
	addableSet := make(map[string]struct{}, len(addableRepoFiles))
	addableNormalized := normalizeUniqueGraphPaths(addableRepoFiles)

	for _, f := range addableNormalized {
		addableSet[f] = struct{}{}
	}

	// Build basename to file map for unique basename detection
	basenameMap := make(map[string][]string)
	for _, f := range addableNormalized {
		basename := path.Base(f)
		if basename != "" {
			basenameMap[basename] = append(basenameMap[basename], f)
		}
	}

	mentioned := make(map[string]struct{})

	// First pass: try exact relative path matches.
	// Match any sequence of non-whitespace that looks like a path.
	for _, match := range mentionPathPattern.FindAllString(text, -1) {
		candidate := strings.TrimSpace(match)
		if candidate == "" {
			continue
		}

		normalized := normalizeGraphRelPath(candidate)
		if normalized == "" {
			continue
		}

		// Check if the normalized path is an exact match in addable files
		if _, exists := addableSet[normalized]; exists {
			mentioned[normalized] = struct{}{}
			// Remove from basename map to avoid duplicate basename matching
			delete(basenameMap, path.Base(normalized))
		}
	}

	// Second pass: unique basename matches - only for basenames mentioned in text
	for basename, files := range basenameMap {
		// Skip if basename is already in chat files
		if _, inChat := basenameInChatSet[basename]; inChat {
			continue
		}
		// Only add if unique basename (single file with this basename)
		// and the basename appears in the text
		if len(files) == 1 {
			// Check if this basename is mentioned in the text
			if strings.Contains(text, basename) {
				mentioned[files[0]] = struct{}{}
			}
		}
	}

	if len(mentioned) == 0 {
		return nil
	}

	result := make([]string, 0, len(mentioned))
	for p := range mentioned {
		result = append(result, p)
	}
	sort.Strings(result)
	return result
}

// ExtractIdents extracts identifiers from the given text by splitting on
// non-word characters. Empty tokens are ignored.
func ExtractIdents(text string) []string {
	if text == "" {
		return nil
	}

	seen := make(map[string]struct{})
	var ident strings.Builder
	ident.Grow(32) // Pre-allocate reasonable size

	for i := 0; i < len(text); i++ {
		c := text[i]
		if isWordChar(c) {
			ident.WriteByte(c)
		} else {
			if ident.Len() > 0 {
				seen[ident.String()] = struct{}{}
				ident.Reset()
				ident.Grow(32)
			}
		}
	}

	// Handle trailing identifier
	if ident.Len() > 0 {
		seen[ident.String()] = struct{}{}
	}

	if len(seen) == 0 {
		return nil
	}

	result := make([]string, 0, len(seen))
	for ident := range seen {
		result = append(result, ident)
	}
	sort.Strings(result)
	return result
}

// IdentFilenameMatches matches identifiers to repository files based on
// lowercase stem matching. For each identifier, it checks if the lowercase
// version exists as a stem in any filename (stem = filename without extension).
// Only identifiers with length >= 5 are considered.
//
// Returns deterministic sorted list of matching filenames.
func IdentFilenameMatches(idents []string, allRepoFiles []string) []string {
	if len(idents) == 0 || len(allRepoFiles) == 0 {
		return nil
	}

	// Normalize files
	normalizedFiles := normalizeUniqueGraphPaths(allRepoFiles)

	// Build stem (basename without extension) to file map
	stemMap := make(map[string][]string)
	for _, f := range normalizedFiles {
		basename := path.Base(f)
		ext := path.Ext(basename)
		stem := strings.ToLower(strings.TrimSuffix(basename, ext))
		if stem != "" {
			stemMap[stem] = append(stemMap[stem], f)
		}
	}

	matched := make(map[string]struct{})

	for _, ident := range idents {
		ident = strings.TrimSpace(ident)
		if len(ident) < 5 {
			continue // Skip short identifiers
		}
		lowerIdent := strings.ToLower(ident)

		// First, try exact basename match (if ident contains an extension)
		if strings.Contains(ident, ".") {
			for _, f := range normalizedFiles {
				if strings.ToLower(path.Base(f)) == lowerIdent {
					matched[f] = struct{}{}
				}
			}
		}

		// Check if lowercase ident matches any stem
		if files, exists := stemMap[lowerIdent]; exists {
			for _, f := range files {
				matched[f] = struct{}{}
			}
		}
	}

	if len(matched) == 0 {
		return nil
	}

	result := make([]string, 0, len(matched))
	for p := range matched {
		result = append(result, p)
	}
	sort.Strings(result)
	return result
}
