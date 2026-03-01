package explorer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/crush/internal/treesitter"
)

const (
	MaxFullLoadSize = 50 * 1024 * 1024 // 50 MB
	HexDumpBytes    = 32
	SampleChunkSize = 4096
)

// ExploreInput is the input to the explorer registry.
type ExploreInput struct {
	Path    string
	Content []byte
	// Model is reserved for future model-specific strategies.
	Model any
	// SessionID is the parent session ID. When non-empty and an AgentFunc is
	// configured, agent-based exploration (tier 3) is attempted.
	SessionID string
}

// ExploreResult is the result of exploring a file.
type ExploreResult struct {
	Summary       string
	ExplorerUsed  string
	TokenEstimate int
}

// Explorer is the interface all file explorers implement.
type Explorer interface {
	// CanHandle returns true if this explorer handles the given file.
	CanHandle(path string, content []byte) bool
	// Explore returns a structured summary of the file.
	Explore(ctx context.Context, input ExploreInput) (ExploreResult, error)
}

// RegistryOption configures a Registry.
type RegistryOption func(*Registry)

// WithTreeSitter adds a tree-sitter parser to the registry with TreeSitterExplorer.
func WithTreeSitter(parser treesitter.Parser) RegistryOption {
	return func(r *Registry) {
		r.tsParser = parser
	}
}

// WithOutputProfile sets formatter profile behavior for truncation markers.
func WithOutputProfile(profile OutputProfile) RegistryOption {
	return func(r *Registry) {
		r.formatterProfile = profile
	}
}

// Registry is an ordered list of explorers with optional LLM enhancement.
type Registry struct {
	explorers        []Explorer
	llm              LLMClient // nil when LLM is unavailable (tier 1 only)
	agentFn          AgentFunc // nil when agent-based exploration is unavailable
	tsParser         treesitter.Parser
	formatterProfile OutputProfile
}

// NewRegistry creates a registry with all built-in explorers.
func NewRegistry(opts ...RegistryOption) *Registry {
	r := &Registry{formatterProfile: OutputProfileEnhancement}
	// Register in priority order.
	// Binary -> Data formats -> Code -> Shell -> Text -> Fallback.
	r.explorers = []Explorer{
		// Phase 1: Binary/executable types
		&BinaryExplorer{},
		// Phase 2: Data/document explorers (checked before code)
		&JSONExplorer{},
		&CSVExplorer{},
		&YAMLExplorer{},
		&TOMLExplorer{},
		&INIExplorer{},
		&XMLExplorer{},
		&HTMLExplorer{},
		&MarkdownExplorer{},
		&LatexExplorer{},
		&SQLiteExplorer{},
		&LogsExplorer{},
		// Phase 3: Shell scripts (checked before generic text)
		&ShellExplorer{},
		// Phase 4: Generic text fallback
		&TextExplorer{},
		// Phase 5: Final fallback
		&FallbackExplorer{},
	}
	// Apply options.
	for _, opt := range opts {
		opt(r)
	}
	// Apply formatter profile to explorers that need it.
	// This must be done after options are applied so we have the final profile.
	for i, e := range r.explorers {
		if sqlExp, ok := e.(*SQLiteExplorer); ok {
			sqlExp.formatterProfile = r.formatterProfile
			r.explorers[i] = sqlExp
		}
		if latexExp, ok := e.(*LatexExplorer); ok {
			latexExp.formatterProfile = r.formatterProfile
			r.explorers[i] = latexExp
		}
	}
	// If a tree-sitter parser is provided, add TreeSitterExplorer to the chain.
	// It's inserted after all data format explorers to handle code files
	// before shell-specific handling while preserving data-format-first ordering.
	if r.tsParser != nil {
		tsExp := &TreeSitterExplorer{parser: r.tsParser, formatterProfile: r.formatterProfile}
		// Insert after LogsExplorer (last data format) and before ShellExplorer.
		newExplorers := make([]Explorer, 0, len(r.explorers)+1)
		for _, e := range r.explorers {
			newExplorers = append(newExplorers, e)
			if _, ok := e.(*LogsExplorer); ok {
				newExplorers = append(newExplorers, tsExp)
			}
		}
		r.explorers = newExplorers
	}
	return r
}

// Explore finds the best explorer for the file and runs it, then optionally
// enhances the result via LLM (tier 2) or agent (tier 3) if configured.
//
// Three-tier dispatch:
//  1. Template only — no LLM, no agent; returns static formatSummary output.
//  2. O19a single-call LLM — LLM present, no sessionID; sends truncated
//     content for summarization.
//  3. O19b agent-based — LLM + sessionID; spawns agent with language-specific
//     prompt.
//
// Python exception: Python files skip tier 2 and go directly from tier 1 to
// tier 3 when an agent is available.
func (r *Registry) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	// Step 1: always run the static explorer to get a baseline result.
	staticResult, err := r.exploreStatic(ctx, input)
	if err != nil {
		return staticResult, err
	}

	// If no LLM capability is configured, return the static result (tier 1).
	if r.llm == nil && r.agentFn == nil {
		return staticResult, nil
	}

	// Attempt LLM-enhanced exploration (tiers 2 and 3).
	enhanced := exploreLLMEnhanced(ctx, r.llm, r.agentFn, input, staticResult)
	return formatExploreResult(enhanced, r.formatterProfile), nil
}

// exploreStatic runs the static (template-based) explorer chain.
func (r *Registry) exploreStatic(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	for _, e := range r.explorers {
		if e.CanHandle(input.Path, input.Content) {
			result, err := e.Explore(ctx, input)
			if err == nil {
				return formatExploreResult(result, r.formatterProfile), nil
			}
		}
	}
	// Should never reach here since FallbackExplorer handles everything.
	result := ExploreResult{Summary: "Unknown file type", ExplorerUsed: "fallback"}
	return formatExploreResult(result, r.formatterProfile), nil
}

// looksLikeText returns true if content appears to be text (not binary).
func looksLikeText(content []byte) bool {
	if len(content) == 0 {
		return true
	}
	sample := content
	if len(sample) > 512 {
		sample = sample[:512]
	}
	// Count non-printable bytes (excluding common whitespace).
	nonPrintable := 0
	for _, b := range sample {
		if b == 0 {
			return false // null byte = binary
		}
		if b < 32 && b != '\t' && b != '\n' && b != '\r' {
			nonPrintable++
		}
	}
	// If >30% non-printable, probably binary.
	return float64(nonPrintable)/float64(len(sample)) < 0.30
}

// sampleContent returns begin+middle+end samples of large content.
func sampleContent(content []byte, maxTotal int) (string, bool) {
	if len(content) <= maxTotal {
		return string(content), false
	}
	third := maxTotal / 3
	begin := string(content[:third])
	midStart := len(content)/2 - third/2
	middle := string(content[midStart : midStart+third])
	end := string(content[len(content)-third:])
	return begin + "\n...[SAMPLED]...\n" + middle + "\n...[SAMPLED]...\n" + end, true
}

// hexDump returns a hex representation of the first n bytes.
func hexDump(content []byte) string {
	if len(content) > HexDumpBytes {
		content = content[:HexDumpBytes]
	}
	var sb strings.Builder
	for i, b := range content {
		if i > 0 {
			sb.WriteString(" ")
		}
		fmt.Fprintf(&sb, "%02x", b)
	}
	return sb.String()
}

// estimateTokens estimates token count from character count.
func estimateTokens(s string) int {
	chars := len([]rune(s))
	tokens := (chars + 3) / 4 // ceiling division by 4
	return tokens
}

// detectShebang detects the language from a shebang line.
func detectShebang(content []byte) string {
	if len(content) < 2 || content[0] != '#' || content[1] != '!' {
		return ""
	}
	line := string(content)
	newline := strings.IndexByte(line, '\n')
	if newline >= 0 {
		line = line[:newline]
	}
	line = strings.TrimSpace(line[2:]) // remove #!

	shebangs := map[string]string{
		"python":     "python",
		"python3":    "python",
		"python2":    "python",
		"node":       "javascript",
		"nodejs":     "javascript",
		"ruby":       "ruby",
		"perl":       "perl",
		"php":        "php",
		"bash":       "shell",
		"sh":         "shell",
		"zsh":        "shell",
		"fish":       "shell",
		"dash":       "shell",
		"ksh":        "shell",
		"tcsh":       "shell",
		"csh":        "shell",
		"pwsh":       "shell",
		"powershell": "shell",
		"lua":        "lua",
		"swift":      "swift",
		"groovy":     "groovy",
		"scala":      "scala",
		"kotlin":     "kotlin",
		"r":          "r",
		"Rscript":    "r",
		"julia":      "julia",
		"elixir":     "elixir",
		"erlang":     "erlang",
		"go":         "go",
		"deno":       "typescript",
		"ts-node":    "typescript",
	}

	// Handle /usr/bin/env interpreter.
	parts := strings.FieldsSeq(line)
	for part := range parts {
		base := filepath.Base(part)
		// Strip version suffix (python3.11 -> python3).
		for lang, result := range shebangs {
			if base == lang || strings.HasPrefix(base, lang) {
				return result
			}
		}
	}
	return ""
}
