package explorer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPDFExplorer_CanHandle(t *testing.T) {
	t.Parallel()
	explorer := &PDFExplorer{}

	tests := []struct {
		name     string
		path     string
		content  []byte
		expected bool
	}{
		{
			name:     "pdf extension",
			path:     "report.pdf",
			content:  []byte("some content"),
			expected: true,
		},
		{
			name:     "pdf extension uppercase",
			path:     "REPORT.PDF",
			content:  []byte("some content"),
			expected: true,
		},
		{
			name:     "pdf magic bytes",
			path:     "document",
			content:  append([]byte{0x25, 0x50, 0x44, 0x46}, []byte("-1.7 rest of file")...),
			expected: true,
		},
		{
			name:     "pdf magic bytes short",
			path:     "document",
			content:  []byte{0x25, 0x50, 0x44, 0x46},
			expected: true,
		},
		{
			name:     "not pdf - txt file",
			path:     "readme.txt",
			content:  []byte("Hello world"),
			expected: false,
		},
		{
			name:     "not pdf - png magic",
			path:     "image.png",
			content:  []byte{0x89, 0x50, 0x4E, 0x47},
			expected: false,
		},
		{
			name:     "not pdf - too short for magic",
			path:     "document",
			content:  []byte{0x25, 0x50, 0x44},
			expected: false,
		},
		{
			name:     "empty content with non-pdf extension",
			path:     "file.doc",
			content:  nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := explorer.CanHandle(tt.path, tt.content)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestPDFExplorer_Explore_NoTools(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv().
	t.Setenv("PATH", t.TempDir())

	explorer := &PDFExplorer{}
	content := makeSyntheticPDF()

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "document.pdf",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "pdf", result.ExplorerUsed)
	require.Contains(t, result.Summary, "PDF document: document.pdf")
	require.Contains(t, result.Summary, fmt.Sprintf("Size: %d bytes", len(content)))
	require.Greater(t, result.TokenEstimate, 0)
}

func TestPDFExplorer_Explore_MagicBytesOnly(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv().
	t.Setenv("PATH", t.TempDir())

	explorer := &PDFExplorer{}
	// Minimal content with just the magic header.
	content := []byte("%PDF-1.4 minimal")

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "minimal.pdf",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "pdf", result.ExplorerUsed)
	require.Contains(t, result.Summary, "PDF document: minimal.pdf")
}

// TestPDFExplorer_Explore_WithMockPdftotext uses TestHelperProcess
// pattern to test pdftotext integration without requiring the real binary.
func TestPDFExplorer_Explore_WithMockPdftotext(t *testing.T) {
	// Skip if we can't find our own test binary.
	self, err := os.Executable()
	if err != nil {
		t.Skip("Cannot find test executable")
	}

	// Create a fake pdftotext script that delegates to TestHelperProcess.
	tmpDir := t.TempDir()
	scriptPath := tmpDir + "/pdftotext"
	script := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestHelperProcess -- pdftotext \"$@\"\n", self)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	t.Setenv("PATH", tmpDir)
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")

	explorer := &PDFExplorer{}
	content := makeSyntheticPDF()

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "report.pdf",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "pdf", result.ExplorerUsed)
	require.Contains(t, result.Summary, "PDF document: report.pdf")
	// The mock pdftotext outputs text, so we should see text content.
	require.Contains(t, result.Summary, "Text content")
}

// TestPDFExplorer_Explore_EncryptedPDF tests the encrypted PDF detection
// path.
func TestPDFExplorer_Explore_EncryptedPDF(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skip("Cannot find test executable")
	}

	tmpDir := t.TempDir()
	scriptPath := tmpDir + "/pdftotext"
	script := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestHelperProcess -- pdftotext-encrypted \"$@\"\n", self)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	t.Setenv("PATH", tmpDir)
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")

	explorer := &PDFExplorer{}
	content := makeSyntheticPDF()

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "secret.pdf",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "pdf", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Encrypted PDF")
}

// TestPDFExplorer_Explore_ImageOnlyPDF tests the image-only/scanned PDF
// detection when pdftotext returns very little text.
func TestPDFExplorer_Explore_ImageOnlyPDF(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skip("Cannot find test executable")
	}

	tmpDir := t.TempDir()
	scriptPath := tmpDir + "/pdftotext"
	script := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestHelperProcess -- pdftotext-empty \"$@\"\n", self)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	t.Setenv("PATH", tmpDir)
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")

	explorer := &PDFExplorer{}
	content := makeSyntheticPDF()

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "scanned.pdf",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "pdf", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Image-only or scanned PDF")
}

// TestPDFExplorer_Explore_LongTextTruncation verifies that long text output
// is truncated to head + tail.
func TestPDFExplorer_Explore_LongTextTruncation(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skip("Cannot find test executable")
	}

	tmpDir := t.TempDir()
	scriptPath := tmpDir + "/pdftotext"
	script := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestHelperProcess -- pdftotext-long \"$@\"\n", self)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	t.Setenv("PATH", tmpDir)
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")

	explorer := &PDFExplorer{}
	content := makeSyntheticPDF()

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "long.pdf",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "pdf", result.ExplorerUsed)
	require.Contains(t, result.Summary, "[...truncated...]")
	require.Contains(t, result.Summary, "Text content")
}

// TestPDFExplorer_Explore_WithMockPdfinfo tests pdfinfo metadata extraction.
func TestPDFExplorer_Explore_WithMockPdfinfo(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skip("Cannot find test executable")
	}

	tmpDir := t.TempDir()

	// Create mock pdfinfo.
	pdfinfoScript := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestHelperProcess -- pdfinfo \"$@\"\n", self)
	require.NoError(t, os.WriteFile(tmpDir+"/pdfinfo", []byte(pdfinfoScript), 0o755))

	// Create mock pdftotext.
	pdftotextScript := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestHelperProcess -- pdftotext \"$@\"\n", self)
	require.NoError(t, os.WriteFile(tmpDir+"/pdftotext", []byte(pdftotextScript), 0o755))

	t.Setenv("PATH", tmpDir)
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")

	explorer := &PDFExplorer{}
	content := makeSyntheticPDF()

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "info.pdf",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "pdf", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Metadata")
	require.Contains(t, result.Summary, "Title: Test Document")
	require.Contains(t, result.Summary, "Pages: 1")
}

// TestHelperProcess is not a real test. It is used as a helper process
// for mocking external commands (pdftotext, pdfinfo).
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	// Find the "--" separator.
	cmdIdx := -1
	for i, arg := range args {
		if arg == "--" {
			cmdIdx = i + 1
			break
		}
	}
	if cmdIdx < 0 || cmdIdx >= len(args) {
		os.Exit(1)
	}

	cmd := args[cmdIdx]
	switch cmd {
	case "pdftotext":
		fmt.Fprintf(os.Stdout, "This is the extracted text from a PDF document.\nIt has multiple lines and useful content for testing.\n")
		os.Exit(0)

	case "pdftotext-encrypted":
		fmt.Fprintf(os.Stderr, "Error: Encrypted PDF, cannot extract text without password\n")
		os.Exit(1)

	case "pdftotext-empty":
		// Output very little text (below pdfMinTextChars threshold).
		fmt.Fprintf(os.Stdout, "   \n")
		os.Exit(0)

	case "pdftotext-long":
		// Output text longer than pdfMaxTextChars to trigger truncation.
		fmt.Fprintf(os.Stdout, "%s", strings.Repeat("A", 3000))
		os.Exit(0)

	case "pdfinfo":
		fmt.Fprintf(os.Stdout, "Title:          Test Document\nAuthor:         Test Author\nPages:          1\nPage size:      612 x 792 pts (letter)\n")
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "Unknown test helper command: %s\n", cmd)
		os.Exit(1)
	}
}

// TestPDFExplorer_Explore_PdftotextError tests graceful degradation when
// pdftotext fails with a non-encryption error.
func TestPDFExplorer_Explore_PdftotextError(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skip("Cannot find test executable")
	}

	tmpDir := t.TempDir()
	scriptPath := tmpDir + "/pdftotext"
	// Use a command name that will cause TestHelperProcess to exit(1)
	// with an unknown error.
	script := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestHelperProcess -- pdftotext-generic-error \"$@\"\n", self)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	t.Setenv("PATH", tmpDir)
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")

	explorer := &PDFExplorer{}
	content := makeSyntheticPDF()

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "broken.pdf",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "pdf", result.ExplorerUsed)
	// Should still have the basic info but no text content or encryption
	// message.
	require.Contains(t, result.Summary, "PDF document: broken.pdf")
	require.NotContains(t, result.Summary, "Encrypted PDF")
	require.NotContains(t, result.Summary, "Text content")
}

// TestPDFExplorer_NilError verifies that Explore always returns nil error,
// even with various failure conditions.
func TestPDFExplorer_NilError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	explorer := &PDFExplorer{}

	// Empty content.
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "empty.pdf",
		Content: []byte{},
	})
	require.NoError(t, err)
	require.Equal(t, "pdf", result.ExplorerUsed)
}

// makeSyntheticPDF creates minimal bytes that start with the PDF magic
// header.
func makeSyntheticPDF() []byte {
	return []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n")
}

// TestPDFExplorer_LookPathUsed confirms that exec.LookPath is called
// before running external tools by verifying behavior when PATH is empty.
func TestPDFExplorer_LookPathUsed(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	// Verify that LookPath actually fails for our tools.
	_, err := exec.LookPath("pdfinfo")
	require.Error(t, err)
	_, err = exec.LookPath("pdftotext")
	require.Error(t, err)

	explorer := &PDFExplorer{}
	content := makeSyntheticPDF()

	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "test.pdf",
		Content: content,
	})
	// Must return nil error even when tools are missing.
	require.NoError(t, err)
	require.Equal(t, "pdf", result.ExplorerUsed)
	// With no tools, we should only have the basic header.
	require.Contains(t, result.Summary, "PDF document: test.pdf")
	require.Contains(t, result.Summary, "Size:")
	// Should NOT contain metadata or text content since tools are missing.
	require.NotContains(t, result.Summary, "Metadata")
	require.NotContains(t, result.Summary, "Text content")
}
