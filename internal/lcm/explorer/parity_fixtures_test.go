package explorer

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadParityFixtureIndex_Valid(t *testing.T) {
	t.Parallel()

	cfg := NewDefaultParityFixtureConfig(".")
	index, err := LoadParityFixtureIndex(cfg)
	require.NoError(t, err)
	require.NotNil(t, index)

	// Verify metadata is populated
	require.NotEmpty(t, index.Metadata.VoltCommitSHA)
	require.NotEmpty(t, index.Metadata.FixturesSHA256)
	require.NotEmpty(t, index.Metadata.ComparatorPath)
	require.Equal(t, "1", index.Metadata.Version)
	require.NotEmpty(t, index.Metadata.GeneratedAt)

	// Verify language fixtures
	require.Contains(t, index.Language, "go")
	require.Contains(t, index.Language, "python")

	// Verify format fixtures
	require.Contains(t, index.Format, "json")
	require.Contains(t, index.Format, "yaml")
	require.Contains(t, index.Format, "csv")

	// Verify Shell fixtures
	require.Contains(t, index.Shell, "deploy_script")

	// Verify Markdown fixtures
	require.Contains(t, index.Markdown, "readme")
}

func TestValidateFixtureMetadata_Complete(t *testing.T) {
	t.Parallel()

	meta := FixtureMetadata{
		VoltCommitSHA:           strings.Repeat("a", 40),
		FixturesSHA256:          strings.Repeat("b", 64),
		ComparatorPath:          "../volt/tree/" + strings.Repeat("a", 40),
		Version:                 "1",
		GeneratedAt:             "2026-02-26T00:00:00Z",
		Profile:                 "parity",
		DeterministicMode:       true,
		EnhancementTiersEnabled: "none",
		TokenCounterMode:        "tokenizer_backed",
		FixedSeed:               1337,
	}

	err := ValidateFixtureMetadata(meta)
	require.NoError(t, err)
}

func TestValidateFixtureMetadata_MissingFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*FixtureMetadata)
		want   string
	}{
		{
			name: "missing volt_commit_sha",
			mutate: func(m *FixtureMetadata) {
				m.VoltCommitSHA = ""
			},
			want: "volt_commit_sha",
		},
		{
			name: "missing fixtures_sha256",
			mutate: func(m *FixtureMetadata) {
				m.FixturesSHA256 = ""
			},
			want: "fixtures_sha256",
		},
		{
			name: "missing comparator_path",
			mutate: func(m *FixtureMetadata) {
				m.ComparatorPath = ""
			},
			want: "comparator_path",
		},
		{
			name: "missing version",
			mutate: func(m *FixtureMetadata) {
				m.Version = ""
			},
			want: "version",
		},
		{
			name: "missing generated_at",
			mutate: func(m *FixtureMetadata) {
				m.GeneratedAt = ""
			},
			want: "generated_at",
		},
		{
			name: "parity metadata requires deterministic_mode",
			mutate: func(m *FixtureMetadata) {
				m.DeterministicMode = false
			},
			want: "deterministic_mode",
		},
		{
			name: "parity metadata requires enhancement_tiers_enabled none",
			mutate: func(m *FixtureMetadata) {
				m.EnhancementTiersEnabled = "llm"
			},
			want: "enhancement_tiers_enabled",
		},
		{
			name: "parity metadata requires token_counter_mode",
			mutate: func(m *FixtureMetadata) {
				m.TokenCounterMode = ""
			},
			want: "token_counter_mode",
		},
		{
			name: "parity metadata requires fixed_seed",
			mutate: func(m *FixtureMetadata) {
				m.FixedSeed = 0
			},
			want: "fixed_seed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			meta := FixtureMetadata{
				VoltCommitSHA:           strings.Repeat("a", 40),
				FixturesSHA256:          strings.Repeat("b", 64),
				ComparatorPath:          "../volt/tree/" + strings.Repeat("a", 40),
				Version:                 "1",
				GeneratedAt:             "2026-02-26T00:00:00Z",
				Profile:                 "parity",
				DeterministicMode:       true,
				EnhancementTiersEnabled: "none",
				TokenCounterMode:        "tokenizer_backed",
				FixedSeed:               1337,
			}
			tt.mutate(&meta)

			err := ValidateFixtureMetadata(meta)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestDefaultFixtureValidator_ValidateIndex(t *testing.T) {
	t.Parallel()

	validator := &DefaultFixtureValidator{}

	t.Run("valid index", func(t *testing.T) {
		t.Parallel()
		index := &ParityFixtureIndex{
			Language: map[string]string{"go": "test.go"},
			Format:   map[string]string{"json": "test.json"},
			Metadata: FixtureMetadata{
				VoltCommitSHA:  strings.Repeat("a", 40),
				FixturesSHA256: strings.Repeat("b", 64),
				ComparatorPath: "../volt/tree/" + strings.Repeat("a", 40),
				Version:        "1",
				GeneratedAt:    "2026-02-26T00:00:00Z",
			},
		}

		err := validator.ValidateIndex(index)
		require.NoError(t, err)
	})

	t.Run("nil index", func(t *testing.T) {
		t.Parallel()
		err := validator.ValidateIndex(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "index is nil")
	})

	t.Run("missing metadata", func(t *testing.T) {
		t.Parallel()
		index := &ParityFixtureIndex{
			Language: map[string]string{"go": "test.go"},
			Format:   map[string]string{"json": "test.json"},
		}

		err := validator.ValidateIndex(index)
		require.Error(t, err)
		require.Contains(t, err.Error(), "metadata")
	})

	t.Run("empty language fixtures", func(t *testing.T) {
		t.Parallel()
		index := &ParityFixtureIndex{
			Language: map[string]string{},
			Format:   map[string]string{"json": "test.json"},
			Metadata: FixtureMetadata{
				VoltCommitSHA:  strings.Repeat("a", 40),
				FixturesSHA256: strings.Repeat("b", 64),
				ComparatorPath: "../volt/tree/" + strings.Repeat("a", 40),
				Version:        "1",
				GeneratedAt:    "2026-02-26T00:00:00Z",
			},
		}

		err := validator.ValidateIndex(index)
		require.Error(t, err)
		require.Contains(t, err.Error(), "language fixtures")
	})

	t.Run("empty format fixtures", func(t *testing.T) {
		t.Parallel()
		index := &ParityFixtureIndex{
			Language: map[string]string{"go": "test.go"},
			Format:   map[string]string{},
			Metadata: FixtureMetadata{
				VoltCommitSHA:  strings.Repeat("a", 40),
				FixturesSHA256: strings.Repeat("b", 64),
				ComparatorPath: "../volt/tree/" + strings.Repeat("a", 40),
				Version:        "1",
				GeneratedAt:    "2026-02-26T00:00:00Z",
			},
		}

		err := validator.ValidateIndex(index)
		require.Error(t, err)
		require.Contains(t, err.Error(), "format fixtures")
	})
}

func TestParityFixtureLoader_LoadIndex(t *testing.T) {
	t.Parallel()

	cfg := NewDefaultParityFixtureConfig(".")
	loader := NewParityFixtureLoader(cfg)

	index, err := loader.LoadIndex()
	require.NoError(t, err)
	require.NotNil(t, index)

	// Verify fixture categories are present
	require.Greater(t, len(index.Language), 0, "should have language fixtures")
	require.Greater(t, len(index.Format), 0, "should have format fixtures")
}

func TestParityFixtureLoader_LoadAllFixtures(t *testing.T) {
	t.Parallel()

	cfg := NewDefaultParityFixtureConfig(".")
	loader := NewParityFixtureLoader(cfg)

	fixtures, err := loader.LoadAllFixtures()
	require.NoError(t, err)
	require.NotNil(t, fixtures)

	// Verify we loaded multiple fixtures
	require.Greater(t, len(fixtures), 3, "should load multiple fixture files")

	// Verify content is not empty for loaded fixtures
	for name, content := range fixtures {
		require.Greater(t, len(content), 0, "fixture %s should have content", name)
	}

	// Verify specific fixtures were loaded
	require.Contains(t, fixtures, "go", "go fixture should be loaded")
	require.Contains(t, fixtures, "python", "python fixture should be loaded")
	require.Contains(t, fixtures, "json", "json fixture should be loaded")
}

func TestLoadFixtureFile_Existing(t *testing.T) {
	t.Parallel()

	cfg := NewDefaultParityFixtureConfig(".")
	content, err := LoadFixtureFile(cfg, "format_package.json")
	require.NoError(t, err)
	require.NotNil(t, content)
	require.Greater(t, len(content), 0)
	require.Contains(t, string(content), `"data_format"`)
}

func TestLoadFixtureFile_NonExisting(t *testing.T) {
	t.Parallel()

	cfg := NewDefaultParityFixtureConfig(".")
	_, err := LoadFixtureFile(cfg, "nonexistent_file.txt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read")
}

func TestGenerateFixtureIndex(t *testing.T) {
	t.Parallel()

	cfg := NewDefaultParityFixtureConfig(".")
	meta := FixtureMetadata{
		VoltCommitSHA:           strings.Repeat("a", 40),
		FixturesSHA256:          strings.Repeat("b", 64),
		ComparatorPath:          "../volt/tree/" + strings.Repeat("a", 40),
		Version:                 "1",
		GeneratedAt:             "2026-02-26T00:00:00Z",
		Profile:                 "parity",
		DeterministicMode:       true,
		EnhancementTiersEnabled: "none",
		TokenCounterMode:        "tokenizer_backed",
		FixedSeed:               1337,
	}

	index, err := GenerateFixtureIndex(cfg, meta)
	require.NoError(t, err)
	require.NotNil(t, index)

	// Verify metadata was set
	require.Equal(t, meta.VoltCommitSHA, index.Metadata.VoltCommitSHA)
	require.Equal(t, meta.FixturesSHA256, index.Metadata.FixturesSHA256)

	// Verify fixtures were discovered
	require.Contains(t, index.Language, "go")
	require.Contains(t, index.Language, "python")
	require.Contains(t, index.Format, "json")
	require.Contains(t, index.Format, "yaml")
	require.Contains(t, index.Format, "csv")
	require.Contains(t, index.Format, "latex")
	require.Contains(t, index.Format, "logs")
	require.Contains(t, index.Format, "sqlite_seed")
	require.Contains(t, index.Shell, "deploy_script")
	require.Contains(t, index.Markdown, "readme")
}

func TestFixtureFiles_ContentsAreValid(t *testing.T) {
	t.Parallel()

	cfg := NewDefaultParityFixtureConfig(".")
	loader := NewParityFixtureLoader(cfg)

	// Get the fixture index to know what files exist
	index, err := loader.LoadIndex()
	require.NoError(t, err)

	// Check language fixtures
	for name, path := range index.Language {
		content, err := LoadFixtureFile(cfg, path)
		require.NoErrorf(t, err, "language fixture %s should load", name)
		require.Greater(t, len(content), 0, "fixture %s should have content", name)
	}

	// Check format fixtures
	for name, path := range index.Format {
		content, err := LoadFixtureFile(cfg, path)
		require.NoErrorf(t, err, "format fixture %s should load", name)
		require.Greater(t, len(content), 0, "fixture %s should have content", name)
	}

	// Check shell fixtures
	for name, path := range index.Shell {
		content, err := LoadFixtureFile(cfg, path)
		require.NoErrorf(t, err, "shell fixture %s should load", name)
		require.Greater(t, len(content), 0, "fixture %s should have content", name)
	}

	// Check markdown fixtures
	for name, path := range index.Markdown {
		content, err := LoadFixtureFile(cfg, path)
		require.NoErrorf(t, err, "markdown fixture %s should load", name)
		require.Greater(t, len(content), 0, "fixture %s should have content", name)
	}
}

func TestFixtureFile_PathsAreValid(t *testing.T) {
	t.Parallel()

	cfg := NewDefaultParityFixtureConfig(".")
	loader := NewParityFixtureLoader(cfg)
	validator := &DefaultFixtureValidator{}

	index, err := loader.LoadIndex()
	require.NoError(t, err)

	// Check all fixture paths exist on disk
	for name, path := range index.Language {
		err := validator.ValidateFixtureExists(cfg, path)
		require.NoErrorf(t, err, "language fixture %s should exist", name)
	}

	for name, path := range index.Format {
		err := validator.ValidateFixtureExists(cfg, path)
		require.NoErrorf(t, err, "format fixture %s should exist", name)
	}

	for name, path := range index.Shell {
		err := validator.ValidateFixtureExists(cfg, path)
		require.NoErrorf(t, err, "shell fixture %s should exist", name)
	}

	for name, path := range index.Markdown {
		err := validator.ValidateFixtureExists(cfg, path)
		require.NoErrorf(t, err, "markdown fixture %s should exist", name)
	}
}

func TestNewDefaultParityFixtureConfig(t *testing.T) {
	t.Parallel()

	baseDir := "/tmp/test"
	cfg := NewDefaultParityFixtureConfig(baseDir)

	expectedIndexPath := filepath.Join(baseDir, DefaultParityFixturesPath)
	expectedFixturesDir := filepath.Join(baseDir, "testdata", "parity_volt", "fixtures")

	require.Equal(t, expectedIndexPath, cfg.IndexFilePath)
	require.Equal(t, expectedFixturesDir, cfg.FixturesDir)
}

func TestParityProvenanceBundle_Validate(t *testing.T) {
	t.Parallel()

	bundle := ParityProvenanceBundle{
		VoltCommitSHA:  strings.Repeat("e", 40),
		ComparatorPath: "../volt/tree/" + strings.Repeat("e", 40),
		FixturesSHA256: strings.Repeat("f", 64),
	}

	err := bundle.Validate(false)
	require.NoError(t, err)
}

func TestParityProvenanceBundle_ComparatorTupleValidation(t *testing.T) {
	t.Parallel()

	bundle := ParityProvenanceBundle{
		VoltCommitSHA:     strings.Repeat("e", 40),
		ComparatorPath:    "../volt/tree/" + strings.Repeat("e", 40),
		FixturesSHA256:    strings.Repeat("f", 64),
		GrepASTProvenance: "grep-ast@v1.2.3",
		TokenizerID:       "cl100k_base",
		TokenizerVersion:  "v0.1.0",
	}

	err := bundle.Validate(true)
	require.NoError(t, err)
}

func TestParityProvenanceBundle_MissingComparatorTuple(t *testing.T) {
	t.Parallel()

	bundle := ParityProvenanceBundle{
		VoltCommitSHA:  strings.Repeat("a", 40),
		ComparatorPath: "../volt/tree/" + strings.Repeat("a", 40),
		FixturesSHA256: strings.Repeat("b", 64),
	}

	err := bundle.Validate(true)
	require.Error(t, err)
}
