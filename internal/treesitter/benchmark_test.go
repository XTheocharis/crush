package treesitter

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var benchmarkContent = []byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n")

// BenchmarkTreeCacheKeyFmtString benchmarks the original fmt.Sprintf implementation.
func BenchmarkTreeCacheKeyFmtString(b *testing.B) {
	b.ReportAllocs()
	path := "/path/to/file.go"
	h := fnv.New64a()
	for i := 0; i < b.N; i++ {
		h.Reset()
		_, _ = h.Write(benchmarkContent)
		_ = fmt.Sprintf("%s:%d:%x", path, len(benchmarkContent), h.Sum64())
	}
}

// BenchmarkTreeCacheKeyOptimized benchmarks the optimized implementation.
func BenchmarkTreeCacheKeyOptimized(b *testing.B) {
	b.ReportAllocs()
	path := "/path/to/file.go"
	h := fnv.New64a()
	for i := 0; i < b.N; i++ {
		h.Reset()
		_, _ = h.Write(benchmarkContent)
		hash := h.Sum64()
		buf := make([]byte, 0, len(path)+1+19+1+16)
		buf = append(buf, path...)
		buf = append(buf, ':')
		buf = strconv.AppendInt(buf, int64(len(benchmarkContent)), 10)
		buf = append(buf, ':')
		buf = strconv.AppendUint(buf, hash, 16)
		_ = string(buf)
	}
}

// BenchmarkTreeCacheKeyVariousPaths benchmarks keys with different path lengths.
func BenchmarkTreeCacheKeyVariousPaths(b *testing.B) {
	b.ReportAllocs()
	paths := []string{
		"a/b.go",
		"internal/config/config.go",
		"internal/agent/tools/bash.go",
		"github.com/user/project/pkg/module/file.go",
	}
	for _, path := range paths {
		b.Run(path, func(b *testing.B) {
			b.ReportAllocs()
			h := fnv.New64a()
			for i := 0; i < b.N; i++ {
				h.Reset()
				_, _ = h.Write(benchmarkContent)
				hash := h.Sum64()
				buf := make([]byte, 0, len(path)+1+19+1+16)
				buf = append(buf, path...)
				buf = append(buf, ':')
				buf = strconv.AppendInt(buf, int64(len(benchmarkContent)), 10)
				buf = append(buf, ':')
				buf = strconv.AppendUint(buf, hash, 16)
				_ = string(buf)
			}
		})
	}
}

// BenchmarkNewParser benchmarks creating new parsers with caching.
func BenchmarkNewParser(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p := NewParser()
		require.NoError(b, p.Close())
	}
}

// BenchmarkNewParser_Parallel benchmarks creating parsers in parallel.
func BenchmarkNewParser_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p := NewParser()
			require.NoError(b, p.Close())
		}
	})
}

// BenchmarkNewParser_Concurrent benchmarks concurrent parser creation without parallel.
func BenchmarkNewParser_Concurrent(b *testing.B) {
	b.ReportAllocs()
	var wg sync.WaitGroup
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := NewParser()
			require.NoError(b, p.Close())
		}()
	}
	wg.Wait()
}

// BenchmarkLoadLanguagesManifest benchmarks manifest loading (uncached).
func BenchmarkLoadLanguagesManifest(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = LoadLanguagesManifest()
	}
}

// BenchmarkLoadSupportedLanguages benchmarks the loadSupportedLanguages function.
func BenchmarkLoadSupportedLanguages(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		langSet, _ := loadSupportedLanguages()
		_ = langSet
	}
}

// BenchmarkLoadSupportedLanguages_Parallel benchmarks concurrent loadSupportedLanguages calls.
func BenchmarkLoadSupportedLanguages_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			langSet, _ := loadSupportedLanguages()
			_ = langSet
		}
	})
}
