package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// FileSnapshot captures the state of a single file at a point in time.
type FileSnapshot struct {
	FilePath string
	Content  string
	ModTime  time.Time
}

// Snapshot captures the state of multiple files at a point in time.
type Snapshot struct {
	Files      []FileSnapshot
	CapturedAt time.Time
}

// PersistentSnapshot represents a snapshot that has been persisted via the
// Rewind system. It records metadata about when and why the snapshot was
// stored.
type PersistentSnapshot struct {
	ID        string
	CreatedAt time.Time
	Reason    string
	Files     []string
}

// PersistentSnapshotter is the subset of the Rewind Snapshotter interface
// that RollbackManager uses to persist snapshots. This avoids a direct
// dependency on the rewind package.
type PersistentSnapshotter interface {
	// CaptureSnapshot persists file state at the given user message sequence.
	CaptureSnapshot(ctx context.Context, sessionID string, userMessageSeq int) error
}

// RollbackManager captures pre-edit file state and supports restoring files
// on failure. It operates directly on the filesystem and does not depend on
// git. When a PersistentSnapshotter is configured via SetSnapshotter,
// snapshots are also persisted through the Rewind system for durable storage.
type RollbackManager struct {
	mu          sync.Mutex
	snapshotter PersistentSnapshotter
	sessionID   string
	history     []PersistentSnapshot
}

// NewRollbackManager creates a new RollbackManager.
func NewRollbackManager() *RollbackManager {
	return &RollbackManager{}
}

// SetSnapshotter configures an optional PersistentSnapshotter for persisting
// snapshots via the Rewind system. When set, each Capture call also records
// metadata in the rollback history and persists through the Rewind system.
// Pass nil to disable persistence.
func (rm *RollbackManager) SetSnapshotter(s PersistentSnapshotter, sessionID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.snapshotter = s
	rm.sessionID = sessionID
}

// History returns the list of snapshots that were persisted via the Rewind
// system. If no PersistentSnapshotter is configured, the list is empty.
func (rm *RollbackManager) History() []PersistentSnapshot {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	out := make([]PersistentSnapshot, len(rm.history))
	copy(out, rm.history)
	return out
}

// Capture reads the contents and metadata of the given file paths, returning
// a Snapshot that can be used to restore the files later. Files that do not
// exist on disk are captured with empty content and a zero ModTime so that
// they can be tracked as "did not exist before edit".
//
// When a PersistentSnapshotter is configured, the capture is also persisted
// through the Rewind system. Persistence errors are logged but do not fail
// the capture — the in-memory snapshot is always returned.
func (rm *RollbackManager) Capture(filePaths []string) (*Snapshot, error) {
	if len(filePaths) == 0 {
		snap := &Snapshot{
			Files:      []FileSnapshot{},
			CapturedAt: time.Now(),
		}
		rm.persistSnapshot(snap, "empty capture")
		return snap, nil
	}

	files := make([]FileSnapshot, len(filePaths))
	for i, fp := range filePaths {
		info, err := os.Stat(fp)
		if err != nil {
			if os.IsNotExist(err) {
				files[i] = FileSnapshot{
					FilePath: fp,
					Content:  "",
					ModTime:  time.Time{},
				}
				continue
			}
			return nil, fmt.Errorf("stat %s: %w", fp, err)
		}

		content, err := os.ReadFile(fp)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", fp, err)
		}

		files[i] = FileSnapshot{
			FilePath: fp,
			Content:  string(content),
			ModTime:  info.ModTime(),
		}
	}

	snap := &Snapshot{
		Files:      files,
		CapturedAt: time.Now(),
	}

	rm.persistSnapshot(snap, "file capture")
	return snap, nil
}

// persistSnapshot records snapshot metadata in the rollback history and, if
// a PersistentSnapshotter is configured, persists via the Rewind system.
// Errors from the PersistentSnapshotter are logged but not propagated.
func (rm *RollbackManager) persistSnapshot(snap *Snapshot, reason string) {
	rm.mu.Lock()
	snapshotter := rm.snapshotter
	sessionID := rm.sessionID
	rm.mu.Unlock()

	if snapshotter == nil {
		return
	}

	// Build file path list for the history record.
	filePaths := make([]string, len(snap.Files))
	for i, f := range snap.Files {
		filePaths[i] = f.FilePath
	}

	// Persist through the Rewind system. We use seq=0 as a placeholder
	// since the RollbackManager operates outside the turn-sequence model.
	// The Rewind system will assign its own ID.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := snapshotter.CaptureSnapshot(ctx, sessionID, 0); err != nil {
		slog.Error("RollbackManager failed to persist snapshot via Rewind",
			"error", err,
			"sessionID", sessionID,
		)
		return
	}

	entry := PersistentSnapshot{
		ID:        fmt.Sprintf("rollback-%d", snap.CapturedAt.UnixNano()),
		CreatedAt: snap.CapturedAt,
		Reason:    reason,
		Files:     filePaths,
	}

	rm.mu.Lock()
	rm.history = append(rm.history, entry)
	rm.mu.Unlock()
}

// Restore writes the snapshot contents back to their original file paths.
// It restores all files in the snapshot and logs each restoration.
func (rm *RollbackManager) Restore(snapshot *Snapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}

	var firstErr error
	for _, fs := range snapshot.Files {
		if err := rm.restoreFile(fs); err != nil {
			slog.Error("Rollback restore failed", "file", fs.FilePath, "error", err)
			if firstErr == nil {
				firstErr = fmt.Errorf("restore %s: %w", fs.FilePath, err)
			}
		}
	}

	return firstErr
}

// RestorePartial restores only the specified files from the snapshot.
// It validates that all requested files exist in the snapshot before
// attempting any writes. Files not found in the snapshot result in an error.
func (rm *RollbackManager) RestorePartial(snapshot *Snapshot, filePaths []string) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}

	// Build lookup from snapshot.
	lookup := make(map[string]FileSnapshot, len(snapshot.Files))
	for _, fs := range snapshot.Files {
		lookup[fs.FilePath] = fs
	}

	// Validate all requested files exist in the snapshot.
	notFound := make([]string, 0)
	toRestore := make([]FileSnapshot, 0, len(filePaths))
	for _, fp := range filePaths {
		fs, ok := lookup[fp]
		if !ok {
			notFound = append(notFound, fp)
			continue
		}
		toRestore = append(toRestore, fs)
	}

	if len(notFound) > 0 {
		return fmt.Errorf("files not in snapshot: %v", notFound)
	}

	var firstErr error
	for _, fs := range toRestore {
		if err := rm.restoreFile(fs); err != nil {
			slog.Error("Rollback partial restore failed", "file", fs.FilePath, "error", err)
			if firstErr == nil {
				firstErr = fmt.Errorf("restore %s: %w", fs.FilePath, err)
			}
		}
	}

	return firstErr
}

// restoreFile writes the snapshot content back to disk. For files that had
// empty content and a zero ModTime (did not exist at capture time), the file
// is removed if it now exists.
func (rm *RollbackManager) restoreFile(fs FileSnapshot) error {
	// File did not exist at capture time — remove it if it was created.
	if fs.Content == "" && fs.ModTime.IsZero() {
		if _, err := os.Stat(fs.FilePath); err == nil {
			if err := os.Remove(fs.FilePath); err != nil {
				return fmt.Errorf("remove created file %s: %w", fs.FilePath, err)
			}
			slog.Info("Rollback removed new file", "file", fs.FilePath)
		}
		return nil
	}

	if err := os.WriteFile(fs.FilePath, []byte(fs.Content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", fs.FilePath, err)
	}

	// Best-effort ModTime restoration.
	if !fs.ModTime.IsZero() {
		_ = os.Chtimes(fs.FilePath, fs.ModTime, fs.ModTime)
	}

	slog.Info("Rollback restored file", "file", fs.FilePath)
	return nil
}

// DiagnosticDelta holds the result of comparing before/after diagnostics for
// the rollback validation pipeline.
type DiagnosticDelta struct {
	NewErrors      []DiagnosticInfo
	ResolvedErrors []DiagnosticInfo
	NetChange      int
}

// DiffDiagnostics compares before and after diagnostic snapshots and returns
// the delta. It identifies new errors introduced and errors that were resolved.
func DiffDiagnostics(before, after map[string][]DiagnosticInfo) *DiagnosticDelta {
	beforeKeys := make(map[diagnosticKey]DiagnosticInfo)
	for _, diags := range before {
		for _, d := range diags {
			beforeKeys[d.Key()] = d
		}
	}

	afterKeys := make(map[diagnosticKey]DiagnosticInfo)
	for _, diags := range after {
		for _, d := range diags {
			afterKeys[d.Key()] = d
		}
	}

	var newErrors []DiagnosticInfo
	for key, diag := range afterKeys {
		if _, exists := beforeKeys[key]; !exists {
			newErrors = append(newErrors, diag)
		}
	}

	var resolvedErrors []DiagnosticInfo
	for key, diag := range beforeKeys {
		if _, exists := afterKeys[key]; !exists {
			resolvedErrors = append(resolvedErrors, diag)
		}
	}

	return &DiagnosticDelta{
		NewErrors:      newErrors,
		ResolvedErrors: resolvedErrors,
		NetChange:      len(newErrors) - len(resolvedErrors),
	}
}

// ShouldRollback determines whether a validation result warrants rolling back
// file changes. Returns true if there are new errors that weren't present
// before.
func ShouldRollback(delta *DiagnosticDelta) bool {
	return len(delta.NewErrors) > 0
}
