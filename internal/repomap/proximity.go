//go:build treesitter
// +build treesitter

package repomap

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/crush/internal/treesitter"
)

// ProximityScorer scores files based on proximity to test files using naming
// conventions and directory co-location heuristics. Scores range [0.0, 1.0]
// where 1.0 means the file IS a test file and 0.0 means no proximity.
type ProximityScorer struct{}

// NewProximityScorer creates a new ProximityScorer.
func NewProximityScorer() *ProximityScorer {
	return &ProximityScorer{}
}

// Score computes proximity scores for all filePaths relative to the given
// testFiles. Files not in filePaths are ignored. Test files themselves score
// 1.0. Non-test files score between 0.0 and 0.8 based on directory co-location
// and naming conventions.
//
// The scoring heuristics are:
//   - Test file: 1.0
//   - Naming match (e.g. foo.go ↔ foo_test.go, bar.ts ↔ bar.test.ts): 0.5–0.8
//   - Directory co-location (same dir as a test file): 0.3–0.7
//   - No proximity: 0.0
func (s *ProximityScorer) Score(filePaths []string, testFiles []string) map[string]float64 {
	if len(filePaths) == 0 || len(testFiles) == 0 {
		return nil
	}

	fileSet := make(map[string]struct{}, len(filePaths))
	for _, f := range filePaths {
		fileSet[normalizeGraphRelPath(f)] = struct{}{}
	}

	testSet := make(map[string]struct{}, len(testFiles))
	normalizedTests := make([]string, 0, len(testFiles))
	for _, tf := range testFiles {
		ntf := normalizeGraphRelPath(tf)
		if ntf == "" {
			continue
		}
		testSet[ntf] = struct{}{}
		normalizedTests = append(normalizedTests, ntf)
	}

	if len(normalizedTests) == 0 {
		return nil
	}

	// Build set of directories containing test files.
	testDirs := make(map[string]int)
	for _, tf := range normalizedTests {
		dir := path.Dir(tf)
		testDirs[dir]++
	}

	// Build base-name mapping from test files: e.g. "foo_test.go" → "foo",
	// "bar.test.ts" → "bar".
	testBases := make(map[string][]string) // base → test file dirs
	for _, tf := range normalizedTests {
		base := testBaseName(tf)
		if base != "" {
			testBases[base] = append(testBases[base], path.Dir(tf))
		}
	}

	scores := make(map[string]float64, len(filePaths))
	for fPath := range fileSet {
		if _, isTest := testSet[fPath]; isTest {
			scores[fPath] = 1.0
			continue
		}

		var bestScore float64

		// Naming proximity: check if this file's base matches a test file's base.
		fileBase := sourceBaseName(fPath)
		if testDirsForBase, ok := testBases[fileBase]; ok {
			// The file shares a base name with a test file.
			fDir := path.Dir(fPath)
			sameDir := false
			for _, td := range testDirsForBase {
				if td == fDir {
					sameDir = true
					break
				}
			}
			if sameDir {
				bestScore = max(bestScore, 0.8)
			} else {
				bestScore = max(bestScore, 0.5)
			}
		}

		// Directory co-location.
		fDir := path.Dir(fPath)
		if count, ok := testDirs[fDir]; ok && count > 0 {
			// Scale co-location score by density: more test files in the same
			// directory → higher score. Capped at 0.7.
			colocScore := 0.3 + 0.1*float64(min(count, 4))
			bestScore = max(bestScore, colocScore)
		}

		if bestScore > 0 {
			scores[fPath] = bestScore
		}
	}

	if len(scores) == 0 {
		return nil
	}
	return scores
}

// BlendProximityPersonalization merges proximity scores into an existing
// PageRank personalization vector. This is a blending function that does NOT
// modify pagerank.go directly.
//
// The blendFactor controls how much the proximity signal contributes
// (0 = ignore, 1 = full override). For each file with a proximity score, the
// blended value is:
//
//	result = (1-blendFactor)*existing + blendFactor*proximityScore
func BlendProximityPersonalization(
	personalization map[string]float64,
	proximityScores map[string]float64,
	blendFactor float64,
) map[string]float64 {
	if len(proximityScores) == 0 || blendFactor <= 0 {
		return personalization
	}

	result := make(map[string]float64, len(personalization))
	for k, v := range personalization {
		result[k] = v
	}

	for file, pScore := range proximityScores {
		if pScore <= 0 {
			continue
		}
		existing := result[file]
		result[file] = (1-blendFactor)*existing + blendFactor*pScore
	}

	return result
}

// testFilePatterns defines how test files are identified by extension.
// Maps source extension → list of test file suffixes.
var testFilePatterns = map[string][]string{
	".go":   {"_test.go"},
	".py":   {"_test.py"},
	".rs":   {"_test.rs"},
	".ts":   {".test.ts", ".spec.ts"},
	".tsx":  {".test.tsx", ".spec.tsx"},
	".js":   {".test.js", ".spec.js"},
	".jsx":  {".test.jsx", ".spec.jsx"},
	".java": {"Test.java", "Test.java"}, // Java: *Test.java
	".rb":   {"_test.rb", "_spec.rb"},
}

// IsTestFile reports whether a file path looks like a test file based on
// naming conventions for known language extensions in BaseExtensions.
func IsTestFile(filePath string) bool {
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	if ext == "" {
		return false
	}

	// Check known test patterns for this extension.
	if patterns, ok := testFilePatterns[ext]; ok {
		for _, suffix := range patterns {
			if strings.HasSuffix(base, suffix) {
				return true
			}
		}
	}

	// Fallback heuristic: any file ending in _test.* or containing .test. or
	// .spec. for languages in BaseExtensions.
	noExt := strings.TrimSuffix(base, ext)
	lang := treesitter.MapExtension(ext)
	if lang == "" {
		return false
	}

	if strings.HasSuffix(noExt, "_test") {
		return true
	}
	if strings.Contains(noExt, ".test") || strings.Contains(noExt, ".spec") {
		return true
	}
	return false
}

// FindTestFiles filters a list of file paths to only those that look like test
// files.
func FindTestFiles(filePaths []string) []string {
	var tests []string
	for _, f := range filePaths {
		if IsTestFile(f) {
			tests = append(tests, f)
		}
	}
	return tests
}

// testBaseName extracts the source base name from a test file path.
// For example: "foo_test.go" → "foo", "bar.test.ts" → "bar",
// "baz_spec.rb" → "baz". Returns empty string if the file doesn't match
// known test patterns.
func testBaseName(testPath string) string {
	base := path.Base(testPath)
	ext := path.Ext(base)
	if ext == "" {
		return ""
	}
	noExt := strings.TrimSuffix(base, ext)

	// Go/Python/Rust style: foo_test → foo.
	if idx := strings.LastIndex(noExt, "_test"); idx > 0 {
		return noExt[:idx]
	}

	// JS/TS style: foo.test or foo.spec → foo.
	if idx := strings.LastIndex(noExt, ".test"); idx > 0 {
		return noExt[:idx]
	}
	if idx := strings.LastIndex(noExt, ".spec"); idx > 0 {
		return noExt[:idx]
	}

	// Ruby style: foo_spec or foo_test → foo.
	if idx := strings.LastIndex(noExt, "_spec"); idx > 0 {
		return noExt[:idx]
	}

	// Java style: FooTest → Foo.
	if strings.HasSuffix(noExt, "Test") && len(noExt) > 4 {
		return strings.TrimSuffix(noExt, "Test")
	}

	return ""
}

// sourceBaseName extracts the base identifier from a source file path.
// For example: "foo.go" → "foo", "bar.ts" → "bar", "baz/utils.py" → "utils".
func sourceBaseName(srcPath string) string {
	base := path.Base(srcPath)
	ext := path.Ext(base)
	if ext == "" {
		return base
	}
	return strings.TrimSuffix(base, ext)
}
