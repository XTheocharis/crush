package repomap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	tiktoken "github.com/pkoukk/tiktoken-go"
	"github.com/stretchr/testify/require"
)

// testCacheDir is the shared temp directory for tiktoken test state.
// Set once in TestMain.
var testCacheDir string

// testSupportJSON is the embedded tokenizer_support.v1.json fixture.
var testSupportJSON []byte

func TestMain(m *testing.M) {
	// Read support JSON fixture.
	data, err := os.ReadFile("testdata/parity_aider/tokenizer_support.v1.json")
	if err != nil {
		panic("failed to read test fixture: " + err.Error())
	}
	testSupportJSON = data

	// Create a shared cache dir for tiktoken BPE data.
	dir, err := os.MkdirTemp("", "crush-tiktoken-test-*")
	if err != nil {
		panic("failed to create temp dir: " + err.Error())
	}
	testCacheDir = dir

	// Register the BPE loader once before any tests run.
	// This avoids data races on the tiktoken global state.
	tiktoken.SetBpeLoader(newCrushBpeLoader(testCacheDir))

	code := m.Run()
	os.RemoveAll(testCacheDir)
	os.Exit(code)
}

// ---------------------------------------------------------------------------
// TiktokenCounter tests
// ---------------------------------------------------------------------------

func TestNewTiktokenCounter_CL100kBase_Succeeds(t *testing.T) {
	t.Parallel()

	counter, err := NewTiktokenCounter(encodingCL100kBase)
	require.NoError(t, err)
	require.NotNil(t, counter)
}

func TestTiktokenCounter_Count_PositiveForKnownText(t *testing.T) {
	t.Parallel()

	counter, err := NewTiktokenCounter(encodingCL100kBase)
	require.NoError(t, err)

	n, err := counter.Count(context.Background(), "gpt-4", "Hello, world! This is a test.")
	require.NoError(t, err)
	require.Greater(t, n, 0)
}

func TestTiktokenCounter_Count_ZeroForEmptyString(t *testing.T) {
	t.Parallel()

	counter, err := NewTiktokenCounter(encodingCL100kBase)
	require.NoError(t, err)

	n, err := counter.Count(context.Background(), "gpt-4", "")
	require.NoError(t, err)
	require.Equal(t, 0, n)
}

func TestTiktokenCounter_ImplementsTokenCounter(t *testing.T) {
	t.Parallel()

	// Compile-time interface check.
	var _ TokenCounter = (*TiktokenCounter)(nil)
}

// ---------------------------------------------------------------------------
// DefaultTokenCounterProvider tests
// ---------------------------------------------------------------------------

func TestDefaultTokenCounterProvider_ImplementsInterface(t *testing.T) {
	t.Parallel()

	// Compile-time interface check.
	var _ TokenCounterProvider = (*DefaultTokenCounterProvider)(nil)
}

func TestDefaultTokenCounterProvider_CounterForModel_GPT4(t *testing.T) {
	t.Parallel()

	provider, err := NewDefaultTokenCounterProvider(testSupportJSON)
	require.NoError(t, err)

	counter, ok := provider.CounterForModel("gpt-4")
	require.True(t, ok)
	require.NotNil(t, counter)

	// Verify it actually counts tokens.
	n, err := counter.Count(context.Background(), "gpt-4", "Hello, world!")
	require.NoError(t, err)
	require.Greater(t, n, 0)
}

func TestDefaultTokenCounterProvider_CounterForModel_GPT4o_FallsBackToCL100k(t *testing.T) {
	t.Parallel()

	provider, err := NewDefaultTokenCounterProvider(testSupportJSON)
	require.NoError(t, err)

	// When o200k_base fails to load (no cache, no network), CounterForModel
	// should fall back to cl100k_base and still return a valid counter.
	counter, ok := provider.CounterForModel("gpt-4o")
	require.True(t, ok)
	require.NotNil(t, counter)

	n, err := counter.Count(context.Background(), "gpt-4o", "Hello!")
	require.NoError(t, err)
	require.Greater(t, n, 0)
}

func TestDefaultTokenCounterProvider_CounterForModel_Claude(t *testing.T) {
	t.Parallel()

	provider, err := NewDefaultTokenCounterProvider(testSupportJSON)
	require.NoError(t, err)

	// Claude uses cl100k_base approximation.
	counter, ok := provider.CounterForModel("claude-3-opus-20240229")
	require.True(t, ok)
	require.NotNil(t, counter)

	n, err := counter.Count(context.Background(), "claude-3-opus-20240229", "Test input")
	require.NoError(t, err)
	require.Greater(t, n, 0)
}

func TestDefaultTokenCounterProvider_MetadataForModel_Claude(t *testing.T) {
	t.Parallel()

	provider, err := NewDefaultTokenCounterProvider(testSupportJSON)
	require.NoError(t, err)

	meta, ok := provider.MetadataForModel("claude-3-opus-20240229")
	require.True(t, ok)
	require.True(t, meta.Supported)
	require.Equal(t, encodingCL100kBase, meta.TokenizerID)
	require.NotEmpty(t, meta.TokenizerVersion)
}

func TestDefaultTokenCounterProvider_MetadataForModel_GPT4(t *testing.T) {
	t.Parallel()

	provider, err := NewDefaultTokenCounterProvider(testSupportJSON)
	require.NoError(t, err)

	meta, ok := provider.MetadataForModel("gpt-4")
	require.True(t, ok)
	require.True(t, meta.Supported)
	require.Equal(t, encodingCL100kBase, meta.TokenizerID)
}

func TestDefaultTokenCounterProvider_UnknownModel(t *testing.T) {
	t.Parallel()

	provider, err := NewDefaultTokenCounterProvider(testSupportJSON)
	require.NoError(t, err)

	counter, ok := provider.CounterForModel("totally-unknown-model-xyz")
	require.False(t, ok)
	require.Nil(t, counter)

	_, ok = provider.MetadataForModel("totally-unknown-model-xyz")
	require.False(t, ok)
}

func TestDefaultTokenCounterProvider_PrefixMatch(t *testing.T) {
	t.Parallel()

	provider, err := NewDefaultTokenCounterProvider(testSupportJSON)
	require.NoError(t, err)

	// "gpt-4-turbo-preview" is an exact model string in the fixture.
	// "gpt-4-turbo-preview-0125" is not exact, but "gpt-4-turbo-preview"
	// is the longest prefix match.
	counter, ok := provider.CounterForModel("gpt-4-turbo-preview-0125")
	require.True(t, ok)
	require.NotNil(t, counter)

	meta, ok := provider.MetadataForModel("gpt-4-turbo-preview-0125")
	require.True(t, ok)
	require.True(t, meta.Supported)
}

func TestDefaultTokenCounterProvider_CounterCaching(t *testing.T) {
	t.Parallel()

	provider, err := NewDefaultTokenCounterProvider(testSupportJSON)
	require.NoError(t, err)

	// Get counter twice for models using the same encoding.
	c1, ok1 := provider.CounterForModel("gpt-4")
	require.True(t, ok1)

	c2, ok2 := provider.CounterForModel("gpt-3.5-turbo")
	require.True(t, ok2)

	// Both use cl100k_base; the counter is cached by encoding name,
	// so they should be the same pointer.
	require.Same(t, c1, c2)
}

// ---------------------------------------------------------------------------
// O200k cache tests
// ---------------------------------------------------------------------------

func TestO200kBase_CachePersistence(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()

	loader := newCrushBpeLoader(cacheDir)
	cachePath := loader.o200kCachePath()

	// Initially no cache file.
	_, err := os.Stat(cachePath)
	require.True(t, os.IsNotExist(err))

	// Write a minimal valid BPE cache file to simulate a prior download.
	require.NoError(t, os.MkdirAll(filepath.Dir(cachePath), 0o755))
	// Minimal BPE: base64("a") = "YQ==" rank 0.
	require.NoError(t, os.WriteFile(cachePath, []byte("YQ== 0\n"), 0o644))

	// Loading should succeed from cache without network.
	ranks, err := loader.loadO200kBase()
	require.NoError(t, err)
	require.Contains(t, ranks, "a")

	// File should still be there.
	_, err = os.Stat(cachePath)
	require.NoError(t, err)
}

func TestO200kBase_FallbackOnDownloadFailure(t *testing.T) {
	t.Parallel()

	provider, err := NewDefaultTokenCounterProvider(testSupportJSON)
	require.NoError(t, err)

	// The o200k_base download will fail (no cache file in shared test dir,
	// no real network in CI). CounterForModel should fall back to cl100k_base.
	counter, ok := provider.CounterForModel("gpt-4o")
	require.True(t, ok)
	require.NotNil(t, counter)

	// Should still produce valid token counts.
	n, err := counter.Count(context.Background(), "gpt-4o", "Fallback test")
	require.NoError(t, err)
	require.Greater(t, n, 0)
}

// ---------------------------------------------------------------------------
// Helper tests
// ---------------------------------------------------------------------------

func TestParseBpeRanks(t *testing.T) {
	t.Parallel()

	// "YQ==" is base64 for "a", "Yg==" is base64 for "b".
	input := []byte("YQ== 0\nYg== 1\n")
	ranks, err := parseBpeRanks(input)
	require.NoError(t, err)
	require.Equal(t, 0, ranks["a"])
	require.Equal(t, 1, ranks["b"])
	require.Len(t, ranks, 2)
}

func TestResolveEncodingName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tokenizerID string
		want        string
	}{
		{encodingCL100kBase, encodingCL100kBase},
		{encodingO200kBase, encodingO200kBase},
		{"claude", encodingCL100kBase},
		{"gemini", encodingCL100kBase},
		{"unknown", encodingCL100kBase},
	}
	for _, tc := range tests {
		got := resolveEncodingName(familyEntry{TokenizerID: tc.tokenizerID})
		require.Equal(t, tc.want, got, "tokenizer_id=%s", tc.tokenizerID)
	}
}

func TestTiktokenCacheDir(t *testing.T) {
	// Not parallel: uses t.Setenv which modifies process environment.

	t.Run("respects_XDG_CACHE_HOME", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "/tmp/test-xdg")
		dir := TiktokenCacheDir()
		require.Equal(t, "/tmp/test-xdg/crush/tiktoken", dir)
	})

	t.Run("fallback_to_home_cache", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "")
		dir := TiktokenCacheDir()
		require.Contains(t, dir, "crush/tiktoken")
	})
}
