package eval

import (
	"context"
	"math"
	"os"
	"sort"
	"sync"
	"sync/atomic"
)

const (
	defaultMaxWorkers = 8
	charsPerToken     = 4

	// PerFileMaxTokens is the maximum token budget allocated to any single
	// file by the priority-based Allocate method.
	PerFileMaxTokens = 5000
)

// FileReader abstracts file reading for testability.
type FileReader interface {
	ReadFile(ctx context.Context, path string) (string, error)
}

// PrioritySource provides per-file priority scores used for proportional
// budget allocation. Implementations typically wrap repomap.PageRank scores.
type PrioritySource interface {
	Priority(path string) float64
}

// ReadCoordinatorOption configures a ReadCoordinator during construction.
type ReadCoordinatorOption func(*ReadCoordinator)

// WithPrioritySource sets the priority source used for proportional budget
// allocation. When provided, ReadFiles sorts paths by descending priority
// and caps each file's token allocation using the spec formula.
func WithPrioritySource(ps PrioritySource) ReadCoordinatorOption {
	return func(rc *ReadCoordinator) {
		rc.priority = ps
	}
}

// osFileReader is the default FileReader using os.ReadFile.
type osFileReader struct{}

func (osFileReader) ReadFile(_ context.Context, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// readResult holds the outcome of reading a single file.
type readResult struct {
	content string
	tokens  int
	err     error
}

// ReadCoordinator reads multiple files concurrently with deduplication,
// budget enforcement, and a bounded worker pool. When a PrioritySource is
// configured, files are sorted by descending PageRank priority and token
// budgets are allocated proportionally via Allocate.
type ReadCoordinator struct {
	maxWorkers int
	reader     FileReader
	priority   PrioritySource
}

// NewReadCoordinator creates a ReadCoordinator with the given options.
// If maxWorkers is 0, defaultMaxWorkers (8) is used.
// If reader is nil, an OS-based reader is used.
// Optional functional options can configure priority-based allocation.
func NewReadCoordinator(maxWorkers int, reader FileReader, opts ...ReadCoordinatorOption) *ReadCoordinator {
	if maxWorkers <= 0 {
		maxWorkers = defaultMaxWorkers
	}
	if reader == nil {
		reader = osFileReader{}
	}
	rc := &ReadCoordinator{
		maxWorkers: maxWorkers,
		reader:     reader,
	}
	for _, opt := range opts {
		opt(rc)
	}
	return rc
}

// Allocate computes a per-file token budget using the spec formula:
//
//	min(priority[i] / sum(priorities) * budget, PerFileMaxTokens)
//
// Paths with zero priority receive a minimum allocation of 1 token.
// When the priority source is nil or budget is zero, the budget is
// distributed equally across all paths.
func (rc *ReadCoordinator) Allocate(paths []string, budget int) map[string]int {
	alloc := make(map[string]int, len(paths))
	if len(paths) == 0 || budget <= 0 {
		return alloc
	}

	// No priority source: distribute equally.
	if rc.priority == nil {
		perFile := max(budget/len(paths), 1)
		for _, p := range paths {
			alloc[p] = perFile
		}
		return alloc
	}

	// Collect priorities.
	type pathPri struct {
		path string
		pri  float64
	}
	entries := make([]pathPri, len(paths))
	var sum float64
	for i, p := range paths {
		pri := rc.priority.Priority(p)
		entries[i] = pathPri{path: p, pri: pri}
		sum += pri
	}

	// If all priorities are zero, distribute equally.
	if sum <= 0 {
		perFile := max(budget/len(paths), 1)
		for _, p := range paths {
			alloc[p] = perFile
		}
		return alloc
	}

	for _, e := range entries {
		var fileBudget int
		if e.pri <= 0 {
			fileBudget = 1 // Minimum allocation for zero-priority files.
		} else {
			fileBudget = max(int(math.Min(
				e.pri/sum*float64(budget),
				float64(PerFileMaxTokens),
			)), 1)
		}
		alloc[e.path] = fileBudget
	}
	return alloc
}

// ReadFiles reads the given paths concurrently, returning a map of
// path→content. Duplicate paths are read only once and the result is
// shared. Reads stop early when the cumulative token estimate
// (len(content)/charsPerToken) exceeds the budget. A budget of 0 or
// less means no limit.
//
// When a PrioritySource is configured, unique paths are sorted by
// descending priority before reading, and each file's content is
// truncated to its allocated token budget via Allocate.
func (rc *ReadCoordinator) ReadFiles(ctx context.Context, paths []string, budget int) (map[string]string, error) {
	unique := dedupPaths(paths)
	if len(unique) == 0 {
		return map[string]string{}, nil
	}

	// Sort by priority when a source is configured.
	if rc.priority != nil {
		unique = sortByPriority(unique, rc.priority)
	}

	// Compute per-file budgets.
	perFile := rc.Allocate(unique, budget)

	sem := make(chan struct{}, rc.maxWorkers)

	var (
		mu       sync.Mutex
		results  = make(map[string]string, len(unique))
		tokens   atomic.Int64
		budget64 = int64(budget)
	)

	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	var once sync.Once

	for _, p := range unique {
		if budget > 0 && tokens.Load() >= budget64 {
			break
		}

		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(path string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}
			if budget > 0 && tokens.Load() >= budget64 {
				return
			}

			content, err := rc.reader.ReadFile(ctx, path)
			if err != nil {
				once.Do(func() {
					select {
					case errCh <- err:
					default:
					}
				})
				return
			}

			// Truncate content to per-file budget when set.
			if budget > 0 && perFile != nil {
				if fb, ok := perFile[path]; ok && fb > 0 {
					maxChars := fb * charsPerToken
					if len(content) > maxChars {
						content = content[:maxChars]
					}
				}
			}

			t := int64(len(content) / charsPerToken)
			tokens.Add(t)

			mu.Lock()
			results[path] = content
			mu.Unlock()
		}(p)
	}

	wg.Wait()
	close(errCh)

	if err := ctx.Err(); err != nil {
		return results, err
	}

	if err := <-errCh; err != nil {
		return results, err
	}

	return results, nil
}

// dedupPaths returns the unique paths in order of first appearance.
func dedupPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}

// sortByPriority returns paths sorted by descending priority.
func sortByPriority(paths []string, ps PrioritySource) []string {
	sorted := make([]string, len(paths))
	copy(sorted, paths)
	sort.SliceStable(sorted, func(i, j int) bool {
		return ps.Priority(sorted[i]) > ps.Priority(sorted[j])
	})
	return sorted
}
