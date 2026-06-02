package tools

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// anchorCache stores AnchorMaps keyed by file path for reuse across view and
// edit operations.
var anchorCache sync.Map

// storeAnchorMap caches an AnchorMap for the given file path.
func storeAnchorMap(filePath string, am *AnchorMap) {
	if am == nil {
		return
	}
	anchorCache.Store(filePath, am)
}

// loadAnchorMap retrieves a cached AnchorMap for the given file path.
func loadAnchorMap(filePath string) *AnchorMap {
	if v, ok := anchorCache.Load(filePath); ok {
		if am, ok := v.(*AnchorMap); ok {
			return am
		}
	}
	return nil
}

// deleteAnchorMap removes a cached AnchorMap for the given file path.
func deleteAnchorMap(filePath string) {
	anchorCache.Delete(filePath)
}

// anchorHashOnlyRE matches <hash:XXXXXXXX> markers anywhere in a string.
var anchorHashOnlyRE = regexp.MustCompile(`<hash:([0-9a-fA-F]{8,16})>`)

// anchorMarkerRE matches the full comment form "// <hash:XXXXXXXX>" with
// optional surrounding whitespace, for stripping from old_string.
var anchorMarkerRE = regexp.MustCompile(`\s*//\s*<hash:[0-9a-fA-F]{8,16}>`)

// extractAnchorHashes parses <hash:XXXXXXXX> markers from s and returns the
// hash values as uint64.
func extractAnchorHashes(s string) []uint64 {
	matches := anchorHashOnlyRE.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[uint64]bool)
	var hashes []uint64
	for _, m := range matches {
		h, err := strconv.ParseUint(m[1], 16, 64)
		if err != nil {
			continue
		}
		if !seen[h] {
			seen[h] = true
			hashes = append(hashes, h)
		}
	}
	return hashes
}

// stripAnchorMarkers removes all "// <hash:XXXXXXXX>" markers from s.
func stripAnchorMarkers(s string) string {
	return anchorMarkerRE.ReplaceAllString(s, "")
}

// injectAnchorsIntoOutput appends anchor markers (as comments) to the
// appropriate lines in the already-line-numbered output. startLine is
// 1-indexed (first display line number). am's LineNum values are 0-indexed
// relative to the displayed content.
func injectAnchorsIntoOutput(numberedContent string, startLine int, am *AnchorMap) string {
	if am == nil || len(am.Anchors) == 0 {
		return numberedContent
	}

	lines := strings.Split(numberedContent, "\n")

	anchorSet := make(map[int]string, len(am.Anchors))
	for _, a := range am.Anchors {
		anchorSet[a.LineNum] = a.FormatAnchor()
	}

	var b strings.Builder
	for i, line := range lines {
		if marker, ok := anchorSet[i]; ok {
			b.WriteString(line)
			b.WriteString("  // ")
			b.WriteString(marker)
		} else {
			b.WriteString(line)
		}
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// shiftAnchorMap adjusts all anchor positions by delta (positive or negative)
// and rebuilds the Lookup table.
func shiftAnchorMap(am *AnchorMap, delta int) {
	if am == nil || delta == 0 {
		return
	}
	newLookup := make(map[uint64]int, len(am.Lookup))
	for i := range am.Anchors {
		am.Anchors[i].LineNum += delta
		newLookup[am.Anchors[i].Hash] = am.Anchors[i].LineNum
	}
	am.Lookup = newLookup
}

// tryAnchorReplace attempts to use anchor resolution for content replacement.
// It extracts anchor hashes from oldString, resolves them against the cached
// anchor map and the current file content, and uses the resolved positions to
// locate oldString. Returns (newContent, true) if successful, or ("", false)
// if anchors couldn't help.
func tryAnchorReplace(oldContent, oldString, newString, filePath string, replaceAll bool) (string, bool) {
	hashes := extractAnchorHashes(oldString)
	if len(hashes) == 0 {
		return "", false
	}

	am := loadAnchorMap(filePath)
	if am == nil {
		return "", false
	}

	cleanOld := stripAnchorMarkers(oldString)
	if cleanOld == "" {
		return "", false
	}

	hashSet := make(map[uint64]bool, len(hashes))
	for _, h := range hashes {
		hashSet[h] = true
	}

	var resolvedLines []int
	for i := range am.Anchors {
		if hashSet[am.Anchors[i].Hash] {
			line, err := ResolveAnchor(&am.Anchors[i], oldContent)
			if err == nil {
				resolvedLines = append(resolvedLines, line)
			}
		}
	}

	if len(resolvedLines) == 0 {
		return "", false
	}

	if replaceAll {
		replaced := strings.ReplaceAll(oldContent, cleanOld, newString)
		if replaced != oldContent {
			return replaced, true
		}
		return "", false
	}

	var matches []int
	searchFrom := 0
	for {
		idx := strings.Index(oldContent[searchFrom:], cleanOld)
		if idx == -1 {
			break
		}
		matches = append(matches, searchFrom+idx)
		searchFrom = searchFrom + idx + 1
	}

	switch len(matches) {
	case 0:
		return "", false
	case 1:
		idx := matches[0]
		return oldContent[:idx] + newString + oldContent[idx+len(cleanOld):], true
	default:
		// Multiple matches — pick the one closest to a resolved anchor line.
		best := -1
		bestDist := int(^uint(0) >> 1)
		for _, matchIdx := range matches {
			matchLine := strings.Count(oldContent[:matchIdx], "\n")
			for _, resLine := range resolvedLines {
				d := matchLine - resLine
				if d < 0 {
					d = -d
				}
				if d < bestDist {
					bestDist = d
					best = matchIdx
				}
			}
		}
		if best >= 0 {
			return oldContent[:best] + newString + oldContent[best+len(cleanOld):], true
		}
		return "", false
	}
}
