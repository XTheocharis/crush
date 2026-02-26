package explorer

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// LatexExplorer explores LaTeX document files.
type LatexExplorer struct{}

// LatexSection represents a section with its level and title.
type LatexSection struct {
	Level int
	Title string
}

// LatexEnv represents a LaTeX environment found in the document.
type LatexEnv struct {
	Name  string
	Count int
}

// LatexBiblio represents bibliography-related metadata.
type LatexBiblio struct {
	Bibliography    []string // Files from \bibliography{...}
	Addbibresource  []string // Files from \addbibresource{...}
	CiteCount       int      // Count of \cite-like commands
	BibliographySty string   // Content from \bibliographystyle{...}
}

func (e *LatexExplorer) CanHandle(path string, content []byte) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".tex", ".latex", ".bst":
		return true
	}
	return false
}

func (e *LatexExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("LaTeX file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "latex", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	var summary strings.Builder
	fmt.Fprintf(&summary, "LaTeX file: %s\n", filepath.Base(input.Path))

	// Extract sections
	sections := extractLatexSections(content)

	// Extract environments
	envs := extractLatexEnvironments(content)

	// Extract bibliography metadata
	biblio := extractLatexBibliography(content)

	// Section structure
	if len(sections) > 0 {
		sectionCounts := countSectionsByLevel(sections)
		summary.WriteString("\nSection structure:\n")
		if sectionCounts[1] > 0 {
			fmt.Fprintf(&summary, "  - \\section: %d\n", sectionCounts[1])
		}
		if sectionCounts[2] > 0 {
			fmt.Fprintf(&summary, "  - \\subsection: %d\n", sectionCounts[2])
		}
		if sectionCounts[3] > 0 {
			fmt.Fprintf(&summary, "  - \\subsubsection: %d\n", sectionCounts[3])
		}
		if sectionCounts[4] > 0 {
			fmt.Fprintf(&summary, "  - \\paragraph: %d\n", sectionCounts[4])
		}
		if sectionCounts[5] > 0 {
			fmt.Fprintf(&summary, "  - \\subparagraph: %d\n", sectionCounts[5])
		}
	}

	// Environment inventory
	if len(envs) > 0 {
		summary.WriteString("\nEnvironments:\n")
		for _, env := range envs {
			fmt.Fprintf(&summary, "  - %s: %d\n", env.Name, env.Count)
		}
	}

	// Bibliography metadata
	if len(biblio.Bibliography) > 0 || len(biblio.Addbibresource) > 0 || biblio.CiteCount > 0 || biblio.BibliographySty != "" {
		summary.WriteString("\nBibliography:\n")
		if len(biblio.Bibliography) > 0 {
			summary.WriteString("  - \\bibliography:\n")
			for _, bib := range biblio.Bibliography {
				fmt.Fprintf(&summary, "    - %s\n", bib)
			}
		}
		if len(biblio.Addbibresource) > 0 {
			summary.WriteString("  - \\addbibresource:\n")
			for _, res := range biblio.Addbibresource {
				fmt.Fprintf(&summary, "    - %s\n", res)
			}
		}
		if biblio.CiteCount > 0 {
			fmt.Fprintf(&summary, "  - Citations: %d\n", biblio.CiteCount)
		}
		if biblio.BibliographySty != "" {
			fmt.Fprintf(&summary, "  - Style: %s\n", biblio.BibliographySty)
		}
	}

	// Packages
	pkgs := extractLatexPackages(content)
	if len(pkgs) > 0 {
		summary.WriteString("\nPackages:\n")
		for _, pkg := range pkgs[:min(20, len(pkgs))] {
			fmt.Fprintf(&summary, "  - %s\n", pkg)
		}
		if len(pkgs) > 20 {
			fmt.Fprintf(&summary, "  ... (%d more)\n", len(pkgs)-20)
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "latex",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// extractLatexSections extracts all sections from LaTeX content.
func extractLatexSections(content string) []LatexSection {
	// Match all section commands with their titles in document order
	// Matches: \section{...}, \subsection{...}, \subsubsection{...}, \paragraph{...}, \subparagraph{...}
	// And starred variants
	re := regexp.MustCompile(`\\(section|subsection|subsubsection|paragraph|subparagraph)\*?\s*\{([^}]*)\}`)

	var sections []LatexSection
	matches := re.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) != 3 {
			continue
		}

		command := match[1]
		title := strings.TrimSpace(match[2])

		// Determine level from command type
		level := 1
		switch command {
		case "section":
			level = 1
		case "subsection":
			level = 2
		case "subsubsection":
			level = 3
		case "paragraph":
			level = 4
		case "subparagraph":
			level = 5
		}

		sections = append(sections, LatexSection{
			Level: level,
			Title: title,
		})
	}

	return sections
}

// countSectionsByLevel counts sections by their level.
func countSectionsByLevel(sections []LatexSection) map[int]int {
	counts := make(map[int]int)
	for _, s := range sections {
		counts[s.Level]++
	}
	return counts
}

// extractLatexEnvironments extracts and counts LaTeX environments.
func extractLatexEnvironments(content string) []LatexEnv {
	// Matches \begin{env_name} ... \end{env_name}
	beginRe := regexp.MustCompile(`\\begin\s*\{([^}]+)\}`)

	envCount := make(map[string]int)
	matches := beginRe.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		envName := strings.TrimSpace(match[1])
		envCount[envName]++
	}

	// Convert to sorted list
	var envs []LatexEnv
	for name, count := range envCount {
		envs = append(envs, LatexEnv{Name: name, Count: count})
	}

	// Sort by count (descending) then by name
	sortEnvs(envs)

	// Filter out environments that are typically not of interest
	filteredEnvs := make([]LatexEnv, 0, len(envs))
	for _, env := range envs {
		// Skip document, frame, columns environments and internal ones
		if env.Name == "document" || env.Name == "frame" ||
			env.Name == "columns" || env.Name == "column" ||
			env.Name == "tabular" || env.Name == "tabular*" ||
			env.Name == "array" || env.Name == "matrix" ||
			env.Name == "tikzpicture" || env.Name == "axis" ||
			strings.HasPrefix(env.Name, "align") ||
			env.Name == "eqnarray" || env.Name == "gather" {
			continue
		}
		filteredEnvs = append(filteredEnvs, env)
	}

	return filteredEnvs
}

// sortEnvs sorts environments by count (descending), then by name.
func sortEnvs(envs []LatexEnv) {
	// Simple insertion sort for small slices
	for i := 1; i < len(envs); i++ {
		for j := i; j > 0; j-- {
			if envs[j].Count > envs[j-1].Count ||
				(envs[j].Count == envs[j-1].Count && envs[j].Name < envs[j-1].Name) {
				envs[j], envs[j-1] = envs[j-1], envs[j]
			}
		}
	}
}

// extractLatexBibliography extracts bibliography-related metadata.
func extractLatexBibliography(content string) LatexBiblio {
	result := LatexBiblio{}

	// \bibliography{files}
	bibRe := regexp.MustCompile(`\\bibliography\s*\{([^}]+)\}`)
	if matches := bibRe.FindAllStringSubmatch(content, -1); len(matches) > 0 {
		for _, match := range matches {
			files := strings.Split(match[1], ",")
			for _, f := range files {
				result.Bibliography = append(result.Bibliography, strings.TrimSpace(f))
			}
		}
	}

	// \addbibresource{files}
	addbibRe := regexp.MustCompile(`\\addbibresource\s*\{([^}]+)\}`)
	if matches := addbibRe.FindAllStringSubmatch(content, -1); len(matches) > 0 {
		for _, match := range matches {
			result.Addbibresource = append(result.Addbibresource, strings.TrimSpace(match[1]))
		}
	}

	// \bibliographystyle{style}
	styleRe := regexp.MustCompile(`\\bibliographystyle\s*\{([^}]+)\}`)
	if match := styleRe.FindStringSubmatch(content); match != nil {
		result.BibliographySty = strings.TrimSpace(match[1])
	}

	// Count citation commands: \cite, \citep, \citet, \citeauthor, \citeyear, \nocite
	// Including starred variants: \cite*, \citep*, etc.
	citeRe := regexp.MustCompile(`\\(?:cite\*?|citep\*?|citet\*?|citeauthor\*?|citeyear\*?|nocite\*?)(?:\[[^]]*\])*\{[^}]*\}`)
	result.CiteCount = len(citeRe.FindAllString(content, -1))

	return result
}

// extractLatexPackages extracts package names from \usepackage commands.
func extractLatexPackages(content string) []string {
	// Matches \usepackage[...]{packname} or \usepackage{packname} or \usepackage{packname1,packname2}
	pkgRe := regexp.MustCompile(`\\usepackage\s*(?:\[[^\]]*\])?\s*\{([^}]+)\}`)

	var packages []string
	matches := pkgRe.FindAllStringSubmatch(content, -1)

	seen := make(map[string]bool)
	for _, match := range matches {
		// Split by comma for multiple packages in one command
		pkgs := strings.Split(match[1], ",")
		for _, pkg := range pkgs {
			trimmed := strings.TrimSpace(pkg)
			// Handle version specifications like package=v1.0
			if idx := strings.Index(trimmed, "="); idx > 0 {
				trimmed = strings.TrimSpace(trimmed[:idx])
			}
			if trimmed != "" && !seen[trimmed] {
				packages = append(packages, trimmed)
				seen[trimmed] = true
			}
		}
	}

	return packages
}

// min returns the minimum of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
