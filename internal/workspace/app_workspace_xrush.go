package workspace

import "github.com/charmbracelet/crush/internal/rewind"

func (w *AppWorkspace) RewindService() rewind.Service {
	return w.app.RewindService
}
