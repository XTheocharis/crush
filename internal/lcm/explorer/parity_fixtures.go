package explorer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ParityFixtureIndex maps fixture categories to file paths.
type ParityFixtureIndex struct {
	Language map[string]string `json:"language"`
	Format   map[string]string `json:"format"`
	Binary   map[string]string `json:"binary"`
	Shell    map[string]string `json:"shell"`
	Markdown map[string]string `json:"markdown"`
	Negative map[string]string `json:"negative"`
	Metadata FixtureMetadata   `json:"metadata"`
}

// FixtureMetadata contains provenance and metadata for the fixture corpus.
type FixtureMetadata struct {
	VoltCommitSHA           string `json:"volt_commit_sha"`
	FixturesSHA256          string `json:"fixtures_sha256"`
	ComparatorPath          string `json:"comparator_path"`
	Version                 string `json:"version"`
	GeneratedAt             string `json:"generated_at"`
	Profile                 string `json:"profile,omitempty"`
	DeterministicMode       bool   `json:"deterministic_mode,omitempty"`
	EnhancementTiersEnabled string `json:"enhancement_tiers_enabled,omitempty"`
	TokenCounterMode        string `json:"token_counter_mode,omitempty"`
	FixedSeed               int64  `json:"fixed_seed,omitempty"`
}

// ParityTestFixtureConfig defines valid fixture files for parity testing.
type ParityTestFixtureConfig struct {
	IndexFilePath string
	FixturesDir   string
}

// DefaultParityFixturesPath is the default location for parity fixture index.
const DefaultParityFixturesPath = "testdata/parity_volt/fixture_index.v1.json"

// NewDefaultParityFixtureConfig returns config for default fixture location.
func NewDefaultParityFixtureConfig(baseDir string) ParityTestFixtureConfig {
	return ParityTestFixtureConfig{
		IndexFilePath: filepath.Join(baseDir, DefaultParityFixturesPath),
		FixturesDir:   filepath.Join(baseDir, "testdata", "parity_volt", "fixtures"),
	}
}

// LoadParityFixtureIndex loads the fixture index from disk.
func LoadParityFixtureIndex(cfg ParityTestFixtureConfig) (*ParityFixtureIndex, error) {
	content, err := os.ReadFile(cfg.IndexFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read fixture index: %w", err)
	}

	var index ParityFixtureIndex
	if err := json.Unmarshal(content, &index); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fixture index: %w", err)
	}

	return &index, nil
}

// LoadFixtureFile loads a specific fixture file by path.
func LoadFixtureFile(cfg ParityTestFixtureConfig, filename string) ([]byte, error) {
	fullPath := filepath.Join(cfg.FixturesDir, filename)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read fixture file %s: %w", filename, err)
	}
	return content, nil
}

// ValidateFixtureMetadata validates the fixture metadata fields.
func ValidateFixtureMetadata(meta FixtureMetadata) error {
	if meta.VoltCommitSHA == "" {
		return fmt.Errorf("metadata missing volt_commit_sha")
	}
	if isPlaceholderCommitSHA(meta.VoltCommitSHA) {
		return fmt.Errorf("metadata contains placeholder volt_commit_sha")
	}
	if meta.FixturesSHA256 == "" {
		return fmt.Errorf("metadata missing fixtures_sha256")
	}
	if isPlaceholderFixtureHash(meta.FixturesSHA256) {
		return fmt.Errorf("metadata contains placeholder fixtures_sha256")
	}
	if meta.ComparatorPath == "" {
		return fmt.Errorf("metadata missing comparator_path")
	}
	if isPlaceholderComparatorPath(meta.ComparatorPath) {
		return fmt.Errorf("metadata contains placeholder comparator_path")
	}
	if meta.Version == "" {
		return fmt.Errorf("metadata missing version")
	}
	if meta.GeneratedAt == "" {
		return fmt.Errorf("metadata missing generated_at")
	}
	if strings.EqualFold(strings.TrimSpace(meta.Profile), "parity") {
		if !meta.DeterministicMode {
			return fmt.Errorf("metadata parity profile requires deterministic_mode=true")
		}
		if strings.ToLower(strings.TrimSpace(meta.EnhancementTiersEnabled)) != "none" {
			return fmt.Errorf("metadata parity profile requires enhancement_tiers_enabled=none")
		}
		counterMode := strings.ToLower(strings.TrimSpace(meta.TokenCounterMode))
		if counterMode != "tokenizer_backed" && counterMode != "heuristic" {
			return fmt.Errorf("metadata parity profile requires token_counter_mode tokenizer_backed or heuristic")
		}
		if meta.FixedSeed <= 0 {
			return fmt.Errorf("metadata parity profile requires positive fixed_seed")
		}
	}
	return nil
}

// FixtureIndexValidator validates a fixture index structure.
type FixtureIndexValidator interface {
	ValidateIndex(index *ParityFixtureIndex) error
	ValidateFixtureExists(cfg ParityTestFixtureConfig, path string) error
}

// DefaultFixtureValidator implements standard fixture validation.
type DefaultFixtureValidator struct{}

func (v *DefaultFixtureValidator) ValidateIndex(index *ParityFixtureIndex) error {
	if index == nil {
		return fmt.Errorf("index is nil")
	}

	// Validate metadata
	if err := ValidateFixtureMetadata(index.Metadata); err != nil {
		return fmt.Errorf("metadata validation failed: %w", err)
	}

	// Validate language fixtures
	if len(index.Language) == 0 {
		return fmt.Errorf("index has no language fixtures")
	}

	// Validate format fixtures
	if len(index.Format) == 0 {
		return fmt.Errorf("index has no format fixtures")
	}

	return nil
}

func (v *DefaultFixtureValidator) ValidateFixtureExists(cfg ParityTestFixtureConfig, path string) error {
	fullPath := filepath.Join(cfg.FixturesDir, path)
	_, err := os.Stat(fullPath)
	if err != nil {
		return fmt.Errorf("fixture file not found: %s: %w", path, err)
	}
	return nil
}

// ParityFixtureLoader handles loading parity fixtures for testing.
type ParityFixtureLoader struct {
	cfg       ParityTestFixtureConfig
	validator FixtureIndexValidator
}

// NewParityFixtureLoader creates a new fixture loader.
func NewParityFixtureLoader(cfg ParityTestFixtureConfig) *ParityFixtureLoader {
	return &ParityFixtureLoader{
		cfg:       cfg,
		validator: &DefaultFixtureValidator{},
	}
}

// LoadIndex loads and validates the fixture index.
func (l *ParityFixtureLoader) LoadIndex() (*ParityFixtureIndex, error) {
	index, err := LoadParityFixtureIndex(l.cfg)
	if err != nil {
		return nil, err
	}

	if err := l.validator.ValidateIndex(index); err != nil {
		return nil, fmt.Errorf("index validation failed: %w", err)
	}

	return index, nil
}

// LoadAllFixtures loads all fixtures referenced in the index.
func (l *ParityFixtureLoader) LoadAllFixtures() (map[string][]byte, error) {
	index, err := l.LoadIndex()
	if err != nil {
		return nil, err
	}

	fixtures := make(map[string][]byte)

	// Load language fixtures
	for name, path := range index.Language {
		content, err := LoadFixtureFile(l.cfg, path)
		if err != nil {
			return nil, err
		}
		fixtures[name] = content
	}

	// Load format fixtures
	for name, path := range index.Format {
		content, err := LoadFixtureFile(l.cfg, path)
		if err != nil {
			return nil, err
		}
		fixtures[name] = content
	}

	// Load other fixture categories
	if len(index.Shell) > 0 {
		for name, path := range index.Shell {
			content, err := LoadFixtureFile(l.cfg, path)
			if err != nil {
				return nil, err
			}
			fixtures[name] = content
		}
	}

	if len(index.Markdown) > 0 {
		for name, path := range index.Markdown {
			content, err := LoadFixtureFile(l.cfg, path)
			if err != nil {
				return nil, err
			}
			fixtures[name] = content
		}
	}

	return fixtures, nil
}

// GenerateFixtureIndex creates an index from existing fixture files.
func GenerateFixtureIndex(cfg ParityTestFixtureConfig, meta FixtureMetadata) (*ParityFixtureIndex, error) {
	files, err := os.ReadDir(cfg.FixturesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read fixtures directory: %w", err)
	}

	index := &ParityFixtureIndex{
		Language: make(map[string]string),
		Format:   make(map[string]string),
		Binary:   make(map[string]string),
		Shell:    make(map[string]string),
		Markdown: make(map[string]string),
		Negative: make(map[string]string),
		Metadata: meta,
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		switch name {
		case "retrieval_README.md":
			index.Markdown["readme"] = name
		case "runtime_deploy.sh":
			index.Shell["deploy_script"] = name
		case "language_go_server.go":
			index.Language["go"] = name
		case "language_python_processor.py":
			index.Language["python"] = name
		case "format_package.json":
			index.Format["json"] = name
		case "format_docker-compose.yml":
			index.Format["yaml"] = name
		case "format_employees.csv":
			index.Format["csv"] = name
		case "format_research.tex":
			index.Format["latex"] = name
		case "format_service.log":
			index.Format["logs"] = name
		case "format_sqlite_seed.sql":
			index.Format["sqlite_seed"] = name
		}
	}

	return index, nil
}

func normalizeFixtureJSONForHash(data []byte) ([]byte, error) {
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	stripFixtureHashFields(payload)
	return json.Marshal(payload)
}

func stripFixtureHashFields(v any) {
	switch typed := v.(type) {
	case map[string]any:
		for k, child := range typed {
			if k == "fixtures_sha256" {
				delete(typed, k)
				continue
			}
			stripFixtureHashFields(child)
		}
	case []any:
		for _, child := range typed {
			stripFixtureHashFields(child)
		}
	}
}

func ComputeFixturesSHA256(fixturesDir string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(fixturesDir, "*"))
	if err != nil {
		return "", fmt.Errorf("glob fixtures: %w", err)
	}

	sort.Strings(paths)
	combined := strings.Builder{}
	for _, path := range paths {
		info, statErr := os.Stat(path)
		if statErr != nil || info.IsDir() {
			continue
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", fmt.Errorf("read fixture %q: %w", path, readErr)
		}

		ext := strings.ToLower(filepath.Ext(path))
		normalized := data
		if ext == ".json" {
			normalized, err = normalizeFixtureJSONForHash(data)
			if err != nil {
				return "", fmt.Errorf("normalize fixture %q for hash: %w", path, err)
			}
		}

		combined.Write(normalized)
	}

	sum := sha256.Sum256([]byte(combined.String()))
	return hex.EncodeToString(sum[:]), nil
}

func VerifyFixturesIntegrity(index *ParityFixtureIndex, cfg ParityTestFixtureConfig) error {
	if index == nil {
		return fmt.Errorf("fixture index is nil")
	}
	expected := strings.TrimSpace(index.Metadata.FixturesSHA256)
	if expected == "" || isPlaceholderFixtureHash(expected) {
		return fmt.Errorf("fixture index has empty or placeholder fixtures_sha256")
	}

	computed, err := ComputeFixturesSHA256(cfg.FixturesDir)
	if err != nil {
		return fmt.Errorf("compute fixtures sha256: %w", err)
	}
	if computed != expected {
		return fmt.Errorf("fixtures_sha256 mismatch: expected=%s computed=%s", expected, computed)
	}

	return nil
}
