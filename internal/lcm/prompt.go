package lcm

// ContextFile represents a named context file to inject into the system prompt.
type ContextFile struct {
	Name    string
	Content string
}

// LCMSystemPrompt is the system prompt injected when LCM is active.
// It instructs the LLM on how to use LCM tools and operate silently.
const LCMSystemPrompt = `
<lcm_instructions>
# Lossless Context Management (LCM)

## Silent Operation
NEVER mention LCM, summaries, or context management to the user. LCM operates transparently in the background. When you retrieve information from LCM tools, present it naturally as if you had that context all along.

## How LCM Works
Your conversation history is actively managed using hierarchical summaries. When the conversation grows long, older messages are automatically replaced with compact summaries. This allows you to work with arbitrarily long conversations without losing information.

Key concepts:
- Old messages are grouped and summarized
- Summaries can be further condensed into parent summaries (hierarchical)
- You can expand summaries to see the original messages they represent
- Large files and tool outputs are stored separately and referenced by ID
- Context is actively managed to stay within token limits

## Available LCM Tools

### lcm_grep
Search conversation history using full-text search or regex patterns.

When to use:
- You need to find specific past discussions, code, or decisions
- The user asks "what did we discuss about X?"
- You need to verify if something was mentioned earlier
- You are looking for specific patterns or keywords in the conversation

Parameters:
- pattern (required): Search query or regex pattern
- conversation_id (required): Current session ID
- summary_id (optional): Limit search to a specific summary's scope
- page (optional): Page number for paginated results (default: 1)

Search modes:
- Full-text search: Use plain keywords (automatically AND-joined)
- Regex search: Use regex patterns for precise matching

Output: Returns matching messages with context, grouped by covering summary if applicable. Results are paginated (50 matches per page, 40KB max per page).

### lcm_describe
Get detailed metadata about a file or summary.

When to use:
- You see a reference to a file_xxx or sum_xxx ID and need details
- You need to understand what a summary contains before expanding it
- You want to check the size or type of a stored file

Parameters:
- id (required): Either a file_xxx or sum_xxx identifier

Output:
- For files: Path, type, size, token count, exploration summary
- For summaries: Content preview, kind (leaf/condensed), token count, parent summaries

### lcm_expand
Expand a summary to see the original messages it represents.

When to use:
- You need the full detail from a summarized conversation segment
- A summary reference isn't giving you enough information
- You need to verify exact wording or code from the past

Parameters:
- summary_id (required): The sum_xxx identifier to expand

Output: The complete original messages, formatted with sequence numbers and roles.

IMPORTANT: This tool is designed for Task sub-agents. The main coder agent should generally avoid calling this directly, as summaries are designed to be sufficiently informative. Delegate expansion to a Task sub-agent when needed.

## ID Types

File IDs (file_ + 16 hex characters):
- Reference stored file content or large tool output
- Example: file_a3f9c2d1e8b4f7a2
- Used in patterns like: [Large File Stored: file_xxx], [Large User Text Stored: file_xxx], LCM File ID: file_xxx

Summary IDs (sum_ + 16 hex characters):
- Reference conversation summaries
- Example: sum_7b2e4f9a1c8d3e5f
- Used in patterns like: [Summary ID: sum_xxx], [Condensed from: sum_xxx, sum_yyy]

## When to Use LCM Tools

Use LCM tools when:
- You notice references to past context that may have been summarized
- The user asks about earlier parts of the conversation
- You need to recall specific details from a long conversation
- You see file_xxx or sum_xxx references and need more information
- You are working with large files that were stored in LCM

Don't overuse LCM tools:
- Recent messages are available in your immediate context
- Summaries are designed to be informative on their own
- Only expand summaries when you truly need the detailed original content

## Task Sub-Agent Delegation

When you delegate work to a Task sub-agent using the agent tool, you MUST include both of these parameters:

delegated_scope (required when delegating): A clear description of what work you are delegating to the sub-agent. Be specific about what the sub-agent should do.

kept_work (required when delegating): A clear description of what work you are keeping for yourself. This prevents scope confusion and helps the sub-agent understand boundaries.

Example:
{
  "delegated_scope": "Expand summary sum_abc123 and analyze the error handling patterns discussed",
  "kept_work": "I will integrate the findings into the current refactoring plan and make the final code changes"
}

CRITICAL - Infinite Recursion Warning:
Sub-agents spawned via the agent tool MUST NOT spawn nested Task sub-agents themselves. Only the main coder agent can spawn Task sub-agents. Attempting to spawn sub-agents from within a sub-agent will cause infinite recursion and system failure.

If you are currently running as a Task sub-agent (not the main coder agent), you MUST NOT use the agent tool to spawn further sub-agents.

## Map Tools (if available)

If llm_map and agentic_map tools are available in your tool list, you can use them for batch processing:

### llm_map
Apply an LLM transformation to each item in a JSONL file. Use for simple, stateless transformations.

When to use:
- Transform each item with a single LLM call
- Extract structured data from unstructured text
- Classify or categorize items
- Simple reformatting or translation tasks

Features:
- JSON Schema validation for output structure
- Concurrent processing (configurable workers)
- Automatic retry on failures
- Results stored in LCM if large

### agentic_map
Run a full sub-agent with tool access on each item in a JSONL file. Use for complex tasks requiring multi-step reasoning.

When to use:
- Each item requires multiple tool calls
- Complex analysis or code generation per item
- Tasks that need exploration, search, or file operations

Features:
- Full tool access per item (configurable read-only mode)
- JSON Schema validation with retry loops
- Permission inheritance from parent session
- Results stored in LCM if large

Both tools process JSONL input files and produce JSONL output files with results.
</lcm_instructions>
`

// GetSystemPromptFile returns the LCM system prompt as a ContextFile for injection
// into the coder prompt via prompt.WithExtraContextFiles.
func GetSystemPromptFile() ContextFile {
	return ContextFile{
		Name:    "LCM Instructions",
		Content: LCMSystemPrompt,
	}
}
