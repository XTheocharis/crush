package repomap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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

// extractTags derives defs/refs from the file universe, normalizes all paths,
// persists incremental updates in repo_map tables, and returns a deterministic
// tag slice for downstream graph construction.
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
	if err := pruneStaleCacheEntries(ctx, qtx, repoKey, normalizedFiles); err != nil {
		return nil, err
	}

	for _, relPath := range normalizedFiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := s.upsertPathTags(ctx, qtx, parser, rootDir, repoKey, relPath, forceRefresh); err != nil {
			return nil, err
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

func (s *Service) upsertPathTags(ctx context.Context, qtx *db.Queries, parser treesitter.Parser, rootDir, repoKey, relPath string, forceRefresh bool) error {
	absPath := filepath.Join(rootDir, filepath.FromSlash(relPath))
	st, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if delErr := qtx.DeleteRepoMapFileCache(ctx, db.DeleteRepoMapFileCacheParams{RepoKey: repoKey, RelPath: relPath}); delErr != nil {
				return fmt.Errorf("delete stale file cache for %q: %w", relPath, delErr)
			}
			return nil
		}
		return fmt.Errorf("stat %q: %w", relPath, err)
	}
	if !st.Mode().IsRegular() {
		return nil
	}

	mtime := st.ModTime().UnixNano()
	if !forceRefresh {
		cached, cacheErr := qtx.GetRepoMapFileCacheByPath(ctx, db.GetRepoMapFileCacheByPathParams{RepoKey: repoKey, RelPath: relPath})
		if cacheErr == nil && cached.Mtime == mtime {
			return nil
		}
		if cacheErr != nil && !errors.Is(cacheErr, sql.ErrNoRows) {
			return fmt.Errorf("read file cache for %q: %w", relPath, cacheErr)
		}
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read %q: %w", relPath, err)
	}

	analysis, err := parser.Analyze(ctx, relPath, content)
	if err != nil {
		return fmt.Errorf("analyze %q: %w", relPath, err)
	}

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

	if err := qtx.UpsertRepoMapFileCache(ctx, db.UpsertRepoMapFileCacheParams{
		RepoKey:  repoKey,
		RelPath:  relPath,
		Mtime:    mtime,
		Language: language,
		TagCount: int64(len(tags)),
	}); err != nil {
		return fmt.Errorf("upsert file cache for %q: %w", relPath, err)
	}

	if err := qtx.DeleteRepoMapTagsByPath(ctx, db.DeleteRepoMapTagsByPathParams{RepoKey: repoKey, RelPath: relPath}); err != nil {
		return fmt.Errorf("delete existing tags for %q: %w", relPath, err)
	}

	for _, tag := range tags {
		if err := qtx.InsertRepoMapTag(ctx, db.InsertRepoMapTagParams{
			RepoKey:  repoKey,
			RelPath:  relPath,
			Name:     tag.Name,
			Kind:     tag.Kind,
			NodeType: tag.NodeType,
			Line:     int64(tag.Line),
			Language: tag.Language,
		}); err != nil {
			return fmt.Errorf("insert tag %q for %q: %w", tag.Name, relPath, err)
		}
	}

	return nil
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
