//go:build treesitter

package tools

// DiffOp represents a single operation in a diff edit script.
type DiffOp int

const (
	DiffKeep   DiffOp = iota // Anchor unchanged.
	DiffInsert               // Anchor inserted in new sequence.
	DiffDelete               // Anchor removed from old sequence.
)

// AnchorDrift describes a single change between old and new anchor sequences.
type AnchorDrift struct {
	Index   int    // Position in the old anchor sequence.
	Op      DiffOp // Keep, Insert, or Delete.
	OldHash uint64 // Original hash (0 for Insert).
	NewHash uint64 // New hash (0 for Delete).
	Shift   int    // Line number shift (positive = moved down).
}

// AnchorStateManager tracks anchor hash maps across file modifications and
// detects drift between snapshots using Myers diff on hash sequences.
type AnchorStateManager struct {
	states map[string][]HashAnchor // fileID → last captured anchors.
}

// NewAnchorStateManager creates a new AnchorStateManager.
func NewAnchorStateManager() *AnchorStateManager {
	return &AnchorStateManager{
		states: make(map[string][]HashAnchor),
	}
}

// CaptureState snapshots the current anchors for a file.
func (m *AnchorStateManager) CaptureState(fileID string, anchors []HashAnchor) {
	cp := make([]HashAnchor, len(anchors))
	copy(cp, anchors)
	m.states[fileID] = cp
}

// DetectDrift compares current anchors against the last captured state using
// Myers diff. Returns AnchorDrift entries describing what changed.
func (m *AnchorStateManager) DetectDrift(fileID string, current []HashAnchor) ([]AnchorDrift, error) {
	old, ok := m.states[fileID]
	if !ok {
		return nil, nil
	}

	oldHashes := make([]uint64, len(old))
	for i, a := range old {
		oldHashes[i] = a.Hash
	}
	newHashes := make([]uint64, len(current))
	for i, a := range current {
		newHashes[i] = a.Hash
	}

	ops := myersDiffHashes(oldHashes, newHashes)

	var drifts []AnchorDrift
	oldIdx, newIdx := 0, 0
	for _, op := range ops {
		switch op {
		case DiffKeep:
			shift := 0
			if oldIdx < len(old) && newIdx < len(current) {
				shift = current[newIdx].LineNum - old[oldIdx].LineNum
			}
			drifts = append(drifts, AnchorDrift{
				Index:   oldIdx,
				Op:      DiffKeep,
				OldHash: old[oldIdx].Hash,
				NewHash: current[newIdx].Hash,
				Shift:   shift,
			})
			oldIdx++
			newIdx++
		case DiffInsert:
			drifts = append(drifts, AnchorDrift{
				Index:   oldIdx,
				Op:      DiffInsert,
				OldHash: 0,
				NewHash: current[newIdx].Hash,
				Shift:   0,
			})
			newIdx++
		case DiffDelete:
			drifts = append(drifts, AnchorDrift{
				Index:   oldIdx,
				Op:      DiffDelete,
				OldHash: old[oldIdx].Hash,
				NewHash: 0,
				Shift:   0,
			})
			oldIdx++
		}
	}

	return drifts, nil
}

// Reconcile produces an updated anchor map from drift information, keeping
// only the anchors that survived (Keep or Insert operations).
func (m *AnchorStateManager) Reconcile(drifts []AnchorDrift) []HashAnchor {
	var result []HashAnchor
	for _, d := range drifts {
		if d.Op == DiffKeep || d.Op == DiffInsert {
			result = append(result, HashAnchor{
				Hash: d.NewHash,
			})
		}
	}
	return result
}

// myersDiffHashes computes the shortest edit script between old and new hash
// sequences using the Myers diff algorithm. Returns DiffOps (Keep/Insert/Delete).
func myersDiffHashes(old, new []uint64) []DiffOp {
	n := len(old)
	m := len(new)

	if n == 0 && m == 0 {
		return nil
	}
	if n == 0 {
		result := make([]DiffOp, m)
		for i := range result {
			result[i] = DiffInsert
		}
		return result
	}
	if m == 0 {
		result := make([]DiffOp, n)
		for i := range result {
			result[i] = DiffDelete
		}
		return result
	}

	max := n + m
	size := 2*max + 1
	offset := max

	v := make([]int, size)
	for i := range v {
		v[i] = -1
	}
	v[1+offset] = 0

	var vv [][]int

	for d := 0; d <= max; d++ {
		snap := make([]int, size)
		copy(snap, v)
		vv = append(vv, snap)

		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[k-1+offset] < v[k+1+offset]) {
				x = v[k+1+offset]
			} else {
				x = v[k-1+offset] + 1
			}

			y := x - k
			for x < n && y < m && old[x] == new[y] {
				x++
				y++
			}
			v[k+offset] = x

			if x >= n && y >= m {
				return myersBacktrack(vv, old, new, offset)
			}
		}
	}

	return nil
}

func myersBacktrack(vv [][]int, old, new []uint64, offset int) []DiffOp {
	n := len(old)
	m := len(new)
	x, y := n, m

	var ops []DiffOp

	for d := len(vv) - 1; d > 0; d-- {
		k := x - y

		var prevK int
		if k == -d || (k != d && vv[d][k-1+offset] < vv[d][k+1+offset]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}

		prevX := vv[d][prevK+offset]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			ops = append(ops, DiffKeep)
			x--
			y--
		}

		if x == prevX {
			ops = append(ops, DiffInsert)
			y--
		} else {
			ops = append(ops, DiffDelete)
			x--
		}
	}

	for x > 0 && y > 0 {
		ops = append(ops, DiffKeep)
		x--
		y--
	}
	for x > 0 {
		ops = append(ops, DiffDelete)
		x--
	}
	for y > 0 {
		ops = append(ops, DiffInsert)
		y--
	}

	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
		ops[i], ops[j] = ops[j], ops[i]
	}

	return ops
}
