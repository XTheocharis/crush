//go:build treesitter

package treesitter

import (
	"fmt"
	"strings"
	"sync"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// QueryLoader manages loading and caching language queries.
type QueryLoader struct {
	mu         sync.RWMutex
	languages  map[string]*tree_sitter.Language
	queries    map[string]*tree_sitter.Query
	captureMap map[string][]string
}

// NewQueryLoader creates a new query loader.
func NewQueryLoader() *QueryLoader {
	return &QueryLoader{
		languages:  make(map[string]*tree_sitter.Language),
		queries:    make(map[string]*tree_sitter.Query),
		captureMap: make(map[string][]string),
	}
}

// RegisterLanguage registers a language for query compilation.
func (q *QueryLoader) RegisterLanguage(key string, language *tree_sitter.Language) {
	if language == nil {
		return
	}
	queryKey := GetQueryKey(key)
	if queryKey == "" {
		return
	}

	q.mu.Lock()
	q.languages[queryKey] = language
	q.mu.Unlock()
}

// LoadQuery loads or retrieves a compiled query for a language.
func (q *QueryLoader) LoadQuery(lang string) (*tree_sitter.Query, error) {
	queryKey := GetQueryKey(lang)
	if queryKey == "" {
		return nil, fmt.Errorf("language key is empty")
	}

	q.mu.RLock()
	if cached := q.queries[queryKey]; cached != nil {
		q.mu.RUnlock()
		return cached, nil
	}
	language := q.languages[queryKey]
	q.mu.RUnlock()

	if language == nil {
		return nil, fmt.Errorf("language %q not registered", queryKey)
	}

	querySource, err := LoadTagsQuery(queryKey)
	if err != nil {
		return nil, fmt.Errorf("load tags query %q: %w", queryKey, err)
	}

	compiled, qErr := tree_sitter.NewQuery(language, string(querySource))
	if qErr != nil {
		return nil, fmt.Errorf("compile tags query %q: %w", queryKey, qErr)
	}

	captures := compiled.CaptureNames()

	q.mu.Lock()
	defer q.mu.Unlock()

	if cached := q.queries[queryKey]; cached != nil {
		compiled.Close()
		return cached, nil
	}

	q.queries[queryKey] = compiled
	q.captureMap[queryKey] = captures
	return compiled, nil
}

// HasTags reports whether tags are available for the language.
func (q *QueryLoader) HasTags(lang string) bool {
	return HasTags(GetQueryKey(lang))
}

// Languages returns the loaded language set.
func (q *QueryLoader) Languages() []string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	langs := make([]string, 0, len(q.languages))
	for lang := range q.languages {
		langs = append(langs, lang)
	}
	return langs
}

// ExtractTagsWithCursor runs query captures and extracts tags/symbols.
func (q *QueryLoader) ExtractTagsWithCursor(lang, relPath string, root *tree_sitter.Node, content []byte, cursor *tree_sitter.QueryCursor) ([]Tag, []SymbolInfo, error) {
	if root == nil {
		return nil, nil, nil
	}

	query, err := q.LoadQuery(lang)
	if err != nil {
		return nil, nil, err
	}

	queryKey := GetQueryKey(lang)
	q.mu.RLock()
	captureNames := q.captureMap[queryKey]
	q.mu.RUnlock()
	if len(captureNames) == 0 {
		captureNames = query.CaptureNames()
	}

	if cursor == nil {
		cursor = tree_sitter.NewQueryCursor()
		defer cursor.Close()
	}

	tags := make([]Tag, 0, 64)
	symbols := make([]SymbolInfo, 0, 32)

	matches := cursor.Matches(query, root, content)
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		if !m.SatisfiesTextPredicate(query, nil, nil, content) {
			continue
		}

		defIdx := -1
		for i, cap := range m.Captures {
			if int(cap.Index) >= len(captureNames) {
				continue
			}
			if _, _, ok := parseModernCapture(captureNames[cap.Index]); ok && strings.HasPrefix(captureNames[cap.Index], "name.definition.") {
				defIdx = i
				break
			}
		}

		if defIdx < 0 {
			continue
		}

		defCap := m.Captures[defIdx]
		defName := captureNames[defCap.Index]
		defNode := &defCap.Node
		tagName := strings.TrimSpace(defNode.Utf8Text(content))
		if tagName == "" {
			continue
		}

		kind, nodeType, _ := parseModernCapture(defName)
		start := defNode.StartPosition()
		tags = append(tags, Tag{
			RelPath:  relPath,
			Name:     tagName,
			Kind:     kind,
			Line:     int(start.Row) + 1,
			Language: queryKey,
			NodeType: nodeType,
		})

		if kind == "def" {
			end := defNode.EndPosition()
			symbols = append(symbols, SymbolInfo{
				Name:    tagName,
				Kind:    nodeType,
				Line:    int(start.Row) + 1,
				EndLine: int(end.Row) + 1,
			})
			symIdx := len(symbols) - 1

			for _, cap := range m.Captures {
				if int(cap.Index) >= len(captureNames) {
					continue
				}
				captureName := captureNames[cap.Index]
				if _, _, ok := parseModernCapture(captureName); ok {
					continue
				}
				enrichSymbolField(&symbols[symIdx], captureName, &cap.Node, content, queryKey)
			}
		}
	}

	// Post-process Python symbols to enrich with parent class and decorator info.
	if queryKey == "python" {
		enrichPythonSymbols(symbols, root, content)
	}

	// Post-process TypeScript symbols to enrich with modifiers (export, async, accessibility).
	if queryKey == "typescript" || queryKey == "javascript" {
		enrichTSSymbols(symbols, root, content)
	}

	// Also process reference-only matches (matches without a name.definition.* capture).
	// We need a second pass through matches for refs.
	refCursor := tree_sitter.NewQueryCursor()
	defer refCursor.Close()
	refMatches := refCursor.Matches(query, root, content)
	for {
		m := refMatches.Next()
		if m == nil {
			break
		}
		if !m.SatisfiesTextPredicate(query, nil, nil, content) {
			continue
		}

		hasDef := false
		for _, cap := range m.Captures {
			if int(cap.Index) >= len(captureNames) {
				continue
			}
			if _, _, ok := parseModernCapture(captureNames[cap.Index]); ok && strings.HasPrefix(captureNames[cap.Index], "name.definition.") {
				hasDef = true
				break
			}
		}
		if hasDef {
			continue
		}

		for _, cap := range m.Captures {
			if int(cap.Index) >= len(captureNames) {
				continue
			}
			captureName := captureNames[cap.Index]
			kind, nodeType, ok := parseModernCapture(captureName)
			if !ok {
				continue
			}
			node := &cap.Node
			tagName := strings.TrimSpace(node.Utf8Text(content))
			if tagName == "" {
				continue
			}

			start := node.StartPosition()
			tags = append(tags, Tag{
				RelPath:  relPath,
				Name:     tagName,
				Kind:     kind,
				Line:     int(start.Row) + 1,
				Language: queryKey,
				NodeType: nodeType,
			})
		}
	}

	return tags, symbols, nil
}

func enrichSymbolField(sym *SymbolInfo, captureName string, node *tree_sitter.Node, content []byte, lang string) {
	text := strings.TrimSpace(node.Utf8Text(content))

	switch captureName {
	case "params":
		if text != "" {
			sym.Params = text
		}
	case "return_type":
		if text != "" {
			text = strings.TrimPrefix(text, ":")
			text = strings.TrimSpace(text)
			sym.ReturnType = text
		}
	case "doc":
		if text != "" && sym.DocComment == "" {
			if lang == "python" {
				text = strings.Trim(text, "\"'")
			}
			sym.DocComment = text
		}
	case "parent":
		parent := extractParentName(text, lang)
		if parent != "" {
			sym.Parent = parent
		}
	case "modifier":
		if text != "" {
			sym.Modifiers = append(sym.Modifiers, text)
		}
	case "decorator":
		if text != "" {
			sym.Decorators = append(sym.Decorators, text)
		}
	}
}

// extractParentName extracts the type name from a receiver/import path text.
func extractParentName(text, lang string) string {
	text = strings.TrimSpace(text)
	switch lang {
	case "go":
		text = strings.TrimPrefix(text, "*")
		text = strings.TrimSpace(text)
		if strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")") {
			text = text[1 : len(text)-1]
		}
		parts := strings.Fields(text)
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimPrefix(parts[i], "*")
			if candidate != "" {
				return candidate
			}
		}
		return text
	default:
		return text
	}
}

// ExtractTags runs query captures with an internal cursor and extracts tags/symbols.
func (q *QueryLoader) ExtractTags(lang, relPath string, root *tree_sitter.Node, content []byte) ([]Tag, []SymbolInfo, error) {
	return q.ExtractTagsWithCursor(lang, relPath, root, content, nil)
}

func parseModernCapture(captureName string) (kind string, nodeType string, ok bool) {
	if t, found := strings.CutPrefix(captureName, "name.definition."); found && t != "" {
		return "def", t, true
	}
	if t, found := strings.CutPrefix(captureName, "name.reference."); found && t != "" {
		return "ref", t, true
	}
	return "", "", false
}

// Close releases compiled query resources.
func (q *QueryLoader) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, query := range q.queries {
		if query != nil {
			query.Close()
		}
	}
	q.queries = map[string]*tree_sitter.Query{}
	q.captureMap = map[string][]string{}
	return nil
}

func enrichPythonSymbols(symbols []SymbolInfo, root *tree_sitter.Node, content []byte) {
	type symKey struct {
		name string
		line int
	}
	symMap := make(map[symKey]*SymbolInfo, len(symbols))
	for i := range symbols {
		k := symKey{symbols[i].Name, symbols[i].Line}
		if _, exists := symMap[k]; !exists {
			symMap[k] = &symbols[i]
		}
	}

	extractDoc := func(bodyNode *tree_sitter.Node) string {
		if bodyNode == nil || bodyNode.Kind() != "block" {
			return ""
		}
		for i := range int(bodyNode.ChildCount()) {
			child := bodyNode.Child(uint(i))
			if child == nil || !child.IsNamed() {
				continue
			}
			if child.Kind() == "string" {
				t := strings.TrimSpace(child.Utf8Text(content))
				return strings.Trim(t, "\"'")
			}
			break
		}
		return ""
	}

	var walk func(n *tree_sitter.Node, parentClass string)
	walk = func(n *tree_sitter.Node, parentClass string) {
		if n == nil {
			return
		}

		switch n.Kind() {
		case "class_definition":
			className := ""
			if nameNode := n.ChildByFieldName("name"); nameNode != nil {
				className = strings.TrimSpace(nameNode.Utf8Text(content))
			}
			if sym, ok := symMap[symKey{className, int(n.StartPosition().Row) + 1}]; ok {
				if sym.DocComment == "" {
					sym.DocComment = extractDoc(n.ChildByFieldName("body"))
				}
			}
			for i := range int(n.ChildCount()) {
				walk(n.Child(uint(i)), className)
			}
			return

		case "decorated_definition":
			var decorators []string
			var funcName string
			var funcLine int
			var funcBody *tree_sitter.Node
			for i := range int(n.ChildCount()) {
				child := n.Child(uint(i))
				if child == nil {
					continue
				}
				if child.Kind() == "decorator" {
					t := strings.TrimSpace(child.Utf8Text(content))
					t = strings.TrimPrefix(t, "@")
					decorators = append(decorators, t)
				}
			}
			defNode := n.ChildByFieldName("definition")
			if defNode != nil && defNode.Kind() == "function_definition" {
				if nameNode := defNode.ChildByFieldName("name"); nameNode != nil {
					funcName = strings.TrimSpace(nameNode.Utf8Text(content))
					funcLine = int(defNode.StartPosition().Row) + 1
					funcBody = defNode.ChildByFieldName("body")
				}
			}
			if funcName != "" {
				if sym, ok := symMap[symKey{funcName, funcLine}]; ok {
					if parentClass != "" {
						sym.Parent = parentClass
					}
					sym.Decorators = append(sym.Decorators, decorators...)
					if sym.DocComment == "" {
						sym.DocComment = extractDoc(funcBody)
					}
				}
			}
			return

		case "function_definition":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				name := strings.TrimSpace(nameNode.Utf8Text(content))
				line := int(n.StartPosition().Row) + 1
				if sym, ok := symMap[symKey{name, line}]; ok {
					if parentClass != "" {
						sym.Parent = parentClass
					}
					if sym.DocComment == "" {
						sym.DocComment = extractDoc(n.ChildByFieldName("body"))
					}
				}
			}
			return
		}

		for i := range int(n.ChildCount()) {
			walk(n.Child(uint(i)), parentClass)
		}
	}

	walk(root, "")
}

func enrichTSSymbols(symbols []SymbolInfo, root *tree_sitter.Node, content []byte) {
	type symKey struct {
		name string
		line int
	}
	symMap := make(map[symKey]*SymbolInfo, len(symbols))
	for i := range symbols {
		k := symKey{symbols[i].Name, symbols[i].Line}
		if _, exists := symMap[k]; !exists {
			symMap[k] = &symbols[i]
		}
	}

	var walk func(n *tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}

		switch n.Kind() {
		case "export_statement":
			for i := range int(n.ChildCount()) {
				child := n.Child(uint(i))
				if child == nil || !child.IsNamed() {
					continue
				}
				switch child.Kind() {
				case "class_declaration", "abstract_class_declaration":
					nameNode := child.ChildByFieldName("name")
					if nameNode != nil {
						name := strings.TrimSpace(nameNode.Utf8Text(content))
						line := int(child.StartPosition().Row) + 1
						if sym, ok := symMap[symKey{name, line}]; ok {
							sym.Modifiers = append(sym.Modifiers, "export")
						}
					}
					walk(child)
				case "function_declaration":
					nameNode := child.ChildByFieldName("name")
					if nameNode != nil {
						name := strings.TrimSpace(nameNode.Utf8Text(content))
						line := int(child.StartPosition().Row) + 1
						if sym, ok := symMap[symKey{name, line}]; ok {
							sym.Modifiers = append(sym.Modifiers, "export")
						}
					}
				}
			}
			return

		case "class_declaration", "abstract_class_declaration":
			for i := range int(n.ChildCount()) {
				walk(n.Child(uint(i)))
			}
			return

		case "method_definition":
			nameNode := n.ChildByFieldName("name")
			if nameNode == nil {
				return
			}
			name := strings.TrimSpace(nameNode.Utf8Text(content))
			line := int(n.StartPosition().Row) + 1
			sym, ok := symMap[symKey{name, line}]
			if !ok {
				return
			}
			for i := range int(n.ChildCount()) {
				child := n.Child(uint(i))
				if child == nil {
					continue
				}
				if child.IsNamed() && child.Kind() == "accessibility_modifier" {
					sym.Modifiers = append(sym.Modifiers, strings.TrimSpace(child.Utf8Text(content)))
				}
				if !child.IsNamed() && child.Utf8Text(content) == "async" {
					sym.Modifiers = append(sym.Modifiers, "async")
				}
			}
			return

		case "function_declaration":
			nameNode := n.ChildByFieldName("name")
			if nameNode == nil {
				return
			}
			name := strings.TrimSpace(nameNode.Utf8Text(content))
			line := int(n.StartPosition().Row) + 1
			sym, ok := symMap[symKey{name, line}]
			if !ok {
				return
			}
			for i := range int(n.ChildCount()) {
				child := n.Child(uint(i))
				if child != nil && !child.IsNamed() && child.Utf8Text(content) == "async" {
					sym.Modifiers = append(sym.Modifiers, "async")
				}
			}
			return
		}

		for i := range int(n.ChildCount()) {
			walk(n.Child(uint(i)))
		}
	}

	walk(root)
}
