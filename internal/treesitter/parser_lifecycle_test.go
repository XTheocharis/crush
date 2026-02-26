package treesitter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParserPoolAcquireReleaseReusesInstances(t *testing.T) {
	t.Parallel()

	var created atomic.Int32
	pool := newParserPoolWithFactory(2, func() *languageParser {
		created.Add(1)
		return &languageParser{}
	})
	t.Cleanup(func() {
		require.NoError(t, pool.Close())
	})

	require.Equal(t, 2, pool.Capacity())
	require.Equal(t, int32(2), created.Load())

	first, ok := pool.Acquire(context.Background(), "go")
	require.True(t, ok)
	second, ok := pool.Acquire(context.Background(), "python")
	require.True(t, ok)
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.NotSame(t, first, second)

	pool.Release("go", first)
	pool.Release("python", second)

	third, ok := pool.Acquire(context.Background(), "go")
	require.True(t, ok)
	fourth, ok := pool.Acquire(context.Background(), "python")
	require.True(t, ok)

	require.True(t, third == first || third == second)
	require.True(t, fourth == first || fourth == second)
	pool.Release("go", third)
	pool.Release("python", fourth)
}

func TestParserPoolAcquireHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	pool := newParserPoolWithFactory(1, func() *languageParser { return &languageParser{} })
	t.Cleanup(func() {
		require.NoError(t, pool.Close())
	})

	held, ok := pool.Acquire(context.Background(), "go")
	require.True(t, ok)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	lp, ok := pool.Acquire(ctx, "go")
	require.False(t, ok)
	require.Nil(t, lp)

	pool.Release("go", held)
}

func TestParserPoolCloseUnblocksWaitingAcquire(t *testing.T) {
	t.Parallel()

	pool := newParserPoolWithFactory(1, func() *languageParser { return &languageParser{} })

	held, ok := pool.Acquire(context.Background(), "go")
	require.True(t, ok)

	acquired := make(chan bool, 1)
	go func() {
		_, ok := pool.Acquire(context.Background(), "python")
		acquired <- ok
	}()

	closeDone := make(chan struct{})
	go func() {
		_ = pool.Close()
		close(closeDone)
	}()

	select {
	case ok := <-acquired:
		require.False(t, ok)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("acquire did not unblock after close")
	}

	// Close should still wait for the already-acquired holder.
	select {
	case <-closeDone:
		t.Fatal("close returned before holder release")
	case <-time.After(50 * time.Millisecond):
	}

	pool.Release("go", held)

	select {
	case <-closeDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("close did not finish after holder release")
	}
}

func TestParserPoolCloseWaitsForOutstandingHolder(t *testing.T) {
	t.Parallel()

	pool := newParserPoolWithFactory(1, func() *languageParser { return &languageParser{} })

	held, ok := pool.Acquire(context.Background(), "go")
	require.True(t, ok)

	closed := make(chan struct{})
	go func() {
		_ = pool.Close()
		close(closed)
	}()

	select {
	case <-closed:
		t.Fatal("close returned before holder release")
	case <-time.After(50 * time.Millisecond):
		// expected to block
	}

	pool.Release("go", held)

	select {
	case <-closed:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("close did not finish after holder release")
	}
}

func TestParserCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	p := NewParser()
	require.NoError(t, p.Close())
	require.NoError(t, p.Close())
}

func TestNewParserWithConfigUsesPoolSize(t *testing.T) {
	t.Parallel()

	p := NewParserWithConfig(ParserConfig{PoolSize: 3})
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	concrete, ok := p.(*parser)
	require.True(t, ok)
	require.NotNil(t, concrete.pool)
	require.Equal(t, 3, concrete.pool.Capacity())
}

func TestNewParserWithConfigUsesDefaultForNonPositivePoolSize(t *testing.T) {
	t.Parallel()

	p := NewParserWithConfig(ParserConfig{PoolSize: 0})
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	concrete, ok := p.(*parser)
	require.True(t, ok)
	require.NotNil(t, concrete.pool)
	require.Equal(t, defaultParserPoolSize(), concrete.pool.Capacity())
}

func TestParserAnalyzeReturnsContextErrorWhenAcquireCanceled(t *testing.T) {
	t.Parallel()

	pp := newParserPoolWithFactory(1, func() *languageParser { return &languageParser{} })
	t.Cleanup(func() {
		require.NoError(t, pp.Close())
	})

	held, ok := pp.Acquire(context.Background(), "go")
	require.True(t, ok)

	pr := &parser{pool: pp}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	analysis, err := pr.Analyze(ctx, "/tmp/file.go", []byte("package main"))
	require.Nil(t, analysis)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	pp.Release("go", held)
}

func TestParserPoolCloseIsSafeConcurrent(t *testing.T) {
	t.Parallel()

	var closedCount atomic.Int32
	pool := newParserPoolWithFactory(3, func() *languageParser {
		return &languageParser{closeFn: func() { closedCount.Add(1) }}
	})

	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			_ = pool.Close()
		})
	}
	wg.Wait()

	// Exactly pool capacity parsers should be closed once.
	require.Equal(t, int32(pool.Capacity()), closedCount.Load())
}

func TestParserAnalyzeGoBootstrapTagsAndSymbols(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	src := []byte(`package main

// Run executes.
func Run() {
	Run()
}
`)

	analysis, err := p.Analyze(context.Background(), "/tmp/main.go", src)
	require.NoError(t, err)
	require.NotNil(t, analysis)
	require.Equal(t, "go", analysis.Language)

	hasRunDef := false
	hasRunRef := false
	for _, tag := range analysis.Tags {
		if tag.Name == "Run" && tag.Kind == "def" && tag.NodeType == "function" {
			hasRunDef = true
		}
		if tag.Name == "Run" && tag.Kind == "ref" && tag.NodeType == "call" {
			hasRunRef = true
		}
	}
	require.True(t, hasRunDef)
	require.True(t, hasRunRef)

	hasRunSymbol := false
	for _, symbol := range analysis.Symbols {
		if symbol.Name == "Run" && symbol.Kind == "function" {
			hasRunSymbol = true
			require.GreaterOrEqual(t, symbol.EndLine, symbol.Line)
		}
	}
	require.True(t, hasRunSymbol)
}

func TestParserAnalyzePythonBootstrapTagsAndSymbols(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	src := []byte(`class Item:
	pass


def make():
	make()
`)

	analysis, err := p.Analyze(context.Background(), "/tmp/main.py", src)
	require.NoError(t, err)
	require.NotNil(t, analysis)
	require.Equal(t, "python", analysis.Language)

	hasItemDef := false
	hasMakeDef := false
	hasMakeRef := false
	for _, tag := range analysis.Tags {
		if tag.Name == "Item" && tag.Kind == "def" && tag.NodeType == "class" {
			hasItemDef = true
		}
		if tag.Name == "make" && tag.Kind == "def" && tag.NodeType == "function" {
			hasMakeDef = true
		}
		if tag.Name == "make" && tag.Kind == "ref" && tag.NodeType == "call" {
			hasMakeRef = true
		}
	}
	require.True(t, hasItemDef)
	require.True(t, hasMakeDef)
	require.True(t, hasMakeRef)

	hasItemSymbol := false
	hasMakeSymbol := false
	for _, symbol := range analysis.Symbols {
		if symbol.Name == "Item" && symbol.Kind == "class" {
			hasItemSymbol = true
			require.GreaterOrEqual(t, symbol.EndLine, symbol.Line)
		}
		if symbol.Name == "make" && symbol.Kind == "function" {
			hasMakeSymbol = true
			require.GreaterOrEqual(t, symbol.EndLine, symbol.Line)
		}
	}
	require.True(t, hasItemSymbol)
	require.True(t, hasMakeSymbol)
}

func TestParserSupportsManifestRuntimeLanguages(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	require.True(t, p.SupportsLanguage("typescript"))
	require.True(t, p.SupportsLanguage("tsx"), "alias should resolve to typescript")
	require.True(t, p.SupportsLanguage("csharp"))
	require.True(t, p.SupportsLanguage("c_sharp"), "alias should resolve to csharp")
}

func TestParserAnalyzeTypeScriptViaMapPathAndAlias(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	src := []byte(`class User {}
function build() {
	return new User();
}
build();
`)

	analysis, err := p.Analyze(context.Background(), "/tmp/main.tsx", src)
	require.NoError(t, err)
	require.NotNil(t, analysis)
	require.Equal(t, "typescript", analysis.Language)
	require.NotEmpty(t, analysis.Tags)

	hasUserClassDef := false
	hasBuildFunctionDef := false
	for _, tag := range analysis.Tags {
		if tag.Name == "User" && tag.Kind == "def" && tag.NodeType == "class" {
			hasUserClassDef = true
		}
		if tag.Name == "build" && tag.Kind == "def" && tag.NodeType == "function" {
			hasBuildFunctionDef = true
		}
	}
	require.True(t, hasUserClassDef)
	require.True(t, hasBuildFunctionDef)
}

func TestParserAnalyzeCSharpViaMapPath(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	src := []byte(`class Item {}
class Program {
	void Run() {}
}
`)

	analysis, err := p.Analyze(context.Background(), "/tmp/main.cs", src)
	require.NoError(t, err)
	require.NotNil(t, analysis)
	require.Equal(t, "csharp", analysis.Language)
	require.NotEmpty(t, analysis.Tags)

	hasItemClassDef := false
	hasRunMethodDef := false
	for _, tag := range analysis.Tags {
		if tag.Name == "Item" && tag.Kind == "def" && tag.NodeType == "class" {
			hasItemClassDef = true
		}
		if tag.Name == "Run" && tag.Kind == "def" && tag.NodeType == "method" {
			hasRunMethodDef = true
		}
	}
	require.True(t, hasItemClassDef)
	require.True(t, hasRunMethodDef)
}

func TestParserAnalyzeManifestLanguageWithoutRuntimeGrammarIsGraceful(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	require.True(t, p.SupportsLanguage("swift"), "manifest-supported language should remain listed")
	require.True(t, p.HasTags("swift"), "embedded query should exist for swift")

	analysis, err := p.Analyze(context.Background(), "/tmp/check.swift", []byte("class Test {}"))
	require.NoError(t, err)
	require.NotNil(t, analysis)
	require.Equal(t, "swift", analysis.Language)
	require.Empty(t, analysis.Tags)
	require.Empty(t, analysis.Symbols)
}
