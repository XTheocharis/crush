package tools

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildAnchoredContent creates content with numLines lines where each line has
// 5 unique words. With interval=5, one anchor lands per line.
func buildAnchoredContent(numLines int) string {
	var b strings.Builder
	for i := 0; i < numLines; i++ {
		b.WriteString("word word word word unique")
		b.WriteString(strings.Repeat("_pad", i))
		b.WriteString("\n")
	}
	return b.String()
}

// findAnchorByLine returns the first anchor at the given 0-indexed line number.
func findAnchorByLine(am *AnchorMap, lineNum int) *HashAnchor {
	for i := range am.Anchors {
		if am.Anchors[i].LineNum == lineNum {
			return &am.Anchors[i]
		}
	}
	return nil
}

func TestInsertBefore_BasicPositioning(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(5)
	am := BuildAnchorMap(content, 5)
	require.NotEmpty(t, am.Anchors, "should generate anchors")

	anchor := findAnchorByLine(am, 2)
	require.NotNil(t, anchor, "should find anchor at line 2")

	result, err := InsertBefore(am, anchor.Hash, content, "INSERTED")
	require.NoError(t, err)

	lines := strings.Split(result, "\n")

	// Line 0 and 1 unchanged.
	require.Contains(t, lines[0], "unique")
	require.Contains(t, lines[1], "unique_pad")

	// "INSERTED" should be at line 2 (before the original line 2).
	require.Equal(t, "INSERTED", lines[2])

	// Original line 2 content shifted to line 3.
	require.Contains(t, lines[3], "unique_pad_pad")
}

func TestInsertBefore_MultiLineInsert(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(5)
	am := BuildAnchorMap(content, 5)

	anchor := findAnchorByLine(am, 0)
	require.NotNil(t, anchor)

	result, err := InsertBefore(am, anchor.Hash, content, "line_a\nline_b\nline_c")
	require.NoError(t, err)

	lines := strings.Split(result, "\n")
	require.Equal(t, "line_a", lines[0])
	require.Equal(t, "line_b", lines[1])
	require.Equal(t, "line_c", lines[2])

	// Original line 0 shifted to line 3.
	require.Contains(t, lines[3], "unique")
}

func TestInsertBefore_AnchorNotFound(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(5)
	am := BuildAnchorMap(content, 5)

	_, err := InsertBefore(am, 99999, content, "x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "insert_before:")
}

func TestInsertAfter_BasicPositioning(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(5)
	am := BuildAnchorMap(content, 5)
	require.NotEmpty(t, am.Anchors)

	anchor := findAnchorByLine(am, 1)
	require.NotNil(t, anchor, "should find anchor at line 1")

	result, err := InsertAfter(am, anchor.Hash, content, "AFTER_LINE1")
	require.NoError(t, err)

	lines := strings.Split(result, "\n")

	// Original line 1 still at position 1.
	require.Contains(t, lines[1], "unique_pad")

	// "AFTER_LINE1" inserted at line 2 (after original line 1).
	require.Equal(t, "AFTER_LINE1", lines[2])

	// Original line 2 shifted to line 3.
	require.Contains(t, lines[3], "unique_pad_pad")
}

func TestInsertAfter_MultiLineInsert(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(3)
	am := BuildAnchorMap(content, 5)

	anchor := findAnchorByLine(am, 1)
	require.NotNil(t, anchor)

	result, err := InsertAfter(am, anchor.Hash, content, "aa\nbb")
	require.NoError(t, err)

	lines := strings.Split(result, "\n")
	require.Contains(t, lines[1], "unique_pad") // original line 1
	require.Equal(t, "aa", lines[2])
	require.Equal(t, "bb", lines[3])
	require.Contains(t, lines[4], "unique_pad_pad") // original line 2
}

func TestInsertAfter_AnchorNotFound(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(3)
	am := BuildAnchorMap(content, 5)

	_, err := InsertAfter(am, 99999, content, "x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "insert_after:")
}

func TestInsertAfter_LastLine(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(3)
	am := BuildAnchorMap(content, 5)

	anchor := findAnchorByLine(am, 2)
	require.NotNil(t, anchor)

	result, err := InsertAfter(am, anchor.Hash, content, "APPENDED")
	require.NoError(t, err)

	lines := strings.Split(result, "\n")
	// Original last line at index 2, "APPENDED" at index 3.
	require.Contains(t, lines[2], "unique_pad_pad")
	require.Equal(t, "APPENDED", lines[3])
}

func TestReplaceRange_SingleLine(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(5)
	am := BuildAnchorMap(content, 5)

	anchor := findAnchorByLine(am, 2)
	require.NotNil(t, anchor)

	// Same start and end anchor → single line replacement.
	result, err := ReplaceRange(am, anchor.Hash, anchor.Hash, content, "REPLACED_SINGLE")
	require.NoError(t, err)

	lines := strings.Split(result, "\n")
	require.Equal(t, "REPLACED_SINGLE", lines[2])

	// Other lines unchanged.
	require.Contains(t, lines[0], "unique")
	require.Contains(t, lines[1], "unique_pad")
	require.Contains(t, lines[3], "unique_pad_pad_pad")
}

func TestReplaceRange_MultiLine(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(6)
	am := BuildAnchorMap(content, 5)

	startAnchor := findAnchorByLine(am, 1)
	endAnchor := findAnchorByLine(am, 3)
	require.NotNil(t, startAnchor)
	require.NotNil(t, endAnchor)

	result, err := ReplaceRange(am, startAnchor.Hash, endAnchor.Hash, content, "NEW_BLOCK")
	require.NoError(t, err)

	lines := strings.Split(result, "\n")

	// Line 0 unchanged.
	require.Contains(t, lines[0], "unique")
	// Lines 1-3 replaced.
	require.Equal(t, "NEW_BLOCK", lines[1])
	// Line 4 still present.
	require.Contains(t, lines[2], "unique_pad_pad_pad_pad")
}

func TestReplaceRange_ReversedHashes(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(5)
	am := BuildAnchorMap(content, 5)

	startAnchor := findAnchorByLine(am, 0)
	endAnchor := findAnchorByLine(am, 2)
	require.NotNil(t, startAnchor)
	require.NotNil(t, endAnchor)

	// Pass end hash as start and start hash as end.
	result, err := ReplaceRange(am, endAnchor.Hash, startAnchor.Hash, content, "REVERSED_OK")
	require.NoError(t, err)

	lines := strings.Split(result, "\n")
	require.Equal(t, "REVERSED_OK", lines[0])
	require.Contains(t, lines[1], "unique_pad_pad_pad")
}

func TestReplaceRange_StartNotFound(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(3)
	am := BuildAnchorMap(content, 5)

	anchor := findAnchorByLine(am, 0)
	require.NotNil(t, anchor)

	_, err := ReplaceRange(am, 99999, anchor.Hash, content, "x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "replace_range start")
}

func TestReplaceRange_EndNotFound(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(3)
	am := BuildAnchorMap(content, 5)

	anchor := findAnchorByLine(am, 0)
	require.NotNil(t, anchor)

	_, err := ReplaceRange(am, anchor.Hash, 99999, content, "x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "replace_range end")
}

func TestDeleteRange_BasicDeletion(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(5)
	am := BuildAnchorMap(content, 5)

	startAnchor := findAnchorByLine(am, 1)
	endAnchor := findAnchorByLine(am, 3)
	require.NotNil(t, startAnchor)
	require.NotNil(t, endAnchor)

	result, err := DeleteRange(am, startAnchor.Hash, endAnchor.Hash, content)
	require.NoError(t, err)

	lines := strings.Split(result, "\n")

	// Line 0 unchanged.
	require.Contains(t, lines[0], "unique")
	// Lines 1-3 deleted; line 4 is now at index 1.
	require.Contains(t, lines[1], "unique_pad_pad_pad_pad")
}

func TestDeleteRange_SingleLine(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(5)
	am := BuildAnchorMap(content, 5)

	anchor := findAnchorByLine(am, 2)
	require.NotNil(t, anchor)

	result, err := DeleteRange(am, anchor.Hash, anchor.Hash, content)
	require.NoError(t, err)

	lines := strings.Split(result, "\n")
	require.Len(t, lines, 5) // 4 content lines + 1 trailing empty from \n

	// Line 2 deleted; line 3 moved up.
	require.Contains(t, lines[2], "unique_pad_pad_pad")
}

func TestDeleteRange_ReversedHashes(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(5)
	am := BuildAnchorMap(content, 5)

	startAnchor := findAnchorByLine(am, 0)
	endAnchor := findAnchorByLine(am, 1)
	require.NotNil(t, startAnchor)
	require.NotNil(t, endAnchor)

	// Pass in reversed order.
	result, err := DeleteRange(am, endAnchor.Hash, startAnchor.Hash, content)
	require.NoError(t, err)

	lines := strings.Split(result, "\n")
	// Lines 0-1 deleted; line 2 is now first.
	require.Contains(t, lines[0], "unique_pad_pad")
}

func TestDeleteRange_StartNotFound(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(3)
	am := BuildAnchorMap(content, 5)

	_, err := DeleteRange(am, 99999, 99998, content)
	require.Error(t, err)
	require.Contains(t, err.Error(), "delete_range start")
}

func TestResolveHash_ValidHash(t *testing.T) {
	t.Parallel()

	content := "alpha\nbeta\ngamma\ndelta\n"
	am := BuildAnchorMap(content, 1)
	require.NotEmpty(t, am.Anchors)

	// Each anchor should resolve to its line.
	for _, a := range am.Anchors {
		lineNum, err := resolveHash(am, a.Hash, content)
		require.NoError(t, err)
		require.Equal(t, a.LineNum, lineNum, "anchor should resolve to its original line")
	}
}

func TestResolveHash_InvalidHash(t *testing.T) {
	t.Parallel()

	content := "alpha\nbeta\n"
	am := BuildAnchorMap(content, 1)

	lineNum, err := resolveHash(am, 99999, content)
	require.Error(t, err)
	require.Equal(t, -1, lineNum)
}

func TestInsertBefore_AtFirstLine(t *testing.T) {
	t.Parallel()

	content := buildAnchoredContent(3)
	am := BuildAnchorMap(content, 5)

	anchor := findAnchorByLine(am, 0)
	require.NotNil(t, anchor)

	result, err := InsertBefore(am, anchor.Hash, content, "HEADER")
	require.NoError(t, err)

	lines := strings.Split(result, "\n")
	require.Equal(t, "HEADER", lines[0])
	require.Contains(t, lines[1], "unique") // original line 0 shifted to 1
}
