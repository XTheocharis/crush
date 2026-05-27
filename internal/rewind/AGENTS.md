# Rewind — Turn-based snapshot, rewind, fork, and message editing

Provides undo/rewind capabilities for the fork branch. After each agent turn,
files are snapshotted so users can rewind to any previous state.

## Structure

| File | Purpose |
|------|---------|
| `types.go` | Core types: `RewindMode`, `TurnSnapshot`, `SnapshotFile`, `RewindResult`, `ForkResult`, `EditResult`. Interfaces: `Snapshotter`, `Rewinder`, `Forker`, `Editor`, `Service`. Options: `SnapshotterOption`, `RewinderOption`, `PostRewindHook`. |
| `snapshot.go` | `Snapshotter` implementation: `CaptureSnapshot`, `GetSnapshotAtOrBeforeSeq`, `GetSnapshotFiles`, `DeleteSnapshotsAfterSeq`, `CleanupOldSnapshots`. Uses `GetLatestUserMessage` → `CreateTurnSnapshot` → `AddSnapshotFile` chain. |
| `rewind.go` | `Rewinder` implementation: three modes via `RewindMode` enum (RewindCode, RewindConvo, RewindBoth). `rewindCode` writes snapshot files to disk. `rewindConvo` deletes messages + snapshots + runs post-rewind hook. `rewindBoth` combines both. |
| `fork.go` | `Forker` implementation: creates new session with unique title (`"Title (fork)"`, `"Title (fork #N)"`), sets `ParentSessionID`, clones messages via `CloneSessionMessages`, clones files via `CloneSessionFiles`, truncates at fork point. |
| `edit.go` | `Editor` implementation: extracts text from user message at given seq, validates role="user", calls `DeleteMessagesAfterSeq(seq-1)` to remove target + all after. Returns `EditResult` with extracted text. |
| `service.go` | `Service` struct composing `Snapshotter + Rewinder + Forker + Editor`. Two constructors: `NewService` (simple) and `NewServiceWithOptions` (accepts separate snapshotter/rewinder options). |

## Key Types

- `RewindMode` — `RewindCode`, `RewindConvo`, `RewindBoth`
- `TurnSnapshot` — ID, SessionID, UserMessageID, UserMessageSeq, CreatedAt
- `SnapshotFile` — SnapshotID, Path, Version, Content
- `PostRewindHook` — `func(ctx context.Context, sessionID string) error`. Called after rewind conversation deletion. Errors logged via `slog.Error` but not propagated (rewind succeeds even if hook fails).

## Database Tables

- `turn_snapshots` — id (UUID), session_id (FK→sessions), user_message_id (FK→messages), user_message_seq, created_at
- `turn_snapshot_files` — snapshot_id (FK→turn_snapshots), file_id (FK→files), path, version. Composite PK (snapshot_id, path).

## Integration Points

- **App wiring** (`app/app.go`): `rewind.NewServiceWithOptions(q, sessions, workdir, snapOpts, rewindOpts)` where `rewindOpts` includes `WithPostRewindHook` that calls `lcmMgr.Compact(ctx, sessionID)`.
- **Coordinator** (`coordinator.go`): After auto-fix loop, calls `captureSnapshot()` which calls `Snapshotter.CaptureSnapshot` then `CleanupOldSnapshots` in a goroutine.
- **Coordinator opts** (`coordinator_opts.go`): `WithSnapshotCapture` option injects the rewind service into the coordinator.
- **UI** (`ui/model/ui.go`): Action routing for `ActionRewind`, `ActionFork`, `ActionEditMessage`, `ActionOpenMessageOptions`. Async execution via `tea.Cmd`.
- **Dialog** (`ui/dialog/message_options.go`): `MessageOptions` dialog with 6 options (3 rewind modes, edit, fork, cancel).
- **Commands** (`ui/dialog/commands.go`): `/rewind` and `/fork` slash commands registered.
- **Chat** (`ui/model/chat.go`): `OnMessageOptions` callback triggered by `o` key or double-click on `UserMessageItem`.
- **Workspace** (`workspace/workspace.go`): `RewindService()` accessor added to `Workspace` interface.

## Anti-Patterns

- Do NOT import `internal/lcm` from this package. Use `PostRewindHook` func type.
- `CleanupOldSnapshots` runs in a goroutine — tests must use `.Maybe()` mock expectations.
- `CloneSessionMessages` SQL uses `ROW_NUMBER() OVER (ORDER BY m.seq ASC)` outside the scalar subquery — the offset must be additive with `MAX(seq)`, not nested inside.
- `session.Save()` persists `ParentSessionID` — `UpdateSession` SQL includes `parent_session_id = ?`.
