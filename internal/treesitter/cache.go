package treesitter

import (
	"sync"
	"sync/atomic"

	lru "github.com/hashicorp/golang-lru/v2"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	defaultTreeCacheEntries  = 5000
	defaultTreeCacheMaxBytes = 256 * 1024 * 1024
	minEstimatedTreeBytes    = 32 * 1024
)

// CacheStats tracks basic cache counters.
type CacheStats struct {
	Hits      int64
	Misses    int64
	Evictions int64
}

type cacheEntry struct {
	tree           *tree_sitter.Tree
	estimatedBytes int64
}

// Cache stores master trees and returns clones to callers.
type Cache struct {
	mu         sync.Mutex
	entries    *lru.Cache[string, *cacheEntry]
	maxEntries int
	maxBytes   int64

	totalBytes atomic.Int64
	hits       atomic.Int64
	misses     atomic.Int64
	evictions  atomic.Int64

	closed bool
}

// DefaultCacheLimits returns default cache limits.
func DefaultCacheLimits() (maxEntries int, maxBytes int64) {
	return defaultTreeCacheEntries, defaultTreeCacheMaxBytes
}

// NewCache creates a new cache with provided limits.
func NewCache(maxEntries int, maxBytes int64) *Cache {
	if maxEntries <= 0 {
		maxEntries = defaultTreeCacheEntries
	}
	if maxBytes <= 0 {
		maxBytes = defaultTreeCacheMaxBytes
	}

	c := &Cache{
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
	}
	c.entries, _ = lru.NewWithEvict[string, *cacheEntry](maxEntries, c.onEvicted)
	return c
}

// EstimateTreeBytes returns the estimated memory footprint for one parsed tree.
func EstimateTreeBytes(content []byte) int64 {
	est := int64(len(content)) * 10
	if est < minEstimatedTreeBytes {
		return minEstimatedTreeBytes
	}
	return est
}

// Get retrieves a cached tree clone.
func (c *Cache) Get(key string) (*tree_sitter.Tree, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries.Get(key)
	if !ok || entry == nil || entry.tree == nil {
		c.misses.Add(1)
		return nil, false
	}

	c.hits.Add(1)
	return entry.tree.Clone(), true
}

// Put stores a master tree in cache.
func (c *Cache) Put(key string, tree *tree_sitter.Tree, content []byte) {
	if tree == nil {
		return
	}

	estimated := EstimateTreeBytes(content)
	entry := &cacheEntry{tree: tree, estimatedBytes: estimated}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		tree.Close()
		return
	}

	if _, exists := c.entries.Get(key); exists {
		c.entries.Remove(key)
	}

	c.totalBytes.Add(estimated)
	c.entries.Add(key, entry)

	for c.totalBytes.Load() > c.maxBytes && c.entries.Len() > 0 {
		c.entries.RemoveOldest()
	}
}

// Invalidate removes a single cache entry.
func (c *Cache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}
	c.entries.Remove(key)
}

// Clear removes all cache entries.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}
	c.entries.Purge()
}

// Stats returns cache statistics snapshot.
func (c *Cache) Stats() CacheStats {
	return CacheStats{
		Hits:      c.hits.Load(),
		Misses:    c.misses.Load(),
		Evictions: c.evictions.Load(),
	}
}

// TotalBytes returns current estimated memory usage.
func (c *Cache) TotalBytes() int64 {
	return c.totalBytes.Load()
}

// Close releases cache resources.
func (c *Cache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.entries.Purge()
	c.closed = true
	return nil
}

func (c *Cache) onEvicted(_ string, entry *cacheEntry) {
	if entry == nil {
		return
	}
	c.evictions.Add(1)
	c.totalBytes.Add(-entry.estimatedBytes)
	if entry.tree != nil {
		entry.tree.Close()
	}
}
