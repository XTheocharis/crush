package repomap

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

// cl100k_base BPE rank data, embedded at compile time (~1.6 MB).
//
//go:embed data/cl100k_base.tiktoken
var cl100kBaseData []byte

//go:embed data/tokenizer_support.v1.json
var defaultSupportJSON []byte

// DefaultSupportJSON returns the embedded tokenizer support JSON data.
func DefaultSupportJSON() []byte { return defaultSupportJSON }

const (
	// encodingCL100kBase is the cl100k_base encoding used by GPT-4 and
	// as an approximation for Anthropic and Google models.
	encodingCL100kBase = "cl100k_base"

	// encodingO200kBase is the o200k_base encoding used by GPT-4o
	// family models.
	encodingO200kBase = "o200k_base"

	// o200kBaseURL is the OpenAI CDN URL for the o200k_base rank file.
	o200kBaseURL = "https://openaipublic.blob.core.windows.net/encodings/o200k_base.tiktoken"

	// cl100kBaseURL is the identifier used by tiktoken-go for cl100k_base.
	cl100kBaseURL = "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken"

	// downloadTimeout is the HTTP timeout for lazy BPE downloads.
	downloadTimeout = 30 * time.Second
)

var initLoaderOnce sync.Once

// InitTiktokenLoader registers the crush BPE loader with tiktoken-go.
// It must be called once before any NewTiktokenCounter or
// NewDefaultTokenCounterProvider calls. It is safe for concurrent use;
// only the first call has any effect.
func InitTiktokenLoader(cacheDir string) {
	initLoaderOnce.Do(func() {
		tiktoken.SetBpeLoader(newCrushBpeLoader(cacheDir))
	})
}

// crushBpeLoader implements tiktoken.BpeLoader. It serves cl100k_base from
// embedded data and o200k_base from a lazy-downloaded local cache.
type crushBpeLoader struct {
	cacheDir string
	mu       sync.Mutex
}

func newCrushBpeLoader(cacheDir string) *crushBpeLoader {
	return &crushBpeLoader{cacheDir: cacheDir}
}

// LoadTiktokenBpe satisfies tiktoken.BpeLoader. For cl100k_base it returns
// the embedded rank data; for o200k_base it attempts a cached download.
func (l *crushBpeLoader) LoadTiktokenBpe(tiktokenBpeFile string) (map[string]int, error) {
	if tiktokenBpeFile == cl100kBaseURL {
		return parseBpeRanks(cl100kBaseData)
	}
	if tiktokenBpeFile == o200kBaseURL {
		return l.loadO200kBase()
	}
	// Unexpected URL; fall through to embedded cl100k_base as best effort.
	slog.Warn("Unknown tiktoken BPE URL, falling back to cl100k_base",
		"url", tiktokenBpeFile)
	return parseBpeRanks(cl100kBaseData)
}

// loadO200kBase tries the local cache first, then downloads from OpenAI CDN.
func (l *crushBpeLoader) loadO200kBase() (map[string]int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	cacheFile := l.o200kCachePath()
	if data, err := os.ReadFile(cacheFile); err == nil {
		return parseBpeRanks(data)
	}

	data, err := downloadBpeFile(o200kBaseURL)
	if err != nil {
		return nil, fmt.Errorf("download o200k_base: %w", err)
	}

	// Persist to cache. Failure to persist is not fatal.
	if mkErr := os.MkdirAll(filepath.Dir(cacheFile), 0o755); mkErr != nil {
		slog.Warn("Failed to create tiktoken cache directory",
			"path", filepath.Dir(cacheFile), "err", mkErr)
	} else if wErr := writeFileAtomic(cacheFile, data); wErr != nil {
		slog.Warn("Failed to persist o200k_base cache",
			"path", cacheFile, "err", wErr)
	}

	return parseBpeRanks(data)
}

func (l *crushBpeLoader) o200kCachePath() string {
	return filepath.Join(l.cacheDir, "o200k_base.tiktoken")
}

// parseBpeRanks parses the tiktoken rank file format: one line per token,
// base64-encoded token followed by a space and integer rank.
func parseBpeRanks(data []byte) (map[string]int, error) {
	ranks := make(map[string]int, 100000)
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, " ")
		if len(parts) != 2 {
			continue
		}
		token, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, fmt.Errorf("decode BPE token: %w", err)
		}
		rank, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("parse BPE rank: %w", err)
		}
		ranks[string(token)] = rank
	}
	return ranks, nil
}

// downloadBpeFile fetches a BPE rank file from a URL.
func downloadBpeFile(url string) ([]byte, error) {
	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// writeFileAtomic writes data via a temp file + rename for crash safety.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// TiktokenCounter
// ---------------------------------------------------------------------------

// TiktokenCounter implements TokenCounter using a tiktoken encoding.
type TiktokenCounter struct {
	encoding *tiktoken.Tiktoken
}

// NewTiktokenCounter creates a counter for the given encoding name
// (e.g. "cl100k_base", "o200k_base"). InitTiktokenLoader must have been
// called before this function.
func NewTiktokenCounter(encodingName string) (*TiktokenCounter, error) {
	enc, err := tiktoken.GetEncoding(encodingName)
	if err != nil {
		return nil, fmt.Errorf("load tiktoken encoding %q: %w",
			encodingName, err)
	}
	return &TiktokenCounter{encoding: enc}, nil
}

// Count returns the number of tokens in text. The model parameter is
// accepted to satisfy the TokenCounter interface but not used since
// the encoding is already bound at construction time.
func (t *TiktokenCounter) Count(_ context.Context, _ string, text string) (int, error) {
	tokens := t.encoding.Encode(text, nil, nil)
	return len(tokens), nil
}

// ---------------------------------------------------------------------------
// DefaultTokenCounterProvider
// ---------------------------------------------------------------------------

// tokenizerSupportFile represents the tokenizer_support.v1.json schema.
type tokenizerSupportFile struct {
	SupportedFamilies []familyEntry `json:"supported_families"`
}

// familyEntry is a single family in the support file.
type familyEntry struct {
	ModelFamily      string   `json:"model_family"`
	TokenizerID      string   `json:"tokenizer_id"`
	TokenizerVersion string   `json:"tokenizer_version"`
	Supported        bool     `json:"supported"`
	Models           []string `json:"models"`
}

// DefaultTokenCounterProvider resolves TokenCounters for known model
// families using tiktoken encodings. Anthropic and Google models use
// cl100k_base as an approximation since their native tokenizers are
// not publicly available.
type DefaultTokenCounterProvider struct {
	mu       sync.Mutex
	counters map[string]*TiktokenCounter // encoding name -> counter
	families map[string]familyEntry      // model string -> family entry
}

// NewDefaultTokenCounterProvider creates a provider by loading model-family
// mappings from the tokenizer support JSON data. InitTiktokenLoader must
// have been called before this function.
func NewDefaultTokenCounterProvider(
	supportJSON []byte,
) (*DefaultTokenCounterProvider, error) {
	var support tokenizerSupportFile
	if err := json.Unmarshal(supportJSON, &support); err != nil {
		return nil, fmt.Errorf("parse tokenizer support JSON: %w", err)
	}

	families := make(map[string]familyEntry, 32)
	for _, fam := range support.SupportedFamilies {
		for _, model := range fam.Models {
			families[model] = fam
		}
	}

	return &DefaultTokenCounterProvider{
		counters: make(map[string]*TiktokenCounter, 4),
		families: families,
	}, nil
}

// CounterForModel returns a TokenCounter for the given model string.
// Resolution order:
//  1. Exact match in the family index.
//  2. Longest prefix match against known model strings.
//  3. No match: returns nil, false.
func (p *DefaultTokenCounterProvider) CounterForModel(model string) (TokenCounter, bool) {
	fam, ok := p.resolveFamily(model)
	if !ok {
		return nil, false
	}

	encName := resolveEncodingName(fam)

	p.mu.Lock()
	defer p.mu.Unlock()

	if c, cached := p.counters[encName]; cached {
		return c, true
	}

	c, err := NewTiktokenCounter(encName)
	if err != nil {
		// For o200k_base download failures, fall back to cl100k_base.
		if encName == encodingO200kBase {
			slog.Warn("Failed to load o200k_base, falling back to cl100k_base",
				"err", err)
			c, err = NewTiktokenCounter(encodingCL100kBase)
			if err != nil {
				slog.Warn("Failed to load cl100k_base fallback", "err", err)
				return nil, false
			}
			p.counters[encName] = c
			return c, true
		}
		slog.Warn("Failed to load tiktoken encoding",
			"encoding", encName, "err", err)
		return nil, false
	}

	p.counters[encName] = c
	return c, true
}

// MetadataForModel returns tokenizer metadata for a model string.
func (p *DefaultTokenCounterProvider) MetadataForModel(model string) (TokenizerMetadata, bool) {
	fam, ok := p.resolveFamily(model)
	if !ok {
		return TokenizerMetadata{}, false
	}
	return TokenizerMetadata{
		TokenizerID:      resolveEncodingName(fam),
		TokenizerVersion: fam.TokenizerVersion,
		Supported:        fam.Supported,
	}, true
}

// resolveFamily looks up a model by exact match then by longest prefix.
func (p *DefaultTokenCounterProvider) resolveFamily(model string) (familyEntry, bool) {
	// Exact match.
	if fam, ok := p.families[model]; ok {
		return fam, true
	}

	// Longest prefix match.
	var best familyEntry
	bestLen := 0
	for key, fam := range p.families {
		if strings.HasPrefix(model, key) && len(key) > bestLen {
			best = fam
			bestLen = len(key)
		}
	}
	if bestLen > 0 {
		return best, true
	}
	return familyEntry{}, false
}

// resolveEncodingName maps a family entry to the tiktoken encoding name.
// Anthropic and Google models use cl100k_base as an approximation since
// their native tokenizers are not publicly available.
func resolveEncodingName(fam familyEntry) string {
	switch fam.TokenizerID {
	case encodingCL100kBase:
		return encodingCL100kBase
	case encodingO200kBase:
		return encodingO200kBase
	case "claude":
		// Claude models do not have a public tokenizer. Use cl100k_base
		// as a reasonable approximation for token budget planning.
		return encodingCL100kBase
	case "gemini":
		// Gemini models use SentencePiece internally. cl100k_base is a
		// reasonable approximation for token budget planning.
		return encodingCL100kBase
	default:
		return encodingCL100kBase
	}
}

// TiktokenCacheDir returns the cache directory for tiktoken BPE files.
// It respects $XDG_CACHE_HOME, falling back to ~/.cache.
func TiktokenCacheDir() string {
	cache := os.Getenv("XDG_CACHE_HOME")
	if cache == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.TempDir()
		}
		cache = filepath.Join(home, ".cache")
	}
	return filepath.Join(cache, "crush", "tiktoken")
}
