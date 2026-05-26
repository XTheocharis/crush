package prompt

import (
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	frontmatterRe = regexp.MustCompile(`(?s)\A---\s*\r?\n(.*?\r?\n)?---\s*\r?\n?`)
	htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
)

// StripYAMLFrontmatter removes YAML frontmatter (--- delimited block at file
// start). Content after the closing --- is returned unchanged.
func StripYAMLFrontmatter(content string) string {
	content = strings.TrimPrefix(content, "\uFEFF")
	return frontmatterRe.ReplaceAllString(content, "")
}

// StripHTMLComments removes all HTML comments from content.
func StripHTMLComments(content string) string {
	return htmlCommentRe.ReplaceAllString(content, "")
}

// SanitizeContent strips YAML frontmatter and HTML comments from content.
func SanitizeContent(content string) string {
	content = StripYAMLFrontmatter(content)
	content = StripHTMLComments(content)
	return content
}

// cacheEntry holds processed file content with its source mtime.
type cacheEntry struct {
	content string
	modTime time.Time
}

// ContextCache caches sanitized file contents, invalidating on mtime change.
type ContextCache struct {
	mu    sync.RWMutex
	files map[string]*cacheEntry
}

// NewContextCache creates a ready-to-use ContextCache.
func NewContextCache() *ContextCache {
	return &ContextCache{
		files: make(map[string]*cacheEntry),
	}
}

// Get returns a sanitized ContextFile for path, using cached data when the
// file's mtime has not changed. Returns nil when the file cannot be read.
func (c *ContextCache) Get(path string) *ContextFile {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	mt := info.ModTime()

	c.mu.RLock()
	entry, ok := c.files[path]
	c.mu.RUnlock()

	if ok && entry.modTime.Equal(mt) {
		return &ContextFile{Path: path, Content: entry.content}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	processed := SanitizeContent(string(raw))
	newEntry := &cacheEntry{content: processed, modTime: mt}

	c.mu.Lock()
	c.files[path] = newEntry
	c.mu.Unlock()

	return &ContextFile{Path: path, Content: processed}
}

// Invalidate removes a single path from the cache.
func (c *ContextCache) Invalidate(path string) {
	c.mu.Lock()
	delete(c.files, path)
	c.mu.Unlock()
}

// InvalidateAll clears the entire cache.
func (c *ContextCache) InvalidateAll() {
	c.mu.Lock()
	c.files = make(map[string]*cacheEntry)
	c.mu.Unlock()
}
