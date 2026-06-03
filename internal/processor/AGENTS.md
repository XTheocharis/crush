# Processor Pipeline

Message processing pipeline that intercepts LLM input and output across four
sequential phases. 16 processors total; 1 excluded (structured_output),
15 configurable via `options.processors.list`, 3 active by default
(TokenLimiter, SystemPromptScrubber, PIIDetector).

## Structure

- `types.go` -- `Processor` interface, `ProcessorPhase`, `ProcessorAction`,
  `ProcessorContext`, `ProcessorResult`, `TripWire`
- `runner.go` -- `ProcessorRunner`: chains processors per phase, accumulates
  state, supports TripWire abort
- `testutil.go` -- exported test helpers: `MockProcessor`, `MockLLMClient`,
  `ProcessorTestSuite`, fixtures
- `<name>.go` -- one file per processor implementation

## Pipeline Phases

Phases execute in order via `ProcessorRunner.Execute()` or `RunAll()`:

1. **InputPhase** -- messages before they reach the LLM
2. **OutputStreamPhase** -- streaming output chunks
3. **OutputResultPhase** -- final assembled output
4. **APIErrorPhase** -- errors returned by the LLM API

`ProcessorRunner` has three processor slices:
`InputProcessors`, `OutputProcessors`, `ErrorProcessors`. The runner maps
phases to slices: Input goes to InputProcessors, OutputStream and
OutputResult both go to OutputProcessors, APIError goes to ErrorProcessors.

## Processor Interface

```go
type Processor interface {
    ID() string
    ProcessInput(ctx, ProcessorContext) (ProcessorResult, error)
    ProcessOutputStream(ctx, ProcessorContext) (ProcessorResult, error)
    ProcessOutputResult(ctx, ProcessorContext) (ProcessorResult, error)
    ProcessAPIError(ctx, ProcessorContext) (ProcessorResult, error)
}
```

Return `ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}` for
phases you don't handle. Use `ActionRewrite` when modifying messages, or
`ActionAbort` to halt the chain.

Registration happens via `RunnerOption` functions: `WithInputProcessors()`,
`WithOutputProcessors()`, `WithErrorProcessors()`.

## Enabled Processors

| Processor | Phase(s) | Notes |
|-----------|----------|-------|
| `TokenLimiter` | Input | Drops oldest messages when token budget (~4 chars/token) exceeded |
| `SystemPromptScrubber` | OutputStream, OutputResult | Noop without LLM client; sends output to LLM to detect/strip leaked system prompts |
| `PIIDetector` | Input, OutputStream, OutputResult | Regex PII redaction (SSN, CC, email, phone). LLM-based contextual detection at high sensitivity |

## Config-Activatable Processors (15)

These are the processors that can be enabled via
`options.processors.list` in `crush.json`. Three of them are active by
default (marked with *):

TokenLimiter*, SystemPromptScrubber*, PIIDetector*, UnicodeNormalizer,
BatchParts, MessageSelection, ToolCallFilter, ToolSearch, Skills,
SkillSearch, ModerationProcessor, PromptInjectionDetector,
LanguageDetector, WorkspaceInstructions, MessageHistory

## Excluded (1)

`StructuredOutput` is excluded from configuration.

## Testing

`testutil.go` exports everything needed for processor tests (same package, not
a separate testutil package):

- `MockProcessor` -- per-phase configurable callbacks
- `MockLLMClient` -- records calls, returns preset responses
- `ProcessorTestSuite` -- full lifecycle harness with phase execution tracking
- `RunAllPhases()` -- runs a single processor through all four phases
- `NewTestContext()` -- prefilled `ProcessorContext` with sample messages
- `MessageFactory` -- fluent builder for realistic conversation sequences
- `TestFixtures()` -- named inputs covering PII, toxic, injection, and normal
  content
