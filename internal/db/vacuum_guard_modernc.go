//go:build (darwin && (amd64 || arm64)) || (freebsd && (amd64 || arm64)) || (linux && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || s390x)) || (windows && (386 || amd64 || arm64))

package db

// VACUUM Guard for modernc.org/sqlite
//
// The modernc.org/sqlite driver does not expose SQLite's authorizer API at the
// package level, which would allow us to intercept and deny VACUUM operations.
//
// IMPORTANT: Do NOT run VACUUM on databases containing contentless FTS5 tables
// (such as messages_fts). VACUUM reassigns rowids, which will permanently and
// silently corrupt the contentless FTS5 index.
//
// This limitation is mitigated by:
// 1. Not exposing VACUUM functionality in the application
// 2. Documentation warnings
// 3. The ncruces build (used on other platforms) actively blocks VACUUM
//
// If VACUUM guard is critical for your platform, consider using the ncruces
// build instead by setting GOOS/GOARCH to a platform that uses ncruces driver.
