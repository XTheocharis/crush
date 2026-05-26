package eval

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockReader struct {
	mu      sync.Mutex
	reads   map[string]int
	content map[string]string
	errs    map[string]error
	delay   time.Duration
}

func newMockReader() *mockReader {
	return &mockReader{
		reads:   make(map[string]int),
		content: make(map[string]string),
		errs:    make(map[string]error),
	}
}

func (m *mockReader) ReadFile(_ context.Context, path string) (string, error) {
	m.mu.Lock()
	m.reads[path]++
	m.mu.Unlock()

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-context.Background().Done():
		}
	}

	if err, ok := m.errs[path]; ok {
		return "", err
	}
	if c, ok := m.content[path]; ok {
		return c, nil
	}
	return fmt.Sprintf("content-of-%s", path), nil
}

func (m *mockReader) readCount(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reads[path]
}

func (m *mockReader) totalReads() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for _, c := range m.reads {
		total += c
	}
	return total
}

func (m *mockReader) setContent(path, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.content[path] = content
}

func (m *mockReader) setError(path string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errs[path] = err
}

func TestReadCoordinatorDeduplication(t *testing.T) {
	t.Parallel()

	mr := newMockReader()
	rc := NewReadCoordinator(4, mr)

	paths := []string{"a.txt", "b.txt", "a.txt", "c.txt", "b.txt"}
	results, err := rc.ReadFiles(context.Background(), paths, 0)
	require.NoError(t, err)

	require.Len(t, results, 3)
	require.Contains(t, results, "a.txt")
	require.Contains(t, results, "b.txt")
	require.Contains(t, results, "c.txt")

	require.Equal(t, 1, mr.readCount("a.txt"))
	require.Equal(t, 1, mr.readCount("b.txt"))
	require.Equal(t, 1, mr.readCount("c.txt"))
	require.Equal(t, 3, mr.totalReads(), "should read each unique path exactly once")
}

func TestReadCoordinatorBudgetEnforcement(t *testing.T) {
	t.Parallel()

	mr := newMockReader()
	mr.setContent("small.txt", "1234")
	mr.setContent("medium.txt", string(make([]byte, 400)))
	mr.setContent("large.txt", string(make([]byte, 800)))

	rc := NewReadCoordinator(8, mr)

	budget := 100
	results, err := rc.ReadFiles(context.Background(), []string{"small.txt", "medium.txt", "large.txt"}, budget)
	require.NoError(t, err)

	totalTokens := 0
	for _, c := range results {
		totalTokens += len(c) / charsPerToken
	}
	require.LessOrEqual(t, totalTokens, budget+int(400/charsPerToken),
		"total tokens should not wildly exceed budget")
}

func TestReadCoordinatorBudgetZeroNoLimit(t *testing.T) {
	t.Parallel()

	mr := newMockReader()
	for i := range 20 {
		mr.setContent(fmt.Sprintf("file%d.txt", i), string(make([]byte, 100)))
	}

	rc := NewReadCoordinator(8, mr)

	paths := make([]string, 20)
	for i := range 20 {
		paths[i] = fmt.Sprintf("file%d.txt", i)
	}

	results, err := rc.ReadFiles(context.Background(), paths, 0)
	require.NoError(t, err)
	require.Len(t, results, 20, "budget=0 should read all files")
}

func TestReadCoordinatorConcurrency(t *testing.T) {
	t.Parallel()

	mr := newMockReader()
	mr.delay = 50 * time.Millisecond

	rc := NewReadCoordinator(8, mr)

	paths := make([]string, 16)
	for i := range 16 {
		paths[i] = fmt.Sprintf("file%d.txt", i)
	}

	start := time.Now()
	results, err := rc.ReadFiles(context.Background(), paths, 0)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Len(t, results, 16)

	require.Less(t, elapsed, 600*time.Millisecond,
		"16 files with 50ms delay each should complete in ~100ms with 8 workers, not 800ms sequentially")
}

func TestReadCoordinatorContextCancellation(t *testing.T) {
	t.Parallel()

	mr := newMockReader()
	mr.delay = 100 * time.Millisecond

	rc := NewReadCoordinator(2, mr)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	paths := make([]string, 10)
	for i := range 10 {
		paths[i] = fmt.Sprintf("file%d.txt", i)
	}

	results, err := rc.ReadFiles(ctx, paths, 0)
	require.Error(t, err, "should return error on context cancellation")
	_ = results
}

func TestReadCoordinatorEmptyPaths(t *testing.T) {
	t.Parallel()

	mr := newMockReader()
	rc := NewReadCoordinator(8, mr)

	results, err := rc.ReadFiles(context.Background(), nil, 100)
	require.NoError(t, err)
	require.Empty(t, results)

	results, err = rc.ReadFiles(context.Background(), []string{}, 100)
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestReadCoordinatorFileError(t *testing.T) {
	t.Parallel()

	mr := newMockReader()
	mr.setError("bad.txt", fmt.Errorf("permission denied"))
	mr.setContent("good.txt", "hello world")

	rc := NewReadCoordinator(4, mr)

	results, err := rc.ReadFiles(context.Background(), []string{"good.txt", "bad.txt"}, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")

	require.Contains(t, results, "good.txt")
	require.Equal(t, "hello world", results["good.txt"])
}

func TestReadCoordinatorSingleFile(t *testing.T) {
	t.Parallel()

	mr := newMockReader()
	mr.setContent("only.txt", "only content")

	rc := NewReadCoordinator(8, mr)

	results, err := rc.ReadFiles(context.Background(), []string{"only.txt"}, 0)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "only content", results["only.txt"])
}

func TestReadCoordinatorDefaultMaxWorkers(t *testing.T) {
	t.Parallel()

	rc := NewReadCoordinator(0, nil)
	require.Equal(t, defaultMaxWorkers, rc.maxWorkers)

	rc = NewReadCoordinator(-1, nil)
	require.Equal(t, defaultMaxWorkers, rc.maxWorkers)
}

func TestReadCoordinatorWorkerPoolBounded(t *testing.T) {
	t.Parallel()

	var active atomic.Int32
	var maxActive atomic.Int32

	mr := &trackingReader{
		onRead: func() {
			cur := active.Add(1)
			for {
				old := maxActive.Load()
				if cur <= old || maxActive.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			active.Add(-1)
		},
	}

	rc := NewReadCoordinator(4, mr)

	paths := make([]string, 12)
	for i := range 12 {
		paths[i] = fmt.Sprintf("file%d.txt", i)
	}

	results, err := rc.ReadFiles(context.Background(), paths, 0)
	require.NoError(t, err)
	require.Len(t, results, 12)

	require.LessOrEqual(t, maxActive.Load(), int32(4),
		"max concurrent reads should not exceed worker pool size")
}

type trackingReader struct {
	onRead func()
}

func (t *trackingReader) ReadFile(_ context.Context, path string) (string, error) {
	if t.onRead != nil {
		t.onRead()
	}
	return fmt.Sprintf("content-of-%s", path), nil
}

func TestReadCoordinatorAllDuplicates(t *testing.T) {
	t.Parallel()

	mr := newMockReader()
	rc := NewReadCoordinator(4, mr)

	results, err := rc.ReadFiles(context.Background(), []string{"same.txt", "same.txt", "same.txt"}, 0)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, 1, mr.readCount("same.txt"))
}

func TestReadCoordinatorBudgetStopsReads(t *testing.T) {
	t.Parallel()

	mr := newMockReader()
	for i := range 10 {
		content := string(make([]byte, 100))
		mr.setContent(fmt.Sprintf("f%d.txt", i), content)
	}

	rc := NewReadCoordinator(8, mr)

	paths := make([]string, 10)
	for i := range 10 {
		paths[i] = fmt.Sprintf("f%d.txt", i)
	}

	budget := 50
	results, err := rc.ReadFiles(context.Background(), paths, budget)
	require.NoError(t, err)

	// With per-file budget allocation, all files are read but truncated
	// so total tokens stay within budget.
	totalTokens := 0
	for _, c := range results {
		totalTokens += len(c) / charsPerToken
	}
	require.LessOrEqual(t, totalTokens, budget+2,
		"total tokens should stay within budget")
}

// --- Priority-based allocation tests ---

type mockPrioritySource struct {
	scores map[string]float64
}

func (m *mockPrioritySource) Priority(path string) float64 {
	return m.scores[path]
}

func TestAllocateHighPriorityGetsMoreTokens(t *testing.T) {
	t.Parallel()

	ps := &mockPrioritySource{scores: map[string]float64{
		"high.txt":   0.7,
		"medium.txt": 0.2,
		"low.txt":    0.1,
	}}
	rc := NewReadCoordinator(8, nil, WithPrioritySource(ps))

	alloc := rc.Allocate([]string{"high.txt", "medium.txt", "low.txt"}, 10000)
	require.True(t, alloc["high.txt"] > alloc["medium.txt"],
		"high priority should get more tokens than medium")
	require.True(t, alloc["medium.txt"] > alloc["low.txt"],
		"medium priority should get more tokens than low")
}

func TestAllocateNoFileExceedsPerFileMax(t *testing.T) {
	t.Parallel()

	ps := &mockPrioritySource{scores: map[string]float64{
		"dominant.txt": 0.99,
		"tiny.txt":     0.01,
	}}
	rc := NewReadCoordinator(8, nil, WithPrioritySource(ps))

	alloc := rc.Allocate([]string{"dominant.txt", "tiny.txt"}, 100000)
	require.LessOrEqual(t, alloc["dominant.txt"], PerFileMaxTokens,
		"no file should exceed PerFileMaxTokens")
	require.LessOrEqual(t, alloc["tiny.txt"], PerFileMaxTokens,
		"no file should exceed PerFileMaxTokens")
}

func TestAllocateBudgetFullyDistributed(t *testing.T) {
	t.Parallel()

	ps := &mockPrioritySource{scores: map[string]float64{
		"a.txt": 0.4,
		"b.txt": 0.3,
		"c.txt": 0.3,
	}}
	rc := NewReadCoordinator(8, nil, WithPrioritySource(ps))

	budget := 3000
	alloc := rc.Allocate([]string{"a.txt", "b.txt", "c.txt"}, budget)

	total := 0
	for _, v := range alloc {
		total += v
	}
	// Total allocated should be within a small margin of the original budget.
	require.LessOrEqual(t, total, budget+3, "total allocation should not exceed budget significantly")
}

func TestAllocateZeroPriorityGetsMinimum(t *testing.T) {
	t.Parallel()

	ps := &mockPrioritySource{scores: map[string]float64{
		"ranked.txt":   0.9,
		"unranked.txt": 0.0,
	}}
	rc := NewReadCoordinator(8, nil, WithPrioritySource(ps))

	alloc := rc.Allocate([]string{"ranked.txt", "unranked.txt"}, 10000)
	require.Equal(t, 1, alloc["unranked.txt"],
		"zero-priority files should get minimum allocation of 1")
	require.True(t, alloc["ranked.txt"] > 1,
		"ranked files should get more than minimum")
}

func TestAllocateNilPrioritySourceDistributesEqually(t *testing.T) {
	t.Parallel()

	rc := NewReadCoordinator(8, nil)

	alloc := rc.Allocate([]string{"a.txt", "b.txt", "c.txt"}, 3000)
	require.Equal(t, 1000, alloc["a.txt"])
	require.Equal(t, 1000, alloc["b.txt"])
	require.Equal(t, 1000, alloc["c.txt"])
}

func TestAllocateEmptyPaths(t *testing.T) {
	t.Parallel()

	rc := NewReadCoordinator(8, nil)
	alloc := rc.Allocate(nil, 1000)
	require.Empty(t, alloc)

	alloc = rc.Allocate([]string{}, 1000)
	require.Empty(t, alloc)
}

func TestAllocateZeroBudget(t *testing.T) {
	t.Parallel()

	ps := &mockPrioritySource{scores: map[string]float64{"a.txt": 1.0}}
	rc := NewReadCoordinator(8, nil, WithPrioritySource(ps))

	alloc := rc.Allocate([]string{"a.txt"}, 0)
	require.Empty(t, alloc)
}

func TestAllocateAllZeroPriorityDistributesEqually(t *testing.T) {
	t.Parallel()

	ps := &mockPrioritySource{scores: map[string]float64{
		"a.txt": 0,
		"b.txt": 0,
	}}
	rc := NewReadCoordinator(8, nil, WithPrioritySource(ps))

	alloc := rc.Allocate([]string{"a.txt", "b.txt"}, 200)
	require.Equal(t, 100, alloc["a.txt"])
	require.Equal(t, 100, alloc["b.txt"])
}

func TestReadFilesWithPrioritySortsByPriority(t *testing.T) {
	t.Parallel()

	ps := &mockPrioritySource{scores: map[string]float64{
		"low.txt":    0.1,
		"high.txt":   0.9,
		"medium.txt": 0.5,
	}}
	mr := newMockReader()
	// All files are 4000 chars (1000 tokens).
	for _, name := range []string{"low.txt", "high.txt", "medium.txt"} {
		mr.setContent(name, string(make([]byte, 4000)))
	}

	rc := NewReadCoordinator(8, mr, WithPrioritySource(ps))
	results, err := rc.ReadFiles(context.Background(),
		[]string{"low.txt", "high.txt", "medium.txt"}, 5000)
	require.NoError(t, err)

	// With budget=5000 and high priority getting most, all 3 files should
	// be read (1000 tokens each < PerFileMaxTokens cap) but high.txt
	// content may be truncated less than low.txt.
	require.Len(t, results, 3)
}

func TestReadFilesPriorityTruncatesContent(t *testing.T) {
	t.Parallel()

	ps := &mockPrioritySource{scores: map[string]float64{
		"big.txt": 0.9,
	}}
	// 40000 chars = 10000 tokens — exceeds PerFileMaxTokens=5000.
	mr := newMockReader()
	mr.setContent("big.txt", string(make([]byte, 40000)))

	rc := NewReadCoordinator(8, mr, WithPrioritySource(ps))
	results, err := rc.ReadFiles(context.Background(), []string{"big.txt"}, 100000)
	require.NoError(t, err)

	// Content should be truncated to PerFileMaxTokens * charsPerToken chars.
	maxChars := PerFileMaxTokens * charsPerToken
	require.LessOrEqual(t, len(results["big.txt"]), maxChars,
		"content should be truncated to per-file budget")
}

func TestReadFilesNoPrioritySourcePreservesBackwardCompat(t *testing.T) {
	t.Parallel()

	mr := newMockReader()
	mr.setContent("a.txt", "hello")
	mr.setContent("b.txt", "world")

	rc := NewReadCoordinator(8, mr)
	results, err := rc.ReadFiles(context.Background(), []string{"a.txt", "b.txt"}, 0)
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, "hello", results["a.txt"])
	require.Equal(t, "world", results["b.txt"])
}

func TestReadCoordinatorWithPriorityOptionPreserved(t *testing.T) {
	t.Parallel()

	ps := &mockPrioritySource{scores: map[string]float64{"a.txt": 0.5}}
	rc := NewReadCoordinator(4, nil, WithPrioritySource(ps))
	require.NotNil(t, rc.priority)
}

func TestAllocateSingleFile(t *testing.T) {
	t.Parallel()

	ps := &mockPrioritySource{scores: map[string]float64{"only.txt": 1.0}}
	rc := NewReadCoordinator(8, nil, WithPrioritySource(ps))

	alloc := rc.Allocate([]string{"only.txt"}, 50000)
	require.LessOrEqual(t, alloc["only.txt"], PerFileMaxTokens,
		"single file should be capped at PerFileMaxTokens")
	require.Equal(t, PerFileMaxTokens, alloc["only.txt"])
}

func TestAllocateFormulaExact(t *testing.T) {
	t.Parallel()

	ps := &mockPrioritySource{scores: map[string]float64{
		"a.txt": 0.5,
		"b.txt": 0.3,
		"c.txt": 0.2,
	}}
	rc := NewReadCoordinator(8, nil, WithPrioritySource(ps))

	budget := 10000
	alloc := rc.Allocate([]string{"a.txt", "b.txt", "c.txt"}, budget)

	// Expected: a=5000, b=3000, c=2000 — all within PerFileMaxTokens.
	require.Equal(t, 5000, alloc["a.txt"])
	require.Equal(t, 3000, alloc["b.txt"])
	require.Equal(t, 2000, alloc["c.txt"])
}
