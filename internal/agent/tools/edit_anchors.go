package tools

import (
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
)

const defaultAnchorInterval = 50

const anchorDriftTolerance = 5

var errAnchorNotFound = errors.New("anchor not found within drift tolerance")

// HashAnchor is a content-addressed marker that records an FNV-1a hash derived
// from the line content and its surrounding context, the line number, and the
// original line text.
type HashAnchor struct {
	Hash    uint64
	LineNum int
	Content string
}

// FormatAnchor returns the invisible marker representation for embedding in
// comments, e.g. "<hash:a1b2c3d4>".
func (a *HashAnchor) FormatAnchor() string {
	return fmt.Sprintf("<hash:%08x>", a.Hash)
}

// AnchorMap holds a collection of HashAnchors and a hash→line lookup table
// built from file content.
type AnchorMap struct {
	Anchors []HashAnchor
	Lookup  map[uint64]int
}

// BuildAnchorMap creates an AnchorMap by hashing every interval-th word in
// content. Each anchor records the FNV-1a 64-bit hash (derived from the line
// and its neighbors), the 0-indexed line number, and the line text. A zero or
// negative interval falls back to defaultAnchorInterval (50).
func BuildAnchorMap(content string, interval int) *AnchorMap {
	if interval <= 0 {
		interval = defaultAnchorInterval
	}

	lines := strings.Split(content, "\n")

	type wordRef struct {
		lineNum int
	}

	var words []wordRef
	for i, line := range lines {
		for w := range strings.FieldsSeq(line) {
			if strings.TrimSpace(w) == "" {
				continue
			}
			words = append(words, wordRef{lineNum: i})
		}
	}

	seen := make(map[uint64]bool)
	var anchors []HashAnchor
	lookup := make(map[uint64]int)

	for i := interval - 1; i < len(words); i += interval {
		lineNum := words[i].lineNum
		hash := hashLineWindow(lines, lineNum)

		if seen[hash] {
			hash = hashLineWithContext(lines, lineNum, i)
		}
		seen[hash] = true

		anchor := HashAnchor{
			Hash:    hash,
			LineNum: lineNum,
			Content: lines[lineNum],
		}
		anchors = append(anchors, anchor)
		lookup[hash] = lineNum
	}

	return &AnchorMap{Anchors: anchors, Lookup: lookup}
}

// ResolveAnchor locates an anchor in content. It first tries an exact hash
// match at the recorded line. If that fails, it searches ±anchorDriftTolerance
// (5) lines for a line matching the anchor's content. Returns the resolved
// 0-indexed line number or an error if the anchor cannot be located.
func ResolveAnchor(anchor *HashAnchor, content string) (int, error) {
	lines := strings.Split(content, "\n")

	if anchor.LineNum >= 0 && anchor.LineNum < len(lines) {
		hash := hashLineWindow(lines, anchor.LineNum)
		if hash == anchor.Hash {
			return anchor.LineNum, nil
		}
	}

	lower := max(anchor.LineNum-anchorDriftTolerance, 0)
	upper := anchor.LineNum + anchorDriftTolerance
	if upper >= len(lines) {
		upper = len(lines) - 1
	}

	for i := lower; i <= upper; i++ {
		if lines[i] == anchor.Content {
			return i, nil
		}
	}

	return -1, errAnchorNotFound
}

func hashLineWindow(lines []string, lineNum int) uint64 {
	h := fnv.New64a()
	start := max(lineNum-2, 0)
	end := lineNum + 2
	if end >= len(lines) {
		end = len(lines) - 1
	}
	for i := start; i <= end; i++ {
		h.Write([]byte(lines[i]))
		h.Write([]byte{'\n'})
	}
	return h.Sum64()
}

func hashLineWithContext(lines []string, lineNum int, wordIndex int) uint64 {
	h := fnv.New64a()
	h.Write([]byte(lines[lineNum]))
	fmt.Fprintf(h, ":%d", wordIndex)
	return h.Sum64()
}
