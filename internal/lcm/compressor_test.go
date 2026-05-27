package lcm

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRangeCompression_Name(t *testing.T) {
	t.Parallel()
	s := NewRangeCompression(&mockLLMClient{})
	require.Equal(t, "range", s.Name())
}

func TestRangeCompression_EstimateRatio(t *testing.T) {
	t.Parallel()
	s := NewRangeCompression(&mockLLMClient{})
	r := s.EstimateRatio()
	require.Greater(t, r, 0.0)
	require.LessOrEqual(t, r, 1.0)
}

func TestRangeCompression_Compress_EmptyInput(t *testing.T) {
	t.Parallel()
	s := NewRangeCompression(&mockLLMClient{})
	out, err := s.Compress(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, "", out.Content)
	require.Equal(t, 1.0, out.Ratio)
	require.Equal(t, "range", out.Strategy)
}

func TestRangeCompression_Compress_NonEmpty(t *testing.T) {
	t.Parallel()
	mock := &mockLLMClient{
		response: "lines 1-5: function main\nlines 6-10: helper func",
	}
	s := NewRangeCompression(mock)
	input := "func main() {\n\tfmt.Println(\"hello\")\n}\n\nfunc helper() {\n\treturn\n}"
	out, err := s.Compress(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "lines 1-5: function main\nlines 6-10: helper func", out.Content)
	require.Equal(t, "range", out.Strategy)
	require.Greater(t, out.Ratio, 0.0)
	require.LessOrEqual(t, out.Ratio, 1.0)
	require.Equal(t, 1, mock.callCount)
}

func TestRangeCompression_Compress_LLMError(t *testing.T) {
	t.Parallel()
	mock := &mockLLMClient{err: fmt.Errorf("unavailable")}
	s := NewRangeCompression(mock)
	_, err := s.Compress(context.Background(), "some input")
	require.Error(t, err)
	require.Contains(t, err.Error(), "range compression")
}

func TestMessageCompression_Name(t *testing.T) {
	t.Parallel()
	s := NewMessageCompression(&mockLLMClient{})
	require.Equal(t, "message", s.Name())
}

func TestMessageCompression_EstimateRatio(t *testing.T) {
	t.Parallel()
	s := NewMessageCompression(&mockLLMClient{})
	r := s.EstimateRatio()
	require.Greater(t, r, 0.0)
	require.LessOrEqual(t, r, 1.0)
}

func TestMessageCompression_Compress_EmptyInput(t *testing.T) {
	t.Parallel()
	s := NewMessageCompression(&mockLLMClient{})
	out, err := s.Compress(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, "", out.Content)
	require.Equal(t, 1.0, out.Ratio)
	require.Equal(t, "message", out.Strategy)
}

func TestMessageCompression_Compress_NonEmpty(t *testing.T) {
	t.Parallel()
	mock := &mockLLMClient{
		response: "Decision: use SQLite for storage. Fixed auth bug in login.go.",
	}
	s := NewMessageCompression(mock)
	input := "I think we should use SQLite for the storage layer because it's simple and reliable. " +
		"Also, I fixed the authentication bug in login.go by adding a nil check."
	out, err := s.Compress(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "Decision: use SQLite for storage. Fixed auth bug in login.go.", out.Content)
	require.Equal(t, "message", out.Strategy)
	require.Greater(t, out.Ratio, 0.0)
	require.LessOrEqual(t, out.Ratio, 1.0)
	require.Equal(t, 1, mock.callCount)
}

func TestMessageCompression_Compress_LLMError(t *testing.T) {
	t.Parallel()
	mock := &mockLLMClient{err: fmt.Errorf("timeout")}
	s := NewMessageCompression(mock)
	_, err := s.Compress(context.Background(), "some input")
	require.Error(t, err)
	require.Contains(t, err.Error(), "message compression")
}

func TestDedupCompression_Name(t *testing.T) {
	t.Parallel()
	s := NewDedupCompression(&mockLLMClient{})
	require.Equal(t, "dedup", s.Name())
}

func TestDedupCompression_EstimateRatio(t *testing.T) {
	t.Parallel()
	s := NewDedupCompression(&mockLLMClient{})
	r := s.EstimateRatio()
	require.Greater(t, r, 0.0)
	require.LessOrEqual(t, r, 1.0)
}

func TestDedupCompression_Compress_EmptyInput(t *testing.T) {
	t.Parallel()
	s := NewDedupCompression(&mockLLMClient{})
	out, err := s.Compress(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, "", out.Content)
	require.Equal(t, 1.0, out.Ratio)
	require.Equal(t, "dedup", out.Strategy)
}

func TestDedupCompression_Compress_NonEmpty(t *testing.T) {
	t.Parallel()
	mock := &mockLLMClient{
		response: "Config uses SQLite. Auth fixed in login.go.",
	}
	s := NewDedupCompression(mock)
	input := "We decided to use SQLite for storage. " +
		"Storage decision: SQLite. " +
		"Fixed auth bug in login.go. " +
		"Also fixed the login.go auth issue."
	out, err := s.Compress(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "Config uses SQLite. Auth fixed in login.go.", out.Content)
	require.Equal(t, "dedup", out.Strategy)
	require.Greater(t, out.Ratio, 0.0)
	require.LessOrEqual(t, out.Ratio, 1.0)
	require.Equal(t, 1, mock.callCount)
}

func TestDedupCompression_Compress_LLMError(t *testing.T) {
	t.Parallel()
	mock := &mockLLMClient{err: fmt.Errorf("unavailable")}
	s := NewDedupCompression(mock)
	_, err := s.Compress(context.Background(), "some input")
	require.Error(t, err)
	require.Contains(t, err.Error(), "dedup compression")
}

func TestPurgeErrorsCompression_Name(t *testing.T) {
	t.Parallel()
	s := NewPurgeErrorsCompression(&mockLLMClient{})
	require.Equal(t, "purge_errors", s.Name())
}

func TestPurgeErrorsCompression_EstimateRatio(t *testing.T) {
	t.Parallel()
	s := NewPurgeErrorsCompression(&mockLLMClient{})
	r := s.EstimateRatio()
	require.Greater(t, r, 0.0)
	require.LessOrEqual(t, r, 1.0)
}

func TestPurgeErrorsCompression_Compress_EmptyInput(t *testing.T) {
	t.Parallel()
	s := NewPurgeErrorsCompression(&mockLLMClient{})
	out, err := s.Compress(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, "", out.Content)
	require.Equal(t, 1.0, out.Ratio)
	require.Equal(t, "purge_errors", out.Strategy)
}

func TestPurgeErrorsCompression_Compress_NonEmpty(t *testing.T) {
	t.Parallel()
	mock := &mockLLMClient{
		response: "Fixed: missing import in foo.go. Build succeeded.",
	}
	s := NewPurgeErrorsCompression(mock)
	input := "Error: undefined reference to 'fmt' in foo.go\n" +
		"Stack trace:\n" +
		"  at foo.go:12\n" +
		"  at main.go:5\n" +
		"Fixed: added 'import \"fmt\"' to foo.go. Build now succeeds."
	out, err := s.Compress(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "Fixed: missing import in foo.go. Build succeeded.", out.Content)
	require.Equal(t, "purge_errors", out.Strategy)
	require.Greater(t, out.Ratio, 0.0)
	require.LessOrEqual(t, out.Ratio, 1.0)
	require.Equal(t, 1, mock.callCount)
}

func TestPurgeErrorsCompression_Compress_LLMError(t *testing.T) {
	t.Parallel()
	mock := &mockLLMClient{err: fmt.Errorf("timeout")}
	s := NewPurgeErrorsCompression(mock)
	_, err := s.Compress(context.Background(), "some input")
	require.Error(t, err)
	require.Contains(t, err.Error(), "purge-errors compression")
}

func TestCompressionRatio(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		output string
		want   float64
	}{
		{"empty input", "", "output", 1.0},
		{"equal length", "abc", "xyz", 1.0},
		{"half length", "abcdef", "abc", 0.5},
		{"output longer clamped", "ab", "abcdef", 1.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := compressionRatio(tc.input, tc.output)
			require.InDelta(t, tc.want, got, 0.001)
		})
	}
}

func TestAllStrategies_RatioOrdering(t *testing.T) {
	t.Parallel()
	mock := &mockLLMClient{response: "compressed"}
	strategies := []CompressionStrategy{
		NewRangeCompression(mock),
		NewMessageCompression(mock),
		NewDedupCompression(mock),
		NewPurgeErrorsCompression(mock),
	}
	for _, s := range strategies {
		r := s.EstimateRatio()
		require.Greater(t, r, 0.0, "strategy %s: ratio must be > 0", s.Name())
		require.LessOrEqual(t, r, 1.0, "strategy %s: ratio must be <= 1", s.Name())
	}
}

func TestAllStrategies_CompressNonEmptyInput(t *testing.T) {
	t.Parallel()
	input := strings.Repeat("Some input text with various details about the code. ", 10)
	mock := &mockLLMClient{response: "Compressed output"}
	strategies := []CompressionStrategy{
		NewRangeCompression(mock),
		NewMessageCompression(mock),
		NewDedupCompression(mock),
		NewPurgeErrorsCompression(mock),
	}
	for _, s := range strategies {
		out, err := s.Compress(context.Background(), input)
		require.NoError(t, err, "strategy %s: unexpected error", s.Name())
		require.NotEmpty(t, out.Content, "strategy %s: output should not be empty for non-empty input", s.Name())
		require.Equal(t, s.Name(), out.Strategy)
		require.Greater(t, out.Ratio, 0.0, "strategy %s: ratio must be > 0", s.Name())
		require.LessOrEqual(t, out.Ratio, 1.0, "strategy %s: ratio must be <= 1", s.Name())
	}
}

func TestStrategies_SatisfyInterface(t *testing.T) {
	t.Parallel()
	var _ CompressionStrategy = NewRangeCompression(nil)
	var _ CompressionStrategy = NewMessageCompression(nil)
	var _ CompressionStrategy = NewDedupCompression(nil)
	var _ CompressionStrategy = NewPurgeErrorsCompression(nil)
}
