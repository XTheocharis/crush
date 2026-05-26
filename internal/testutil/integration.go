// Package testutil provides shared helpers for integration tests.
package testutil

import (
	"os"
	"testing"
)

// SkipIfNoIntegration skips the test unless CRUSH_INTEGRATION=1 is set.
func SkipIfNoIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("CRUSH_INTEGRATION") == "" {
		t.Skip("skipping integration test (set CRUSH_INTEGRATION=1 to run)")
	}
}
