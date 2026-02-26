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

	captures := cursor.Captures(query, root, content)
	tags := make([]Tag, 0, 64)
	symbols := make([]SymbolInfo, 0, 32)

	for {
		match, captureIndex := captures.Next()
		if match == nil {
			break
		}
		if !match.SatisfiesTextPredicate(query, nil, nil, content) {
			continue
		}

		if int(captureIndex) >= len(match.Captures) {
			continue
		}
		capture := match.Captures[captureIndex]
		if int(capture.Index) >= len(captureNames) {
			continue
		}

		captureName := captureNames[capture.Index]
		kind, nodeType, ok := parseModernCapture(captureName)
		if !ok {
			continue
		}

		node := &capture.Node
		tagName := strings.TrimSpace(node.Utf8Text(content))
		if tagName == "" {
			continue
		}

		start := node.StartPosition()
		tag := Tag{
			RelPath:  relPath,
			Name:     tagName,
			Kind:     kind,
			Line:     int(start.Row) + 1,
			Language: queryKey,
			NodeType: nodeType,
		}
		tags = append(tags, tag)

		if kind == "def" {
			end := node.EndPosition()
			symbols = append(symbols, SymbolInfo{
				Name:    tagName,
				Kind:    nodeType,
				Line:    int(start.Row) + 1,
				EndLine: int(end.Row) + 1,
			})
		}
	}

	return tags, symbols, nil
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
