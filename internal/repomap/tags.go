package repomap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"golang.org/x/sync/errgroup"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/treesitter"
)

// stringInterner interns repeated short strings to reduce allocations.
type stringInterner struct {
	pool map[string]string
}

func newStringInterner(capacity int) *stringInterner {
	if capacity < 0 {
		capacity = 0
	}
	return &stringInterner{pool: make(map[string]string, capacity)}
}

func (i *stringInterner) Intern(value string) string {
	if i == nil || value == "" {
		return value
	}
	if interned, ok := i.pool[value]; ok {
		return interned
	}
	i.pool[value] = value
	return value
}

// fileCacheEntry holds the pre-loaded mtime from the file cache for
// freshness checks during concurrent parsing (Phase 1).
type fileCacheEntry struct {
	mtime int64
}

// fileParseResult holds the outcome of parsing a single file. Produced
// by Phase 1 (concurrent parse) and consumed by Phase 2 (sequential
// DB writes).
type fileParseResult struct {
	relPath  string
	mtime    int64
	language string
	tags     []treesitter.Tag
	skipped  bool
	deleted  bool
	err      error
}

// preloadFileCache fetches the entire file cache for a repo key in a
// single non-transactional read. This is Phase 0 of the pipeline:
// serial, outside any transaction.
func (s *Service) preloadFileCache(ctx context.Context, repoKey string) (map[string]fileCacheEntry, error) {
	rows, err := s.db.GetRepoMapFileCache(ctx, repoKey)
	if err != nil {
		return nil, fmt.Errorf("preload repo-map file cache: %w", err)
	}
	cache := make(map[string]fileCacheEntry, len(rows))
	for _, row := range rows {
		cache[row.RelPath] = fileCacheEntry{mtime: row.Mtime}
	}
	return cache, nil
}

// parseFile performs all filesystem and tree-sitter work for a single
// file without touching the database. It replicates the language
// detection logic from the original upsertPathTags (lines 156-159)
// inline because resolveLanguage does not exist as a standalone method.
func (s *Service) parseFile(ctx context.Context, parser treesitter.Parser, rootDir, relPath string, forceRefresh bool, cache map[string]fileCacheEntry) fileParseResult {
	absPath := filepath.Join(rootDir, filepath.FromSlash(relPath))
	st, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fileParseResult{relPath: relPath, deleted: true}
		}
		return fileParseResult{relPath: relPath, err: fmt.Errorf("stat %q: %w", relPath, err)}
	}
	if !st.Mode().IsRegular() {
		return fileParseResult{relPath: relPath, skipped: true}
	}

	mtime := st.ModTime().UnixNano()
	if !forceRefresh {
		if cached, ok := cache[relPath]; ok && cached.mtime == mtime {
			return fileParseResult{relPath: relPath, skipped: true}
		}
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return fileParseResult{relPath: relPath, err: fmt.Errorf("read %q: %w", relPath, err)}
	}

	analysis, err := parser.Analyze(ctx, relPath, content)
	if err != nil {
		return fileParseResult{relPath: relPath, err: fmt.Errorf("analyze %q: %w", relPath, err)}
	}

	// Language detection — replicated inline from the original
	// upsertPathTags (lines 156-159). resolveLanguage does not exist
	// as a standalone method.
	language := treesitter.GetQueryKey(treesitter.MapPath(relPath))
	if analysis != nil && analysis.Language != "" {
		language = treesitter.GetQueryKey(analysis.Language)
	}

	tagsCap := 0
	if analysis != nil {
		tagsCap = len(analysis.Tags)
	}
	interner := newStringInterner(tagsCap + 8)
	internedRelPath := interner.Intern(relPath)
	internedLanguage := interner.Intern(language)
	tags := make([]treesitter.Tag, 0, tagsCap)
	if analysis != nil {
		for _, tag := range analysis.Tags {
			if tag.Kind != "def" && tag.Kind != "ref" {
				continue
			}
			tag.RelPath = internedRelPath
			tag.Name = interner.Intern(tag.Name)
			tag.Kind = interner.Intern(tag.Kind)
			tag.NodeType = interner.Intern(tag.NodeType)
			if tag.Language == "" {
				tag.Language = internedLanguage
			} else {
				tag.Language = interner.Intern(tag.Language)
			}
			tags = append(tags, tag)
		}
	}
	sortTagsDeterministic(tags)

	return fileParseResult{
		relPath:  relPath,
		mtime:    mtime,
		language: language,
		tags:     tags,
	}
}

// writePathTags persists the parsed tags for a single file into the
// database inside an existing transaction. Extracted from the DB-write
// portion of the original upsertPathTags.
func (s *Service) writePathTags(ctx context.Context, qtx *db.Queries, repoKey string, r fileParseResult) error {
	if err := qtx.UpsertRepoMapFileCache(ctx, db.UpsertRepoMapFileCacheParams{
		RepoKey:  repoKey,
		RelPath:  r.relPath,
		Mtime:    r.mtime,
		Language: r.language,
		TagCount: int64(len(r.tags)),
	}); err != nil {
		return fmt.Errorf("upsert file cache for %q: %w", r.relPath, err)
	}

	if err := qtx.DeleteRepoMapTagsByPath(ctx, db.DeleteRepoMapTagsByPathParams{RepoKey: repoKey, RelPath: r.relPath}); err != nil {
		return fmt.Errorf("delete existing tags for %q: %w", r.relPath, err)
	}

	for _, tag := range r.tags {
		if err := qtx.InsertRepoMapTag(ctx, db.InsertRepoMapTagParams{
			RepoKey:  repoKey,
			RelPath:  r.relPath,
			Name:     tag.Name,
			Kind:     tag.Kind,
			NodeType: tag.NodeType,
			Line:     int64(tag.Line),
			Language: tag.Language,
		}); err != nil {
			return fmt.Errorf("insert tag %q for %q: %w", tag.Name, r.relPath, err)
		}
	}

	return nil
}

// extractTags derives defs/refs from the file universe, normalizes all paths,
// persists incremental updates in repo_map tables, and returns a deterministic
// tag slice for downstream graph construction.
//
// The pipeline is split into three phases:
//   - Phase 0: Pre-load file cache (serial, non-transactional).
//   - Phase 1: Concurrent parse (no DB, errgroup worker pool).
//   - Phase 2: Sequential DB writes (inside transaction).
func (s *Service) extractTags(ctx context.Context, rootDir string, fileUniverse []string, forceRefresh bool) ([]treesitter.Tag, error) {
	if s == nil {
		return nil, errServiceClosed
	}
	if err := s.checkContextsDone(ctx); err != nil {
		return nil, err
	}
	if s.db == nil || s.rawDB == nil {
		return nil, fmt.Errorf("repo-map database is not configured")
	}

	normalizedFiles, err := normalizeFileUniverse(rootDir, fileUniverse)
	if err != nil {
		return nil, err
	}
	repoKey := repoKeyForRoot(rootDir)
	if repoKey == "" {
		return nil, fmt.Errorf("repo key is empty")
	}

	parser := s.ensureParser()
	if parser == nil {
		return nil, fmt.Errorf("tree-sitter parser is not available")
	}

	// ── Phase 0: Pre-load file cache (serial, non-transactional) ──
	// Uses s.db (not qtx) so this read is outside any transaction.
	cache, err := s.preloadFileCache(ctx, repoKey)
	if err != nil {
		return nil, err
	}

	// ── Phase 1: Concurrent parse (no DB) ──
	// Resolve pool size: config → runtime.NumCPU() → 1.
	// CRITICAL: SetLimit(0) causes deadlock — always clamp to >= 1.
	poolSize := 0
	if s.cfg != nil {
		poolSize = s.cfg.ParserPoolSize
	}
	if poolSize <= 0 {
		poolSize = runtime.NumCPU()
	}
	if poolSize < 1 {
		poolSize = 1
	}

	// Error semantics change (INTENTIONAL): The previous serial loop
	// (lines 84-91) returned immediately on the first upsertPathTags
	// error (fail-fast). The new pipeline stores per-file errors in
	// fileParseResult.err and skips them in Phase 2 (fail-soft). This
	// allows the remaining files to be processed even when individual
	// files fail to parse.
	results := make([]fileParseResult, len(normalizedFiles))

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(poolSize)

	for i, relPath := range normalizedFiles {
		g.Go(func() error {
			results[i] = s.parseFile(gCtx, parser, rootDir, relPath, forceRefresh, cache)
			return nil
		})
	}

	// errgroup goroutines never return errors (they store them in
	// results), so Wait always returns nil. Call it for cleanup.
	_ = g.Wait()

	// Check context cancellation after the parse phase.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// ── Phase 2: Sequential DB writes (inside transaction) ──
	tx, err := s.rawDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin repo-map tag transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		_ = tx.Rollback()
	}()

	qtx := s.db.WithTx(tx)

	// Prune stale cache entries inside the transaction.
	if err := pruneStaleCacheEntries(ctx, qtx, repoKey, normalizedFiles); err != nil {
		return nil, err
	}

	for _, r := range results {
		if r.skipped || r.err != nil {
			if r.err != nil {
				slog.Warn("Skipping file due to parse error",
					"path", r.relPath,
					"error", r.err)
			}
			continue
		}

		if r.deleted {
			if delErr := qtx.DeleteRepoMapFileCache(ctx, db.DeleteRepoMapFileCacheParams{
				RepoKey: repoKey,
				RelPath: r.relPath,
			}); delErr != nil {
				slog.Warn("Failed to delete file cache for deleted path",
					"path", r.relPath,
					"error", delErr)
			}
			continue
		}

		if err := s.writePathTags(ctx, qtx, repoKey, r); err != nil {
			slog.Warn("Failed to write tags for path",
				"path", r.relPath,
				"error", err)
			continue
		}
	}

	tagRows, err := qtx.ListRepoMapTags(ctx, repoKey)
	if err != nil {
		return nil, fmt.Errorf("list repo-map tags: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit repo-map tag transaction: %w", err)
	}
	committed = true

	// Post-loop interner for ListRepoMapTags stays sequential.
	interner := newStringInterner(len(tagRows) * 2)
	tags := make([]treesitter.Tag, 0, len(tagRows))
	for _, row := range tagRows {
		tags = append(tags, treesitter.Tag{
			RelPath:  interner.Intern(row.RelPath),
			Name:     interner.Intern(row.Name),
			Kind:     interner.Intern(row.Kind),
			Line:     int(row.Line),
			Language: interner.Intern(row.Language),
			NodeType: interner.Intern(row.NodeType),
		})
	}
	sortTagsDeterministic(tags)
	return tags, nil
}

func pruneStaleCacheEntries(ctx context.Context, qtx *db.Queries, repoKey string, liveRelPaths []string) error {
	cachedRows, err := qtx.GetRepoMapFileCache(ctx, repoKey)
	if err != nil {
		return fmt.Errorf("list repo-map file cache: %w", err)
	}

	live := make(map[string]struct{}, len(liveRelPaths))
	for _, relPath := range liveRelPaths {
		live[relPath] = struct{}{}
	}

	stale := make([]string, 0)
	for _, row := range cachedRows {
		if _, ok := live[row.RelPath]; ok {
			continue
		}
		stale = append(stale, row.RelPath)
	}
	sort.Strings(stale)

	for _, relPath := range stale {
		if err := qtx.DeleteRepoMapFileCache(ctx, db.DeleteRepoMapFileCacheParams{RepoKey: repoKey, RelPath: relPath}); err != nil {
			return fmt.Errorf("delete stale repo-map cache %q: %w", relPath, err)
		}
	}
	return nil
}

func sortTagsDeterministic(tags []treesitter.Tag) {
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].RelPath != tags[j].RelPath {
			return tags[i].RelPath < tags[j].RelPath
		}
		if tags[i].Line != tags[j].Line {
			return tags[i].Line < tags[j].Line
		}
		if tags[i].Kind != tags[j].Kind {
			return tags[i].Kind < tags[j].Kind
		}
		if tags[i].Name != tags[j].Name {
			return tags[i].Name < tags[j].Name
		}
		if tags[i].NodeType != tags[j].NodeType {
			return tags[i].NodeType < tags[j].NodeType
		}
		return tags[i].Language < tags[j].Language
	})
}

func (s *Service) ensureParser() treesitter.Parser {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.parser == nil {
		poolSize := 0
		if s.cfg != nil {
			poolSize = s.cfg.ParserPoolSize
		}
		factory := s.newParserWithCfg
		if factory == nil {
			factory = treesitter.NewParserWithConfig
		}
		s.parser = factory(treesitter.ParserConfig{PoolSize: poolSize})
	}
	return s.parser
}
