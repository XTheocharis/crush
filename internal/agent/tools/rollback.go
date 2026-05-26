package tools

import (
	"fmt"
	"log/slog"
	"os"
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

// RollbackManager captures pre-edit file state and supports restoring files
// on failure. It operates directly on the filesystem and does not depend on
// git.
type RollbackManager struct{}

// NewRollbackManager creates a new RollbackManager.
func NewRollbackManager() *RollbackManager {
	return &RollbackManager{}
}

// Capture reads the contents and metadata of the given file paths, returning
// a Snapshot that can be used to restore the files later. Files that do not
// exist on disk are captured with empty content and a zero ModTime so that
// they can be tracked as "did not exist before edit".
func (rm *RollbackManager) Capture(filePaths []string) (*Snapshot, error) {
	if len(filePaths) == 0 {
		return &Snapshot{
			Files:      []FileSnapshot{},
			CapturedAt: time.Now(),
		}, nil
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

	return &Snapshot{
		Files:      files,
		CapturedAt: time.Now(),
	}, nil
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
