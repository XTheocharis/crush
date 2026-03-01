package treesitter

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"maps"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	tree_sitter_dart "github.com/UserNobody14/tree-sitter-dart/bindings/go"
	tree_sitter_arduino "github.com/tree-sitter-grammars/tree-sitter-arduino/bindings/go"
	tree_sitter_chatito "github.com/tree-sitter-grammars/tree-sitter-chatito/bindings/go"
	tree_sitter_hcl "github.com/tree-sitter-grammars/tree-sitter-hcl/bindings/go"
	tree_sitter_lua "github.com/tree-sitter-grammars/tree-sitter-lua/bindings/go"
	tree_sitter_properties "github.com/tree-sitter-grammars/tree-sitter-properties/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c_sharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_haskell "github.com/tree-sitter/tree-sitter-haskell/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_julia "github.com/tree-sitter/tree-sitter-julia/bindings/go"
	tree_sitter_ocaml "github.com/tree-sitter/tree-sitter-ocaml/bindings/go"
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_scala "github.com/tree-sitter/tree-sitter-scala/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

var (
	// ErrParserPoolClosed indicates parser acquisition failed because the pool is closed.
	ErrParserPoolClosed = errors.New("treesitter parser pool is closed")

	// cachedSupportedLanguages caches the result of loadSupportedLanguages
	// so repeated NewParser() calls avoid redundant manifest parsing.
	cachedSupportedLanguages     []string
	cachedSupportedLanguageSet   map[string]struct{}
	cachedSupportedLanguagesOnce sync.Once
	cachedSupportedLanguagesErr  error
)

// languageParser wraps a language-specific parser instance.
type languageParser struct {
	lang      string
	parser    *tree_sitter.Parser
	closeOnce sync.Once
	closeFn   func()
}

func newLanguageParser() *languageParser {
	parser := tree_sitter.NewParser()
	return &languageParser{
		parser: parser,
		closeFn: func() {
			parser.Close()
		},
	}
}

func (lp *languageParser) close() {
	if lp == nil {
		return
	}
	lp.closeOnce.Do(func() {
		if lp.closeFn != nil {
			lp.closeFn()
		}
	})
}

// parser is a compile-safe Parser implementation scaffold.
type parser struct {
	pool      *ParserPool
	languages []string
	langSet   map[string]struct{}

	treeCache    *Cache
	queryLoader  *QueryLoader
	treeLangs    map[string]*tree_sitter.Language
	languageInit sync.Once
}

// ParserPool manages language parser instances.
type ParserPool struct {
	poolSize int
	parsers  chan *languageParser
	closeCh  chan struct{}

	closed    atomic.Bool
	closeOnce sync.Once

	lifecycleMu sync.RWMutex
	holders     sync.WaitGroup
	factory     func() *languageParser
}

// ParserConfig configures parser lifecycle/performance behavior.
type ParserConfig struct {
	// PoolSize controls the parser pool capacity.
	// Zero or negative values fall back to runtime defaults.
	PoolSize int
}

// NewParserPool creates a parser pool using runtime defaults.
func NewParserPool() *ParserPool {
	return NewParserPoolWithSize(defaultParserPoolSize())
}

// NewParserPoolWithSize creates a parser pool with explicit capacity.
func NewParserPoolWithSize(size int) *ParserPool {
	return newParserPoolWithFactory(size, nil)
}

func defaultParserPoolSize() int {
	size := runtime.NumCPU()
	if size <= 0 {
		return 1
	}
	return size
}

func newParserPoolWithFactory(size int, factory func() *languageParser) *ParserPool {
	if size <= 0 {
		size = 1
	}
	if factory == nil {
		factory = newLanguageParser
	}

	pool := &ParserPool{
		poolSize: size,
		parsers:  make(chan *languageParser, size),
		closeCh:  make(chan struct{}),
		factory:  factory,
	}

	for range size {
		pool.parsers <- pool.factory()
	}

	return pool
}

// Capacity returns the configured pool size.
func (p *ParserPool) Capacity() int {
	if p == nil {
		return 0
	}
	return p.poolSize
}

// Acquire acquires a parser from the pool, or returns false when canceled/closed.
func (p *ParserPool) Acquire(ctx context.Context, lang string) (*languageParser, bool) {
	if p == nil {
		return nil, false
	}
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil, false
		}
		if p.closed.Load() {
			return nil, false
		}

		select {
		case <-ctx.Done():
			return nil, false
		case <-p.closeCh:
			return nil, false
		case lp := <-p.parsers:
			if lp == nil {
				continue
			}
			if err := ctx.Err(); err != nil {
				if p.closed.Load() {
					lp.close()
				} else {
					select {
					case p.parsers <- lp:
					case <-p.closeCh:
						lp.close()
					}
				}
				return nil, false
			}

			p.lifecycleMu.RLock()
			if p.closed.Load() {
				p.lifecycleMu.RUnlock()
				lp.close()
				return nil, false
			}
			lp.lang = lang
			p.holders.Add(1)
			p.lifecycleMu.RUnlock()
			return lp, true
		}
	}
}

// Get returns a parser for a language.
func (p *ParserPool) Get(lang string) (*languageParser, error) {
	lp, ok := p.Acquire(context.Background(), lang)
	if !ok {
		return nil, ErrParserPoolClosed
	}
	return lp, nil
}

// Release returns a parser to the pool.
func (p *ParserPool) Release(lang string, lp *languageParser) {
	_ = lang
	p.release(lp)
}

func (p *ParserPool) release(lp *languageParser) {
	if p == nil || lp == nil {
		return
	}
	defer p.holders.Done()

	if p.closed.Load() {
		lp.close()
		return
	}

	select {
	case p.parsers <- lp:
		return
	case <-p.closeCh:
		lp.close()
		return
	}
}

// Close releases all parser resources in the pool.
func (p *ParserPool) Close() error {
	if p == nil {
		return nil
	}

	p.closeOnce.Do(func() {
		// Mark closed first so acquires stop immediately.
		p.lifecycleMu.Lock()
		p.closed.Store(true)
		close(p.closeCh)
		p.lifecycleMu.Unlock()

		// Wait only for successfully acquired holders.
		p.holders.Wait()

		// Drain idle parsers and release CGO resources.
		for {
			select {
			case lp := <-p.parsers:
				if lp != nil {
					lp.close()
				}
			default:
				return
			}
		}
	})

	return nil
}

// NewParser creates a Parser scaffold.
func NewParser() Parser {
	return NewParserWithConfig(ParserConfig{})
}

// NewParserWithConfig creates a Parser scaffold with performance options.
func NewParserWithConfig(cfg ParserConfig) Parser {
	poolSize := cfg.PoolSize
	if poolSize <= 0 {
		poolSize = defaultParserPoolSize()
	}

	languages, langSet := loadSupportedLanguages()
	pr := &parser{
		pool:        NewParserPoolWithSize(poolSize),
		languages:   languages,
		langSet:     langSet,
		treeCache:   NewCache(0, 0),
		queryLoader: NewQueryLoader(),
		treeLangs:   map[string]*tree_sitter.Language{},
	}
	pr.initLanguages()
	return pr
}

func (p *parser) initLanguages() {
	p.languageInit.Do(func() {
		for _, lang := range p.languages {
			tsLang := languageForQueryKey(lang)
			if tsLang == nil {
				continue
			}
			p.treeLangs[lang] = tsLang
			p.queryLoader.RegisterLanguage(lang, tsLang)
		}
	})
}

// Analyze analyzes source content and returns a file analysis.
func (p *parser) Analyze(ctx context.Context, path string, content []byte) (*FileAnalysis, error) {
	lang := MapPath(path)

	lp, ok := p.pool.Acquire(ctx, lang)
	if !ok {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, ErrParserPoolClosed
	}
	defer p.pool.release(lp)

	if lang == "" {
		return &FileAnalysis{Language: ""}, nil
	}
	if !p.SupportsLanguage(lang) {
		return &FileAnalysis{Language: lang}, nil
	}
	if !p.HasTags(lang) {
		return &FileAnalysis{Language: lang}, nil
	}

	tsLang := p.treeLangs[GetQueryKey(lang)]
	if tsLang == nil {
		return &FileAnalysis{Language: lang}, nil
	}
	if err := lp.parser.SetLanguage(tsLang); err != nil {
		return nil, fmt.Errorf("set parser language %q: %w", lang, err)
	}

	cacheKey := treeCacheKey(path, content)
	tree, ok := p.treeCache.Get(cacheKey)
	if !ok {
		tree = lp.parser.Parse(content, nil)
		if tree == nil {
			return &FileAnalysis{Language: lang}, nil
		}
		p.treeCache.Put(cacheKey, tree, content)
		tree = tree.Clone()
	}
	defer tree.Close()

	root := tree.RootNode()
	if root == nil {
		return &FileAnalysis{Language: lang}, nil
	}

	cursor := tree_sitter.NewQueryCursor()
	defer cursor.Close()

	relPath := filepath.ToSlash(path)
	tags, symbols, err := p.queryLoader.ExtractTagsWithCursor(lang, relPath, root, content, cursor)
	if err != nil {
		return nil, err
	}

	return &FileAnalysis{
		Language: GetQueryKey(lang),
		Tags:     tags,
		Symbols:  symbols,
	}, nil
}

// ParseTree parses the source content and returns a cloned AST tree.
// The caller owns the returned tree and must call tree.Close() when done.
// Unlike Analyze(), ParseTree does not require a tags query — only a grammar.
func (p *parser) ParseTree(ctx context.Context, path string, content []byte) (*tree_sitter.Tree, error) {
	lang := MapPath(path)
	if lang == "" {
		return nil, fmt.Errorf("unsupported file: %s", path)
	}
	if !p.SupportsLanguage(lang) {
		return nil, fmt.Errorf("unsupported language %q for %s", lang, path)
	}

	lp, ok := p.pool.Acquire(ctx, lang)
	if !ok {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, ErrParserPoolClosed
	}
	defer p.pool.release(lp)

	tsLang := p.treeLangs[GetQueryKey(lang)]
	if tsLang == nil {
		return nil, fmt.Errorf("no grammar loaded for %q", lang)
	}
	if err := lp.parser.SetLanguage(tsLang); err != nil {
		return nil, fmt.Errorf("set parser language %q: %w", lang, err)
	}

	cacheKey := treeCacheKey(path, content)
	tree, ok := p.treeCache.Get(cacheKey)
	if !ok {
		tree = lp.parser.Parse(content, nil)
		if tree == nil {
			return nil, fmt.Errorf("tree-sitter parse returned nil for %s", path)
		}
		p.treeCache.Put(cacheKey, tree, content)
		tree = tree.Clone()
	}

	return tree, nil
}

func treeCacheKey(path string, content []byte) string {
	h := fnv.New64a()
	_, _ = h.Write(content)
	hash := h.Sum64()

	// Pre-allocate buffer: path (len) + ":" (1) + content length (up to 19)
	// + ":" (1) + hash hex (16) + safe margin
	buf := make([]byte, 0, len(path)+1+19+1+16)

	buf = append(buf, path...)
	buf = append(buf, ':')
	buf = strconv.AppendInt(buf, int64(len(content)), 10)
	buf = append(buf, ':')
	buf = strconv.AppendUint(buf, hash, 16)
	return string(buf)
}

// treeCacheKeyFmtString is the original fmt.Sprintf implementation for benchmarking.
func treeCacheKeyFmtString(path string, content []byte) string {
	h := fnv.New64a()
	_, _ = h.Write(content)
	return fmt.Sprintf("%s:%d:%x", path, len(content), h.Sum64())
}

// treeCacheKeyOptimized is the optimized implementation reducing allocations.
func treeCacheKeyOptimized(path string, content []byte) string {
	h := fnv.New64a()
	_, _ = h.Write(content)
	hash := h.Sum64()

	// Pre-allocate buffer: path (len) + ":" (1) + content length (up to 19)
	// + ":" (1) + hash hex (16) + safe margin
	buf := make([]byte, 0, len(path)+1+19+1+16)

	buf = append(buf, path...)
	buf = append(buf, ':')
	buf = strconv.AppendInt(buf, int64(len(content)), 10)
	buf = append(buf, ':')
	buf = strconv.AppendUint(buf, hash, 16)
	return string(buf)
}

// Languages returns supported language IDs.
func (p *parser) Languages() []string {
	out := make([]string, len(p.languages))
	copy(out, p.languages)
	return out
}

// SupportsLanguage reports whether lang is supported.
func (p *parser) SupportsLanguage(lang string) bool {
	_, ok := p.langSet[GetQueryKey(lang)]
	return ok
}

// HasTags reports whether the language has tags query support.
func (p *parser) HasTags(lang string) bool {
	lang = GetQueryKey(lang)
	if !p.SupportsLanguage(lang) {
		return false
	}
	return HasTagsQuery(lang)
}

// Close closes parser resources.
func (p *parser) Close() error {
	if p.queryLoader != nil {
		if err := p.queryLoader.Close(); err != nil {
			return err
		}
	}
	if p.treeCache != nil {
		if err := p.treeCache.Close(); err != nil {
			return err
		}
	}
	if p.pool != nil {
		return p.pool.Close()
	}
	return nil
}

func loadSupportedLanguages() ([]string, map[string]struct{}) {
	cachedSupportedLanguagesOnce.Do(func() {
		cachedSupportedLanguages, cachedSupportedLanguageSet = loadSupportedLanguagesImpl()
	})

	// Return copies to prevent mutation of cached values
	setCopy := make(map[string]struct{}, len(cachedSupportedLanguageSet))
	maps.Copy(setCopy, cachedSupportedLanguageSet)
	langCopy := make([]string, len(cachedSupportedLanguages))
	copy(langCopy, cachedSupportedLanguages)

	return langCopy, setCopy
}

func loadSupportedLanguagesImpl() ([]string, map[string]struct{}) {
	manifest, err := LoadLanguagesManifest()
	if err != nil {
		cachedSupportedLanguagesErr = err
		return nil, map[string]struct{}{}
	}

	set := make(map[string]struct{}, len(manifest.Languages))
	languages := make([]string, 0, len(manifest.Languages))
	for _, lang := range manifest.Languages {
		name := GetQueryKey(lang.Name)
		if name == "" {
			continue
		}
		if _, exists := set[name]; exists {
			continue
		}
		set[name] = struct{}{}
		languages = append(languages, name)
	}
	sort.Strings(languages)

	return languages, set
}

// languageForQueryKey registers runtime grammars for canonical query keys.
//
// The manifest/query canonical set can be larger than this runtime set;
// tests enforce that every canonical manifest language is either
// runtime-activated here or listed in
// testdata/runtime_language_exceptions.v1.json.
func languageForQueryKey(queryKey string) *tree_sitter.Language {
	switch queryKey {
	case "arduino":
		return tree_sitter.NewLanguage(tree_sitter_arduino.Language())
	case "c":
		return tree_sitter.NewLanguage(tree_sitter_c.Language())
	case "chatito":
		return tree_sitter.NewLanguage(tree_sitter_chatito.LanguageChatito())
	case "cpp":
		return tree_sitter.NewLanguage(tree_sitter_cpp.Language())
	case "csharp":
		return tree_sitter.NewLanguage(tree_sitter_c_sharp.Language())
	case "dart":
		return tree_sitter.NewLanguage(tree_sitter_dart.Language())
	case "go":
		return tree_sitter.NewLanguage(tree_sitter_go.Language())
	case "haskell":
		return tree_sitter.NewLanguage(tree_sitter_haskell.Language())
	case "hcl":
		return tree_sitter.NewLanguage(tree_sitter_hcl.Language())
	case "java":
		return tree_sitter.NewLanguage(tree_sitter_java.Language())
	case "javascript":
		return tree_sitter.NewLanguage(tree_sitter_javascript.Language())
	case "julia":
		return tree_sitter.NewLanguage(tree_sitter_julia.Language())
	case "lua":
		return tree_sitter.NewLanguage(tree_sitter_lua.Language())
	case "ocaml":
		return tree_sitter.NewLanguage(tree_sitter_ocaml.LanguageOCaml())
	case "ocaml_interface":
		return tree_sitter.NewLanguage(tree_sitter_ocaml.LanguageOCamlInterface())
	case "php":
		return tree_sitter.NewLanguage(tree_sitter_php.LanguagePHP())
	case "properties":
		return tree_sitter.NewLanguage(tree_sitter_properties.Language())
	case "python":
		return tree_sitter.NewLanguage(tree_sitter_python.Language())
	case "ruby":
		return tree_sitter.NewLanguage(tree_sitter_ruby.Language())
	case "rust":
		return tree_sitter.NewLanguage(tree_sitter_rust.Language())
	case "scala":
		return tree_sitter.NewLanguage(tree_sitter_scala.Language())
	case "typescript":
		return tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	default:
		return nil
	}
}
