package tools

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTryAnchorReplaceWithAnchors(t *testing.T) {
	t.Parallel()

	content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}"

	am := BuildAnchorMap(content, 1)
	require.NotEmpty(t, am.Anchors)

	filePath := filepath.Join(t.TempDir(), "test.go")
	err := os.WriteFile(filePath, []byte(content), 0o644)
	require.NoError(t, err)

	storeAnchorMap(filePath, am)
	defer deleteAnchorMap(filePath)

	anchor := am.Anchors[0]
	oldString := "package main // " + anchor.FormatAnchor()
	newString := "package main // edited"

	result, ok := tryAnchorReplace(content, oldString, newString, filePath, false)
	require.True(t, ok)
	require.Contains(t, result, "// edited")
	require.NotContains(t, result, "<hash:")
}

func TestTryAnchorReplaceFallsBackWhenNoAnchors(t *testing.T) {
	t.Parallel()

	content := "hello world\n"
	oldString := "hello"
	newString := "goodbye"

	result, ok := tryAnchorReplace(content, oldString, newString, "fake.go", false)
	require.False(t, ok)
	require.Empty(t, result)
}

func TestTryAnchorReplaceFallsBackWhenNoHashMap(t *testing.T) {
	t.Parallel()

	content := "hello world\n"
	oldString := "hello // <hash:deadbeef>"
	newString := "goodbye // <hash:deadbeef>"

	result, ok := tryAnchorReplace(content, oldString, newString, filepath.Join(t.TempDir(), "missing.go"), false)
	require.False(t, ok)
	require.Empty(t, result)
}

func TestTryAnchorReplaceReplaceAll(t *testing.T) {
	t.Parallel()

	content := "aaa\nbbb\naaa\n"
	am := BuildAnchorMap(content, 1)
	require.NotEmpty(t, am.Anchors)

	filePath := filepath.Join(t.TempDir(), "test.txt")
	err := os.WriteFile(filePath, []byte(content), 0o644)
	require.NoError(t, err)

	storeAnchorMap(filePath, am)
	defer deleteAnchorMap(filePath)

	anchor := am.Anchors[0]
	oldString := "aaa // " + anchor.FormatAnchor()
	newString := "zzz"

	result, ok := tryAnchorReplace(content, oldString, newString, filePath, true)
	require.True(t, ok)
	require.Equal(t, "zzz\nbbb\nzzz\n", result)
}

func TestTryAnchorReplaceMultipleMatchesPicksClosest(t *testing.T) {
	t.Parallel()

	content := "line0 target\nline1 other\nline2 target\nline3 end"

	am := BuildAnchorMap(content, 1)
	require.NotEmpty(t, am.Anchors)

	filePath := filepath.Join(t.TempDir(), "test.txt")
	err := os.WriteFile(filePath, []byte(content), 0o644)
	require.NoError(t, err)

	storeAnchorMap(filePath, am)
	defer deleteAnchorMap(filePath)

	var anchorNearLine0 *HashAnchor
	for i := range am.Anchors {
		if am.Anchors[i].LineNum == 0 {
			anchorNearLine0 = &am.Anchors[i]
			break
		}
	}
	require.NotNil(t, anchorNearLine0)

	oldString := "target // " + anchorNearLine0.FormatAnchor()
	newString := "REPLACED"

	result, ok := tryAnchorReplace(content, oldString, newString, filePath, false)
	require.True(t, ok)
	require.Contains(t, result, "line0 REPLACED")
	require.Contains(t, result, "line2 target")
}

// generateMultiWordContent produces n words across multiple lines, each line
// containing 5 words, to ensure BuildAnchorMap with default interval produces
// anchors when n >= 50.
func generateMultiWordContent(n int) string {
	var lines []string
	wordsPerLine := 5
	for i := 0; i < n; i += wordsPerLine {
		end := i + wordsPerLine
		if end > n {
			end = n
		}
		var words []string
		for j := i; j < end; j++ {
			words = append(words, "word"+strconv.Itoa(j))
		}
		lines = append(lines, "line "+strings.Join(words, " "))
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestReconcileAnchorMap(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "test.go")
	content := generateMultiWordContent(60)
	am := BuildAnchorMap(content, 0)
	storeAnchorMap(filePath, am)
	require.NotEmpty(t, loadAnchorMap(filePath).Anchors)

	modified := strings.ReplaceAll(content, "word0", "EDITED")
	reconcileAnchorMap(filePath, modified)

	updated := loadAnchorMap(filePath)
	require.NotNil(t, updated)
	require.NotEmpty(t, updated.Anchors)

	for _, a := range updated.Anchors {
		require.Contains(t, modified, a.Content)
	}
}

func TestReconcileAnchorMapEmptyDeletes(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "test.go")
	content := "package main\n"
	am := BuildAnchorMap(content, 1)
	storeAnchorMap(filePath, am)
	require.NotNil(t, loadAnchorMap(filePath))

	reconcileAnchorMap(filePath, "")
	require.Nil(t, loadAnchorMap(filePath))
}

func TestInjectAnchorMarkers(t *testing.T) {
	t.Parallel()

	content := "line0 word0 word1\nline2 word2 word3\nline4 word4 word5\n"
	am := BuildAnchorMap(content, 1)

	numbered := addLineNumbers(content, 1)
	result := injectAnchorMarkers(numbered, 1, am)

	// injectAnchorMarkers keeps the last anchor per display line.
	injected := map[int]string{}
	for _, a := range am.Anchors {
		injected[a.LineNum+1] = a.FormatAnchor()
	}
	for displayLine, marker := range injected {
		require.Contains(t, result, "// "+marker, "marker for line %d should appear", displayLine)
	}
}

func TestInjectAnchorMarkersWithOffset(t *testing.T) {
	t.Parallel()

	content := "line0\nline1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\n"
	am := BuildAnchorMap(content, 1)
	if len(am.Anchors) == 0 {
		t.Skip("no anchors generated for this content")
	}

	offset := 5
	sliced := content
	numbered := addLineNumbers(sliced, offset+1)
	result := injectAnchorMarkers(numbered, offset+1, am)

	for _, a := range am.Anchors {
		displayLine := a.LineNum + 1
		marker := "// " + a.FormatAnchor()
		if displayLine >= offset+1 {
			require.Contains(t, result, marker, "marker for display line %d should appear with offset %d", displayLine, offset)
		}
	}
}

func TestInjectAnchorMarkersNilMap(t *testing.T) {
	t.Parallel()

	numbered := "  1|hello\n  2|world\n"
	result := injectAnchorMarkers(numbered, 1, nil)
	require.Equal(t, numbered, result)
}

func TestAnchorRoundTrip(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "roundtrip.go")
	content := generateMultiWordContent(60)

	am := BuildAnchorMap(content, 0)
	storeAnchorMap(filePath, am)
	numbered := addLineNumbers(content, 1)
	numbered = injectAnchorMarkers(numbered, 1, am)

	require.NotEmpty(t, am.Anchors, "should generate anchors for long content")

	anchor := am.Anchors[0]
	oldString := strings.Split(content, "\n")[anchor.LineNum] + " // " + anchor.FormatAnchor()
	newString := "EDITED_LINE"

	result, ok := tryAnchorReplace(content, oldString, newString, filePath, false)
	require.True(t, ok)
	require.Contains(t, result, "EDITED_LINE")

	reconcileAnchorMap(filePath, result)

	updated := loadAnchorMap(filePath)
	require.NotNil(t, updated)
	require.NotEmpty(t, updated.Anchors)

	// Use the first anchor from the reconciled map for a second edit.
	secondAnchor := updated.Anchors[0]
	secondLine := strings.Split(result, "\n")[secondAnchor.LineNum]
	secondOld := secondLine + " // " + secondAnchor.FormatAnchor()
	secondNew := "FINAL_LINE"

	result2, ok2 := tryAnchorReplace(result, secondOld, secondNew, filePath, false)
	require.True(t, ok2, "second edit should work against reconciled cache")
	require.Contains(t, result2, "FINAL_LINE")
}
