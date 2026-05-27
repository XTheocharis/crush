package config

// extensionToolNames returns tool names contributed by extensions.
// Set once during app bootstrap via RegisterExtensionToolNames.
// Nil means no extensions have registered tool names.
var extensionToolNames func() []string

// RegisterExtensionToolNames sets the function that returns
// extension-contributed tool names. Must be called once during
// bootstrap, before SetupAgents(). Not safe for concurrent use.
func RegisterExtensionToolNames(fn func() []string) {
	extensionToolNames = fn
}

// ResetExtensionToolNamesForTesting clears the extension tool names
// function for test isolation.
func ResetExtensionToolNamesForTesting() {
	extensionToolNames = nil
}

// xrushToolNames returns the list of xrush-only tool names in the
// original order they appeared in allToolNames().
func xrushToolNames() []string {
	return []string{
		"agentic_map",
		"batch_edit",
		"lcm_describe",
		"lcm_expand",
		"lcm_grep",
		"llm_map",
		"map_refresh",
		"multiedit",
		"read_mcp_resource",
		"send_message",
		"sourcegraph",
		"synthetic_output",
		"task_stop",
		"team_create",
		"team_delete",
		"list_mcp_resources",
	}
}

// xrushReadOnlyTools returns the list of xrush-only read-only tools.
func xrushReadOnlyTools() []string {
	return []string{"lcm_grep", "lcm_describe", "lcm_expand"}
}

// [XRUSH: begin: allToolNames rewritten to integrate xrush tools via xrushToolNames()]
func allToolNames() []string {
	fork := xrushToolNames()
	base := []string{
		"agent",
		"agentic_fetch",
		fork[0],
		"bash",
		fork[1],
		"crush_info",
		"crush_logs",
		"download",
		"edit",
		"fetch",
		"glob",
		"grep",
		"job_kill",
		"job_output",
		fork[2],
		fork[3],
		fork[4],
		fork[5],
		"ls",
		"lsp_diagnostics",
		"lsp_references",
		"lsp_restart",
		fork[6],
		fork[7],
		fork[8],
		fork[9],
		fork[10],
		fork[11],
		fork[12],
		fork[13],
		fork[14],
		"todos",
		"view",
		"write",
		fork[15],
	}
	if extensionToolNames != nil {
		base = append(base, extensionToolNames()...)
	}
	return base
}

// [XRUSH: end]
