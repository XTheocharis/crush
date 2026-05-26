package lcm

import "errors"

// Sentinel errors for the LCM package. Call sites wrap these with
// fmt.Errorf("context: %w", ErrSentinel) to allow errors.Is matching
// while preserving human-readable context.

var (
	// --- Validation errors (required preconditions) ---

	ErrStoreIsNil       = errors.New("lcm: store is required")
	ErrSessionIDEmpty   = errors.New("lcm: session ID must not be empty")
	ErrLLMClientNil     = errors.New("lcm: LLM client is required")
	ErrNoCompressor     = errors.New("lcm: no compressor configured")
	ErrNoRetrievalStore = errors.New("lcm: no retrieval store configured")

	// --- Storage errors (database / query failures) ---

	ErrStorageTransaction = errors.New("lcm: storage transaction failed")
	ErrStorageQuery       = errors.New("lcm: storage query failed")
	ErrStorageScan        = errors.New("lcm: storage scan failed")
	ErrStorageNotFound    = errors.New("lcm: storage entry not found")
	ErrStorageWrite       = errors.New("lcm: storage write failed")

	// --- Compaction errors (compression / summarization cycle) ---

	ErrCompactionStalled   = errors.New("lcm: compaction stalled")
	ErrCompactionMaxRounds = errors.New("lcm: compaction exceeded max rounds")
	ErrBudgetUnavailable   = errors.New("lcm: token budget unavailable")
	ErrCompactionAborted   = errors.New("lcm: compaction aborted")
	ErrLayerFailed         = errors.New("lcm: compaction layer failed")

	// --- Retrieval errors (summary lookup / expansion) ---

	ErrSummaryNotFound  = errors.New("lcm: summary not found")
	ErrFileNotInSession = errors.New("lcm: file not found in session")
	ErrExpansionFailed  = errors.New("lcm: summary expansion failed")
	ErrInvalidBlockID   = errors.New("lcm: invalid block ID")
	ErrDecompressFailed = errors.New("lcm: decompression failed")
)
