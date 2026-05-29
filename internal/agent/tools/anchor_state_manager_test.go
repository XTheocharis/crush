//go:build treesitter

package tools

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMyersDiffHashes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		old  []uint64
		new  []uint64
		want []DiffOp
	}{
		{
			name: "identical sequences",
			old:  []uint64{1, 2, 3},
			new:  []uint64{1, 2, 3},
			want: []DiffOp{DiffKeep, DiffKeep, DiffKeep},
		},
		{
			name: "insertion at start",
			old:  []uint64{2, 3},
			new:  []uint64{1, 2, 3},
			want: []DiffOp{DiffInsert, DiffKeep, DiffKeep},
		},
		{
			name: "insertion in middle",
			old:  []uint64{1, 3},
			new:  []uint64{1, 2, 3},
			want: []DiffOp{DiffKeep, DiffInsert, DiffKeep},
		},
		{
			name: "insertion at end",
			old:  []uint64{1, 2},
			new:  []uint64{1, 2, 3},
			want: []DiffOp{DiffKeep, DiffKeep, DiffInsert},
		},
		{
			name: "deletion at start",
			old:  []uint64{1, 2, 3},
			new:  []uint64{2, 3},
			want: []DiffOp{DiffDelete, DiffKeep, DiffKeep},
		},
		{
			name: "deletion in middle",
			old:  []uint64{1, 2, 3},
			new:  []uint64{1, 3},
			want: []DiffOp{DiffKeep, DiffDelete, DiffKeep},
		},
		{
			name: "deletion at end",
			old:  []uint64{1, 2, 3},
			new:  []uint64{1, 2},
			want: []DiffOp{DiffKeep, DiffKeep, DiffDelete},
		},
		{
			name: "complete replacement",
			old:  []uint64{1, 2},
			new:  []uint64{3, 4},
			want: []DiffOp{DiffDelete, DiffDelete, DiffInsert, DiffInsert},
		},
		{
			name: "empty old to non-empty new",
			old:  nil,
			new:  []uint64{1, 2},
			want: []DiffOp{DiffInsert, DiffInsert},
		},
		{
			name: "non-empty old to empty new",
			old:  []uint64{1, 2},
			new:  nil,
			want: []DiffOp{DiffDelete, DiffDelete},
		},
		{
			name: "both empty",
			old:  nil,
			new:  nil,
			want: nil,
		},
		{
			name: "single element keep",
			old:  []uint64{42},
			new:  []uint64{42},
			want: []DiffOp{DiffKeep},
		},
		{
			name: "single element replace",
			old:  []uint64{1},
			new:  []uint64{2},
			want: []DiffOp{DiffDelete, DiffInsert},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := myersDiffHashes(tt.old, tt.new)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAnchorStateManagerCaptureAndDrift(t *testing.T) {
	t.Parallel()

	m := NewAnchorStateManager()

	original := []HashAnchor{
		{Hash: 100, LineNum: 10, Content: "line10"},
		{Hash: 200, LineNum: 20, Content: "line20"},
		{Hash: 300, LineNum: 30, Content: "line30"},
	}
	m.CaptureState("file.go", original)

	t.Run("no drift when identical", func(t *testing.T) {
		t.Parallel()
		drifts, err := m.DetectDrift("file.go", original)
		require.NoError(t, err)
		for _, d := range drifts {
			require.Equal(t, DiffKeep, d.Op)
		}
	})

	t.Run("detects insertion", func(t *testing.T) {
		t.Parallel()
		modified := []HashAnchor{
			{Hash: 100, LineNum: 10, Content: "line10"},
			{Hash: 150, LineNum: 15, Content: "line15"},
			{Hash: 200, LineNum: 20, Content: "line20"},
			{Hash: 300, LineNum: 30, Content: "line30"},
		}
		drifts, err := m.DetectDrift("file.go", modified)
		require.NoError(t, err)

		insertCount := 0
		for _, d := range drifts {
			if d.Op == DiffInsert {
				insertCount++
			}
		}
		require.Equal(t, 1, insertCount)
	})

	t.Run("detects deletion", func(t *testing.T) {
		t.Parallel()
		modified := []HashAnchor{
			{Hash: 100, LineNum: 10, Content: "line10"},
			{Hash: 300, LineNum: 30, Content: "line30"},
		}
		drifts, err := m.DetectDrift("file.go", modified)
		require.NoError(t, err)

		deleteCount := 0
		for _, d := range drifts {
			if d.Op == DiffDelete {
				deleteCount++
			}
		}
		require.Equal(t, 1, deleteCount)
	})

	t.Run("no state returns nil", func(t *testing.T) {
		t.Parallel()
		drifts, err := m.DetectDrift("nonexistent.go", nil)
		require.NoError(t, err)
		require.Nil(t, drifts)
	})
}

func TestAnchorStateManagerReconcile(t *testing.T) {
	t.Parallel()

	m := NewAnchorStateManager()

	drifts := []AnchorDrift{
		{Index: 0, Op: DiffKeep, OldHash: 100, NewHash: 100},
		{Index: 1, Op: DiffDelete, OldHash: 200, NewHash: 0},
		{Index: 1, Op: DiffInsert, OldHash: 0, NewHash: 250},
		{Index: 2, Op: DiffKeep, OldHash: 300, NewHash: 300},
	}

	result := m.Reconcile(drifts)
	require.Len(t, result, 3)
	require.Equal(t, uint64(100), result[0].Hash)
	require.Equal(t, uint64(250), result[1].Hash)
	require.Equal(t, uint64(300), result[2].Hash)
}

func TestResolveAnchorFourChecks(t *testing.T) {
	t.Parallel()

	t.Run("check 1 exact hash match", func(t *testing.T) {
		t.Parallel()
		content := "line0\nline1\nline2\nline3\nline4"
		am := BuildAnchorMap(content, 1)
		if len(am.Anchors) == 0 {
			t.Skip("no anchors generated")
		}
		anchor := am.Anchors[0]
		result, err := ResolveAnchorWithConfidence(&anchor, content)
		require.NoError(t, err)
		require.Equal(t, confExact, result.Confidence)
		require.Equal(t, anchor.LineNum, result.LineNum)
	})

	t.Run("check 2 content match in drift window", func(t *testing.T) {
		t.Parallel()
		content := "line0\nline1\nline2\nline3\nline4\nline5"
		am := BuildAnchorMap(content, 1)
		if len(am.Anchors) == 0 {
			t.Skip("no anchors generated")
		}
		anchor := am.Anchors[0]

		modified := "MODIFIED\nline0\nline1\nline2\nline3\nline4\nline5"
		result, err := ResolveAnchorWithConfidence(&anchor, modified)
		require.NoError(t, err)
		require.Equal(t, confContent, result.Confidence)
	})

	t.Run("check 3 fuzzy whitespace match", func(t *testing.T) {
		t.Parallel()

		anchor := &HashAnchor{
			Hash:    0,
			LineNum: 2,
			Content: "hello world",
		}

		content := "line0\nline1\nhello   world\nline3\nline4"
		result, err := ResolveAnchorWithConfidence(anchor, content)
		require.NoError(t, err)
		require.Equal(t, confFuzzy, result.Confidence)
		require.Equal(t, 2, result.LineNum)
	})

	t.Run("check 4 context hash match", func(t *testing.T) {
		t.Parallel()

		content := "x0\nx1\nx2\na3\na4\na5\na6\nx7\nx8\nx9\nx10\nx11"
		contentLines := strings.Split(content, "\n")

		ctxHashAt5 := hashLineWindow(contentLines, 5)
		hashAt7 := hashLineWindow(contentLines, 7)
		require.NotEqual(t, ctxHashAt5, hashAt7,
			"context at line 7 must differ from context at line 5")

		anchor := &HashAnchor{
			Hash:    ctxHashAt5,
			LineNum: 7,
			Content: "DIFFERENT_THAN_ANYTHING",
		}

		hashAtLineNum := hashLineWindow(contentLines, 7)
		require.NotEqual(t, anchor.Hash, hashAtLineNum,
			"check 1 must not fire at anchor.LineNum")

		result, err := ResolveAnchorWithConfidence(anchor, content)
		require.NoError(t, err)
		require.Equal(t, confContextHash, result.Confidence)
		require.Equal(t, 5, result.LineNum)
	})

	t.Run("all checks fail returns error", func(t *testing.T) {
		t.Parallel()

		anchor := &HashAnchor{
			Hash:    99999,
			LineNum: 50,
			Content: "nonexistent",
		}

		content := "line0\nline1\nline2"
		_, err := ResolveAnchorWithConfidence(anchor, content)
		require.ErrorIs(t, err, errAnchorNotFound)
	})
}

func TestResolveAnchorBackwardCompat(t *testing.T) {
	t.Parallel()

	content := "alpha\nbeta\ngamma\ndelta\nepsilon"
	am := BuildAnchorMap(content, 1)
	if len(am.Anchors) == 0 {
		t.Skip("no anchors generated")
	}

	for _, anchor := range am.Anchors {
		line, err := ResolveAnchor(&anchor, content)
		require.NoError(t, err)
		require.Equal(t, anchor.LineNum, line)
	}
}
