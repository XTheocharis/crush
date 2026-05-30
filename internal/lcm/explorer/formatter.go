package explorer

import (
	"fmt"
	"sort"
	"strings"
)

const (
	defaultSectionItemLimit = 8
	defaultSectionLineLimit = 16
)

// OutputProfile controls formatter behavior for truncation/overflow markers.
type OutputProfile string

const (
	// OutputProfileParity emits parity-style overflow markers.
	OutputProfileParity OutputProfile = "parity"
	// OutputProfileEnhancement emits canonical enhancement overflow markers.
	OutputProfileEnhancement OutputProfile = "enhancement"
	// OutputProfileCompact is an alias for parity: minimal output with truncation.
	OutputProfileCompact OutputProfile = "compact"
	// OutputProfileStandard is an alias for enhancement: normal output with truncation.
	OutputProfileStandard OutputProfile = "standard"
	// OutputProfileVerbose shows full output with no truncation.
	OutputProfileVerbose OutputProfile = "verbose"
)

type summarySection struct {
	title string
	lines []string
	raw   bool
}

func formatExploreResult(result ExploreResult, profile OutputProfile) ExploreResult {
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		return result
	}

	normalized := normalizeProfile(profile)
	formatted := formatSummary(summary, normalized)
	result.Summary = formatted
	result.TokenEstimate = estimateTokens(formatted)
	return result
}

func normalizeProfile(profile OutputProfile) OutputProfile {
	switch profile {
	case OutputProfileCompact:
		return OutputProfileParity
	case OutputProfileStandard:
		return OutputProfileEnhancement
	default:
		return profile
	}
}

func formatSummary(summary string, profile OutputProfile) string {
	lines := strings.Split(strings.ReplaceAll(summary, "\r\n", "\n"), "\n")
	header := "File summary"
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			header = strings.TrimSpace(line)
			break
		}
	}

	sections := parseSummarySections(lines[1:])
	if len(sections) == 0 {
		sections = []summarySection{{title: "Overview", lines: []string{summary}}}
	}

	var out strings.Builder
	fmt.Fprintf(&out, "## %s\n", header)
	for _, section := range sections {
		renderSection(&out, section, profile)
	}

	return strings.TrimSpace(out.String())
}

func parseSummarySections(lines []string) []summarySection {
	sections := []summarySection{}
	cur := summarySection{title: "Overview"}
	inContent := false

	flush := func() {
		if len(cur.lines) == 0 {
			return
		}
		sections = append(sections, cur)
	}

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, "://") {
			flush()
			title := strings.TrimSuffix(trimmed, ":")
			cur = summarySection{title: title}
			inContent = strings.EqualFold(title, "content") || strings.EqualFold(title, "content (sampled)")
			if inContent {
				cur.raw = true
			}
			continue
		}

		if inContent {
			cur.lines = append(cur.lines, line)
			continue
		}

		item := normalizeSummaryLine(line)
		if item != "" {
			cur.lines = append(cur.lines, item)
		}
	}

	flush()
	return sections
}

func normalizeSummaryLine(line string) string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "- ")
	trimmed = strings.TrimPrefix(trimmed, "* ")
	trimmed = strings.TrimPrefix(trimmed, "• ")
	trimmed = strings.TrimPrefix(trimmed, "-")
	return strings.TrimSpace(trimmed)
}

func renderSection(out *strings.Builder, section summarySection, profile OutputProfile) {
	fmt.Fprintf(out, "\n### %s\n", section.title)
	if profile == OutputProfileVerbose {
		if section.raw {
			writeSectionLines(out, section.lines, 0, profile, true)
			return
		}
		items := dedupe(section.lines)
		sort.Strings(items)
		writeSectionLines(out, items, 0, profile, false)
		return
	}
	if section.raw {
		writeSectionLines(out, section.lines, defaultSectionLineLimit, profile, true)
		return
	}
	items := dedupe(section.lines)
	sort.Strings(items)
	writeSectionLines(out, items, defaultSectionItemLimit, profile, false)
}

func writeSectionLines(out *strings.Builder, lines []string, cap int, profile OutputProfile, raw bool) {
	if len(lines) == 0 {
		out.WriteString("- (none)\n")
		return
	}

	display := lines
	extra := 0
	if cap > 0 && len(lines) > cap {
		display = lines[:cap]
		extra = len(lines) - cap
	}

	for _, line := range display {
		if raw {
			fmt.Fprintf(out, "- %s\n", line)
		} else {
			fmt.Fprintf(out, "- %s\n", strings.TrimSpace(line))
		}
	}

	if extra > 0 {
		fmt.Fprintf(out, "- %s\n", overflowMarker(profile, extra, raw))
	}
}

func overflowMarker(profile OutputProfile, count int, raw bool) string {
	if count <= 0 {
		return ""
	}
	switch profile {
	case OutputProfileVerbose:
		return ""
	case OutputProfileParity, OutputProfileCompact:
		if raw {
			return fmt.Sprintf("[TRUNCATED] (+%d more lines)", count)
		}
		return fmt.Sprintf("(+%d more)", count)
	default:
		if raw {
			return fmt.Sprintf("[TRUNCATED] ... and %d more lines", count)
		}
		return fmt.Sprintf("... and %d more", count)
	}
}

func dedupe(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
