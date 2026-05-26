package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Compile-time interface check.
var _ Processor = (*PIIDetector)(nil)

// PIISensitivity controls how aggressively the detector scans for PII.
type PIISensitivity string

const (
	// SensitivityLow checks only high-confidence patterns (SSN, credit card).
	SensitivityLow PIISensitivity = "low"
	// SensitivityMedium checks SSN, credit card, email, and phone.
	SensitivityMedium PIISensitivity = "medium"
	// SensitivityMedium also sends content to the LLM for contextual detection.
	SensitivityHigh PIISensitivity = "high"
)

// PIILLClient is the interface used by PIIDetector to call an LLM for
// contextual PII detection. It mirrors MockLLMClient.Complete.
type PIILLClient interface {
	Complete(ctx context.Context, prompt, input string) (string, error)
}

// PIIDetector scans messages for personally identifiable information (PII)
// such as SSNs, email addresses, phone numbers, and credit card numbers. When
// PII is found the processor rewrites the affected messages with redacted
// values and returns ActionRewrite.
type PIIDetector struct {
	sensitivity PIISensitivity
	llm         PIILLClient
}

// NewPIIDetector creates a detector with the given sensitivity and optional LLM
// client. The LLM client is required for SensitivityHigh and is ignored at
// lower levels.
func NewPIIDetector(sensitivity PIISensitivity, llm PIILLClient) *PIIDetector {
	return &PIIDetector{
		sensitivity: sensitivity,
		llm:         llm,
	}
}

// ID returns the unique processor identifier.
func (d *PIIDetector) ID() string {
	return "pii_detector"
}

// ProcessInput scans user messages for PII and rewrites if found.
func (d *PIIDetector) ProcessInput(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return d.detectAndRedact(ctx, pctx)
}

// ProcessOutputStream scans streaming output for PII leaks.
func (d *PIIDetector) ProcessOutputStream(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return d.detectAndRedact(ctx, pctx)
}

// ProcessOutputResult scans the final output for PII leaks.
func (d *PIIDetector) ProcessOutputResult(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return d.detectAndRedact(ctx, pctx)
}

// ProcessAPIError is a pass-through — errors are not scanned for PII.
func (d *PIIDetector) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: pctx.Messages,
		Action:   ActionContinue,
	}, nil
}

// detectAndRedact is the shared implementation for all scanned phases.
func (d *PIIDetector) detectAndRedact(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	var allTypes []string
	var totalRedacted int
	redacted := make([]Message, len(pctx.Messages))

	for i, msg := range pctx.Messages {
		content := msg.Content
		clean, types, count := d.redactPII(content)

		// At high sensitivity, also consult the LLM.
		if d.sensitivity == SensitivityHigh && d.llm != nil && content != "" {
			llmClean, llmTypes, llmCount := d.llmRedact(ctx, content)
			// Merge LLM results on top of regex results.
			if llmCount > 0 {
				clean = llmClean
				types = append(types, llmTypes...)
				count += llmCount
			}
		}

		if count > 0 {
			allTypes = append(allTypes, types...)
			totalRedacted += count
		}

		redacted[i] = Message{
			Role:    msg.Role,
			Content: clean,
			Meta:    msg.Meta,
		}
	}

	state := map[string]any{
		"pii_found":      totalRedacted > 0,
		"pii_types":      uniqueStrings(allTypes),
		"redacted_count": totalRedacted,
	}

	action := ActionContinue
	if totalRedacted > 0 {
		action = ActionRewrite
	}

	return ProcessorResult{
		Messages: redacted,
		State:    state,
		Action:   action,
	}, nil
}

// redactPII applies regex-based PII redaction according to the configured
// sensitivity level.
func (d *PIIDetector) redactPII(content string) (string, []string, int) {
	if content == "" {
		return content, nil, 0
	}

	var types []string
	var count int
	result := content

	// SSN: always checked at every sensitivity.
	result, types, count = applyPattern(result, ssnPattern, "ssn", types, count)

	// Credit card: always checked at every sensitivity.
	result, types, count = applyPattern(result, ccPattern, "credit_card", types, count)

	if d.sensitivity != SensitivityLow {
		// Email.
		result, types, count = applyPattern(result, emailPattern, "email", types, count)
		// Phone.
		result, types, count = applyPattern(result, phonePattern, "phone", types, count)
	}

	return result, types, count
}

// llmRedact sends content to the LLM for contextual PII detection.
func (d *PIIDetector) llmRedact(ctx context.Context, content string) (string, []string, int) {
	prompt := `Analyze the following text for personally identifiable information (PII). ` +
		`PII includes: SSN, email, phone number, credit card number, ` +
		`date of birth, home address, passport number, driver's license number. ` +
		`Return a JSON object with three fields: "redacted" (string, the text with PII replaced by [REDACTED_TYPE]), ` +
		`"types" (array of strings, the PII types found), "count" (integer, total PII items found). ` +
		`If no PII is found, return {"redacted":"<original>","types":[],"count":0}.`

	resp, err := d.llm.Complete(ctx, prompt, content)
	if err != nil {
		// On LLM error, return unmodified content.
		return content, nil, 0
	}

	var parsed struct {
		Redacted string   `json:"redacted"`
		Types    []string `json:"types"`
		Count    int      `json:"count"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		return content, nil, 0
	}
	return parsed.Redacted, parsed.Types, parsed.Count
}

// applyPattern replaces all matches of a regex in content with a redaction
// placeholder and tracks the type and count.
func applyPattern(content string, re *regexp.Regexp, piiType string, types []string, count int) (string, []string, int) {
	matches := re.FindAllString(content, -1)
	if len(matches) == 0 {
		return content, types, count
	}
	replacement := fmt.Sprintf("[REDACTED_%s]", strings.ToUpper(piiType))
	result := re.ReplaceAllString(content, replacement)
	for range matches {
		types = append(types, piiType)
		count++
	}
	return result, types, count
}

// uniqueStrings deduplicates a string slice preserving order.
func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// PII detection regex patterns.
var (
	// SSN matches US Social Security Numbers (XXX-XX-XXXX or XXX XX XXXX).
	ssnPattern = regexp.MustCompile(`\b\d{3}[-\s]\d{2}[-\s]\d{4}\b`)
	// Email matches common email addresses.
	emailPattern = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)
	// Phone matches US phone numbers in various formats (parenthesized area code
	// requires dropping the leading \b since \b fails before `(`).
	phonePattern = regexp.MustCompile(`(?:\+?1[-.\s]?)?(?:\(\d{3}\)|\d{3})[-.\s]?\d{3}[-.\s]?\d{4}`)
	// CC matches 16-digit credit card numbers with optional spaces or dashes.
	ccPattern = regexp.MustCompile(`\b\d{4}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b`)
)
