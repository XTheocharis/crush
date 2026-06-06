//go:build treesitter

package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestApplyEditStage_SubstringContent_ProducesSnippetNotFile demonstrates the
// bug where parseEditInfoFromJSON sets info.oldContent = old_string (a
// substring of the full file). When this substring is passed as Content to
// ApplyEditStage, the stage finds old_string at index 0 and produces only
// new_string — not the full file with the replacement applied.
//
// Root cause: parseEditInfoFromJSON (treesitter_ext.go:229) stores
// old_string into info.oldContent, but the validation pipeline expects
// Content to be the FULL file contents. With Content=old_string:
//
//	idx := strings.Index(input.Content, old)  → idx=0
//	result = input.Content[:0] + new + input.Content[0+len(old):]  → just new
//
// This test FAILS until the bug is fixed: it asserts that editedContent
// should contain the full edited file, but currently produces only the
// replacement snippet "func new()" because Content is the substring.
func TestApplyEditStage_SubstringContent_ProducesSnippetNotFile(t *testing.T) {
	t.Parallel()

	substring := "func old()"

	stage := &ApplyEditStage{}
	input := &ValidationInput{
		FilePath: "test.go",
		Content:  substring,
		EditSpec: EditSpec{
			OldString: "func old()",
			NewString: "func new()",
		},
	}

	result, err := stage.Execute(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, StatusPass, result.Status)

	expectedFullEditedFile := "package main\n\nfunc new() {\n\treturn\n}\n"

	// BUG: This assertion FAILS. editedContent is "func new()" (snippet),
	// not the full edited file. The fix must ensure Content is the full file
	// before ApplyEditStage runs.
	require.Equal(t, expectedFullEditedFile, input.editedContent,
		"ApplyEditStage should produce the full edited file, not just the replacement snippet. "+
			"got %q, want %q", input.editedContent, expectedFullEditedFile)
}

// TestApplyEditStage_FullContent_ProducesFullEditedFile is the control test
// showing that ApplyEditStage works correctly when Content is the full file.
func TestApplyEditStage_FullContent_ProducesFullEditedFile(t *testing.T) {
	t.Parallel()

	stage := &ApplyEditStage{}
	input := &ValidationInput{
		FilePath: "test.go",
		Content:  "package main\n\nfunc old() {\n\treturn\n}\n",
		EditSpec: EditSpec{
			OldString: "func old()",
			NewString: "func new()",
		},
	}

	result, err := stage.Execute(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, StatusPass, result.Status)
	require.Equal(t, "package main\n\nfunc new() {\n\treturn\n}\n", input.editedContent)
}
