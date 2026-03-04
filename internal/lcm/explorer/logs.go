package explorer

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// LogsExplorer handles log files, analyzing log levels, timestamp patterns,
// and sampling error/warning messages.
type LogsExplorer struct {
	formatterProfile OutputProfile
}

// logLevels captures common log level patterns, ordered by severity (highest first).
// Patterns are ordered so bracketed patterns are matched first to capture
// the exact format (e.g., "[ERROR]" vs "ERROR").
var logLevels = []struct {
	name     string
	patterns []*regexp.Regexp
}{
	{
		name: "ERROR",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\[.*?ERROR.*?\]`),
			regexp.MustCompile(`^\[E\]\s`),
			regexp.MustCompile(`(?i)\b(?:ERROR|FATAL|CRITICAL|FAIL(?:URE)?|PANIC|EMRG|EMERG)\b`),
		},
	},
	{
		name: "WARN",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\[.*?WARN(?:ING)?.*\]`),
			regexp.MustCompile(`^\[W\]\s`),
			regexp.MustCompile(`(?i)\b(?:WARN(?:ING)?|ALERT)\b`),
		},
	},
	{
		name: "INFO",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\[.*?INFO.*?\]`),
			regexp.MustCompile(`^\[I\]\s`),
			regexp.MustCompile(`(?i)\b(?:INFO|INFORMATION|NOTE)\b`),
		},
	},
	{
		name: "DEBUG",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\[.*?DEBUG.*?\]`),
			regexp.MustCompile(`^\[D\]\s`),
			regexp.MustCompile(`(?i)\b(?:DEBUG|DBG|VERBOSE)\b`),
		},
	},
	{
		name: "TRACE",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\[.*?TRACE.*?\]`),
			regexp.MustCompile(`^\[T\]\s`),
			regexp.MustCompile(`(?i)\b(?:TRACE|TRC)\b`),
		},
	},
}

// timestampPatterns captures common timestamp formats.
// Patterns are ordered from most specific to least specific to avoid overlap.
var timestampPatterns = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{
		name:    "RFC3339",
		pattern: regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})`),
	},
	{
		name:    "ISO8601",
		pattern: regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?`),
	},
	{
		name:    "CommonLog",
		pattern: regexp.MustCompile(`\d{2}/\w{3}/\d{4}:\d{2}:\d{2}:\d{2}`),
	},
	{
		name:    "Syslog",
		pattern: regexp.MustCompile(`\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}`),
	},
	{
		name:    "CompactDateTime",
		pattern: regexp.MustCompile(`\d{14}`),
	},
	{
		name:    "UnixTime",
		pattern: regexp.MustCompile(`\b(?:^|\D)(1[0-9]{9}|[1-9][0-9]{9})(?:\.\d+)?(?:\D|$)`),
	},
	{
		name:    "CompactDate",
		pattern: regexp.MustCompile(`\d{8}`),
	},
}

// logLinePatterns captures strong indicators that a file is a log file.
var logLinePatterns = []*regexp.Regexp{
	// Standard [LEVEL] prefix
	regexp.MustCompile(`^\[(ERROR|WARN|INFO|DEBUG|TRACE|FATAL|CRITICAL)\]`),
	regexp.MustCompile(`^\[(E|W|I|D|T|V)\]\s`),
	// Common log format patterns
	regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d+\s+\[`),
	// Syslog pattern using timestampPatterns[3] which is defined as "\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}"
	timestampPatterns[3].pattern,
	// Java stack trace start - "at " lines (lines are trimmed before matching)
	regexp.MustCompile(`^at\s+`),
	// Common log level prefixes without brackets
	regexp.MustCompile(`^(ERROR|WARN(?:ING)?|INFO|DEBUG|TRACE|FATAL|CRITICAL):\s*`),
}

// logExtensions defines file extensions that are typically log files.
var logExtensions = map[string]bool{
	"log":    true,
	"stderr": true,
	"stdout": true,
	"txt":    false, // Text files are handled by TextExplorer unless they match log patterns
}

const (
	// maxSampleSize is the maximum number of error/warning samples to collect.
	maxSampleSize = 10
	// maxSampleLineLength is the maximum length of a sampled line.
	maxSampleLineLength = 200
	// logDetectionThreshold is the minimum ratio of lines matching log patterns.
	logDetectionThreshold = 0.15
	// maxLinesToScan is the maximum number of lines to scan for detection.
	maxLinesToScan = 500
	// maxSignatures is the maximum number of error signatures to display.
	maxSignatures = 10
	// maxSignatureLength is the maximum length of a signature to display.
	maxSignatureLength = 150
)

// CanHandle returns true if the file appears to be a log file based on
// extension and/or content patterns.
func (e *LogsExplorer) CanHandle(path string, content []byte) bool {
	// Check extension first - explicit log extensions are strongly indicative.
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if logExtensions[ext] {
		return true
	}

	// For .txt files or unknown extensions, check content patterns.
	// This allows us to detect log files without log extensions.
	if len(content) == 0 {
		return false
	}

	// Convert to string and count lines matching log patterns.
	contentStr := string(content)
	lines := strings.Split(contentStr, "\n")
	matchingLines := 0
	linesToCheck := min(len(lines), maxLinesToScan)

	for i := range linesToCheck {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		for _, pattern := range logLinePatterns {
			if pattern.MatchString(line) {
				matchingLines++
				break
			}
		}
	}

	// If enough lines match log patterns, consider it a log file.
	if matchingLines > 0 {
		ratio := float64(matchingLines) / float64(linesToCheck)
		return ratio >= logDetectionThreshold
	}

	return false
}

// Explore analyzes the log file and returns a structured summary.
func (e *LogsExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	var summary strings.Builder

	fmt.Fprintf(&summary, "Log file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	// Parse the log content.
	lines := strings.Split(string(input.Content), "\n")
	totalLines := len(lines)
	fmt.Fprintf(&summary, "Total lines: %d\n", totalLines)

	// Count levels and detect timestamp patterns in parallel.
	var wg sync.WaitGroup

	levelCounts := make(map[string]int)
	tsPatternCounts := make(map[string]int)

	// Count log levels.
	wg.Go(func() {
		countLogLevels(lines, levelCounts)
	})

	// Count timestamp patterns.
	wg.Go(func() {
		countTimestampPatterns(lines, tsPatternCounts)
	})

	wg.Wait()

	// Write level distribution.
	if len(levelCounts) > 0 {
		summary.WriteString("\nLevel distribution:\n")
		// Sort levels in severity order for consistent output.
		orderedLevels := orderedLevelNames(levelCounts)
		for _, level := range orderedLevels {
			count := levelCounts[level]
			percentage := float64(count) * 100 / float64(totalLines)
			fmt.Fprintf(&summary, "  %s: %d (%.1f%%)\n", level, count, percentage)
		}
	} else {
		summary.WriteString("\nNo standard log levels detected.\n")
	}

	// Write timestamp patterns.
	if len(tsPatternCounts) > 0 {
		summary.WriteString("\nTimestamp patterns:\n")
		sortedPatterns := sortedTimestampPatternNames(tsPatternCounts)
		for _, pattern := range sortedPatterns {
			count := tsPatternCounts[pattern]
			fmt.Fprintf(&summary, "  %s: %d occurrences\n", pattern, count)
		}
	} else {
		summary.WriteString("\nNo standard timestamp patterns detected.\n")
	}

	// Sample errors and warnings.
	samples := sampleErrorsAndWarnings(lines)
	if len(samples) > 0 {
		summary.WriteString("\nSample errors/warnings:\n")
		for i, sample := range samples {
			fmt.Fprintf(&summary, "  %d. %s\n", i+1, sample)
		}
	}

	// EXCEED MODE: Repeated error-signature aggregation
	if e.formatterProfile == OutputProfileEnhancement {
		signatures := aggregateErrorSignatures(lines)
		if len(signatures) > 0 {
			summary.WriteString("\nRepeated error signatures:\n")
			for i, sig := range signatures {
				if i >= maxSignatures {
					overflow := overflowMarker(OutputProfileEnhancement, len(signatures)-maxSignatures, false)
					fmt.Fprintf(&summary, "  %s\n", overflow)
					break
				}
				sigDisplay := sig.signature
				if len(sigDisplay) > maxSignatureLength {
					sigDisplay = sigDisplay[:maxSignatureLength] + "..."
				}
				fmt.Fprintf(&summary, "  %s: %d occurrences\n", sigDisplay, sig.count)
			}
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "logs",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// countLogLevels counts occurrences of each log level in the lines.
func countLogLevels(lines []string, counts map[string]int) {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Check each level group in order of severity (ERROR first).
		// Only the first matching level group is counted to avoid double-counting.
		for _, level := range logLevels {
			for _, pattern := range level.patterns {
				if pattern.MatchString(line) {
					counts[level.name]++
					// Break both loops and move to next line after finding match.
					goto nextLine
				}
			}
		}
	nextLine:
	}
}

// countTimestampPatterns counts occurrences of each timestamp pattern.
func countTimestampPatterns(lines []string, counts map[string]int) {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, ts := range timestampPatterns {
			matches := ts.pattern.FindAllString(line, -1)
			if matches != nil {
				counts[ts.name] += len(matches)
				// Break after matching the first timestamp type for the line.
				break
			}
		}
	}
}

// sampleErrorsAndWarnings deterministically samples error and warning lines.
func sampleErrorsAndWarnings(lines []string) []string {
	var errorLines []string
	var warnLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for error patterns.
		for _, pattern := range logLevels[0].patterns {
			if pattern.MatchString(line) {
				errorLines = append(errorLines, truncateSample(line, maxSampleLineLength))
				break
			}
		}

		// Check for warning patterns.
		for _, pattern := range logLevels[1].patterns {
			if pattern.MatchString(line) {
				warnLines = append(warnLines, truncateSample(line, maxSampleLineLength))
				break
			}
		}
	}

	// Deterministically select up to maxSampleSize samples for each level.
	samples := make([]string, 0, maxSampleSize)
	samples = append(samples, deterministicallySample(errorLines, maxSampleSize/2)...)
	samples = append(samples, deterministicallySample(warnLines, maxSampleSize-len(samples))...)

	return samples
}

// deterministicallySample deterministically selects up to n samples from items.
// The selection uses a hash-based approach to ensure stable results.
func deterministicallySample(items []string, n int) []string {
	if len(items) <= n {
		return items
	}

	// Use a hash-based selection for deterministic sampling.
	// Hash each item and select those with lowest hash values modulo count.
	type hashItem struct {
		hash uint32
		item string
	}

	hashed := make([]hashItem, len(items))
	for i, item := range items {
		hashed[i] = hashItem{
			hash: fnv1aHash(item),
			item: item,
		}
	}

	// Sort by hash for deterministic selection.
	sort.Slice(hashed, func(i, j int) bool {
		return hashed[i].hash < hashed[j].hash
	})

	// Take first n items.
	result := make([]string, 0, n)
	for i := 0; i < n && i < len(hashed); i++ {
		result = append(result, hashed[i].item)
	}

	return result
}

// fnv1aHash computes a 32-bit FNV-1a hash of the input string.
// This provides deterministic hash values for stable sampling.
func fnv1aHash(s string) uint32 {
	const (
		offset32 = uint32(2166136261)
		prime32  = uint32(16777619)
	)
	h := offset32
	for _, c := range s {
		h ^= uint32(c)
		h *= prime32
	}
	return h
}

// truncateSample truncates a line to maxLen for sampling display.
func truncateSample(line string, maxLen int) string {
	if len(line) <= maxLen {
		return line
	}
	return line[:maxLen] + "..."
}

// orderedLevelNames returns level names sorted by severity order (ERROR first).
func orderedLevelNames(counts map[string]int) []string {
	// Create a lookup map for quick severity lookup.
	severity := make(map[string]int)
	for i, level := range logLevels {
		severity[level.name] = i
	}

	var names []string
	for name := range counts {
		names = append(names, name)
	}

	// Sort by severity order.
	sort.Slice(names, func(i, j int) bool {
		sevi, oki := severity[names[i]]
		sevj, okj := severity[names[j]]
		if !oki || !okj {
			return names[i] < names[j]
		}
		return sevi < sevj
	})

	return names
}

// sortedTimestampPatternNames returns timestamp pattern names sorted by count (descending).
func sortedTimestampPatternNames(counts map[string]int) []string {
	var names []string
	for name := range counts {
		names = append(names, name)
	}

	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})

	return names
}

// errorSignature represents an error signature with its occurrence count.
type errorSignature struct {
	signature string
	count     int
}

// aggregateErrorSignatures aggregates error and warning messages into signatures,
// removing dynamic elements like timestamps, IDs, paths, and UUIDs for exceed mode.
func aggregateErrorSignatures(lines []string) []errorSignature {
	sigCounts := make(map[string]int)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if this is an error or warning line
		isError := false
		for _, lvl := range logLevels[0].patterns {
			if lvl.MatchString(line) {
				isError = true
				break
			}
		}
		if !isError {
			for _, lvl := range logLevels[1].patterns {
				if lvl.MatchString(line) {
					isError = true
					break
				}
			}
		}
		if !isError {
			continue
		}

		// Generate signature by normalizing the line
		signature := normalizeForSignature(line)
		sigCounts[signature]++
	}

	// Build sorted list by count (descending)
	signatures := make([]errorSignature, 0, len(sigCounts))
	for sig, count := range sigCounts {
		if count >= 2 { // Only include repeated errors (2+ occurrences)
			signatures = append(signatures, errorSignature{
				signature: sig,
				count:     count,
			})
		}
	}

	// Sort by count (descending)
	sort.Slice(signatures, func(i, j int) bool {
		if signatures[i].count != signatures[j].count {
			return signatures[i].count > signatures[j].count
		}
		return signatures[i].signature < signatures[j].signature
	})

	return signatures
}

// normalizeForSignature normalizes a log line to create a signature by removing
// dynamic elements like timestamps, IDs, paths, and UUIDs.
func normalizeForSignature(line string) string {
	sig := line

	// Remove timestamps (all timestamp patterns)
	for _, ts := range timestampPatterns {
		sig = ts.pattern.ReplaceAllString(sig, `<ts:`+ts.name+`>`)
	}

	// Remove UUID-like patterns (8-4-4-4-12 hex)
	sig = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`).ReplaceAllString(sig, `<uuid>`)

	// Remove hexadecimal IDs (commonly used for hashes, object IDs)
	sig = regexp.MustCompile(`\b0x[0-9a-fA-F]+\b`).ReplaceAllString(sig, `<hex>`)
	sig = regexp.MustCompile(`\b[0-9a-fA-F]{16,}\b`).ReplaceAllString(sig, `<hex>`) // Long hex strings

	// Remove numeric IDs (sequences of 3+ digits)
	sig = regexp.MustCompile(`\b[0-9]{3,}\b`).ReplaceAllString(sig, `<num>`)

	// Remove IPv4 addresses
	sig = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`).ReplaceAllString(sig, `<ip>`)

	// Remove IPv6 addresses (simplified)
	sig = regexp.MustCompile(`\b[0-9a-fA-F]{1,4}(:[0-9a-fA-F]{1,4}){1,7}\b`).ReplaceAllString(sig, `<ip6>`)

	// Remove file paths with varying names
	sig = regexp.MustCompile(`(/[a-zA-Z0-9_\-\.]+)+(/[a-zA-Z0-9_\-\.]+\.[a-zA-Z]{2,})?`).ReplaceAllString(sig, `<path>`)
	sig = regexp.MustCompile(`[A-Za-z]:\\([a-zA-Z0-9_\-\.]+\\)+`).ReplaceAllString(sig, `<path>`)

	// Remove common variable patterns like ${VAR} or %VAR%
	sig = regexp.MustCompile(`\$\{[^}]+\}`).ReplaceAllString(sig, `<env>`)
	sig = regexp.MustCompile(`%[^%]+%`).ReplaceAllString(sig, `<env>`)

	// Remove memory addresses (0x7ff...)
	sig = regexp.MustCompile(`0x[0-9a-f]{10,16}p?`).ReplaceAllString(sig, `<addr>`)

	// Remove port numbers after colons (common in URLs)
	sig = regexp.MustCompile(`:\d{2,5}\b`).ReplaceAllString(sig, `:<port>`)

	// Trim and collapse multiple spaces
	sig = regexp.MustCompile(`\s+`).ReplaceAllString(sig, ` `)
	sig = strings.TrimSpace(sig)

	// Add prefix to indicate signature type
	prefix := "SIG:"
	if strings.Contains(sig, "[W") || strings.Contains(sig, "[WARN") {
		prefix = "WARN-SIG:"
	}
	return prefix + sig
}

// parseLogLine extracts components from a log line for analysis.
// This is a helper that can be extended for more detailed parsing.
func parseLogLine(line string) (timestamp string, level string, message string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	// Try to extract timestamp first.
	for _, ts := range timestampPatterns {
		if matches := ts.pattern.FindString(line); matches != "" {
			timestamp = matches
			line = strings.TrimSpace(strings.TrimPrefix(line, matches))
			break
		}
	}

	// Try to extract log level.
	// We need to capture the exact level match format from the pattern.
	for _, lvl := range logLevels {
		for _, pattern := range lvl.patterns {
			if matches := pattern.FindString(line); matches != "" {
				// Preserve the exact match case and format from the original line.
				level = matches
				// Remove the matched part from the line.
				line = strings.TrimSpace(strings.TrimPrefix(line, matches))
				break
			}
		}
		if level != "" {
			break
		}
	}

	// Remaining content is the message.
	message = line

	return
}

// LogLine represents a parsed log line with its components.
type LogLine struct {
	Timestamp string
	Level     string
	Message   string
	Raw       string
}

// ParseLogLines parses all log lines into structured LogLine objects.
func ParseLogLines(content []byte) []LogLine {
	lines := strings.Split(string(content), "\n")
	result := make([]LogLine, 0, len(lines))

	for _, line := range lines {
		timestamp, level, message := parseLogLine(line)
		if timestamp != "" || level != "" || message != "" {
			result = append(result, LogLine{
				Timestamp: timestamp,
				Level:     level,
				Message:   message,
				Raw:       line,
			})
		}
	}

	return result
}

// FilterByLevel filters log lines by the specified log level (case-insensitive).
// Supports both bracketed levels like "[ERROR]" and plain levels like "ERROR".
func FilterByLevel(lines []LogLine, level string) []LogLine {
	target := normalizeLevel(level)
	result := make([]LogLine, 0)

	for _, line := range lines {
		if normalizeLevel(line.Level) == target {
			result = append(result, line)
		}
	}

	return result
}

// normalizeLevel normalizes a log level by removing brackets and converting to uppercase.
func normalizeLevel(level string) string {
	normalized := strings.ToUpper(level)
	// Remove brackets like "[ERROR]" -> "ERROR"
	normalized = strings.Trim(normalized, "[]")
	return normalized
}

// GetLevelCounts returns the count of lines for each log level.
func GetLevelCounts(lines []LogLine) map[string]int {
	counts := make(map[string]int)

	for _, line := range lines {
		if line.Level != "" {
			// Normalize level to uppercase and strip brackets.
			normalized := normalizeLevel(line.Level)
			counts[normalized]++
		}
	}

	return counts
}

// GetTimestampStats returns statistics about timestamps in the log lines.
func GetTimestampStats(lines []LogLine) map[string]int {
	stats := make(map[string]int)

	for _, line := range lines {
		if line.Timestamp != "" {
			// Determine timestamp type by matching against patterns.
			for _, ts := range timestampPatterns {
				if ts.pattern.MatchString(line.Timestamp) {
					stats[ts.name]++
					break
				}
			}
		}
	}

	return stats
}

// ExportAsCSV exports log lines as CSV format with columns: timestamp,level,message.
func ExportAsCSV(lines []LogLine) string {
	var builder strings.Builder
	builder.WriteString("timestamp,level,message\n")

	for _, line := range lines {
		// Escape CSV special characters.
		timestamp := escapeCSV(line.Timestamp)
		levelStr := escapeCSV(line.Level)
		message := escapeCSV(line.Message)

		fmt.Fprintf(&builder, "%s,%s,%s\n", timestamp, levelStr, message)
	}

	return builder.String()
}

// escapeCSV escapes a string for CSV output.
func escapeCSV(s string) string {
	if strings.Contains(s, ",") || strings.Contains(s, "\"") || strings.Contains(s, "\n") {
		// Replace double quotes with escaped double quotes.
		s = strings.ReplaceAll(s, "\"", "\"\"")
		// Replace actual newline with literal \n
		s = strings.ReplaceAll(s, "\n", "\\n")
		return `"` + s + `"`
	}
	return s
}
