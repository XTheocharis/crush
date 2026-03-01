package repomap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/fsext"
	"github.com/charmbracelet/crush/internal/treesitter"
	"golang.org/x/sync/singleflight"
)

var errServiceClosed = errors.New("repomap service is closed")

// RunInjectionKey identifies one logical Run() for injection gating.
type RunInjectionKey struct {
	RootUserMessageID string
	QueueGeneration   int64
}

type runInjectionKeyContextKey string

const runInjectionKeyCtxKey runInjectionKeyContextKey = "run_injection_key"

// WithRunInjectionKey stores the run injection key in context.
func WithRunInjectionKey(ctx context.Context, key RunInjectionKey) context.Context {
	return context.WithValue(ctx, runInjectionKeyCtxKey, key)
}

// RunInjectionKeyFromContext retrieves the run injection key from context.
func RunInjectionKeyFromContext(ctx context.Context) (RunInjectionKey, bool) {
	v := ctx.Value(runInjectionKeyCtxKey)
	if v == nil {
		return RunInjectionKey{}, false
	}
	key, ok := v.(RunInjectionKey)
	if !ok {
		return RunInjectionKey{}, false
	}
	if key.RootUserMessageID == "" {
		return RunInjectionKey{}, false
	}
	return key, true
}

// GenerateOpts specifies options for repo-map generation.
type GenerateOpts struct {
	SessionID            string
	ChatFiles            []string
	MentionedFnames      []string
	MentionedIdents      []string
	TokenBudget          int
	MaxContextWindow     int
	ForceRefresh         bool
	ParityMode           bool
	PromptCachingEnabled bool
	EnhancementTiers     string
	DeterministicMode    bool
	TokenCounterMode     string
	Model                string
	TokenCounter         TokenCounter
}

// Service handles repo-map generation and lifecycle.
type Service struct {
	parser           treesitter.Parser
	newParserWithCfg func(cfg treesitter.ParserConfig) treesitter.Parser
	db               *db.Queries
	rawDB            *sql.DB
	rootDir          string
	cfg              *config.RepoMapOptions
	lifecycleCtx     context.Context
	serviceCtx       context.Context
	cancel           context.CancelFunc
	closed           chan struct{}

	wg sync.WaitGroup

	mu                   sync.RWMutex
	sessionCaches        *SessionCacheSet
	renderCaches         *SessionRenderCacheSet
	injectedBySessionRun map[string]map[RunInjectionKey]struct{}
	allFiles             []string
	preIndexDone         chan struct{}
	preIndexRunning      bool
	preIndexFlight       singleflight.Group
	refreshFlight        singleflight.Group
	onPreIndexRun        func()
	onRefreshRun         func()

	disabledSessions sync.Map // one-way disable latch per session

	closeOnce sync.Once
}

// NewService creates a new repo-map service scaffold.
func NewService(cfg *config.Config, q *db.Queries, rawDB *sql.DB, rootDir string, lifecycleCtx context.Context) *Service {
	var repoCfg *config.RepoMapOptions
	if cfg != nil && cfg.Options != nil {
		repoCfg = cfg.Options.RepoMap
	}

	baseCtx := lifecycleCtx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	serviceCtx, cancel := context.WithCancel(baseCtx)

	preIndexDone := make(chan struct{})
	close(preIndexDone)

	return &Service{
		newParserWithCfg:     treesitter.NewParserWithConfig,
		db:                   q,
		rawDB:                rawDB,
		rootDir:              rootDir,
		cfg:                  repoCfg,
		lifecycleCtx:         lifecycleCtx,
		serviceCtx:           serviceCtx,
		cancel:               cancel,
		closed:               make(chan struct{}),
		sessionCaches:        NewSessionCacheSet(),
		renderCaches:         NewSessionRenderCacheSet(),
		injectedBySessionRun: make(map[string]map[RunInjectionKey]struct{}),
		preIndexDone:         preIndexDone,
	}
}

// Generate produces a repo map.
func (s *Service) Generate(ctx context.Context, opts GenerateOpts) (string, int, error) {
	if err := s.checkContextsDone(ctx); err != nil {
		return "", 0, err
	}

	sessionID := strings.TrimSpace(opts.SessionID)
	if sessionID == "" {
		return "", 0, nil
	}

	mode := s.effectiveRefreshMode(opts)

	lastMap, lastTok := s.sessionCaches.Load(sessionID)
	cacheKey := buildRenderCacheKey(mode, opts)
	renderCache := s.renderCaches.GetOrCreate(sessionID)

	loadRenderCache := func() (string, int, bool) {
		if cacheKey == "" {
			return "", 0, false
		}
		m, tok, ok := renderCache.Get(cacheKey)
		if ok {
			s.sessionCaches.Store(sessionID, m, tok)
		}
		return m, tok, ok
	}

	fallback := func(genErr error) (string, int, error) {
		if lastMap != "" || lastTok > 0 {
			return lastMap, lastTok, nil
		}
		if m, tok, ok := loadRenderCache(); ok {
			return m, tok, nil
		}
		if genErr != nil {
			return "", 0, genErr
		}
		return "", 0, nil
	}

	// One-way disable latch: if this session was permanently disabled due to
	// resource exhaustion in parity mode, return the last known-good map.
	if s.isDisabledForSession(sessionID) {
		return fallback(nil)
	}

	if !opts.ForceRefresh {
		switch mode {
		case "manual":
			if lastMap != "" || lastTok > 0 {
				return lastMap, lastTok, nil
			}
			return "", 0, nil
		case "files", "auto":
			if lastMap != "" || lastTok > 0 {
				return lastMap, lastTok, nil
			}
			if m, tok, ok := loadRenderCache(); ok {
				return m, tok, nil
			}
		case "always":
			if lastMap != "" || lastTok > 0 {
				return lastMap, lastTok, nil
			}
		default:
			if lastMap != "" || lastTok > 0 {
				return lastMap, lastTok, nil
			}
			if m, tok, ok := loadRenderCache(); ok {
				return m, tok, nil
			}
		}
	} else {
		s.sessionCaches.Clear(sessionID)
		s.renderCaches.Clear(sessionID)
		lastMap, lastTok = "", 0
		renderCache = s.renderCaches.GetOrCreate(sessionID)
	}

	if s.db == nil || s.rawDB == nil || strings.TrimSpace(s.rootDir) == "" {
		return fallback(nil)
	}

	fileUniverse := s.AllFiles(ctx)
	if len(fileUniverse) == 0 {
		fileUniverse = s.walkAllFiles(ctx)
	}
	if len(fileUniverse) == 0 {
		return fallback(nil)
	}

	tags, err := s.extractTags(ctx, s.rootDir, fileUniverse, opts.ForceRefresh)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && opts.ParityMode {
			slog.Warn("Disabling repo map for session — extractTags timed out",
				"session", sessionID)
			s.disableForSession(sessionID)
		}
		return fallback(err)
	}

	// W6.1: Build tagsByFile map for efficient lookup by RenderRepoMap.
	tagsByFile := make(map[string][]treesitter.Tag, len(tags)/10+1)
	for _, tag := range tags {
		tagsByFile[tag.RelPath] = append(tagsByFile[tag.RelPath], tag)
	}

	graph := buildGraph(tags, opts.ChatFiles, opts.MentionedIdents)
	personalization := BuildPersonalization(fileUniverse, opts.ChatFiles, opts.MentionedFnames, opts.MentionedIdents)
	rankedDefs := Rank(graph, personalization)
	rankedFiles := AggregateRankedFiles(rankedDefs, tags)

	specialPrelude := BuildSpecialPrelude(fileUniverse, rankedFilePaths(rankedFiles), opts.ParityMode)
	entries := AssembleStageEntries(
		specialPrelude,
		rankedDefs,
		graph.Nodes,
		fileUniverse,
		opts.ChatFiles,
		opts.ParityMode,
	)

	// Parity mode requires tokenizer-backed counting; fail hard if unavailable.
	if opts.ParityMode && opts.TokenCounter == nil {
		return "", 0, fmt.Errorf("parity mode requires tokenizer-backed counting; TokenCounter is nil")
	}

	// W6.5 Layer 1: Expansion factor — scope-aware output is typically
	// 3-10x larger than compact format. Reduce the budget for FitToBudget
	// so fewer entries survive, then verify post-render.
	const scopeExpansionFactor = 4
	budgetProfile := BudgetProfile{
		ParityMode:   opts.ParityMode,
		TokenBudget:  resolveTokenBudget(s.cfg, opts),
		Model:        opts.Model,
		LanguageHint: "default",
	}
	originalBudget := budgetProfile.TokenBudget
	budgetProfile.TokenBudget = max(budgetProfile.TokenBudget/scopeExpansionFactor, 1)

	fit, err := FitToBudget(ctx, entries, budgetProfile, opts.TokenCounter)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && opts.ParityMode {
			slog.Warn("Disabling repo map for session — FitToBudget timed out",
				"session", sessionID)
			s.disableForSession(sessionID)
		}
		return fallback(err)
	}

	// Restore original budget for post-render compliance checks.
	budgetProfile.TokenBudget = originalBudget

	// W6.2: Scope-aware rendering via RenderRepoMap, with compact fallback.
	counter := opts.TokenCounter
	model := opts.Model
	parser := s.ensureParser()
	rootDir := s.rootDir

	mapText, renderErr := RenderRepoMap(ctx, fit.Entries, tagsByFile, parser, rootDir)
	if renderErr != nil {
		if errors.Is(renderErr, context.DeadlineExceeded) && opts.ParityMode {
			slog.Warn("Disabling repo map for session — render timed out",
				"session", sessionID)
			s.disableForSession(sessionID)
		}
		mapText = renderStageEntries(fit.Entries)
	}

	// W6.5 Layer 2: Post-render trim loop — monotonic hard-cap acceptance.
	// Binary search to find the largest prefix of entries that fits within
	// the original budget after scope-aware rendering.
	fitsWithinBudget := func(text string) (bool, int) {
		m, err := CountParityAndSafetyTokens(ctx, counter, model, text, "default")
		if err != nil {
			est := EstimateTokens(text, "default")
			return est <= originalBudget, est
		}
		return m.SafetyTokens <= originalBudget, m.SafetyTokens
	}

	accepted, tokenCount := fitsWithinBudget(mapText)
	if !accepted && len(fit.Entries) > 0 {
		lo, hi := 0, len(fit.Entries)-1
		for lo < hi {
			mid := (lo + hi + 1) / 2
			candidate := fit.Entries[:mid]
			text, trimRenderErr := RenderRepoMap(ctx, candidate, tagsByFile, parser, rootDir)
			if trimRenderErr != nil {
				break // Context cancelled.
			}
			ok, _ := fitsWithinBudget(text)
			if ok {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		if lo == 0 {
			slog.Warn("Post-render trim reduced repo map to zero entries",
				"original_entries", len(fit.Entries),
				"budget", originalBudget)
		}
		fit.Entries = fit.Entries[:lo]
		mapText, _ = RenderRepoMap(ctx, fit.Entries, tagsByFile, parser, rootDir)
		_, tokenCount = fitsWithinBudget(mapText)
	}

	// Post-trim parity quality check (parity mode only).
	if budgetProfile.ParityMode && tokenCount > 0 {
		m, mErr := CountParityAndSafetyTokens(ctx, counter, model, mapText, "default")
		if mErr == nil {
			delta := parityComparatorDelta(m.ParityTokens, originalBudget)
			if delta > 0.15 {
				slog.Warn("Parity mode: repo map token count diverges from budget",
					"parity_tokens", m.ParityTokens,
					"budget", originalBudget,
					"delta", delta)
			}
		}
	}

	s.sessionCaches.Store(sessionID, mapText, tokenCount)
	if cacheKey != "" {
		renderCache.Set(cacheKey, mapText, tokenCount)
	}

	repoKey := repoKeyForRoot(s.rootDir)
	readOnly := append(append([]string(nil), opts.ChatFiles...), opts.MentionedFnames...)
	s.persistSessionArtifacts(ctx, sessionID, repoKey, rankedFiles, readOnly)

	return mapText, tokenCount, nil
}

// Available returns whether repo-map service is ready.
func (s *Service) Available() bool {
	if s == nil {
		return false
	}
	if s.isClosed() {
		return false
	}
	return s.cfg != nil && !s.cfg.Disabled
}

// AllFiles returns all files in the repository for repo-map generation.
func (s *Service) AllFiles(ctx context.Context) []string {
	if s == nil {
		return nil
	}

	done := s.preIndexSignal()
	if done != nil {
		select {
		case <-done:
		case <-s.serviceCtx.Done():
		case <-ctxDone(ctx):
		}
	}

	s.mu.RLock()
	files := append([]string(nil), s.allFiles...)
	s.mu.RUnlock()

	return files
}

// LastGoodMap returns most recent successful map for a session.
// Returns empty string if no cached value exists for the session.
func (s *Service) LastGoodMap(sessionID string) string {
	lastMap, _ := s.sessionCaches.Load(sessionID)
	return lastMap
}

// LastTokenCount returns last generated map token count for a session.
// Returns 0 if no cached value exists for the session.
func (s *Service) LastTokenCount(sessionID string) int {
	_, lastTok := s.sessionCaches.Load(sessionID)
	return lastTok
}

// SessionReadOnlyFiles returns persisted repo-map read-only paths for a session.
func (s *Service) SessionReadOnlyFiles(ctx context.Context, sessionID string) []string {
	if s == nil || s.isClosed() || s.db == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	repoKey := repoKeyForRoot(s.rootDir)
	if repoKey == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	paths, err := s.db.ListSessionReadOnlyPaths(ctx, db.ListSessionReadOnlyPathsParams{
		RepoKey:   repoKey,
		SessionID: sessionID,
	})
	if err != nil {
		return nil
	}
	return normalizeUniqueGraphPaths(paths)
}

// ShouldInject reports whether map should be injected for this run.
func (s *Service) ShouldInject(sessionID string, runKey RunInjectionKey) bool {
	if sessionID == "" || runKey.RootUserMessageID == "" {
		return false
	}
	if s.isClosed() {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	runs, ok := s.injectedBySessionRun[sessionID]
	if !ok {
		runs = make(map[RunInjectionKey]struct{})
		s.injectedBySessionRun[sessionID] = runs
	}
	if _, exists := runs[runKey]; exists {
		return false
	}
	runs[runKey] = struct{}{}
	return true
}

// RefreshAsync schedules async refresh.
func (s *Service) RefreshAsync(sessionID string, opts GenerateOpts) {
	if s == nil || s.isClosed() {
		return
	}
	if opts.SessionID == "" {
		opts.SessionID = sessionID
	}
	flightKey := s.buildRefreshFlightKey(sessionID, opts)

	s.wg.Add(1)
	go func(key string, sid string, o GenerateOpts) {
		defer s.wg.Done()
		_, _, _ = s.refreshFlight.Do(key, func() (any, error) {
			if s.onRefreshRun != nil {
				s.onRefreshRun()
			}
			_, _, err := s.Refresh(s.serviceCtx, sid, o)
			return nil, err
		})
	}(flightKey, sessionID, opts)
}

// Refresh performs synchronous refresh.
func (s *Service) Refresh(ctx context.Context, sessionID string, opts GenerateOpts) (string, int, error) {
	if opts.SessionID == "" {
		opts.SessionID = sessionID
	}

	if opts.ForceRefresh {
		s.sessionCaches.Clear(opts.SessionID)
		s.renderCaches.Clear(opts.SessionID)
	}

	m, tok, err := s.Generate(ctx, opts)
	if err != nil {
		return "", 0, err
	}

	if opts.SessionID != "" {
		s.sessionCaches.Store(opts.SessionID, m, tok)
		key := buildRenderCacheKey(s.effectiveRefreshMode(opts), opts)
		if key != "" {
			if m != "" || tok > 0 {
				s.renderCaches.GetOrCreate(opts.SessionID).Set(key, m, tok)
			} else {
				s.renderCaches.GetOrCreate(opts.SessionID).Delete(key)
			}
		}
	}

	return m, tok, nil
}

// Reset clears cached repo-map state for a session.
func (s *Service) Reset(ctx context.Context, sessionID string) error {
	if err := s.checkContextsDone(ctx); err != nil {
		return err
	}

	s.sessionCaches.Clear(sessionID)
	s.renderCaches.Clear(sessionID)
	s.disabledSessions.Delete(sessionID)

	repoKey := repoKeyForRoot(s.rootDir)
	if repoKey != "" && s.db != nil {
		_ = s.db.DeleteSessionRankings(ctx, db.DeleteSessionRankingsParams{RepoKey: repoKey, SessionID: sessionID})
		_ = s.db.DeleteSessionReadOnlyPaths(ctx, db.DeleteSessionReadOnlyPathsParams{RepoKey: repoKey, SessionID: sessionID})
	}

	s.mu.Lock()
	delete(s.injectedBySessionRun, sessionID)
	s.mu.Unlock()
	return nil
}

// PreIndex starts background pre-index work.
func (s *Service) PreIndex() {
	if s == nil || s.isClosed() {
		return
	}

	s.mu.Lock()
	if s.preIndexRunning {
		s.mu.Unlock()
		return
	}
	done := make(chan struct{})
	s.preIndexDone = done
	s.preIndexRunning = true
	s.mu.Unlock()

	s.wg.Go(func() {
		defer func() {
			s.mu.Lock()
			s.preIndexRunning = false
			close(done)
			s.mu.Unlock()
		}()

		_, _, _ = s.preIndexFlight.Do(repoKeyForRoot(s.rootDir), func() (any, error) {
			if s.onPreIndexRun != nil {
				s.onPreIndexRun()
			}
			files := s.walkAllFiles(s.serviceCtx)
			s.mu.Lock()
			s.allFiles = files
			s.mu.Unlock()
			return nil, nil
		})
	})
}

// Close releases resources.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}

	var err error
	s.closeOnce.Do(func() {
		s.cancel()
		close(s.closed)
		s.wg.Wait()
		if s.parser != nil {
			err = s.parser.Close()
		}
	})
	return err
}

func (s *Service) disableForSession(sessionID string) {
	s.disabledSessions.Store(sessionID, struct{}{})
}

func (s *Service) isDisabledForSession(sessionID string) bool {
	_, ok := s.disabledSessions.Load(sessionID)
	return ok
}

func (s *Service) preIndexSignal() <-chan struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.preIndexDone
}

func (s *Service) isClosed() bool {
	if s == nil {
		return true
	}
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *Service) checkContextsDone(ctx context.Context) error {
	if s == nil {
		return errServiceClosed
	}
	if s.isClosed() {
		return errServiceClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.serviceCtx.Done():
		if s.isClosed() {
			return errServiceClosed
		}
		return s.serviceCtx.Err()
	default:
		return nil
	}
}

func (s *Service) walkAllFiles(ctx context.Context) []string {
	root := strings.TrimSpace(s.rootDir)
	if root == "" {
		return nil
	}

	walker := fsext.NewFastGlobWalker(root)
	files := make([]string, 0, 256)

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		select {
		case <-ctxDone(ctx):
			return context.Canceled
		default:
		}

		if d.IsDir() {
			if walker.ShouldSkipDir(path) {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip non-regular files (symlinks, devices, etc.).
		if !d.Type().IsRegular() {
			return nil
		}

		if walker.ShouldSkip(path) {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})

	// Apply ExcludeGlobs filtering via doublestar.Match.
	if s.cfg != nil && len(s.cfg.ExcludeGlobs) > 0 {
		filtered := make([]string, 0, len(files))
		for _, f := range files {
			if !matchesAnyGlob(f, s.cfg.ExcludeGlobs) {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}

	sort.Strings(files)
	return files
}

// matchesAnyGlob reports whether the given path matches any of the
// provided glob patterns using doublestar.Match. Malformed patterns
// are silently skipped.
func matchesAnyGlob(path string, patterns []string) bool {
	for _, p := range patterns {
		if matched, err := doublestar.Match(p, path); err == nil && matched {
			return true
		}
	}
	return false
}

func ctxDone(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	return ctx.Done()
}

// renderCacheVersion distinguishes scope-aware output from the prior
// compact S0|/S1| format. Bump when the rendering strategy changes to
// invalidate all cached entries.
const renderCacheVersion = "v2-scope"

func buildRenderCacheKey(mode string, opts GenerateOpts) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "auto"
	}

	chatFiles := normalizeUniqueGraphPaths(opts.ChatFiles)
	mentionedFnames := normalizeUniqueGraphPaths(opts.MentionedFnames)
	mentionedIdents := normalizeUniqueStrings(opts.MentionedIdents)

	switch mode {
	case "manual":
		return "manual"
	case "always":
		return ""
	case "files":
		return strings.Join([]string{
			renderCacheVersion,
			mode,
			joinParts(chatFiles),
			itoa(opts.TokenBudget),
		}, "|")
	case "auto":
		return strings.Join([]string{
			renderCacheVersion,
			mode,
			joinParts(chatFiles),
			joinParts(mentionedFnames),
			joinParts(mentionedIdents),
			itoa(opts.TokenBudget),
		}, "|")
	default:
		return strings.Join([]string{
			renderCacheVersion,
			"auto",
			joinParts(chatFiles),
			joinParts(mentionedFnames),
			joinParts(mentionedIdents),
			itoa(opts.TokenBudget),
		}, "|")
	}
}

func joinParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ",")
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func normalizeUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, val := range values {
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	sort.Strings(out)
	return out
}

func (s *Service) buildRefreshFlightKey(sessionID string, opts GenerateOpts) string {
	mode := s.effectiveRefreshMode(opts)
	cacheKey := buildRenderCacheKey(mode, opts)
	if cacheKey == "" {
		cacheKey = strings.Join([]string{
			mode,
			joinParts(normalizeUniqueGraphPaths(opts.ChatFiles)),
			joinParts(normalizeUniqueGraphPaths(opts.MentionedFnames)),
			joinParts(normalizeUniqueStrings(opts.MentionedIdents)),
			itoa(opts.TokenBudget),
			itoa(opts.MaxContextWindow),
			strconv.FormatBool(opts.ForceRefresh),
		}, "|")
	}
	repoKey := repoKeyForRoot("")
	if s != nil {
		repoKey = repoKeyForRoot(s.rootDir)
	}
	return strings.Join([]string{repoKey, sessionID, cacheKey}, "|")
}

func (s *Service) refreshMode() string {
	if s != nil && s.cfg != nil {
		if mode := strings.ToLower(strings.TrimSpace(s.cfg.RefreshMode)); mode != "" {
			return mode
		}
	}
	return "auto"
}

func (s *Service) effectiveRefreshMode(opts GenerateOpts) string {
	mode := strings.ToLower(strings.TrimSpace(s.refreshMode()))
	if mode == "" {
		mode = "auto"
	}
	if opts.ParityMode && opts.PromptCachingEnabled && mode == "auto" {
		return "files"
	}
	return mode
}

func resolveTokenBudget(cfg *config.RepoMapOptions, opts GenerateOpts) int {
	if opts.TokenBudget > 0 {
		return opts.TokenBudget
	}
	if cfg != nil && cfg.MaxTokens > 0 {
		return cfg.MaxTokens
	}
	contextWindow := opts.MaxContextWindow
	if cfg != nil && len(opts.ChatFiles) == 0 && cfg.MapMulNoFiles > 0 {
		adjusted := int(math.Ceil(float64(contextWindow) * cfg.MapMulNoFiles))
		if adjusted > contextWindow {
			contextWindow = adjusted
		}
	}
	return config.DefaultRepoMapMaxTokens(contextWindow)
}

func rankedFilePaths(files []RankedFile) []string {
	if len(files) == 0 {
		return nil
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		rel := normalizeGraphRelPath(f.Path)
		if rel == "" {
			continue
		}
		paths = append(paths, rel)
	}
	return normalizeUniqueGraphPaths(paths)
}

func (s *Service) persistSessionArtifacts(ctx context.Context, sessionID, repoKey string, ranked []RankedFile, readOnlyPaths []string) {
	if s == nil || s.db == nil || repoKey == "" || strings.TrimSpace(sessionID) == "" {
		return
	}

	if err := s.db.DeleteSessionRankings(ctx, db.DeleteSessionRankingsParams{RepoKey: repoKey, SessionID: sessionID}); err != nil {
		return
	}
	if err := s.db.DeleteSessionReadOnlyPaths(ctx, db.DeleteSessionReadOnlyPathsParams{RepoKey: repoKey, SessionID: sessionID}); err != nil {
		return
	}

	for _, file := range ranked {
		rel := normalizeGraphRelPath(file.Path)
		if rel == "" {
			continue
		}
		if err := s.db.UpsertSessionRanking(ctx, db.UpsertSessionRankingParams{
			RepoKey:   repoKey,
			SessionID: sessionID,
			RelPath:   rel,
			Rank:      file.Rank,
		}); err != nil {
			return
		}
	}

	for _, p := range normalizeUniqueGraphPaths(readOnlyPaths) {
		if err := s.db.UpsertSessionReadOnlyPath(ctx, db.UpsertSessionReadOnlyPathParams{
			RepoKey:   repoKey,
			SessionID: sessionID,
			RelPath:   p,
		}); err != nil {
			return
		}
	}
}
