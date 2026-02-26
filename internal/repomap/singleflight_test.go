package repomap

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRefreshAsyncSingleflightDedup(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{Options: &config.Options{RepoMap: &config.RepoMapOptions{RefreshMode: "auto"}}}
	svc := NewService(cfg, nil, nil, ".", context.Background())
	svc.sessionCaches.Store("sess-1", "cached-map", 10)

	var runs atomic.Int64
	started := make(chan struct{})
	svc.onRefreshRun = func() {
		if runs.Add(1) == 1 {
			close(started)
		}
		time.Sleep(30 * time.Millisecond)
	}

	opts := GenerateOpts{SessionID: "sess-1", ChatFiles: []string{"a.go"}, TokenBudget: 100, ForceRefresh: true}
	for range 32 {
		svc.RefreshAsync("sess-1", opts)
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for refresh singleflight")
	}

	require.NoError(t, svc.Close())
	require.EqualValues(t, 1, runs.Load())
}

func TestPreIndexSingleflightDedup(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, t.TempDir(), context.Background())

	var runs atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	svc.onPreIndexRun = func() {
		if runs.Add(1) == 1 {
			close(started)
		}
		<-release
	}

	for range 32 {
		svc.PreIndex()
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pre-index singleflight")
	}

	close(release)

	select {
	case <-svc.preIndexSignal():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pre-index completion")
	}

	require.EqualValues(t, 1, runs.Load())
	require.NoError(t, svc.Close())
}
