// [XRUSH: begin: rewind service and agent config restoration]
package app

import (
	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/extensions"
	"github.com/charmbracelet/crush/internal/rewind"
	"github.com/charmbracelet/crush/internal/session"
)

// [XRUSH: begin: initRewindService]
// initRewindService creates the rewind service from the database and config.
// Returns nil if snapshot config is missing (feature disabled).
func initRewindService(q db.Querier, sessions session.Service, store *config.ConfigStore) rewind.Service {
	cfg := store.Config()
	var opts []rewind.SnapshotterOption
	if cfg.Options.Snapshot != nil && cfg.Options.Snapshot.MaxPerSession > 0 {
		opts = append(opts, rewind.WithMaxPerSession(cfg.Options.Snapshot.MaxPerSession))
	}
	return rewind.NewService(q, sessions, store.WorkingDir(), opts...)
}
// [XRUSH: end]

// [XRUSH: begin: wireAgentConfigRestorer]
// WireAgentConfigRestorer connects the agent config restorer to the LCM
// manager for post-compaction skill restoration.
func wireAgentConfigRestorer(coord agent.Coordinator) {
	if mgr := extensions.TheLCMExtension.Manager(); mgr != nil {
		mgr.SetAgentConfigRestorer(coord)
	}
}
// [XRUSH: end]
// [XRUSH: end: rewind service and agent config restoration]
