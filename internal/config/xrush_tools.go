package config

import "sync"

// extensionToolNames returns tool names contributed by extensions.
// Set once during app bootstrap via RegisterExtensionToolNames.
// Nil means no extensions have registered tool names.
var (
	extensionToolNames   func() []string
	extensionToolNamesMu sync.Mutex
)

// RegisterExtensionToolNames sets the function that returns
// extension-contributed tool names. Must be called once during
// bootstrap, before SetupAgents().
func RegisterExtensionToolNames(fn func() []string) {
	extensionToolNamesMu.Lock()
	defer extensionToolNamesMu.Unlock()
	extensionToolNames = fn
}

// ResetExtensionToolNamesForTesting clears the extension tool names
// function for test isolation.
func ResetExtensionToolNamesForTesting() {
	extensionToolNamesMu.Lock()
	defer extensionToolNamesMu.Unlock()
	extensionToolNames = nil
}

// xrushToolNames returns the list of xrush-only tool names in
// alphabetical order for deterministic interleaving in allToolNames().
func xrushToolNames() []string {
	return []string{
		"agentic_map",
		"batch_edit",
		"lcm_active_context",
		"lcm_ancestry",
		"lcm_archive",
		"lcm_bindle",
		"lcm_compact",
		"lcm_describe",
		"lcm_dolt",
		"lcm_expand",
		"lcm_file_search",
		"lcm_grep",
		"lcm_lineage",
		"lcm_sprig",
		"lcm_time_query",
		"list_mcp_resources",
		"llm_map",
		"map_refresh",
		"multiedit",
		"productive_execute",
		"read_mcp_resource",
		"send_message",
		"sourcegraph",
		"swarm_execute",
		"synthetic_output",
		"task_stop",
		"team_create",
		"team_delete",
	}
}

// xrushReadOnlyTools returns the list of xrush-only read-only tools.
func xrushReadOnlyTools() []string {
	return []string{
		"lcm_grep",
		"lcm_describe",
		"lcm_expand",
		"lcm_compact",
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
		fork[6],  // lcm_compact
		fork[7],  // lcm_describe
		fork[8],  // lcm_dolt
		fork[9],  // lcm_expand
		fork[10], // lcm_file_search
		fork[11], // lcm_grep
		fork[12], // lcm_lineage
		fork[13], // lcm_sprig
		fork[14], // lcm_time_query
		fork[15], // list_mcp_resources
		fork[16], // llm_map
		"ls",
		"lsp_diagnostics",
		"lsp_document_symbols",
		"lsp_references",
		"lsp_restart",
		"lsp_symbols",
		"lsp_workspace_symbols",
		fork[17], // map_refresh
		fork[18], // multiedit
		fork[19], // productive_execute
		fork[20], // read_mcp_resource
		fork[21], // send_message
		fork[22], // sourcegraph
		fork[23], // swarm_execute
		fork[24], // synthetic_output
		fork[25], // task_stop
		fork[26], // team_create
		fork[27], // team_delete
		"todos",
		"view",
		"write",
	}
	extensionToolNamesMu.Lock()
	ext := extensionToolNames
	extensionToolNamesMu.Unlock()
	if ext != nil {
		base = append(base, ext()...)
	}
	return base
}

// [XRUSH: end]
