package explorer

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

// DiagramExplorer explores diagram files (drawio, mermaid, d2, excalidraw).
// All formats are text/XML/JSON based. No external dependencies.
type DiagramExplorer struct{}

var diagramExtensions = map[string]string{
	"drawio":     "drawio",
	"dio":        "drawio",
	"mermaid":    "mermaid",
	"mmd":        "mermaid",
	"d2":         "d2",
	"excalidraw": "excalidraw",
	"plantuml":   "plantuml",
	"puml":       "plantuml",
}

func (e *DiagramExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if _, ok := diagramExtensions[ext]; ok {
		return true
	}
	return false
}

func (e *DiagramExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.Path)), ".")
	subType := diagramExtensions[ext]
	name := filepath.Base(input.Path)

	var summary strings.Builder
	fmt.Fprintf(&summary, "Diagram file: %s\n", name)
	fmt.Fprintf(&summary, "Format: %s\n", subType)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	switch subType {
	case "drawio":
		exploreDrawio(&summary, input.Content)
	case "mermaid":
		exploreMermaid(&summary, input.Content)
	case "d2":
		exploreD2(&summary, input.Content)
	case "excalidraw":
		exploreExcalidraw(&summary, input.Content)
	case "plantuml":
		explorePlantUML(&summary, input.Content)
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "diagram",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// exploreDrawio parses draw.io XML format extracting cells and connections.
func exploreDrawio(summary *strings.Builder, content []byte) {
	decoder := xml.NewDecoder(strings.NewReader(string(content)))
	cellCount := 0
	edgeCount := 0
	var shapes []string
	shapeSeen := make(map[string]bool)

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "mxCell" {
			isEdge := false
			hasValue := false
			style := ""
			for _, attr := range se.Attr {
				switch attr.Name.Local {
				case "edge":
					isEdge = attr.Value == "1"
				case "value":
					hasValue = attr.Value != ""
				case "style":
					style = attr.Value
				}
			}
			if isEdge {
				edgeCount++
			} else if hasValue {
				cellCount++
				if shape := extractDrawioShape(style); shape != "" && !shapeSeen[shape] {
					shapeSeen[shape] = true
					shapes = append(shapes, shape)
				}
			}
		}
	}

	fmt.Fprintf(summary, "Nodes: %d\n", cellCount)
	fmt.Fprintf(summary, "Edges (connections): %d\n", edgeCount)
	if len(shapes) > 0 {
		fmt.Fprintf(summary, "Shape types: %s\n", strings.Join(shapes, ", "))
	}
}

// extractDrawioShape extracts the shape type from a draw.io style string.
func extractDrawioShape(style string) string {
	if style == "" {
		return "rectangle"
	}
	if strings.Contains(style, "ellipse") {
		return "ellipse"
	}
	if strings.Contains(style, "rhombus") || strings.Contains(style, "diamond") {
		return "diamond"
	}
	if strings.Contains(style, "rounded=1") {
		return "rounded-rectangle"
	}
	if strings.Contains(style, "shape=cylinder") {
		return "cylinder"
	}
	if strings.Contains(style, "shape=hexagon") {
		return "hexagon"
	}
	if strings.Contains(style, "shape=cloud") {
		return "cloud"
	}
	if strings.Contains(style, "shape=mxgraph.basic") {
		return "custom"
	}
	if strings.Contains(style, "triangle") {
		return "triangle"
	}
	return ""
}

// exploreMermaid parses Mermaid diagram text extracting diagram type and counts.
func exploreMermaid(summary *strings.Builder, content []byte) {
	text := strings.TrimSpace(string(content))
	if text == "" {
		return
	}
	lines := strings.Split(text, "\n")

	firstLine := strings.TrimSpace(lines[0])
	diagramType := detectMermaidType(firstLine)
	fmt.Fprintf(summary, "Diagram type: %s\n", diagramType)

	nodeCount := 0
	edgeCount := 0
	nodeSeen := make(map[string]bool)

	// Match node and edge patterns.
	nodeRe := regexp.MustCompile(`^(\w+)(?:\[.*?\]|\(.*?\)|\{.*?\}|>.*?<|\(\/.*?\/\))?\s*$`)
	edgePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(\w+)\s*--?>\s*[\|>]?\s*(\w+)`),
		regexp.MustCompile(`(\w+)\s*-{1,3}>?\s*(?:\|[^|]*\|\s*)?(\w+)`),
		regexp.MustCompile(`(\w+)\s*==>?\s*(?:\|[^|]*\|\s*)?(\w+)`),
		regexp.MustCompile(`(\w+)\s*-\.-?>?\s*(?:\|[^|]*\|\s*)?(\w+)`),
		regexp.MustCompile(`(\w+)\s*==>\s*(\w+)`),
	}

	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "%%") || strings.HasPrefix(line, "#") {
			continue
		}

		for _, pat := range edgePatterns {
			matches := pat.FindStringSubmatch(line)
			if len(matches) >= 3 {
				edgeCount++
				if !nodeSeen[matches[1]] {
					nodeSeen[matches[1]] = true
					nodeCount++
				}
				if !nodeSeen[matches[2]] {
					nodeSeen[matches[2]] = true
					nodeCount++
				}
				break
			}
		}

		if m := nodeRe.FindStringSubmatch(line); len(m) >= 2 {
			if !nodeSeen[m[1]] {
				nodeSeen[m[1]] = true
				nodeCount++
			}
		}
	}

	fmt.Fprintf(summary, "Nodes: %d\n", nodeCount)
	fmt.Fprintf(summary, "Edges: %d\n", edgeCount)
	fmt.Fprintf(summary, "Lines: %d\n", len(lines))
}

// detectMermaidType returns the Mermaid diagram type from the first line.
func detectMermaidType(firstLine string) string {
	fields := strings.Fields(firstLine)
	if len(fields) == 0 {
		return "unknown"
	}
	keyword := strings.ToLower(fields[0])
	switch keyword {
	case "graph", "flowchart":
		return "flowchart"
	case "sequenceDiagram":
		return "sequence"
	case "classDiagram", "classDiagram-v2":
		return "class"
	case "stateDiagram", "stateDiagram-v2":
		return "state"
	case "erDiagram":
		return "ER"
	case "gantt":
		return "gantt"
	case "pie":
		return "pie"
	case "gitgraph", "gitGraph":
		return "git"
	case "journey":
		return "user-journey"
	case "mindmap":
		return "mindmap"
	case "timeline":
		return "timeline"
	case "quadrantChart":
		return "quadrant"
	case "sankey", "block", "xychart":
		return keyword
	default:
		return keyword
	}
}

// exploreD2 parses D2 diagram text extracting shapes and connections.
func exploreD2(summary *strings.Builder, content []byte) {
	text := strings.TrimSpace(string(content))
	lines := strings.Split(text, "\n")

	shapeCount := 0
	edgeCount := 0
	connectionRe := regexp.MustCompile(`^\s*([^:;\n]+?)\s*->\s*([^:;\n]+?)(?:\s*:\s*(.*))?$`)
	shapeRe := regexp.MustCompile(`^\s*([^:;\n]+?)\s*:\s*(.*)$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if connectionRe.MatchString(line) {
			edgeCount++
		} else if shapeRe.MatchString(line) && !strings.HasPrefix(line, "style") && !strings.HasPrefix(line, "classes") {
			shapeCount++
		}
	}

	fmt.Fprintf(summary, "Shapes: %d\n", shapeCount)
	fmt.Fprintf(summary, "Connections: %d\n", edgeCount)
	fmt.Fprintf(summary, "Lines: %d\n", len(lines))
}

// exploreExcalidraw parses Excalidraw JSON format extracting elements.
func exploreExcalidraw(summary *strings.Builder, content []byte) {
	var data struct {
		Elements []struct {
			Type string `json:"type"`
		} `json:"elements"`
	}

	if err := json.Unmarshal(content, &data); err != nil {
		text, _ := sampleContent(content, 2000)
		fmt.Fprintf(summary, "Content (parse error, sampled):\n%s\n", text)
		return
	}

	typeCounts := make(map[string]int)
	totalElements := len(data.Elements)
	for _, el := range data.Elements {
		typeCounts[el.Type]++
	}

	fmt.Fprintf(summary, "Elements: %d\n", totalElements)
	if len(typeCounts) > 0 {
		summary.WriteString("\nElement types:\n")
		for typ, count := range typeCounts {
			fmt.Fprintf(summary, "  - %s: %d\n", typ, count)
		}
	}
}

// explorePlantUML parses PlantUML text extracting diagram type and counts.
func explorePlantUML(summary *strings.Builder, content []byte) {
	text := string(content)
	lines := strings.Split(text, "\n")

	diagramType := "class"
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@start") {
			keyword := strings.TrimPrefix(line, "@start")
			switch {
			case strings.HasPrefix(keyword, "uml"):
				diagramType = "class"
			case strings.HasPrefix(keyword, "salt"):
				diagramType = "wireframe"
			case strings.HasPrefix(keyword, "mindmap"):
				diagramType = "mindmap"
			case strings.HasPrefix(keyword, "wbs"):
				diagramType = "WBS"
			case strings.HasPrefix(keyword, "gantt"):
				diagramType = "gantt"
			case strings.HasPrefix(keyword, "json"):
				diagramType = "JSON"
			case strings.HasPrefix(keyword, "yaml"):
				diagramType = "YAML"
			}
			break
		}
	}

	fmt.Fprintf(summary, "Diagram type: %s\n", diagramType)
	fmt.Fprintf(summary, "Lines: %d\n", len(lines))
}
