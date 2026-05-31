package tools

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/fsnotify/fsnotify"
)

var watchedExtensions = map[string]bool{
	".go": true,
	".ts": true,
	".py": true,
}

// cacheTTL is how long cached diagnostics remain valid.
const cacheTTL = 30 * time.Second

// debounceInterval is the batching window for file change events.
const debounceInterval = 500 * time.Millisecond

// diagCacheEntry holds diagnostics for a single file alongside its expiry.
type diagCacheEntry struct {
	diagnostics []DiagnosticInfo
	cachedAt    time.Time
}

// DiagnosticWatcher uses fsnotify to monitor file changes, triggers LSP
// diagnostic collection, and caches results for fast agent access.
type DiagnosticWatcher struct {
	manager    *lsp.Manager
	projectDir string

	watcher *fsnotify.Watcher

	mu    sync.RWMutex
	cache map[string]*diagCacheEntry

	pending   map[string]bool
	pendingMu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// NewDiagnosticWatcher creates a watcher for the given project directory.
func NewDiagnosticWatcher(manager *lsp.Manager, projectDir string) (*DiagnosticWatcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &DiagnosticWatcher{
		manager:    manager,
		projectDir: projectDir,
		watcher:    fw,
		cache:      make(map[string]*diagCacheEntry),
		pending:    make(map[string]bool),
		done:       make(chan struct{}),
	}, nil
}

// Start begins watching for file changes and starts the event loop goroutine.
func (dw *DiagnosticWatcher) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	dw.ctx = ctx
	dw.cancel = cancel

	dw.addWatches()

	go dw.run()
}

// Stop shuts down the watcher and blocks until the event loop has exited.
func (dw *DiagnosticWatcher) Stop() {
	if dw.cancel != nil {
		dw.cancel()
	}
	if dw.watcher != nil {
		_ = dw.watcher.Close()
	}
	<-dw.done
}

// GetCachedDiagnostics returns cached diagnostics for filePath, or nil if
// the cache is empty or expired.
func (dw *DiagnosticWatcher) GetCachedDiagnostics(filePath string) []DiagnosticInfo {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil
	}

	dw.mu.RLock()
	entry, ok := dw.cache[absPath]
	dw.mu.RUnlock()

	if !ok {
		return nil
	}

	if time.Since(entry.cachedAt) > cacheTTL {
		return nil
	}

	result := make([]DiagnosticInfo, len(entry.diagnostics))
	copy(result, entry.diagnostics)
	return result
}

// WatchedFiles returns the number of files currently being watched.
func (dw *DiagnosticWatcher) WatchedFiles() int {
	if dw.watcher == nil {
		return 0
	}
	return len(dw.watcher.WatchList())
}

func (dw *DiagnosticWatcher) addWatches() {
	_ = dw.watcher.Add(dw.projectDir)
	dw.addSubdirWatches(dw.projectDir)
}

// addSubdirWatches adds watches for common source directories. fsnotify on
// Linux requires per-directory watches; on macOS/Windows it can watch
// recursively.
func (dw *DiagnosticWatcher) addSubdirWatches(basePath string) {
	subdirs := []string{
		"src", "pkg", "internal", "cmd", "app", "lib",
		"test", "tests", "spec", "specs",
	}
	for _, sub := range subdirs {
		_ = dw.watcher.Add(filepath.Join(basePath, sub))
	}
}

// run is the main event loop that processes fsnotify events and timer-based
// debounce flushes.
func (dw *DiagnosticWatcher) run() {
	defer close(dw.done)

	debounceTimer := time.NewTimer(debounceInterval)
	if !debounceTimer.Stop() {
		<-debounceTimer.C
	}

	for {
		select {
		case <-dw.ctx.Done():
			return

		case event, ok := <-dw.watcher.Events:
			if !ok {
				return
			}
			dw.handleEvent(event)

		case err, ok := <-dw.watcher.Errors:
			if !ok {
				return
			}
			slog.Debug("DiagnosticWatcher fsnotify error", "error", err)

		case <-debounceTimer.C:
			dw.flushPending()
			debounceTimer.Reset(debounceInterval)
		}
	}
}

func (dw *DiagnosticWatcher) handleEvent(event fsnotify.Event) {
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
		return
	}

	path := event.Name
	if path == "" {
		return
	}

	ext := strings.ToLower(filepath.Ext(path))
	if !watchedExtensions[ext] {
		return
	}

	if !isSubPath(dw.projectDir, path) {
		return
	}

	dw.pendingMu.Lock()
	dw.pending[path] = true
	dw.pendingMu.Unlock()
}

// flushPending collects all pending file changes and refreshes diagnostics
// for them in a single batch.
func (dw *DiagnosticWatcher) flushPending() {
	dw.pendingMu.Lock()
	if len(dw.pending) == 0 {
		dw.pendingMu.Unlock()
		return
	}
	files := make([]string, 0, len(dw.pending))
	for f := range dw.pending {
		files = append(files, f)
		delete(dw.pending, f)
	}
	dw.pendingMu.Unlock()

	dw.refreshDiagnostics(files)
}

// refreshDiagnostics collects fresh diagnostics from the LSP manager for the
// given files and updates the cache.
func (dw *DiagnosticWatcher) refreshDiagnostics(files []string) {
	if dw.manager == nil {
		return
	}
	for _, f := range files {
		absPath, err := filepath.Abs(f)
		if err != nil {
			continue
		}

		diags := collectFileDiagnostics(absPath, dw.manager)

		dw.mu.Lock()
		dw.cache[absPath] = &diagCacheEntry{
			diagnostics: diags,
			cachedAt:    time.Now(),
		}
		dw.mu.Unlock()
	}
}

// isSubPath returns true if sub is within the parent directory.
func isSubPath(parent, sub string) bool {
	rel, err := filepath.Rel(parent, sub)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}
