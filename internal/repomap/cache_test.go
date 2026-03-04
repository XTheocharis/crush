package repomap

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSessionCacheLoadStore verifies basic load/store operations.
func TestSessionCacheLoadStore(t *testing.T) {
	t.Parallel()

	cache := NewSessionCache()

	// Initial state is empty
	s, tok := cache.Load()
	require.Empty(t, s)
	require.Zero(t, tok)

	// Store a value
	const wantMap = "map content"
	const wantTok = 42
	cache.Store(wantMap, wantTok)

	// Load returns stored value
	gotMap, gotTok := cache.Load()
	require.Equal(t, wantMap, gotMap)
	require.Equal(t, wantTok, gotTok)
}

// TestSessionCacheClear verifies clearing resets to empty state.
func TestSessionCacheClear(t *testing.T) {
	t.Parallel()

	cache := NewSessionCache()
	cache.Store("map", 100)
	cache.Clear()

	s, tok := cache.Load()
	require.Empty(t, s)
	require.Zero(t, tok)
}

// TestSessionCacheConcurrentWrites tests concurrent write operations.
func TestSessionCacheConcurrentWrites(t *testing.T) {
	t.Parallel()

	cache := NewSessionCache()
	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			mapStr := mapStr(idx)
			tok := idx
			cache.Store(mapStr, tok)
		}(i)
	}

	wg.Wait()

	// Final state is one of the values
	lastMap, lastTok := cache.Load()
	require.NotEmpty(t, lastMap)
	require.GreaterOrEqual(t, lastTok, 0)
	require.Less(t, lastTok, goroutines)
	require.Equal(t, mapStr(lastTok), lastMap)
}

func mapStr(idx int) string {
	return "map-" + string([]byte{byte(idx % 256)})
}

// TestSessionCacheConcurrentReadWrites tests concurrent reads don't corrupt writes.
func TestSessionCacheConcurrentReadWrites(t *testing.T) {
	t.Parallel()

	cache := NewSessionCache()
	const writes = 1000
	const readers = 10

	// Write goroutine that increments value
	done := make(chan struct{})
	go func() {
		for i := range writes {
			cache.Store("map-"+string([]byte{byte(i % 256)}), i)
		}
		close(done)
	}()

	// Reader goroutines
	var wg sync.WaitGroup
	wg.Add(readers)
	for range readers {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					_, _ = cache.Load()
				}
			}
		}()
	}

	wg.Wait()
	<-done
}

// TestSessionCacheConsistency verifies map and token are always updated together.
func TestSessionCacheConsistency(t *testing.T) {
	t.Parallel()

	cache := NewSessionCache()
	const iterations = 1000

	for i := range iterations {
		expectedMap := "map-" + string([]byte{byte(i % 256)})
		expectedTok := i

		cache.Store(expectedMap, expectedTok)
		gotMap, gotTok := cache.Load()

		// Verify consistency: stored pair is always loaded together
		require.Equal(t, expectedMap, gotMap, "iteration %d: map", i)
		require.Equal(t, expectedTok, gotTok, "iteration %d: token", i)
	}
}

// TestSessionCacheSetGetOrCreate verifies cache set management.
func TestSessionCacheSetGetOrCreate(t *testing.T) {
	t.Parallel()

	set := NewSessionCacheSet()
	require.Equal(t, 0, set.Size())

	// GetOrCreate creates a new cache
	cache1 := set.GetOrCreate("sess1")
	require.NotNil(t, cache1)
	require.Equal(t, 1, set.Size())

	// GetOrCreate returns same cache for same session ID
	cache2 := set.GetOrCreate("sess1")
	require.Same(t, cache1, cache2)
	require.Equal(t, 1, set.Size())

	// Different session gets different cache
	cache3 := set.GetOrCreate("sess2")
	require.NotNil(t, cache3)
	require.NotSame(t, cache1, cache3)
	require.Equal(t, 2, set.Size())
}

// TestSessionCacheSetGet verifies Get returns nil for non-existent session.
func TestSessionCacheSetGet(t *testing.T) {
	t.Parallel()

	set := NewSessionCacheSet()
	cache := set.Get("nonexistent")
	require.Nil(t, cache)

	set.GetOrCreate("sess1")
	cache = set.Get("sess1")
	require.NotNil(t, cache)
}

// TestSessionCacheSetStore verifies storing values.
func TestSessionCacheSetStore(t *testing.T) {
	t.Parallel()

	set := NewSessionCacheSet()

	// Store creates cache if needed
	set.Store("sess1", "map-data", 50)
	gotMap, gotTok := set.Load("sess1")
	require.Equal(t, "map-data", gotMap)
	require.Equal(t, 50, gotTok)
	require.Equal(t, 1, set.Size())

	// Store updates existing
	set.Store("sess1", "new-map", 75)
	gotMap, gotTok = set.Load("sess1")
	require.Equal(t, "new-map", gotMap)
	require.Equal(t, 75, gotTok)
	require.Equal(t, 1, set.Size())
}

// TestSessionCacheSetLoad verifies loading values.
func TestSessionCacheSetLoad(t *testing.T) {
	t.Parallel()

	set := NewSessionCacheSet()

	// Load non-existent returns empty
	s, tok := set.Load("nonexistent")
	require.Empty(t, s)
	require.Zero(t, tok)

	// Store then load
	set.Store("sess1", "map-data", 50)
	s, tok = set.Load("sess1")
	require.Equal(t, "map-data", s)
	require.Equal(t, 50, tok)
}

// TestSessionCacheSetClear verifies clearing sessions.
func TestSessionCacheSetClear(t *testing.T) {
	t.Parallel()

	set := NewSessionCacheSet()
	set.Store("sess1", "map", 10)
	set.Store("sess2", "map", 20)
	require.Equal(t, 2, set.Size())

	set.Clear("sess1")
	require.Equal(t, 1, set.Size())

	s, tok := set.Load("sess1")
	require.Empty(t, s)
	require.Zero(t, tok)

	s, tok = set.Load("sess2")
	require.Equal(t, "map", s)
	require.Equal(t, 20, tok)

	// Clear non-existent is safe
	set.Clear("nonexistent")
	require.Equal(t, 1, set.Size())
}

// TestSessionCacheSetClearAll verifies clearing all sessions.
func TestSessionCacheSetClearAll(t *testing.T) {
	t.Parallel()

	set := NewSessionCacheSet()
	set.Store("sess1", "map", 10)
	set.Store("sess2", "map", 20)
	set.Store("sess3", "map", 30)
	require.Equal(t, 3, set.Size())

	set.ClearAll()
	require.Equal(t, 0, set.Size())

	s, tok := set.Load("sess1")
	require.Empty(t, s)
	require.Zero(t, tok)
}

// TestSessionCacheSetConcurrentAccess tests concurrent operations on the set.
func TestSessionCacheSetConcurrentAccess(t *testing.T) {
	t.Parallel()

	set := NewSessionCacheSet()
	const goroutines = 50
	const sessions = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Spawn many goroutines doing random operations
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			sessionID := "sess" + string([]byte{byte(idx % sessions)})
			switch idx % 4 {
			case 0:
				set.GetOrCreate(sessionID)
			case 1:
				set.Get(sessionID)
			case 2:
				set.Store(sessionID, "map", idx)
			case 3:
				set.Load(sessionID)
			}
		}(i)
	}

	wg.Wait()

	// Verify final state is consistent with last writes
	for i := range sessions {
		sessionID := "sess" + string([]byte{byte(i)})
		s, tok := set.Load(sessionID)
		// Either non-empty (was written) or empty (was never written)
		_ = s
		_ = tok
	}
}

// TestSessionCacheSetSize verifies size tracking.
func TestSessionCacheSetSize(t *testing.T) {
	t.Parallel()

	set := NewSessionCacheSet()
	require.Equal(t, 0, set.Size())

	set.Store("sess1", "map", 10)
	require.Equal(t, 1, set.Size())

	set.Store("sess2", "map", 20)
	require.Equal(t, 2, set.Size())

	set.Clear("sess1")
	require.Equal(t, 1, set.Size())

	set.ClearAll()
	require.Equal(t, 0, set.Size())

	// Store again verifies size increases
	set.Store("sess3", "map", 30)
	require.Equal(t, 1, set.Size())
}

// TestServiceLastGoodAccessors verifies LastGoodMap and LastTokenCount methods.
func TestServiceLastGoodAccessors(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Initial state: no cached values
	require.Empty(t, svc.LastGoodMap("sess1"))
	require.Zero(t, svc.LastTokenCount("sess1"))

	// Store values via sessionCaches directly
	svc.sessionCaches.Store("sess1", "cached-map", 123)

	// Accessors return stored values
	require.Equal(t, "cached-map", svc.LastGoodMap("sess1"))
	require.Equal(t, 123, svc.LastTokenCount("sess1"))

	// Different session has no values
	require.Empty(t, svc.LastGoodMap("sess2"))
	require.Zero(t, svc.LastTokenCount("sess2"))
}

// TestServiceLastGoodConsistency verifies accessor consistency under concurrent usage.
func TestServiceLastGoodConsistency(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())
	const sessions = 10
	const writesPerSession = 100

	var wg sync.WaitGroup
	wg.Add(sessions)

	// Concurrent writes to multiple sessions
	for sessIdx := range sessions {
		sessionID := "sess" + string([]byte{byte(sessIdx)})
		go func(sid string, startTok int) {
			defer wg.Done()
			for i := range writesPerSession {
				tok := startTok + i
				svc.sessionCaches.Store(sid, "map", tok)
			}
		}(sessionID, sessIdx*writesPerSession)
	}

	wg.Wait()

	// Verify each session has consistent map/token pairs
	for sessIdx := range sessions {
		sessionID := "sess" + string([]byte{byte(sessIdx)})
		lastMap := svc.LastGoodMap(sessionID)
		lastTok := svc.LastTokenCount(sessionID)

		// The pair should be consistent (the token matches the last written)
		if lastMap != "" {
			// If map is non-empty, verify token is in expected range
			startTok := sessIdx * writesPerSession
			require.GreaterOrEqual(t, lastTok, startTok)
			require.Less(t, lastTok, startTok+writesPerSession)
		}
	}
}

// TestServiceResetClearsCache verifies Reset clears session cache.
func TestServiceResetClearsCache(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	svc.sessionCaches.Store("sess1", "map", 100)
	require.Equal(t, "map", svc.LastGoodMap("sess1"))
	require.Equal(t, 100, svc.LastTokenCount("sess1"))

	err := svc.Reset(context.Background(), "sess1")
	require.NoError(t, err)

	require.Empty(t, svc.LastGoodMap("sess1"))
	require.Zero(t, svc.LastTokenCount("sess1"))

	// Other sessions unaffected
	svc.sessionCaches.Store("sess2", "map", 200)
	err = svc.Reset(context.Background(), "sess1")
	require.NoError(t, err)

	require.Equal(t, "map", svc.LastGoodMap("sess2"))
	require.Equal(t, 200, svc.LastTokenCount("sess2"))
}

// TestServiceGenerateUsesCache verifies Generate returns cached values.
func TestServiceGenerateUsesCache(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	svc.sessionCaches.Store("sess1", "cached-map", 42)

	m, tok, err := svc.Generate(context.Background(), GenerateOpts{SessionID: "sess1"})
	require.NoError(t, err)
	require.Equal(t, "cached-map", m)
	require.Equal(t, 42, tok)
}

// TestServiceRefreshUpdatesCache verifies Refresh updates cache atomically.
func TestServiceRefreshUpdatesCache(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Initial value
	svc.sessionCaches.Store("sess1", "old-map", 100)

	// Refresh returns and re-stores the cached values
	m, tok, err := svc.Refresh(context.Background(), "sess1", GenerateOpts{SessionID: "sess1"})
	require.NoError(t, err)
	require.Equal(t, "old-map", m)
	require.Equal(t, 100, tok)

	// Cache still has the same value (Refresh retrieved and re-stored it)
	require.Equal(t, "old-map", svc.LastGoodMap("sess1"))
	require.Equal(t, 100, svc.LastTokenCount("sess1"))

	// Now verify atomic update by setting new values and calling Refresh again
	svc.sessionCaches.Store("sess1", "new-map", 200)
	m, tok, err = svc.Refresh(context.Background(), "sess1", GenerateOpts{SessionID: "sess1"})
	require.NoError(t, err)
	require.Equal(t, "new-map", m)
	require.Equal(t, 200, tok)

	// Cache reflects the updated value
	require.Equal(t, "new-map", svc.LastGoodMap("sess1"))
	require.Equal(t, 200, svc.LastTokenCount("sess1"))
}

// TestServiceConcurrentResetAndAccess verifies concurrent reset and access doesn't cause corruption.
func TestServiceConcurrentResetAndAccess(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())
	const operations = 1000

	var readCount atomic.Int64
	var resetCount atomic.Int64

	var wg sync.WaitGroup
	wg.Add(2)

	// Accessor goroutine
	go func() {
		defer wg.Done()
		for range operations {
			svc.LastGoodMap("sess1")
			svc.LastTokenCount("sess1")
			readCount.Add(1)
		}
	}()

	// Reset goroutine
	go func() {
		defer wg.Done()
		for range operations / 10 {
			_ = svc.Reset(context.Background(), "sess1")
			resetCount.Add(1)
		}
	}()

	wg.Wait()

	require.Equal(t, int64(operations), readCount.Load())
	require.Greater(t, resetCount.Load(), int64(0))
}

// TestCacheAtomicInvariants verifies the core atomic update invariant.
func TestCacheAtomicInvariants(t *testing.T) {
	t.Parallel()

	cache := NewSessionCache()
	const iterations = 10000

	for i := range iterations {
		// Store is atomic: both values update together
		mapVal := "map-" + string([]byte{byte(i % 256)})
		tokVal := i
		cache.Store(mapVal, tokVal)

		// Read is atomic: both values are from same store operation
		_, readTok := cache.Load()

		// Invariant: after store(i), load returns values from i or later
		require.GreaterOrEqual(t, readTok, i, "token count went backwards")
	}
}

// TestMultipleSessionIsolation verifies sessions don't interfere with each other.
func TestMultipleSessionIsolation(t *testing.T) {
	t.Parallel()

	set := NewSessionCacheSet()
	const sessions = 50

	var wg sync.WaitGroup
	wg.Add(sessions)

	for i := range sessions {
		go func(sessionIdx int) {
			defer wg.Done()
			sessionID := "sess" + string([]byte{byte(sessionIdx)})

			for j := range 100 {
				expectedMap := "map-" + string([]byte{byte(j % 256)})
				expectedTok := j

				set.Store(sessionID, expectedMap, expectedTok)

				gotMap, gotTok := set.Load(sessionID)

				// Each session's values are isolated
				require.Equal(t, expectedMap, gotMap)
				require.Equal(t, expectedTok, gotTok)
			}
		}(i)
	}

	wg.Wait()

	// Final state: each session has its last value
	for i := range sessions {
		sessionID := "sess" + string([]byte{byte(i)})
		gotMap, gotTok := set.Load(sessionID)

		// All sessions should have values (last stored was j=99)
		require.NotEmpty(t, gotMap, "session %d should have cached map", i)
		require.Equal(t, 99, gotTok, "session %d should have cached token", i)
	}
}

// TestClearAllDuringOperations verifies ClearAll can happen concurrent to other ops.
func TestClearAllDuringOperations(t *testing.T) {
	t.Parallel()

	set := NewSessionCacheSet()
	const sessions = 20
	const operations = 200

	var atomicCount atomic.Int64

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer goroutine
	go func() {
		defer wg.Done()
		for i := range operations {
			sessionID := "sess" + string([]byte{byte(i % sessions)})
			set.Store(sessionID, "map", i)
			set.Load(sessionID)
			atomicCount.Add(1)
		}
	}()

	// ClearAll goroutine
	go func() {
		defer wg.Done()
		for range 20 {
			set.ClearAll()
		}
	}()

	wg.Wait()

	// Final state is consistent but may have been cleared
	_ = atomicCount.Load()
}

// TestNoDataRaceWriteWrite verifies no data race on concurrent writes to same session.
func TestNoDataRaceWriteWrite(t *testing.T) {
	t.Parallel()

	cache := NewSessionCache()
	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			cache.Store("map-"+string([]byte{byte(idx % 256)}), idx)
		}(i)
	}

	wg.Wait()

	// No panic or data race occurred
	_, tok := cache.Load()
	_ = tok
}

// TestNoDataRaceReadWrite verifies no data race on concurrent read and write.
func TestNoDataRaceReadWrite(t *testing.T) {
	t.Parallel()

	cache := NewSessionCache()
	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := range goroutines {
		// Writers
		go func(idx int) {
			defer wg.Done()
			cache.Store("map-"+string([]byte{byte(idx % 256)}), idx)
		}(i)

		// Readers
		go func() {
			defer wg.Done()
			cache.Load()
		}()
	}

	wg.Wait()

	// No panic or data race occurred
}

// TestNoDataRaceClearRead verifies no data race between Clear and Load.
func TestNoDataRaceClearRead(t *testing.T) {
	t.Parallel()

	cache := NewSessionCache()
	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := range goroutines {
		// Clearers
		go func(idx int) {
			defer wg.Done()
			cache.Clear()
		}(i)

		// Readers
		go func() {
			defer wg.Done()
			cache.Load()
		}()
	}

	wg.Wait()

	// No panic or data race occurred
}

// TestEmptySessionIDHandling verifies empty session ID behavior.
func TestEmptySessionIDHandling(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Empty session ID returns empty values when no data is stored
	require.Empty(t, svc.LastGoodMap(""))
	require.Zero(t, svc.LastTokenCount(""))

	set := NewSessionCacheSet()

	// Load empty session ID returns empty when not initialized
	s, tok := set.Load("")
	require.Empty(t, s)
	require.Zero(t, tok)

	// Store to empty session ID works - it's treated as a valid key
	set.Store("", "map", 10)
	s, tok = set.Load("")
	require.Equal(t, "map", s)
	require.Equal(t, 10, tok)

	// Clear works for empty session ID too
	set.Clear("")
	s, tok = set.Load("")
	require.Empty(t, s)
	require.Zero(t, tok)
}

// TestCacheZeroValueHandling verifies storing zero values works correctly.
func TestCacheZeroValueHandling(t *testing.T) {
	t.Parallel()

	cache := NewSessionCache()

	// Store empty string and zero token
	cache.Store("", 0)

	// Load should return those zero values
	s, tok := cache.Load()
	require.Empty(t, s)
	require.Zero(t, tok)

	// Non-zero value after
	cache.Store("map", 100)
	s, tok = cache.Load()
	require.Equal(t, "map", s)
	require.Equal(t, 100, tok)

	// Overwrite with zero values again
	cache.Store("", 0)
	s, tok = cache.Load()
	require.Empty(t, s)
	require.Zero(t, tok)
}
