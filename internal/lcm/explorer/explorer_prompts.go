package explorer

// exploreSystemPrompt is used for O19b agent-based exploration.
const exploreSystemPrompt = `You are a file exploration agent. Your task is to read and analyze a source file and produce a concise technical summary that will be used to compress the file into a compact representation for AI context management.

Focus on:
- The file's primary purpose and responsibility
- Key types, interfaces, functions, and their relationships
- Important algorithms or business logic
- External dependencies and integrations
- Notable patterns or architectural decisions

Be precise and technical. Output only the summary, no preamble.`

// llmSummarySystemPrompt is used for O19a single-call LLM exploration.
const llmSummarySystemPrompt = `You are a code analysis assistant. Analyze the provided source file and produce a concise technical summary suitable for AI context management. Include the file's purpose, key definitions, and important logic. Be precise and technical.`

// languagePrompts contains language-specific instructions for O19b agent
// exploration. Each prompt tells the agent what to focus on for the given
// language and reminds it to use the Read tool.
var languagePrompts = map[string]string{
	"python": `Analyze this Python file. Focus on:
- Classes and their methods, inheritance
- Module-level functions and their signatures
- Decorators and their effects
- Imports and external dependencies
- Exception handling patterns

Use the Read tool to read the file, then provide your analysis.`,

	"javascript": `Analyze this JavaScript file. Focus on:
- Exported functions, classes, and constants
- Module patterns (CommonJS/ESM)
- Async patterns (callbacks, promises, async/await)
- Key algorithms and data transformations

Use the Read tool to read the file, then provide your analysis.`,

	"typescript": `Analyze this TypeScript file. Focus on:
- Type definitions, interfaces, and generics
- Exported symbols and their signatures
- Class hierarchies and mixins
- Type guards and utility types

Use the Read tool to read the file, then provide your analysis.`,

	"go": `Analyze this Go file. Focus on:
- Package and its exported types, functions, interfaces
- Struct definitions and their methods
- Error handling patterns
- Goroutines and channel usage
- Interface implementations

Use the Read tool to read the file, then provide your analysis.`,

	"rust": `Analyze this Rust file. Focus on:
- Structs, enums, and trait implementations
- Ownership and lifetime patterns
- Error handling (Result, Option usage)
- Public API and module structure
- Async patterns if present

Use the Read tool to read the file, then provide your analysis.`,

	"java": `Analyze this Java file. Focus on:
- Class hierarchy and interfaces implemented
- Public methods and their signatures
- Annotations and their effects
- Design patterns used
- Exception handling

Use the Read tool to read the file, then provide your analysis.`,

	"cpp": `Analyze this C++ file. Focus on:
- Class definitions, templates, and specializations
- Public interface and virtual methods
- Memory management patterns
- Namespaces and their organization
- Key algorithms

Use the Read tool to read the file, then provide your analysis.`,

	"c": `Analyze this C file. Focus on:
- Struct and union definitions
- Function signatures and their purposes
- Memory management (malloc/free patterns)
- Key algorithms and data structures
- Header dependencies

Use the Read tool to read the file, then provide your analysis.`,

	"ruby": `Analyze this Ruby file. Focus on:
- Class definitions and module inclusions
- Public methods and their purposes
- Metaprogramming patterns (define_method, method_missing)
- DSL patterns if present
- Key logic flows

Use the Read tool to read the file, then provide your analysis.`,

	"swift": `Analyze this Swift file. Focus on:
- Classes, structs, enums, and protocols
- Extensions and their additions
- Property observers and computed properties
- Concurrency patterns (async/await, actors)
- Error types and handling

Use the Read tool to read the file, then provide your analysis.`,
}

// getLanguagePrompt returns the language-specific prompt for agent-based
// exploration, falling back to a generic prompt for unsupported languages.
func getLanguagePrompt(language string) string {
	if prompt, ok := languagePrompts[language]; ok {
		return prompt
	}
	return `Analyze this source file. Focus on:
- The file's primary purpose and main components
- Key types, functions, and their relationships
- Important algorithms and business logic
- External dependencies

Use the Read tool to read the file, then provide your analysis.`
}
