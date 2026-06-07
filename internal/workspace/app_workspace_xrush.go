package workspace

import (
	"log/slog"

	"github.com/charmbracelet/crush/internal/extensions"
	"github.com/charmbracelet/crush/internal/rewind"
	"github.com/charmbracelet/crush/internal/session"
)

func (w *AppWorkspace) RewindService() rewind.Service {
	return w.app.RewindService
}

func (w *AppWorkspace) SetOperationalMemoryEnabled(enabled bool) error {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		return nil
	}

	if enabled {
		om := session.NewOperationalMemory(w.app.DB)
		mgr.SetOperationalMemory(om)
		mgr.SetOperationalMemoryEnabled(true)
		slog.Info("Operational memory enabled via command palette")
	} else {
		mgr.SetOperationalMemoryEnabled(false)
		slog.Info("Operational memory disabled via command palette")
	}
	return nil
}
