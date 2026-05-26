package agent

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	DefaultDiagnosticsTTL = 5 * time.Minute
	DefaultRepoMapTTL     = 30 * time.Minute
)

type cacheEntry struct {
	value  any
	expiry time.Time
}

func (e cacheEntry) expired() bool {
	return !e.expiry.IsZero() && time.Now().After(e.expiry)
}

// SharedCache is a thread-safe, TTL-aware key-value store shared between the
// coordinator and its subagents. Keys are colon-separated with a category
// prefix (e.g. "diagnostics:session-abc") so that Invalidate("diagnostics:*")
// can evict a whole category at once.
type SharedCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

func NewSharedCache() *SharedCache {
	return &SharedCache{
		entries: make(map[string]cacheEntry),
	}
}

// Get retrieves a cached value by key. It returns the value and true if the
// entry exists and has not expired. Expired entries are lazily evicted.
func (c *SharedCache) Get(key string) (any, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	if entry.expired() {
		c.mu.Lock()
		if e, still := c.entries[key]; still && e.expired() {
			delete(c.entries, key)
		}
		c.mu.Unlock()
		return nil, false
	}

	return entry.value, true
}

// Set stores value under key with the given TTL. A zero TTL means the entry
// never expires.
func (c *SharedCache) Set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expiry time.Time
	if ttl > 0 {
		expiry = time.Now().Add(ttl)
	}
	c.entries[key] = cacheEntry{value: value, expiry: expiry}
}

// Invalidate removes all entries whose keys match the given pattern.
// The pattern supports a single trailing wildcard ("*") which matches any
// suffix. For example, Invalidate("diagnostics:*") removes all diagnostic
// entries while Invalidate("diagnostics:session-1") removes only that one.
func (c *SharedCache) Invalidate(pattern string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		for k := range c.entries {
			if strings.HasPrefix(k, prefix) {
				delete(c.entries, k)
			}
		}
		return
	}

	delete(c.entries, pattern)
}

func (c *SharedCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry)
}

func (c *SharedCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func (c *SharedCache) KeysByPrefix(prefix string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var keys []string
	now := time.Now()
	for k, e := range c.entries {
		if strings.HasPrefix(k, prefix) && (e.expiry.IsZero() || now.Before(e.expiry)) {
			keys = append(keys, k)
		}
	}
	return keys
}

func CacheKey(parts ...string) string {
	normalized := make([]string, len(parts))
	for i, p := range parts {
		normalized[i] = filepath.ToSlash(p)
	}
	return strings.Join(normalized, ":")
}
