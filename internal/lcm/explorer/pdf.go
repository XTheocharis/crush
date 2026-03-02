package explorer

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PDFExplorer explores PDF files using pdfinfo and pdftotext.
type PDFExplorer struct {
	formatterProfile OutputProfile
}

// pdfMagicBytes is the PDF file signature (%PDF).
var pdfMagicBytes = []byte{0x25, 0x50, 0x44, 0x46}

const (
	// pdfToolTimeout is the timeout for external PDF tool invocations.
	pdfToolTimeout = 5 * time.Second
	// pdfMaxTextChars is the maximum number of characters to keep from
	// pdftotext output. When the output exceeds this limit, the first
	// pdfHeadChars and last pdfTailChars are kept.
	pdfMaxTextChars = 2000
	pdfHeadChars    = 1600
	pdfTailChars    = 400
	// pdfMinTextChars is the threshold below which the PDF is considered
	// image-only or scanned.
	pdfMinTextChars = 10
)

func (e *PDFExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "pdf" {
		return true
	}
	// Check magic bytes: %PDF (0x25504446).
	return len(content) >= len(pdfMagicBytes) &&
		bytes.HasPrefix(content, pdfMagicBytes)
}

func (e *PDFExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	name := filepath.Base(input.Path)
	var summary strings.Builder

	fmt.Fprintf(&summary, "PDF document: %s\n", name)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))

	// Write content to a temp file for external tool invocation.
	err := withTempFile("crush-pdf-*.pdf", input.Content, func(tempPath string) error {
		return e.explorePDF(ctx, &summary, tempPath)
	})
	if err != nil {
		summary.WriteString("\nError: " + err.Error())
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "pdf",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// explorePDF runs pdfinfo and pdftotext against the temp file and writes
// the extracted information into the summary builder.
func (e *PDFExplorer) explorePDF(ctx context.Context, summary *strings.Builder, path string) error {
	// Try pdfinfo for metadata (non-fatal if missing).
	e.extractMetadata(ctx, summary, path)

	// Try pdftotext for text content.
	return e.extractText(ctx, summary, path)
}

// extractMetadata runs pdfinfo and parses key-value lines into the summary.
// Missing pdfinfo tool is non-fatal.
func (e *PDFExplorer) extractMetadata(ctx context.Context, summary *strings.Builder, path string) {
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		return
	}

	tctx, cancel := context.WithTimeout(ctx, pdfToolTimeout)
	defer cancel()

	out, err := exec.CommandContext(tctx, "pdfinfo", path).Output()
	if err != nil {
		return
	}

	lines := strings.Split(string(out), "\n")
	summary.WriteString("\nMetadata:\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// pdfinfo outputs "Key:  Value" lines.
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			if val != "" {
				fmt.Fprintf(summary, "  %s: %s\n", key, val)
			}
		}
	}
}

// extractText runs pdftotext -layout and captures stdout. It truncates long
// output and detects encrypted or image-only PDFs.
func (e *PDFExplorer) extractText(ctx context.Context, summary *strings.Builder, path string) error {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		// No pdftotext available; degrade gracefully to file-size only.
		return nil
	}

	tctx, cancel := context.WithTimeout(ctx, pdfToolTimeout)
	defer cancel()

	// pdftotext -layout <input> - (dash sends output to stdout).
	cmd := exec.CommandContext(tctx, "pdftotext", "-layout", path, "-")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Check for encryption error in stderr.
		stderrStr := stderr.String()
		if strings.Contains(stderrStr, "ncrypt") ||
			strings.Contains(stderrStr, "password") {
			summary.WriteString("\nEncrypted PDF")
			return nil
		}
		// Other errors are non-fatal; degrade to file-size only.
		return nil
	}

	text := strings.TrimSpace(stdout.String())

	// Detect image-only / scanned PDFs.
	if len(text) < pdfMinTextChars {
		summary.WriteString("\nImage-only or scanned PDF")
		return nil
	}

	// Truncate to limit: head + tail.
	if len(text) > pdfMaxTextChars {
		head := text[:pdfHeadChars]
		tail := text[len(text)-pdfTailChars:]
		text = head + "\n[...truncated...]\n" + tail
	}

	summary.WriteString("\nText content:\n")
	summary.WriteString(text)

	return nil
}
