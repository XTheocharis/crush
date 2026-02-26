package explorer

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// BinaryExplorer handles known binary file types.
type BinaryExplorer struct{}

func (e *BinaryExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	_, isBinary := BINARY_EXTENSIONS[ext]
	if isBinary {
		return true
	}
	return hasBinarySignature(content)
}

func (e *BinaryExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	summary := fmt.Sprintf("Binary file: %s (%d bytes)\nHex preview: %s",
		filepath.Base(input.Path), len(input.Content), hexDump(input.Content))
	return ExploreResult{
		Summary:       summary,
		ExplorerUsed:  "binary",
		TokenEstimate: estimateTokens(summary),
	}, nil
}

func hasBinarySignature(content []byte) bool {
	signatures := [][]byte{
		{0x7F, 0x45, 0x4C, 0x46},             // ELF
		{0x89, 0x50, 0x4E, 0x47},             // PNG
		{0xFF, 0xD8, 0xFF},                   // JPEG
		{0x50, 0x4B, 0x03, 0x04},             // ZIP
		{0x25, 0x50, 0x44, 0x46},             // PDF
		{0x4D, 0x5A},                         // PE/MZ
		{0xCA, 0xFE, 0xBA, 0xBE},             // Java class
		{0x00, 0x61, 0x73, 0x6D},             // WASM
		{0x1F, 0x8B},                         // gzip
		{0x52, 0x61, 0x72, 0x21, 0x1A, 0x07}, // RAR
	}
	for _, sig := range signatures {
		if len(content) >= len(sig) && bytes.HasPrefix(content, sig) {
			return true
		}
	}
	return false
}

// TextExplorer handles generic text files not matched by specific explorers.
type TextExplorer struct{}

func (e *TextExplorer) CanHandle(path string, content []byte) bool {
	// Check if extension is in known text extensions
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if _, ok := TEXT_EXTENSIONS[ext]; ok {
		return true
	}
	// Or if it looks like text content
	return looksLikeText(content)
}

func (e *TextExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("Text file too large: %s (%d bytes)\nMaximum size for full load: %d bytes",
			filepath.Base(input.Path), len(input.Content), MaxFullLoadSize)
		return ExploreResult{
			Summary:       summary,
			ExplorerUsed:  "text",
			TokenEstimate: estimateTokens(summary),
		}, nil
	}

	content, sampled := sampleContent(input.Content, 12000)
	lineCount := strings.Count(string(input.Content), "\n") + 1

	var summary strings.Builder
	fmt.Fprintf(&summary, "Text file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Lines: %d\n", lineCount)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))
	if sampled {
		summary.WriteString("Content (sampled):\n")
	} else {
		summary.WriteString("Content:\n")
	}
	summary.WriteString(content)

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "text",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// FallbackExplorer is the last resort that handles everything.
type FallbackExplorer struct{}

func (e *FallbackExplorer) CanHandle(path string, content []byte) bool {
	// Always returns true - this is the final fallback
	return true
}

func (e *FallbackExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if !looksLikeText(input.Content) {
		// Treat as binary
		summary := fmt.Sprintf("Unknown binary file: %s (%d bytes)\nHex preview: %s",
			filepath.Base(input.Path), len(input.Content), hexDump(input.Content))
		return ExploreResult{
			Summary:       summary,
			ExplorerUsed:  "fallback",
			TokenEstimate: estimateTokens(summary),
		}, nil
	}

	// Treat as text
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("File too large: %s (%d bytes)\nMaximum size for full load: %d bytes",
			filepath.Base(input.Path), len(input.Content), MaxFullLoadSize)
		return ExploreResult{
			Summary:       summary,
			ExplorerUsed:  "fallback",
			TokenEstimate: estimateTokens(summary),
		}, nil
	}

	content, sampled := sampleContent(input.Content, 12000)
	lineCount := strings.Count(string(input.Content), "\n") + 1

	var summary strings.Builder
	fmt.Fprintf(&summary, "Unknown text file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Lines: %d\n", lineCount)
	fmt.Fprintf(&summary, "Size: %d bytes\n", len(input.Content))
	if sampled {
		summary.WriteString("Content (sampled):\n")
	} else {
		summary.WriteString("Content:\n")
	}
	summary.WriteString(content)

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "fallback",
		TokenEstimate: estimateTokens(result),
	}, nil
}
