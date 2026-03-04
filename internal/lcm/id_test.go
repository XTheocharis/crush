package lcm

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGenerateSummaryID_Format(t *testing.T) {
	t.Parallel()
	id, ts := GenerateSummaryID("session-1")

	require.True(t, strings.HasPrefix(id, SummaryIDPrefix), "ID should start with %q, got %q", SummaryIDPrefix, id)
	// sum_ (4) + 16 hex chars = 20 total.
	require.Len(t, id, 4+16, "ID length should be 20, got %d", len(id))
	require.Greater(t, ts, int64(0), "timestamp should be positive")
}

func TestGenerateSummaryID_DifferentSessions(t *testing.T) {
	t.Parallel()
	// Same timestamp but different session IDs should produce different IDs.
	// We can only test different sessions generate different IDs.
	id1, _ := GenerateSummaryID("session-a")
	id2, _ := GenerateSummaryID("session-b")
	require.NotEqual(t, id1, id2, "different sessions should produce different IDs")
}

func TestGenerateSummaryID_UniqueOverTime(t *testing.T) {
	t.Parallel()
	// Generate IDs with small sleeps to ensure unique timestamps.
	seen := make(map[string]struct{}, 10)
	for range 10 {
		id, _ := GenerateSummaryID("session-uniq")
		_, dup := seen[id]
		require.False(t, dup, "duplicate summary ID: %s", id)
		seen[id] = struct{}{}
		time.Sleep(time.Millisecond)
	}
}

func TestGenerateFileID_Format(t *testing.T) {
	t.Parallel()
	id := GenerateFileID("session-1", "hello world")

	require.True(t, strings.HasPrefix(id, FileIDPrefix), "ID should start with %q, got %q", FileIDPrefix, id)
	require.Len(t, id, 5+16, "ID length should be 21 (file_ + 16 hex)")
}

func TestGenerateFileID_Deterministic(t *testing.T) {
	t.Parallel()
	id1 := GenerateFileID("sess", "content")
	id2 := GenerateFileID("sess", "content")
	require.Equal(t, id1, id2, "same inputs should produce the same file ID")
}

func TestGenerateFileID_DifferentInputs(t *testing.T) {
	t.Parallel()
	id1 := GenerateFileID("sess", "content-a")
	id2 := GenerateFileID("sess", "content-b")
	require.NotEqual(t, id1, id2, "different content should produce different IDs")

	id3 := GenerateFileID("sess-x", "content")
	id4 := GenerateFileID("sess-y", "content")
	require.NotEqual(t, id3, id4, "different sessions should produce different IDs")
}

func TestGenerateFileID_HexCharsOnly(t *testing.T) {
	t.Parallel()
	id := GenerateFileID("test-session", "test-content")
	// Remove prefix.
	hex := strings.TrimPrefix(id, FileIDPrefix)
	for _, c := range hex {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		require.True(t, isHex, "file ID should contain only hex chars, got %c", c)
	}
}

func TestGenerateSummaryID_HexCharsOnly(t *testing.T) {
	t.Parallel()
	id, _ := GenerateSummaryID("test-session")
	hex := strings.TrimPrefix(id, SummaryIDPrefix)
	for _, c := range hex {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		require.True(t, isHex, "summary ID should contain only hex chars, got %c", c)
	}
}

func BenchmarkGenerateSummaryID(b *testing.B) {
	for b.Loop() {
		GenerateSummaryID("bench-session")
	}
}

func BenchmarkGenerateFileID(b *testing.B) {
	content := strings.Repeat("x", 1000)
	for b.Loop() {
		GenerateFileID("bench-session", content)
	}
}
