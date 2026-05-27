package processor

import (
	"context"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Compile-time interface check.
var _ Processor = (*UnicodeNormalizer)(nil)

// UnicodeNormalizer applies NFKC normalization, strips zero-width characters,
// and normalizes whitespace in all message content. It is pure computation —
// no LLM calls are required.
type UnicodeNormalizer struct{}

func (UnicodeNormalizer) ID() string {
	return "unicode_normalizer"
}

func (n UnicodeNormalizer) ProcessInput(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: normalizeMessages(pctx.Messages),
		Action:   ActionContinue,
	}, nil
}

func (n UnicodeNormalizer) ProcessOutputStream(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: normalizeMessages(pctx.Messages),
		Action:   ActionContinue,
	}, nil
}

func (n UnicodeNormalizer) ProcessOutputResult(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: normalizeMessages(pctx.Messages),
		Action:   ActionContinue,
	}, nil
}

func (n UnicodeNormalizer) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: normalizeMessages(pctx.Messages),
		Action:   ActionContinue,
	}, nil
}

func normalizeMessages(msgs []Message) []Message {
	out := make([]Message, len(msgs))
	for i, m := range msgs {
		out[i] = Message{
			Role:    m.Role,
			Content: normalizeString(m.Content),
			Meta:    m.Meta,
		}
	}
	return out
}

// normalizeString applies NFKC normalization, strips zero-width characters,
// and collapses whitespace.
func normalizeString(s string) string {
	// Step 1: NFKC normalization.
	s = norm.NFKC.String(s)

	// Step 2: Strip zero-width characters.
	s = strings.Map(func(r rune) rune {
		switch r {
		case '\u200b', // Zero-width space
			'\u200c', // Zero-width non-joiner
			'\u200d', // Zero-width joiner
			'\ufeff', // Byte order mark / zero-width no-break space
			'\u00ad': // Soft hyphen
			return -1
		}
		return r
	}, s)

	// Step 3: Normalize whitespace — collapse consecutive spaces/tabs to a
	// single space, then trim.
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
			continue
		}
		inSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
