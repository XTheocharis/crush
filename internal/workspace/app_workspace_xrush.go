package workspace

import "github.com/charmbracelet/crush/internal/rewind" // XRUSH: rewind service

// [XRUSH: begin: rewind stub for non-rewind builds]
func (w *AppWorkspace) RewindService() rewind.Service {
	return nil
}

// [XRUSH: end]
