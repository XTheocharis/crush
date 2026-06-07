package workspace

import "github.com/charmbracelet/crush/internal/rewind" // XRUSH: rewind service

func (w *ClientWorkspace) RewindService() rewind.Service {
	return nil
}

func (w *ClientWorkspace) SetOperationalMemoryEnabled(_ bool) error {
	return nil
}
