package repomap

import (
	"strings"
	"sync"
)

// SessionCache holds the cached repo map and its associated token count for a single session.
// This struct ensures atomic updates and consistent reads of the map and token pair.
type SessionCache struct {
	mu    sync.RWMutex
	value sessionStorageValue
}

type sessionStorageValue struct {
	mapString  string
	tokenCount int
}

type renderCacheEntry struct {
	mapString  string
	tokenCount int
}

type RenderCache struct {
	mu      sync.RWMutex
	entries map[string]renderCacheEntry
}

type SessionRenderCacheSet struct {
	mu       sync.RWMutex
	sessions map[string]*RenderCache
}

func NewRenderCache() *RenderCache {
	return &RenderCache{entries: make(map[string]renderCacheEntry)}
}

func NewSessionRenderCacheSet() *SessionRenderCacheSet {
	return &SessionRenderCacheSet{sessions: make(map[string]*RenderCache)}
}

func (s *SessionRenderCacheSet) GetOrCreate(sessionID string) *RenderCache {
	s.mu.RLock()
	cache, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if ok {
		return cache
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if cache, ok = s.sessions[sessionID]; ok {
		return cache
	}
	cache = NewRenderCache()
	s.sessions[sessionID] = cache
	return cache
}

func (s *SessionRenderCacheSet) Get(sessionID string) *RenderCache {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionID]
}

func (s *SessionRenderCacheSet) Clear(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

func (s *SessionRenderCacheSet) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = make(map[string]*RenderCache)
}

func (c *RenderCache) Get(key string) (string, int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok {
		return "", 0, false
	}
	return entry.mapString, entry.tokenCount, true
}

func (c *RenderCache) Set(key, mapString string, tokenCount int) {
	if strings.TrimSpace(key) == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = renderCacheEntry{mapString: mapString, tokenCount: tokenCount}
}

func (c *RenderCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]renderCacheEntry)
}

// NewSessionCache creates a new session cache with empty initial state.
func NewSessionCache() *SessionCache {
	return &SessionCache{}
}

// Store atomically updates the cache with a new map and token count.
// Both values are updated together to maintain consistency invariant.
func (c *SessionCache) Store(mapString string, tokenCount int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = sessionStorageValue{
		mapString:  mapString,
		tokenCount: tokenCount,
	}
}

// Load returns the cached map and token count as a consistent pair.
// The read is performed under a read lock to prevent observing partial updates.
func (c *SessionCache) Load() (mapString string, tokenCount int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value.mapString, c.value.tokenCount
}

// Clear removes all cached values for this session.
func (c *SessionCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = sessionStorageValue{}
}

// SessionCacheSet manages per-session caches with thread-safe access.
type SessionCacheSet struct {
	mu       sync.RWMutex
	sessions map[string]*SessionCache
}

// NewSessionCacheSet creates a new set of session caches.
func NewSessionCacheSet() *SessionCacheSet {
	return &SessionCacheSet{
		sessions: make(map[string]*SessionCache),
	}
}

// GetOrCreate returns an existing cache for the session or creates a new one.
// The returned cache pointer is stable and can be used for subsequent operations.
func (s *SessionCacheSet) GetOrCreate(sessionID string) *SessionCache {
	s.mu.RLock()
	cache, exists := s.sessions[sessionID]
	s.mu.RUnlock()

	if exists {
		return cache
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check in case another goroutine created it while we waited
	if cache, exists = s.sessions[sessionID]; exists {
		return cache
	}

	cache = NewSessionCache()
	s.sessions[sessionID] = cache
	return cache
}

// Get returns the cache for a session, or nil if not found.
func (s *SessionCacheSet) Get(sessionID string) *SessionCache {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionID]
}

// Store atomically sets the map and token count for a session.
// If the session doesn't have a cache, one is created automatically.
func (s *SessionCacheSet) Store(sessionID, mapString string, tokenCount int) {
	cache := s.GetOrCreate(sessionID)
	cache.Store(mapString, tokenCount)
}

// Load returns the cached map and token count for a session.
// If the session has no cache or the cache is empty, returns empty values.
func (s *SessionCacheSet) Load(sessionID string) (mapString string, tokenCount int) {
	cache := s.Get(sessionID)
	if cache == nil {
		return "", 0
	}
	return cache.Load()
}

// Clear removes the cache for a session.
// If the session has no cache, this is a no-op.
func (s *SessionCacheSet) Clear(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

// ClearAll removes all session caches.
func (s *SessionCacheSet) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = make(map[string]*SessionCache)
}

// Size returns the number of sessions with active caches.
func (s *SessionCacheSet) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}
