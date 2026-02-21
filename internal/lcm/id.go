package lcm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateSummaryID generates a unique summary ID based on session ID and
// current timestamp. Returns both the ID and the timestamp used.
func GenerateSummaryID(sessionID string) (string, int64) {
	ts := time.Now().UnixMilli()
	input := fmt.Sprintf("%s:%d", sessionID, ts)
	h := sha256.Sum256([]byte(input))
	id := SummaryIDPrefix + hex.EncodeToString(h[:])[:16]
	return id, ts
}

// GenerateFileID generates a unique file ID based on content hash.
func GenerateFileID(sessionID, content string) string {
	input := fmt.Sprintf("%s:%s", sessionID, content)
	h := sha256.Sum256([]byte(input))
	return FileIDPrefix + hex.EncodeToString(h[:])[:16]
}
