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
		"lcm_active_context",
		"lcm_ancestry",
		"lcm_archive",
		"lcm_bindle",
		"lcm_describe",
		"lcm_dolt",
		"lcm_expand",
		"lcm_file_search",
		"lcm_grep",
		"lcm_lineage",
		"lcm_sprig",
		"lcm_time_query",
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
	return []string{
		"lcm_grep",
		"lcm_describe",
		"lcm_expand",
		"lcm_bindle",
		"lcm_ancestry",
		"lcm_dolt",
		"lcm_archive",
		"lcm_sprig",
		"lcm_time_query",
		"lcm_file_search",
		"lcm_active_context",
		"lcm_lineage",
	}
}

// [XRUSH: begin: allToolNames rewritten to integrate xrush tools via xrushToolNames()]
func allToolNames() []string {
	fork := xrushToolNames()
	base := []string{
		"agent",
		"agentic_fetch",
		fork[0], // agentic_map
		"bash",
		fork[1], // batch_edit
		"crush_info",
		"crush_logs",
		"download",
		"edit",
		"fetch",
		"glob",
		"grep",
		"job_kill",
		"job_output",
		fork[2],  // lcm_active_context
		fork[3],  // lcm_ancestry
		fork[4],  // lcm_archive
		fork[5],  // lcm_bindle
		fork[6],  // lcm_describe
		fork[7],  // lcm_dolt
		fork[8],  // lcm_expand
		fork[9],  // lcm_file_search
		fork[10], // lcm_grep
		fork[11], // lcm_lineage
		fork[12], // lcm_sprig
		fork[13], // lcm_time_query
		fork[24], // list_mcp_resources
		fork[14], // llm_map
		"ls",
		"lsp_diagnostics",
		"lsp_document_symbols",
		"lsp_references",
		"lsp_restart",
		"lsp_symbols",
		"lsp_workspace_symbols",
		fork[15], // map_refresh
		fork[16], // multiedit
		fork[17], // read_mcp_resource
		fork[18], // send_message
		fork[19], // sourcegraph
		fork[20], // synthetic_output
		fork[21], // task_stop
		fork[22], // team_create
		fork[23], // team_delete
		"todos",
		"view",
		"write",
	}
	if extensionToolNames != nil {
		base = append(base, extensionToolNames()...)
	}
	return base
}

// [XRUSH: end]
