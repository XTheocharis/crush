package lcm

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSentinelErrors(t *testing.T) {
	t.Parallel()

	all := []error{
		// Validation.
		ErrStoreIsNil,
		ErrSessionIDEmpty,
		ErrLLMClientNil,
		ErrNoCompressor,
		ErrNoRetrievalStore,
		// Storage.
		ErrStorageTransaction,
		ErrStorageQuery,
		ErrStorageScan,
		ErrStorageNotFound,
		ErrStorageWrite,
		// Compaction.
		ErrCompactionStalled,
		ErrCompactionMaxRounds,
		ErrBudgetUnavailable,
		ErrCompactionAborted,
		ErrLayerFailed,
		// Retrieval.
		ErrSummaryNotFound,
		ErrFileNotInSession,
		ErrExpansionFailed,
		ErrInvalidBlockID,
		ErrDecompressFailed,
	}

	for _, sentinel := range all {
		wrapped := fmt.Errorf("context: %w", sentinel)
		require.True(t, errors.Is(wrapped, sentinel),
			"errors.Is should match wrapped sentinel %v", sentinel)
		require.True(t, errors.Is(fmt.Errorf("outer: %w", wrapped), sentinel),
			"errors.Is should match doubly-wrapped sentinel %v", sentinel)
	}
}

func TestSentinelErrorDistinctness(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, 20)
	all := []error{
		ErrStoreIsNil, ErrSessionIDEmpty, ErrLLMClientNil, ErrNoCompressor, ErrNoRetrievalStore,
		ErrStorageTransaction, ErrStorageQuery, ErrStorageScan, ErrStorageNotFound, ErrStorageWrite,
		ErrCompactionStalled, ErrCompactionMaxRounds, ErrBudgetUnavailable, ErrCompactionAborted, ErrLayerFailed,
		ErrSummaryNotFound, ErrFileNotInSession, ErrExpansionFailed, ErrInvalidBlockID, ErrDecompressFailed,
	}
	for _, err := range all {
		msg := err.Error()
		_, dup := seen[msg]
		require.False(t, dup, "duplicate error message: %s", msg)
		seen[msg] = struct{}{}
	}
	require.Len(t, seen, 20, "expected exactly 20 distinct sentinel errors")
}
