package tools

import (
	"os"
	"path/filepath"
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
