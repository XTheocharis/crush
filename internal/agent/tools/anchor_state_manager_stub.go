//go:build !treesitter

package tools

// StubDiffOp is a no-op placeholder for DiffOp when treesitter is not enabled.
type StubDiffOp int

// StubAnchorDrift is a no-op placeholder for AnchorDrift when treesitter is
// not enabled.
type StubAnchorDrift struct {
	Index   int
	Op      StubDiffOp
	OldHash uint64
	NewHash uint64
	Shift   int
}

// StubAnchorStateManager is a no-op placeholder for AnchorStateManager when
// treesitter is not enabled.
type StubAnchorStateManager struct{}

// NewAnchorStateManager returns a stub when treesitter is not enabled.
func NewAnchorStateManager() *StubAnchorStateManager {
	return &StubAnchorStateManager{}
}

func (m *StubAnchorStateManager) CaptureState(_ string, _ []HashAnchor) {}

func (m *StubAnchorStateManager) DetectDrift(_ string, _ []HashAnchor) ([]StubAnchorDrift, error) {
	return nil, nil
}

func (m *StubAnchorStateManager) Reconcile(_ []StubAnchorDrift) []HashAnchor {
	return nil
}
