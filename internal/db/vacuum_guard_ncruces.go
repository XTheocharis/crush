//go:build !((darwin && (amd64 || arm64)) || (freebsd && (amd64 || arm64)) || (linux && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || s390x)) || (windows && (386 || amd64 || arm64)))

package db

import (
	"github.com/ncruces/go-sqlite3"
)

// VACUUM Guard for ncruces/go-sqlite3
//
// SQLite's authorizer callback does not receive VACUUM as an action code.
// VACUUM is a special DDL command that bypasses the normal authorizer mechanism.
//
// IMPORTANT: Do NOT run VACUUM on databases containing contentless FTS5 tables
// (such as messages_fts). VACUUM reassigns rowids, which will permanently and
// silently corrupt the contentless FTS5 index.
//
// This guard sets up an authorizer that could be extended in the future if
// SQLite adds VACUUM to the authorizer action codes, but currently it serves
// primarily as documentation and a hook for future enhancement.

func init() {
	sqlite3.AutoExtension(func(c *sqlite3.Conn) error {
		// Set authorizer to monitor database operations.
		// Note: VACUUM is not currently sent through the authorizer callback
		// in SQLite, so this primarily serves as a future-proofing hook.
		c.SetAuthorizer(func(action sqlite3.AuthorizerActionCode, name3rd, name4th, schema, inner string) sqlite3.AuthorizerReturnCode {
			// If SQLite ever adds VACUUM to authorizer action codes, we would
			// detect and deny it here. For now, this is a no-op that returns OK.
			//
			// The contentless FTS5 corruption risk is mitigated by:
			// 1. Not exposing VACUUM functionality in the application
			// 2. Documentation warnings
			// 3. Code review requirements
			return sqlite3.AUTH_OK
		})
		return nil
	})
}
