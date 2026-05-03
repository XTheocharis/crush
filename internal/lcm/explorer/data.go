package explorer

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// JSONExplorer explores JSON files.
type JSONExplorer struct{}

func (e *JSONExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	return ext == "json" || ext == "jsonc" || ext == "json5"
}

func (e *JSONExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("JSON file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "json", TokenEstimate: estimateTokens(summary)}, nil
	}

	var data any
	if err := json.Unmarshal(input.Content, &data); err != nil {
		// Invalid JSON, fallback to text
		content, _ := sampleContent(input.Content, 12000)
		summary := fmt.Sprintf("JSON file (parse error): %s\n%s", filepath.Base(input.Path), content)
		return ExploreResult{Summary: summary, ExplorerUsed: "json", TokenEstimate: estimateTokens(summary)}, nil
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "JSON file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	// Describe structure
	summary.WriteString("\nStructure:\n")
	describeJSONValue(&summary, data, 0, 3)

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "json",
		TokenEstimate: estimateTokens(result),
	}, nil
}

func describeJSONValue(sb *strings.Builder, value any, depth int, maxDepth int) {
	indent := strings.Repeat("  ", depth)

	if depth >= maxDepth {
		fmt.Fprintf(sb, "%s...\n", indent)
		return
	}

	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			fmt.Fprintf(sb, "%s{} (empty object)\n", indent)
			return
		}
		for key, val := range v {
			switch typed := val.(type) {
			case map[string]any:
				fmt.Fprintf(sb, "%s%s: object (%d keys)\n", indent, key, len(typed))
				describeJSONValue(sb, typed, depth+1, maxDepth)
			case []any:
				fmt.Fprintf(sb, "%s%s: array (%d items)\n", indent, key, len(typed))
				if len(typed) > 0 {
					describeJSONValue(sb, typed[0], depth+1, maxDepth)
				}
			case string:
				if len(typed) > 50 {
					fmt.Fprintf(sb, "%s%s: string (%d chars)\n", indent, key, len(typed))
				} else {
					fmt.Fprintf(sb, "%s%s: \"%s\"\n", indent, key, typed)
				}
			case float64:
				fmt.Fprintf(sb, "%s%s: %v (number)\n", indent, key, typed)
			case bool:
				fmt.Fprintf(sb, "%s%s: %v (boolean)\n", indent, key, typed)
			case nil:
				fmt.Fprintf(sb, "%s%s: null\n", indent, key)
			}
		}
	case []any:
		if len(v) == 0 {
			fmt.Fprintf(sb, "%s[] (empty array)\n", indent)
			return
		}
		fmt.Fprintf(sb, "%sArray with %d items\n", indent, len(v))
		if len(v) > 0 {
			fmt.Fprintf(sb, "%sFirst item:\n", indent)
			describeJSONValue(sb, v[0], depth+1, maxDepth)
		}
	case string:
		fmt.Fprintf(sb, "%sstring: %s\n", indent, v)
	case float64:
		fmt.Fprintf(sb, "%snumber: %v\n", indent, v)
	case bool:
		fmt.Fprintf(sb, "%sboolean: %v\n", indent, v)
	case nil:
		fmt.Fprintf(sb, "%snull\n", indent)
	}
}

// CSVExplorer explores CSV files.
type CSVExplorer struct{}

func (e *CSVExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	return ext == "csv" || ext == "tsv"
}

func (e *CSVExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("CSV file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "csv", TokenEstimate: estimateTokens(summary)}, nil
	}

	reader := csv.NewReader(strings.NewReader(string(input.Content)))
	// Detect TSV
	if strings.HasSuffix(strings.ToLower(input.Path), ".tsv") {
		reader.Comma = '\t'
	}

	records, err := reader.ReadAll()
	if err != nil {
		// Fallback to text
		content, _ := sampleContent(input.Content, 12000)
		summary := fmt.Sprintf("CSV file (parse error): %s\n%s", filepath.Base(input.Path), content)
		return ExploreResult{Summary: summary, ExplorerUsed: "csv", TokenEstimate: estimateTokens(summary)}, nil
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "CSV file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Rows: %d\n", len(records))

	if len(records) > 0 {
		fmt.Fprintf(&summary, "Columns: %d\n", len(records[0]))
		summary.WriteString("\nColumn headers:\n")
		for i, col := range records[0] {
			fmt.Fprintf(&summary, "  %d. %s\n", i+1, col)
		}

		if len(records) > 1 {
			summary.WriteString("\nSample rows (first 3):\n")
			maxSample := min(len(records)-1, 3)
			for i := 1; i <= maxSample; i++ {
				fmt.Fprintf(&summary, "  Row %d: %v\n", i, records[i])
			}
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "csv",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// YAMLExplorer explores YAML files.
type YAMLExplorer struct{}

func (e *YAMLExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	return ext == "yaml" || ext == "yml"
}

func (e *YAMLExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("YAML file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "yaml", TokenEstimate: estimateTokens(summary)}, nil
	}

	var data any
	if err := yaml.Unmarshal(input.Content, &data); err != nil {
		// Fallback to text
		content, _ := sampleContent(input.Content, 12000)
		summary := fmt.Sprintf("YAML file (parse error): %s\n%s", filepath.Base(input.Path), content)
		return ExploreResult{Summary: summary, ExplorerUsed: "yaml", TokenEstimate: estimateTokens(summary)}, nil
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "YAML file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	summary.WriteString("\nStructure:\n")
	describeYAMLValue(&summary, data, 0, 3)

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "yaml",
		TokenEstimate: estimateTokens(result),
	}, nil
}

func describeYAMLValue(sb *strings.Builder, value any, depth int, maxDepth int) {
	indent := strings.Repeat("  ", depth)

	if depth >= maxDepth {
		fmt.Fprintf(sb, "%s...\n", indent)
		return
	}

	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			fmt.Fprintf(sb, "%s{} (empty map)\n", indent)
			return
		}
		for key, val := range v {
			switch typed := val.(type) {
			case map[string]any:
				fmt.Fprintf(sb, "%s%s: map (%d keys)\n", indent, key, len(typed))
				describeYAMLValue(sb, typed, depth+1, maxDepth)
			case []any:
				fmt.Fprintf(sb, "%s%s: array (%d items)\n", indent, key, len(typed))
				if len(typed) > 0 {
					describeYAMLValue(sb, typed[0], depth+1, maxDepth)
				}
			case string:
				if len(typed) > 50 {
					fmt.Fprintf(sb, "%s%s: string (%d chars)\n", indent, key, len(typed))
				} else {
					fmt.Fprintf(sb, "%s%s: \"%s\"\n", indent, key, typed)
				}
			case int, int64, float64:
				fmt.Fprintf(sb, "%s%s: %v (number)\n", indent, key, typed)
			case bool:
				fmt.Fprintf(sb, "%s%s: %v (boolean)\n", indent, key, typed)
			case nil:
				fmt.Fprintf(sb, "%s%s: null\n", indent, key)
			}
		}
	case []any:
		if len(v) == 0 {
			fmt.Fprintf(sb, "%s[] (empty array)\n", indent)
			return
		}
		fmt.Fprintf(sb, "%sArray with %d items\n", indent, len(v))
		if len(v) > 0 {
			describeYAMLValue(sb, v[0], depth+1, maxDepth)
		}
	}
}

// TOMLExplorer explores TOML files.
type TOMLExplorer struct{}

func (e *TOMLExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	return ext == "toml"
}

func (e *TOMLExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("TOML file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "toml", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	var summary strings.Builder
	fmt.Fprintf(&summary, "TOML file: %s\n", filepath.Base(input.Path))

	// Extract sections
	sectionRe := regexp.MustCompile(`(?m)^\[([^\]]+)\]`)
	sections := sectionRe.FindAllStringSubmatch(content, -1)
	if len(sections) > 0 {
		summary.WriteString("\nSections:\n")
		for _, match := range sections {
			fmt.Fprintf(&summary, "  - [%s]\n", match[1])
		}
	}

	// Extract top-level keys
	keyRe := regexp.MustCompile(`(?m)^(\w+)\s*=`)
	keys := keyRe.FindAllStringSubmatch(content, -1)
	if len(keys) > 0 {
		summary.WriteString("\nTop-level keys:\n")
		seen := make(map[string]bool)
		for _, match := range keys {
			key := match[1]
			if !seen[key] {
				fmt.Fprintf(&summary, "  - %s\n", key)
				seen[key] = true
			}
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "toml",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// INIExplorer explores INI/config files.
type INIExplorer struct{}

func (e *INIExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	return ext == "ini" || ext == "cfg" || ext == "conf" || ext == "config" || ext == "properties"
}

func (e *INIExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("INI file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "ini", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	var summary strings.Builder
	fmt.Fprintf(&summary, "INI/Config file: %s\n", filepath.Base(input.Path))

	// Extract sections
	sectionRe := regexp.MustCompile(`(?m)^\[([^\]]+)\]`)
	sections := sectionRe.FindAllStringSubmatch(content, -1)
	if len(sections) > 0 {
		summary.WriteString("\nSections:\n")
		for _, match := range sections {
			fmt.Fprintf(&summary, "  - [%s]\n", match[1])
		}
	}

	// Extract keys
	keyRe := regexp.MustCompile(`(?m)^([^=\[#;]+)\s*=`)
	keys := keyRe.FindAllStringSubmatch(content, -1)
	if len(keys) > 0 {
		summary.WriteString("\nKeys:\n")
		seen := make(map[string]bool)
		for _, match := range keys {
			key := strings.TrimSpace(match[1])
			if !seen[key] && key != "" {
				fmt.Fprintf(&summary, "  - %s\n", key)
				seen[key] = true
			}
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "ini",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// XMLExplorer explores XML files.
type XMLExplorer struct{}

func (e *XMLExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "xml" || ext == "xsd" || ext == "xsl" || ext == "xslt" || ext == "svg" {
		return true
	}
	// Check if content starts with XML declaration
	return strings.HasPrefix(strings.TrimSpace(string(content)), "<?xml")
}

func (e *XMLExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("XML file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "xml", TokenEstimate: estimateTokens(summary)}, nil
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "XML file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	// Parse XML to get element structure
	decoder := xml.NewDecoder(strings.NewReader(string(input.Content)))
	elements := make(map[string]int)
	var currentPath []string

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Parse error, fallback to text
			content, _ := sampleContent(input.Content, 12000)
			summary := fmt.Sprintf("XML file (parse error): %s\n%s", filepath.Base(input.Path), content)
			return ExploreResult{Summary: summary, ExplorerUsed: "xml", TokenEstimate: estimateTokens(summary)}, nil
		}

		switch se := tok.(type) {
		case xml.StartElement:
			currentPath = append(currentPath, se.Name.Local)
			path := strings.Join(currentPath, "/")
			elements[path]++
		case xml.EndElement:
			if len(currentPath) > 0 {
				currentPath = currentPath[:len(currentPath)-1]
			}
		}
	}

	if len(elements) > 0 {
		summary.WriteString("\nElement hierarchy:\n")
		for path, count := range elements {
			if count > 1 {
				fmt.Fprintf(&summary, "  - %s (×%d)\n", path, count)
			} else {
				fmt.Fprintf(&summary, "  - %s\n", path)
			}
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "xml",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// HTMLExplorer explores HTML files.
type HTMLExplorer struct{}

func (e *HTMLExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "html" || ext == "htm" || ext == "xhtml" {
		return true
	}
	// Check if content looks like HTML
	contentLower := strings.ToLower(string(content))
	return strings.Contains(contentLower, "<!doctype html") ||
		strings.Contains(contentLower, "<html")
}

func (e *HTMLExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("HTML file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "html", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	var summary strings.Builder
	fmt.Fprintf(&summary, "HTML file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	// Extract title
	titleRe := regexp.MustCompile(`(?i)<title[^>]*>([^<]+)</title>`)
	if match := titleRe.FindStringSubmatch(content); match != nil {
		fmt.Fprintf(&summary, "Title: %s\n", strings.TrimSpace(match[1]))
	}

	// Count common elements
	elemCounts := make(map[string]int)
	for _, elem := range []string{"div", "span", "p", "a", "img", "script", "link", "style", "form", "input", "button"} {
		re := regexp.MustCompile(fmt.Sprintf(`(?i)<%s[\s>]`, elem))
		matches := re.FindAllString(content, -1)
		if len(matches) > 0 {
			elemCounts[elem] = len(matches)
		}
	}

	if len(elemCounts) > 0 {
		summary.WriteString("\nElement counts:\n")
		for elem, count := range elemCounts {
			fmt.Fprintf(&summary, "  - <%s>: %d\n", elem, count)
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "html",
		TokenEstimate: estimateTokens(result),
	}, nil
}
