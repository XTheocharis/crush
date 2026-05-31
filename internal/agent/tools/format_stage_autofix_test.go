//go:build treesitter

package tools

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatStageAutoFix_ReadOnlyMode(t *testing.T) {
	t.Parallel()

	stage := &FormatStageAutoFix{AutoFix: false}
	input := &ValidationInput{
		FilePath:      "test.go",
		Content:       "package main\n\nfunc main(){\n}\n",
		editedContent: "package main\n\nfunc main(){\n}\n",
	}

	result, err := stage.Execute(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "Format", result.StageName)
	require.Equal(t, StatusFail, result.Status)
	require.Equal(t, "file needs formatting", result.Message)
	require.Equal(t, "package main\n\nfunc main(){\n}\n", input.editedContent)
}

func TestFormatStageAutoFix_AutoFixMode(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not available")
	}

	stage := &FormatStageAutoFix{AutoFix: true}
	input := &ValidationInput{
		FilePath:      "test.go",
		Content:       "package main\n\nfunc main(){\n}\n",
		editedContent: "package main\n\nfunc main(){\n}\n",
	}

	result, err := stage.Execute(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "Format", result.StageName)
	require.Equal(t, StatusPass, result.Status)
	require.Contains(t, result.Message, "auto-fix applied")
	require.NotEqual(t, "package main\n\nfunc main(){\n}\n", input.editedContent)
	require.Contains(t, input.editedContent, "main() {")
}

func TestFormatStageAutoFix_AlreadyFormatted(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not available")
	}

	wellFormatted := "package main\n\nfunc main() {\n}\n"
	stage := &FormatStageAutoFix{AutoFix: true}
	input := &ValidationInput{
		FilePath:      "test.go",
		Content:       wellFormatted,
		editedContent: wellFormatted,
	}

	result, err := stage.Execute(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, StatusPass, result.Status)
	require.Equal(t, "file is well formatted", result.Message)
	require.Equal(t, wellFormatted, input.editedContent)
}

func TestFormatStageAutoFix_SkipNonGo(t *testing.T) {
	t.Parallel()

	stage := &FormatStageAutoFix{AutoFix: true}
	input := &ValidationInput{
		FilePath:      "test.py",
		Content:       "def main():\n    pass\n",
		editedContent: "def main():\n    pass\n",
	}

	result, err := stage.Execute(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, StatusSkip, result.Status)
}

func TestFormatStageAutoFix_SkipEmpty(t *testing.T) {
	t.Parallel()

	stage := &FormatStageAutoFix{AutoFix: true}
	input := &ValidationInput{
		FilePath:      "test.go",
		Content:       "package main",
		editedContent: "",
	}

	result, err := stage.Execute(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, StatusSkip, result.Status)
}

func TestFormatStageAutoFix_FallsBackToContent(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not available")
	}

	stage := &FormatStageAutoFix{AutoFix: true}
	input := &ValidationInput{
		FilePath:      "test.go",
		Content:       "package main\n\nfunc main(){\n}\n",
		editedContent: "",
	}

	// When editedContent is empty, FormatStage returns StatusSkip, so auto-fix
	// won't trigger.
	result, err := stage.Execute(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, StatusSkip, result.Status)
}

func TestFormatStageAutoFix_GofmtNotInstalled(t *testing.T) {
	t.Parallel()

	stage := &FormatStageAutoFix{
		AutoFix: true,
		FormatStage: FormatStage{
			lookPath: func(_ string) (string, error) {
				return "", &os.PathError{Op: "lookup", Path: "gofmt"}
			},
		},
	}
	input := &ValidationInput{
		FilePath:      "test.go",
		Content:       "package main\n\nfunc main(){\n}\n",
		editedContent: "package main\n\nfunc main(){\n}\n",
	}

	result, err := stage.Execute(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, StatusSkip, result.Status)
}
