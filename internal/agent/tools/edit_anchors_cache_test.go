package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStoreAndLoadAnchorMap(t *testing.T) {
	t.Parallel()

	content := "word word word word unique0\nword word word word unique1\n"
	am := BuildAnchorMap(content, 5)
	require.NotEmpty(t, am.Anchors)

	fp := filepath.Join(t.TempDir(), "cache_test.go")
	storeAnchorMap(fp, am)

	loaded := loadAnchorMap(fp)
	require.NotNil(t, loaded)
	require.Equal(t, am.Anchors, loaded.Anchors)
}

func TestLoadAnchorMap_NotCached(t *testing.T) {
	t.Parallel()

	loaded := loadAnchorMap("/nonexistent/path.go")
	require.Nil(t, loaded)
}

func TestStoreAnchorMap_NilValue(t *testing.T) {
	t.Parallel()

	fp := filepath.Join(t.TempDir(), "nil_test.go")
	storeAnchorMap(fp, nil)

	loaded := loadAnchorMap(fp)
	require.Nil(t, loaded)
}

func TestDeleteAnchorMap(t *testing.T) {
	t.Parallel()

	content := "word word word word unique0\n"
	am := BuildAnchorMap(content, 5)
	require.NotEmpty(t, am.Anchors)

	fp := filepath.Join(t.TempDir(), "delete_test.go")
	storeAnchorMap(fp, am)
	require.NotNil(t, loadAnchorMap(fp))

	deleteAnchorMap(fp)
	require.Nil(t, loadAnchorMap(fp))
}

func TestDeleteAnchorMap_NotCached_NoPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		deleteAnchorMap("/never/cached/path.go")
	})
}

func TestExtractAnchorHashes_Single(t *testing.T) {
	t.Parallel()

	input := "some code // <hash:a1b2c3d4>"
	hashes := extractAnchorHashes(input)
	require.Len(t, hashes, 1)
	require.Equal(t, uint64(0xa1b2c3d4), hashes[0])
}

func TestExtractAnchorHashes_Multiple(t *testing.T) {
	t.Parallel()

	input := "// <hash:00000001> and <hash:00000002>"
	hashes := extractAnchorHashes(input)
	require.Len(t, hashes, 2)
	require.Equal(t, uint64(0x1), hashes[0])
	require.Equal(t, uint64(0x2), hashes[1])
}

func TestExtractAnchorHashes_Duplicates(t *testing.T) {
	t.Parallel()

	input := "<hash:abcdef01> also <hash:abcdef01>"
	hashes := extractAnchorHashes(input)
	require.Len(t, hashes, 1, "duplicate hashes should be deduplicated")
}

func TestExtractAnchorHashes_NoHashes(t *testing.T) {
	t.Parallel()

	hashes := extractAnchorHashes("no hashes here")
	require.Nil(t, hashes)
}

func TestExtractAnchorHashes_InvalidHex(t *testing.T) {
	t.Parallel()

	input := "<hash:ZZZZZZZZ>"
	hashes := extractAnchorHashes(input)
	require.Nil(t, hashes, "invalid hex should produce no hashes")
}

func TestExtractAnchorHashes_ShortHash(t *testing.T) {
	t.Parallel()

	input := "<hash:ff>"
	hashes := extractAnchorHashes(input)
	require.Empty(t, hashes, "regex requires 8+ hex chars, short hash does not match")
}

func TestExtractAnchorHashes_LongHash(t *testing.T) {
	t.Parallel()

	input := "<hash:deadbeef12345678>"
	hashes := extractAnchorHashes(input)
	require.Len(t, hashes, 1)
	require.Equal(t, uint64(0xdeadbeef12345678), hashes[0])
}

func TestStripAnchorMarkers_Single(t *testing.T) {
	t.Parallel()

	input := "code // <hash:a1b2c3d4>"
	result := stripAnchorMarkers(input)
	require.Equal(t, "code", result)
}

func TestStripAnchorMarkers_Multiple(t *testing.T) {
	t.Parallel()

	input := "line1 // <hash:00000001>\nline2 // <hash:00000002>"
	result := stripAnchorMarkers(input)
	require.Equal(t, "line1\nline2", result)
}

func TestStripAnchorMarkers_NoMarkers(t *testing.T) {
	t.Parallel()

	input := "clean code without markers"
	result := stripAnchorMarkers(input)
	require.Equal(t, input, result)
}

func TestStripAnchorMarkers_WithLeadingWhitespace(t *testing.T) {
	t.Parallel()

	input := "  // <hash:cafebabe>"
	result := stripAnchorMarkers(input)
	require.Equal(t, "", result)
}

func TestReconcileAnchorMap_NewContent(t *testing.T) {
	t.Parallel()

	fp := filepath.Join(t.TempDir(), "reconcile.go")
	content := generateMultiWordContent(60)
	am := BuildAnchorMap(content, 0)
	storeAnchorMap(fp, am)
	require.NotNil(t, loadAnchorMap(fp))

	modified := strings.ReplaceAll(content, "word0", "EDITED")
	reconcileAnchorMap(fp, modified)

	updated := loadAnchorMap(fp)
	require.NotNil(t, updated)
	require.NotEmpty(t, updated.Anchors)
}

func TestReconcileAnchorMap_EmptyContentDeletes(t *testing.T) {
	t.Parallel()

	fp := filepath.Join(t.TempDir(), "reconcile_empty.go")
	content := "word word word word unique0\n"
	am := BuildAnchorMap(content, 5)
	storeAnchorMap(fp, am)
	require.NotNil(t, loadAnchorMap(fp))

	reconcileAnchorMap(fp, "")
	require.Nil(t, loadAnchorMap(fp))
}

func TestReconcileAnchorMap_FileRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fp := filepath.Join(dir, "roundtrip.go")

	original := generateMultiWordContent(60)
	require.NoError(t, os.WriteFile(fp, []byte(original), 0o644))

	am := BuildAnchorMap(original, 0)
	storeAnchorMap(fp, am)
	require.NotEmpty(t, am.Anchors)

	modified := strings.ReplaceAll(original, "word0", "EDITED")
	reconcileAnchorMap(fp, modified)

	updated := loadAnchorMap(fp)
	require.NotNil(t, updated)
	require.NotEmpty(t, updated.Anchors)
}
