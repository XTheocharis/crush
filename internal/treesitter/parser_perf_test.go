package treesitter

import (
	"fmt"
	"hash/fnv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTreeCacheKeyStability verifies treeCacheKey produces stable results.
func TestTreeCacheKeyStability(t *testing.T) {
	t.Parallel()

	path := "/path/to/file.go"
	content := []byte("hello world")

	// Run the function multiple times and ensure it produces identical results
	var results []string
	for i := 0; i < 5; i++ {
		results = append(results, treeCacheKey(path, content))
	}

	for i := 1; i < len(results); i++ {
		require.Equal(t, results[0], results[i], "treeCacheKey must be stable across calls")
	}
}

// TestTreeCacheKeyFormat verifies treeCacheKey matches expected format.
func TestTreeCacheKeyFormat(t *testing.T) {
	t.Parallel()

	path := "/path/to/file.go"
	content := []byte("hello world")

	key := treeCacheKey(path, content)

	// Verify format: path:length:hash
	// The hash is FNV-64a encoded as hex (16 lowercase hex chars)
	// The length is a decimal integer
	expectedFmt := fmt.Sprintf("%s:%d:%x", path, len(content), fnvHash(content))
	require.Equal(t, expectedFmt, key, "treeCacheKey must match expected format")

	// Verify key contains components
	require.Contains(t, key, path, "key must contain path")
	require.Contains(t, key, fmt.Sprintf("%d", len(content)), "key must contain content length")
	require.Len(t, key, len(path)+1+len(fmt.Sprintf("%d", len(content)))+1+16, "key must have expected length")
}

// TestTreeCacheKeyChangesWithContent verifies cache key changes when content changes.
func TestTreeCacheKeyChangesWithContent(t *testing.T) {
	t.Parallel()

	path := "/path/to/file.go"
	content1 := []byte("hello world")
	content2 := []byte("hello mars")

	key1 := treeCacheKey(path, content1)
	key2 := treeCacheKey(path, content2)

	require.NotEqual(t, key1, key2, "cache key must change when content changes")
}

// TestTreeCacheKeyChangesWithPath verifies cache key changes when path changes.
func TestTreeCacheKeyChangesWithPath(t *testing.T) {
	t.Parallel()

	content := []byte("hello world")
	path1 := "/path/to/file.go"
	path2 := "/other/path/file.go"

	key1 := treeCacheKey(path1, content)
	key2 := treeCacheKey(path2, content)

	require.NotEqual(t, key1, key2, "cache key must change when path changes")
}

// TestTreeCacheKeyHashIsLowercaseHex verifies hash portion is lowercase hex.
func TestTreeCacheKeyHashIsLowercaseHex(t *testing.T) {
	t.Parallel()

	path := "/path/to/file.go"
	content := []byte("hello world")
	key := treeCacheKey(path, content)

	// Extract hash portion (last 16 characters, all hex)
	hashPart := key[len(key)-16:]
	require.Len(t, hashPart, 16, "hash portion must be 16 characters")

	// Verify all lowercase hex digits
	for _, c := range hashPart {
		validHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		require.True(t, validHex, "hash must be lowercase hex, invalid char: %c", c)
	}
}

// TestLoadSupportedLanguagesCaching verifies loadSupportedLanguages is cached.
func TestLoadSupportedLanguagesCaching(t *testing.T) {
	t.Parallel()

	// First call should load the manifest
	langs1, set1 := loadSupportedLanguages()
	require.NotEmpty(t, langs1)
	require.NotEmpty(t, set1)

	// Second call should return cached data (copies to prevent mutation)
	langs2, set2 := loadSupportedLanguages()
	require.Equal(t, langs1, langs2, "languages should be equal")
	require.Equal(t, set1, set2, "language sets should be equal")

	// Test that mutation of returned values doesn't affect cache
	langs2[0] = "fake-language-xyz"
	langs3, _ := loadSupportedLanguages()
	require.NotEqual(t, langs2, langs3, "mutation of returned languages should not affect cache")
	require.Equal(t, langs1, langs3, "cached languages should remain unchanged")
}

// fnvHash computes FNV-64a hash for testing purposes.
func fnvHash(data []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(data)
	return h.Sum64()
}
