package app

import (
	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/extensions"
)

// wireAgentConfigRestorer connects the agent config restorer to the LCM
// manager for post-compaction skill restoration.
func wireAgentConfigRestorer(coord agent.Coordinator) {
	if mgr := extensions.TheLCMExtension.Manager(); mgr != nil {
		mgr.SetAgentConfigRestorer(coord)
	}
}
