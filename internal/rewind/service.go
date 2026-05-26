package rewind

import (
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/session"
)

// service composes all rewind sub-services into a single Service.
type service struct {
	Snapshotter
	Rewinder
	Forker
	Editor
}

// NewService creates a composite rewind Service backed by the given
// Querier, session service, and working directory. Snapshotter options
// are forwarded to the underlying snapshotter.
func NewService(q db.Querier, sessions session.Service, workingDir string, snapOpts ...SnapshotterOption) Service {
	snap := NewSnapshotter(q, snapOpts...)
	return &service{
		Snapshotter: snap,
		Rewinder:    NewRewinder(q, snap, workingDir),
		Forker:      NewForker(q, sessions),
		Editor:      NewEditor(q),
	}
}

// NewServiceWithOptions creates a composite rewind Service with separate
// options for the snapshotter and rewinder.
func NewServiceWithOptions(q db.Querier, sessions session.Service, workingDir string, snapOpts []SnapshotterOption, rewinderOpts []RewinderOption) Service {
	snap := NewSnapshotter(q, snapOpts...)
	return &service{
		Snapshotter: snap,
		Rewinder:    NewRewinder(q, snap, workingDir, rewinderOpts...),
		Forker:      NewForker(q, sessions),
		Editor:      NewEditor(q),
	}
}
