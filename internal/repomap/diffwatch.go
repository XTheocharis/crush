package repomap

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DiffWatcher monitors git diff for changed files and invalidates
// entries in the SessionRenderCacheSet and SessionCacheSet.
type DiffWatcher struct {
	rootDir       string
	interval      time.Duration
	renderCaches  *SessionRenderCacheSet
	sessionCaches *SessionCacheSet

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}

	gitDiffCmd func(ctx context.Context, rootDir string) ([]byte, error)
	lastKnown  map[string]struct{}
}

// DiffWatcherConfig configures a DiffWatcher.
type DiffWatcherConfig struct {
	RootDir       string
	Interval      time.Duration
	RenderCaches  *SessionRenderCacheSet
	SessionCaches *SessionCacheSet
	GitDiffCmd    func(ctx context.Context, rootDir string) ([]byte, error)
}

// NewDiffWatcher creates a new DiffWatcher. Default interval is 30s.
func NewDiffWatcher(cfg DiffWatcherConfig) *DiffWatcher {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &DiffWatcher{
		rootDir:       cfg.RootDir,
		interval:      interval,
		renderCaches:  cfg.RenderCaches,
		sessionCaches: cfg.SessionCaches,
		gitDiffCmd:    cfg.GitDiffCmd,
		lastKnown:     make(map[string]struct{}),
	}
}

// Start begins polling for git diff changes. It is safe to call Start
// multiple times; only one goroutine runs at a time.
func (dw *DiffWatcher) Start(ctx context.Context) {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	if dw.running {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	dw.cancel = cancel
	dw.done = make(chan struct{})
	dw.running = true
	go dw.run(ctx)
}

// Stop halts the polling goroutine and waits for it to finish.
func (dw *DiffWatcher) Stop() {
	dw.mu.Lock()
	if !dw.running {
		dw.mu.Unlock()
		return
	}
	dw.cancel()
	dw.running = false
	dw.mu.Unlock()

	if dw.done != nil {
		<-dw.done
	}
}

// ChangedFiles returns the set of files that have changed since the last
// invalidation cycle.
func (dw *DiffWatcher) ChangedFiles() []string {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	out := make([]string, 0, len(dw.lastKnown))
	for f := range dw.lastKnown {
		out = append(out, f)
	}
	return out
}

func (dw *DiffWatcher) run(ctx context.Context) {
	defer close(dw.done)

	dw.poll(ctx)

	ticker := time.NewTicker(dw.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dw.poll(ctx)
		}
	}
}

func (dw *DiffWatcher) poll(ctx context.Context) {
	files, err := dw.getChangedFiles(ctx)
	if err != nil {
		slog.Debug("DiffWatcher poll failed", "error", err)
		return
	}

	if len(files) == 0 {
		return
	}

	dw.mu.Lock()
	dw.lastKnown = files
	dw.mu.Unlock()

	dw.invalidate(files)
}

func (dw *DiffWatcher) getChangedFiles(ctx context.Context) (map[string]struct{}, error) {
	cmd := dw.gitDiffCmd
	if cmd == nil {
		cmd = defaultGitDiffCmd
	}
	out, err := cmd(ctx, dw.rootDir)
	if err != nil {
		return nil, nil
	}
	return parseDiffOutput(out), nil
}

func (dw *DiffWatcher) invalidate(files map[string]struct{}) {
	if dw.renderCaches != nil {
		dw.renderCaches.ClearAll()
	}
	if dw.sessionCaches != nil {
		dw.sessionCaches.ClearAll()
	}
	slog.Debug("DiffWatcher invalidated caches", "changed_files", len(files))
}

func defaultGitDiffCmd(ctx context.Context, rootDir string) ([]byte, error) {
	unstaged, err := runGitCommand(ctx, rootDir, "diff", "--name-only")
	if err != nil {
		return nil, fmt.Errorf("git diff unstaged: %w", err)
	}

	staged, err := runGitCommand(ctx, rootDir, "diff", "--name-only", "--cached")
	if err != nil {
		return nil, fmt.Errorf("git diff staged: %w", err)
	}

	untracked, err := runGitCommand(ctx, rootDir, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		untracked = nil
	}

	var buf bytes.Buffer
	buf.Write(unstaged)
	buf.WriteByte('\n')
	buf.Write(staged)
	buf.WriteByte('\n')
	buf.Write(untracked)
	return buf.Bytes(), nil
}

func runGitCommand(ctx context.Context, rootDir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = rootDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func parseDiffOutput(data []byte) map[string]struct{} {
	result := make(map[string]struct{})
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		result[line] = struct{}{}
	}
	return result
}
