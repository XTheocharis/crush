package explorer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

type gateB1LanguageScore struct {
	Language              string
	MicroRecall           float64
	MicroPrecision        float64
	MicroImportAccuracy   float64
	MicroVisibility       float64
	MacroRecall           float64
	MacroPrecision        float64
	MacroImportAccuracy   float64
	MacroVisibility       float64
	PerLanguageRecall     float64
	PerLanguagePrecision  float64
	PerLanguageImport     float64
	PerLanguageVisibility float64
}

type disclosureMarkerClass string

const (
	markerClassParityList       disclosureMarkerClass = "parity_list"
	markerClassParityTruncated  disclosureMarkerClass = "parity_truncated"
	markerClassEnhancementList  disclosureMarkerClass = "enhancement_list"
	markerClassEnhanceTruncated disclosureMarkerClass = "enhancement_truncated"
)

var (
	reParityListMarker         = regexp.MustCompile(`\(\+\s*(\d+)\s*more\)`)
	reParityTruncatedMarker    = regexp.MustCompile(`\[\s*truncated\s*\]\s*\(\+\s*(\d+)\s*more\s*lines\)`)
	reEnhancementListCanonical = regexp.MustCompile(`\.\.\. and (\d+) more`)
	reEnhanceRawCanonical      = regexp.MustCompile(`\[TRUNCATED\] \.\.\. and (\d+) more lines`)
)

func TestParityGateB1ExtractionQualityScoring(t *testing.T) {
	t.Parallel()
	require.NoError(t, runParityGateB1ExtractionQualityScoringCheck(OutputProfileParity))
	require.NoError(t, runParityGateB1ExtractionQualityScoringCheck(OutputProfileEnhancement))

	t.Run("detects intentional low-quality summary", func(t *testing.T) {
		t.Parallel()
		low := map[string]gateB1LanguageScore{
			"go": {
				Language:              "go",
				MicroRecall:           0.05,
				MicroPrecision:        0.05,
				MicroImportAccuracy:   0.05,
				MicroVisibility:       0.10,
				MacroRecall:           0.05,
				MacroPrecision:        0.05,
				MacroImportAccuracy:   0.05,
				MacroVisibility:       0.10,
				PerLanguageRecall:     0.05,
				PerLanguagePrecision:  0.05,
				PerLanguageImport:     0.05,
				PerLanguageVisibility: 0.10,
			},
		}
		err := enforceB1Thresholds(low, OutputProfileParity)
		require.Error(t, err)
		require.Contains(t, err.Error(), "B1 threshold miss")
	})
}

func TestParityGateB2DisclosureMarkerParity(t *testing.T) {
	t.Parallel()
	require.NoError(t, runParityGateB2DisclosureMarkerParityCheck())

	t.Run("enhancement rejects non-canonical parity marker", func(t *testing.T) {
		t.Parallel()
		marker := "(+2 more)"
		_, _, ok := parseCanonicalEnhancementMarker(marker)
		require.False(t, ok)
	})
}

func TestParityGateB3RuntimePathMatrixChecks(t *testing.T) {
	t.Parallel()
	require.NoError(t, runParityGateB3RuntimePathMatrixChecks())

	t.Run("detects drift and invalid path", func(t *testing.T) {
		t.Parallel()
		inventory, err := LoadRuntimeInventory()
		require.NoError(t, err)

		parser := treesitter.NewParser()
		defer parser.Close()
		registry := NewRegistry(WithOutputProfile(OutputProfileEnhancement), WithTreeSitter(parser))
		discovered := DiscoverRuntimePaths(registry, OutputProfileEnhancement)
		discovered = append(discovered, DiscoveredPath{
			ExplorerName: "InvalidExplorer",
			Kind:         "invalid_path_kind",
			Position:     999,
		})

		err = validateRuntimePathMatrixAgainstInventory(inventory, discovered)
		require.Error(t, err)
		require.Contains(t, err.Error(), "runtime-path matrix mismatch")
	})
}

func TestParityGateB4DataFormatDepthChecks(t *testing.T) {
	t.Parallel()
	require.NoError(t, runParityGateB4DataFormatDepthChecks())

	t.Run("detects missing required field in json fixture", func(t *testing.T) {
		t.Parallel()
		invalidJSON := `{"content":{"project":{"name":"x"}}}`
		err := validateJSONFixtureDepthAndFields([]byte(invalidJSON))
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing top-level required field")
	})
}

func TestParityGateB5DeterministicE2EParityCheck(t *testing.T) {
	t.Parallel()
	require.NoError(t, runParityGateB5DeterministicE2EParityCheck())

	t.Run("fails closed when enhancement tiers are enabled", func(t *testing.T) {
		t.Parallel()
		bundle := validParityBundleForGateB()
		err := RunParityHarnessPreflight(bundle, ParityPreflightOptions{
			RequireComparatorTuple: true,
			CorpusBasePath:         ".",
			Profile: &ParityPreflightProfile{
				ID:                      "parity-gate-b5-negative",
				TokenBudget:             4096,
				RepeatRuns:              2,
				ParityMode:              true,
				DeterministicMode:       true,
				EnhancementTiersEnabled: "all",
				TokenCounterMode:        "tokenizer_backed",
				FixedSeed:               1337,
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "enhancement_tiers_enabled")
	})
}

func TestParityGateBAggregate(t *testing.T) {
	t.Parallel()

	checks := []struct {
		name string
		run  func() error
	}{
		{name: "B1 extraction quality scoring", run: func() error { return runParityGateB1ExtractionQualityScoringCheck(OutputProfileParity) }},
		{name: "B2 disclosure marker parity", run: runParityGateB2DisclosureMarkerParityCheck},
		{name: "B3 runtime-path matrix checks", run: runParityGateB3RuntimePathMatrixChecks},
		{name: "B4 data-format depth checks", run: runParityGateB4DataFormatDepthChecks},
		{name: "B5 deterministic E2E parity check", run: runParityGateB5DeterministicE2EParityCheck},
	}

	var failures []string
	for _, check := range checks {
		if err := check.run(); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", check.name, err))
		}
	}

	require.Empty(t, failures, "Gate B aggregate failed:\n%s", strings.Join(failures, "\n"))
}

func runParityGateB1ExtractionQualityScoringCheck(profile OutputProfile) error {
	cfg := NewDefaultParityFixtureConfig(".")
	loader := NewParityFixtureLoader(cfg)
	index, err := loader.LoadIndex()
	if err != nil {
		return fmt.Errorf("load fixture index: %w", err)
	}

	parser := treesitter.NewParser()
	defer parser.Close()
	registry := NewRegistry(WithOutputProfile(profile), WithTreeSitter(parser))

	type languageCase struct {
		expectedCapabilities []string
		expectedImports      map[string]string
		expectedVisibility   map[string]string
	}

	languageExpectations := map[string]languageCase{
		"go": {
			expectedCapabilities: []string{"language: go", "symbols", "imports", "tags"},
			expectedImports: map[string]string{
				"context":                "stdlib",
				"fmt":                    "stdlib",
				"net/http":               "stdlib",
				"strings":                "stdlib",
				"time":                   "stdlib",
				"github.com/gorilla/mux": "third_party",
				"gorm.io/gorm":           "third_party",
			},
			expectedVisibility: map[string]string{
				"Server":        "public",
				"Middleware":    "public",
				"NewServer":     "public",
				"Start":         "public",
				"Shutdown":      "public",
				"AddMiddleware": "public",
				"routeHandler":  "private",
				"handle":        "private",
			},
		},
		"python": {
			expectedCapabilities: []string{"language: python", "imports", "symbols", "tags"},
			expectedImports: map[string]string{
				"argparse":    "stdlib",
				"json":        "stdlib",
				"pathlib":     "stdlib",
				"typing":      "stdlib",
				"dataclasses": "stdlib",
				".models":     "local",
				".utils":      "local",
			},
			expectedVisibility: map[string]string{
				"ProcessingResult": "public",
				"FileProcessor":    "public",
				"main":             "public",
				"__init__":         "private",
				"_process_content": "private",
				"process_file":     "public",
			},
		},
	}

	scores := make(map[string]gateB1LanguageScore)
	for lang, expected := range languageExpectations {
		fixtureName, ok := index.Language[lang]
		if !ok {
			return fmt.Errorf("missing language fixture for %q", lang)
		}
		content, err := LoadFixtureFile(cfg, fixtureName)
		if err != nil {
			return fmt.Errorf("load %s fixture: %w", lang, err)
		}

		result, err := registry.exploreStatic(context.Background(), ExploreInput{Path: fixtureName, Content: content})
		if err != nil {
			return fmt.Errorf("explore %s fixture: %w", lang, err)
		}

		scores[lang] = scoreExtractionQuality(lang, result.Summary, expected.expectedCapabilities, expected.expectedImports, expected.expectedVisibility)
	}

	if err := enforceB1Thresholds(scores, profile); err != nil {
		return err
	}
	return nil
}

func scoreExtractionQuality(
	language, summary string,
	expectedCapabilities []string,
	expectedImports map[string]string,
	expectedVisibility map[string]string,
) gateB1LanguageScore {
	normalized := strings.ToLower(summary)

	capMatched := 0
	for _, token := range expectedCapabilities {
		if strings.Contains(normalized, strings.ToLower(token)) {
			capMatched++
		}
	}
	recall := float64(capMatched) / float64(maxInt(1, len(expectedCapabilities)))

	precision := recall

	importMatched := 0
	for imp, category := range expectedImports {
		needle := fmt.Sprintf("- %s (%s)", strings.ToLower(strings.TrimSpace(imp)), strings.ToLower(strings.TrimSpace(category)))
		if strings.Contains(normalized, needle) {
			importMatched++
		}
	}
	importAccuracy := float64(importMatched) / float64(maxInt(1, len(expectedImports)))

	visibilityMatched := 0
	for sym, vis := range expectedVisibility {
		needle := fmt.Sprintf("%s (%s", strings.ToLower(strings.TrimSpace(sym)), strings.ToLower(strings.TrimSpace(vis)))
		if strings.Contains(normalized, needle) {
			visibilityMatched++
		}
	}
	visibility := float64(visibilityMatched) / float64(maxInt(1, len(expectedVisibility)))

	return gateB1LanguageScore{
		Language:              language,
		MicroRecall:           recall,
		MicroPrecision:        precision,
		MicroImportAccuracy:   importAccuracy,
		MicroVisibility:       visibility,
		MacroRecall:           recall,
		MacroPrecision:        precision,
		MacroImportAccuracy:   importAccuracy,
		MacroVisibility:       visibility,
		PerLanguageRecall:     recall,
		PerLanguagePrecision:  precision,
		PerLanguageImport:     importAccuracy,
		PerLanguageVisibility: visibility,
	}
}

func enforceB1Thresholds(scores map[string]gateB1LanguageScore, profile OutputProfile) error {
	proto, err := LoadB1ScoringProtocol()
	if err != nil {
		return fmt.Errorf("B1 threshold miss: load protocol artifact: %w", err)
	}
	if err := ValidateProtocolArtifact(proto); err != nil {
		return fmt.Errorf("B1 threshold miss: protocol artifact invalid: %w", err)
	}
	if len(scores) == 0 {
		return fmt.Errorf("B1 threshold miss: no language scores computed")
	}
	if len(scores) < proto.MinLanguageSamples {
		return fmt.Errorf("B1 threshold miss: insufficient language samples %d < %d", len(scores), proto.MinLanguageSamples)
	}

	langs := make([]string, 0, len(scores))
	for lang := range scores {
		langs = append(langs, lang)
	}
	sort.Strings(langs)

	thresholds := proto.ParityThresholds
	if profile == OutputProfileEnhancement {
		thresholds = proto.EnhancementThresholds
	}
	perLang := thresholds.PerLanguageFloor
	macro := thresholds.Macro
	micro := thresholds.Micro

	sumRecall := 0.0
	sumPrecision := 0.0
	sumImport := 0.0
	sumVisibility := 0.0

	for _, lang := range langs {
		s := scores[lang]
		if s.PerLanguageRecall < perLang.SymbolRecall {
			return fmt.Errorf("B1 threshold miss: %s symbol_recall %.2f < %.2f", lang, s.PerLanguageRecall, perLang.SymbolRecall)
		}
		if s.PerLanguagePrecision < perLang.SymbolPrecision {
			return fmt.Errorf("B1 threshold miss: %s symbol_precision %.2f < %.2f", lang, s.PerLanguagePrecision, perLang.SymbolPrecision)
		}
		if s.PerLanguageImport < perLang.ImportCategoryAccuracy {
			return fmt.Errorf("B1 threshold miss: %s import_category_accuracy %.2f < %.2f", lang, s.PerLanguageImport, perLang.ImportCategoryAccuracy)
		}
		if s.PerLanguageVisibility < perLang.VisibilityAccuracy {
			return fmt.Errorf("B1 threshold miss: %s visibility_accuracy %.2f < %.2f", lang, s.PerLanguageVisibility, perLang.VisibilityAccuracy)
		}

		sumRecall += s.MacroRecall
		sumPrecision += s.MacroPrecision
		sumImport += s.MacroImportAccuracy
		sumVisibility += s.MacroVisibility
	}

	denom := float64(len(langs))
	macroRecall := sumRecall / denom
	macroPrecision := sumPrecision / denom
	macroImport := sumImport / denom
	macroVisibility := sumVisibility / denom

	if macroRecall < macro.SymbolRecall {
		return fmt.Errorf("B1 threshold miss: macro symbol_recall %.2f < %.2f", macroRecall, macro.SymbolRecall)
	}
	if macroPrecision < macro.SymbolPrecision {
		return fmt.Errorf("B1 threshold miss: macro symbol_precision %.2f < %.2f", macroPrecision, macro.SymbolPrecision)
	}
	if macroImport < macro.ImportCategoryAccuracy {
		return fmt.Errorf("B1 threshold miss: macro import_category_accuracy %.2f < %.2f", macroImport, macro.ImportCategoryAccuracy)
	}
	if macroVisibility < macro.VisibilityAccuracy {
		return fmt.Errorf("B1 threshold miss: macro visibility_accuracy %.2f < %.2f", macroVisibility, macro.VisibilityAccuracy)
	}

	for _, lang := range langs {
		s := scores[lang]
		if s.MicroRecall < micro.SymbolRecall {
			return fmt.Errorf("B1 threshold miss: %s micro symbol_recall %.2f < %.2f", lang, s.MicroRecall, micro.SymbolRecall)
		}
		if s.MicroPrecision < micro.SymbolPrecision {
			return fmt.Errorf("B1 threshold miss: %s micro symbol_precision %.2f < %.2f", lang, s.MicroPrecision, micro.SymbolPrecision)
		}
		if s.MicroImportAccuracy < micro.ImportCategoryAccuracy {
			return fmt.Errorf("B1 threshold miss: %s micro import_category_accuracy %.2f < %.2f", lang, s.MicroImportAccuracy, micro.ImportCategoryAccuracy)
		}
		if s.MicroVisibility < micro.VisibilityAccuracy {
			return fmt.Errorf("B1 threshold miss: %s micro visibility_accuracy %.2f < %.2f", lang, s.MicroVisibility, micro.VisibilityAccuracy)
		}
	}

	return nil
}

func runParityGateB2DisclosureMarkerParityCheck() error {
	listOverflowRaw := `TypeScript file: Component.tsx
Functions:
  - one
  - two
  - three
  - four
  - five
  - six
  - seven
  - eight
  - nine
  - ten
`

	rawOverflowRaw := `Text file: notes.txt
Content:
line 1
line 2
line 3
line 4
line 5
line 6
line 7
line 8
line 9
line 10
line 11
line 12
line 13
line 14
line 15
line 16
line 17
`

	parityList := formatSummary(listOverflowRaw, OutputProfileParity)
	parityRaw := formatSummary(rawOverflowRaw, OutputProfileParity)
	enhList := formatSummary(listOverflowRaw, OutputProfileEnhancement)
	enhRaw := formatSummary(rawOverflowRaw, OutputProfileEnhancement)

	if err := verifyParityMarkerClasses(parityList); err != nil {
		return fmt.Errorf("parity list marker class check failed: %w", err)
	}
	if err := verifyParityMarkerClasses(strings.ReplaceAll(parityRaw, "[TRUNCATED]", "[ truncated ]")); err != nil {
		return fmt.Errorf("parity raw marker normalization check failed: %w", err)
	}
	if err := verifyCanonicalEnhancementMarkers(enhList); err != nil {
		return fmt.Errorf("enhancement list canonical marker check failed: %w", err)
	}
	if err := verifyCanonicalEnhancementMarkers(enhRaw); err != nil {
		return fmt.Errorf("enhancement raw canonical marker check failed: %w", err)
	}

	// Verify parity profile enforces section item caps.
	parityListLines := strings.Split(strings.TrimSpace(parityList), "\n")
	displayedItems := 0
	for _, line := range parityListLines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		if _, _, isMarker := parseNormalizedParityMarker(trimmed); isMarker {
			break
		}
		displayedItems++
	}
	if displayedItems > defaultSectionItemLimit {
		return fmt.Errorf("parity list displayed %d items, exceeds cap %d", displayedItems, defaultSectionItemLimit)
	}

	// Verify parity raw content enforces line caps.
	parityRawLines := strings.Split(strings.TrimSpace(parityRaw), "\n")
	displayedContentLines := 0
	for _, line := range parityRawLines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		if _, _, isMarker := parseNormalizedParityMarker(trimmed); isMarker {
			break
		}
		displayedContentLines++
	}
	if displayedContentLines > defaultSectionLineLimit {
		return fmt.Errorf("parity raw displayed %d content lines, exceeds cap %d", displayedContentLines, defaultSectionLineLimit)
	}

	return nil
}

func verifyParityMarkerClasses(summary string) error {
	for line := range strings.SplitSeq(summary, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "more") {
			continue
		}
		class, count, ok := parseNormalizedParityMarker(trimmed)
		if !ok {
			continue
		}
		if count <= 0 {
			return fmt.Errorf("invalid parity marker count in %q", trimmed)
		}
		if class != markerClassParityList && class != markerClassParityTruncated {
			return fmt.Errorf("unexpected parity marker class %q in %q", class, trimmed)
		}
		return nil
	}
	return fmt.Errorf("no parity marker found")
}

func parseNormalizedParityMarker(line string) (disclosureMarkerClass, int, bool) {
	normalized := strings.ToLower(strings.TrimSpace(line))
	normalized = strings.TrimPrefix(normalized, "- ")
	normalized = strings.Join(strings.Fields(normalized), " ")

	if m := reParityListMarker.FindStringSubmatch(normalized); len(m) == 2 {
		n, _ := strconv.Atoi(m[1])
		return markerClassParityList, n, true
	}
	if m := reParityTruncatedMarker.FindStringSubmatch(normalized); len(m) == 2 {
		n, _ := strconv.Atoi(m[1])
		return markerClassParityTruncated, n, true
	}

	return "", 0, false
}

func verifyCanonicalEnhancementMarkers(summary string) error {
	for line := range strings.SplitSeq(summary, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "more") {
			continue
		}
		class, count, ok := parseCanonicalEnhancementMarker(trimmed)
		if !ok {
			continue
		}
		if count <= 0 {
			return fmt.Errorf("invalid enhancement marker count in %q", trimmed)
		}
		if class != markerClassEnhancementList && class != markerClassEnhanceTruncated {
			return fmt.Errorf("unexpected enhancement marker class %q in %q", class, trimmed)
		}
		return nil
	}
	return fmt.Errorf("no canonical enhancement marker found")
}

func parseCanonicalEnhancementMarker(line string) (disclosureMarkerClass, int, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(line, "- "))
	if m := reEnhancementListCanonical.FindStringSubmatch(trimmed); len(m) == 2 {
		n, _ := strconv.Atoi(m[1])
		return markerClassEnhancementList, n, true
	}
	if m := reEnhanceRawCanonical.FindStringSubmatch(trimmed); len(m) == 2 {
		n, _ := strconv.Atoi(m[1])
		return markerClassEnhanceTruncated, n, true
	}
	return "", 0, false
}

func runParityGateB3RuntimePathMatrixChecks() error {
	inventory, err := LoadRuntimeInventory()
	if err != nil {
		return fmt.Errorf("load runtime inventory: %w", err)
	}
	if err := ValidateInventory(inventory); err != nil {
		return fmt.Errorf("runtime inventory invalid: %w", err)
	}

	parser := treesitter.NewParser()
	defer parser.Close()
	registry := NewRegistry(
		WithOutputProfile(OutputProfileEnhancement),
		WithTreeSitter(parser),
	)
	discovered := DiscoverRuntimePaths(registry, OutputProfileEnhancement)
	if err := validateRuntimePathMatrixAgainstInventory(inventory, discovered); err != nil {
		return err
	}
	if err := validateRuntimeRetrievalPersistenceExpectations(inventory); err != nil {
		return err
	}

	return nil
}

func inventoryToDiscovered(inventory *RuntimeInventory) []DiscoveredPath {
	discovered := make([]DiscoveredPath, 0, len(inventory.Paths))
	for _, p := range inventory.Paths {
		position := 0
		switch v := p.FallbackChainPosition.(type) {
		case float64:
			position = int(v)
		case int:
			position = v
		case string:
			position = 0
		}
		discovered = append(discovered, DiscoveredPath{
			ExplorerName: p.Explorer,
			Kind:         p.PathKind,
			Position:     position,
		})
	}
	return discovered
}

func validateRuntimePathMatrixAgainstInventory(inventory *RuntimeInventory, discovered []DiscoveredPath) error {
	if inventory == nil {
		return fmt.Errorf("runtime-path matrix mismatch: inventory is nil")
	}

	requiredKinds := []string{
		"archive_format_native",
		"document_format_native",
		"image_format_native",
		"executable_format_native",
		"native_binary",
		"data_format_native",
		"code_format_enhanced",
		"shell_format_native",
		"text_format_generic",
		"fallback_final",
	}
	discoveredByKind := make(map[string]map[string]struct{}, len(requiredKinds))
	allowedKinds := make(map[string]struct{}, len(requiredKinds))
	for _, kind := range requiredKinds {
		allowedKinds[kind] = struct{}{}
	}
	for _, d := range discovered {
		kind := strings.TrimSpace(d.Kind)
		explorer := strings.TrimSpace(d.ExplorerName)
		if kind == "" || explorer == "" {
			return fmt.Errorf("runtime-path matrix mismatch: discovered path has empty kind or explorer")
		}
		if _, ok := allowedKinds[kind]; !ok {
			return fmt.Errorf("runtime-path matrix mismatch: discovered invalid kind %s", kind)
		}
		if _, ok := discoveredByKind[kind]; !ok {
			discoveredByKind[kind] = make(map[string]struct{})
		}
		discoveredByKind[kind][explorer] = struct{}{}
	}
	for _, kind := range requiredKinds {
		if len(discoveredByKind[kind]) == 0 {
			return fmt.Errorf("runtime-path matrix mismatch: discovered runtime missing kind %s", kind)
		}
	}

	requiredInventory := map[string]string{
		"lcm.tool_output.create":            "ingestion",
		"lcm.describe.readback":             "retrieval",
		"lcm.expand.readback":               "retrieval",
		"volt.prompt.file.persist":          "ingestion",
		"volt.prompt.user_text.nonpersist":  "ingestion",
		"volt.tool.large_output.nonpersist": "ingestion",
		"volt.tool.read.nonpersist":         "ingestion",
		"volt.map_shared.persist":           "ingestion",
	}
	inventoryByID := make(map[string]RuntimeIngestionPath, len(inventory.Paths))
	for _, p := range inventory.Paths {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			return fmt.Errorf("runtime-path matrix mismatch: inventory contains empty id")
		}
		if _, exists := inventoryByID[id]; exists {
			return fmt.Errorf("runtime-path matrix mismatch: duplicate inventory id %s", id)
		}
		inventoryByID[id] = p
	}
	if len(inventoryByID) != len(requiredInventory) {
		return fmt.Errorf("runtime-path matrix mismatch: unexpected inventory path count %d (expected %d)", len(inventoryByID), len(requiredInventory))
	}
	for id, expectedKind := range requiredInventory {
		path, ok := inventoryByID[id]
		if !ok {
			return fmt.Errorf("runtime-path matrix mismatch: missing required inventory path %s", id)
		}
		if strings.TrimSpace(path.PathKind) != expectedKind {
			return fmt.Errorf("runtime-path matrix mismatch: %s path_kind=%s expected=%s", id, path.PathKind, expectedKind)
		}
	}

	createExplorer := strings.TrimSpace(inventoryByID["lcm.tool_output.create"].Explorer)
	if _, ok := discoveredByKind["text_format_generic"][createExplorer]; !ok {
		return fmt.Errorf("runtime-path matrix mismatch: lcm.tool_output.create explorer %s not discovered for kind text_format_generic", createExplorer)
	}
	describeExplorer := strings.TrimSpace(inventoryByID["lcm.describe.readback"].Explorer)
	if _, ok := discoveredByKind["fallback_final"][describeExplorer]; !ok {
		return fmt.Errorf("runtime-path matrix mismatch: lcm.describe.readback explorer %s not discovered for kind fallback_final", describeExplorer)
	}
	expandExplorer := strings.TrimSpace(inventoryByID["lcm.expand.readback"].Explorer)
	if _, ok := discoveredByKind["fallback_final"][expandExplorer]; !ok {
		return fmt.Errorf("runtime-path matrix mismatch: lcm.expand.readback explorer %s not discovered for kind fallback_final", expandExplorer)
	}

	return nil
}

func validateRuntimeRetrievalPersistenceExpectations(inventory *RuntimeInventory) error {
	byID := make(map[string]RuntimeIngestionPath, len(inventory.Paths))
	for _, p := range inventory.Paths {
		byID[p.ID] = p
		if strings.TrimSpace(p.EntryPoint) == "" {
			return fmt.Errorf("runtime-path matrix mismatch: missing entry point for %s", p.ID)
		}
		if len(p.ConfigGates) == 0 {
			return fmt.Errorf("runtime-path matrix mismatch: missing config gates for %s", p.ID)
		}
	}

	create, ok := byID["lcm.tool_output.create"]
	if !ok {
		return fmt.Errorf("runtime-path matrix mismatch: missing lcm.tool_output.create")
	}
	if create.PathKind != "ingestion" || !create.PersistsExplorationEnhanced {
		return fmt.Errorf("runtime-path matrix mismatch: invalid create ingestion/persistence contract")
	}

	describe, ok := byID["lcm.describe.readback"]
	if !ok {
		return fmt.Errorf("runtime-path matrix mismatch: missing lcm.describe.readback")
	}
	if describe.PathKind != "retrieval" || !describe.PersistsExplorationEnhanced {
		return fmt.Errorf("runtime-path matrix mismatch: invalid describe retrieval/persistence contract")
	}

	expand, ok := byID["lcm.expand.readback"]
	if !ok {
		return fmt.Errorf("runtime-path matrix mismatch: missing lcm.expand.readback")
	}
	if expand.PathKind != "retrieval" || !expand.PersistsExplorationEnhanced {
		return fmt.Errorf("runtime-path matrix mismatch: invalid expand retrieval/persistence contract")
	}

	return nil
}

type gateB4FormatScore struct {
	Format                string
	Profile               OutputProfile
	RequiredFieldCoverage float64
	MicroF1               float64
	MacroF1               float64
	MAPE                  float64
}

type gateB4FeatureSpec struct {
	RequiredFields []string
	ExpectedCounts map[string]float64
}

func runParityGateB4DataFormatDepthChecks() error {
	cfg := NewDefaultParityFixtureConfig(".")
	loader := NewParityFixtureLoader(cfg)
	index, err := loader.LoadIndex()
	if err != nil {
		return fmt.Errorf("load fixture index: %w", err)
	}

	fixtureByFormat := map[string]string{
		"latex":       index.Format["latex"],
		"logs":        index.Format["logs"],
		"sqlite_seed": index.Format["sqlite_seed"],
		"markdown":    index.Markdown["readme"],
	}
	for key, fixture := range fixtureByFormat {
		if strings.TrimSpace(fixture) == "" {
			return fmt.Errorf("missing required B4 fixture: %s", key)
		}
	}

	profiles := []OutputProfile{OutputProfileParity, OutputProfileEnhancement}
	scoresByProfile := make(map[OutputProfile]map[string]gateB4FormatScore, len(profiles))

	for _, profile := range profiles {
		registry := NewRegistry(WithOutputProfile(profile))
		profileScores := make(map[string]gateB4FormatScore, len(fixtureByFormat))

		for key, fixtureName := range fixtureByFormat {
			raw, err := LoadFixtureFile(cfg, fixtureName)
			if err != nil {
				return fmt.Errorf("load %s fixture %q: %w", key, fixtureName, err)
			}

			input, spec, err := buildB4GateInputAndSpec(key, fixtureName, raw, profile)
			if err != nil {
				return fmt.Errorf("prepare B4 fixture %s: %w", key, err)
			}

			result, err := registry.exploreStatic(context.Background(), input)
			if err != nil {
				return fmt.Errorf("explore B4 fixture %s (%s): %w", key, profile, err)
			}

			actualCounts := extractB4ActualCounts(key, result.Summary)
			score := scoreB4FormatMetrics(key, profile, result.Summary, spec, actualCounts)
			profileScores[key] = score
		}

		scoresByProfile[profile] = profileScores
	}

	for _, profile := range profiles {
		if err := enforceB4Thresholds(profile, scoresByProfile[profile]); err != nil {
			return err
		}
	}

	if err := runGateB4ArtifactCoverageChecks(index); err != nil {
		return err
	}

	return nil
}

func buildB4GateInputAndSpec(formatKey, fixtureName string, raw []byte, profile OutputProfile) (ExploreInput, gateB4FeatureSpec, error) {
	switch formatKey {
	case "markdown":
		explorer := &MarkdownExplorer{}
		content := string(raw)
		_, frontmatter := explorer.extractFrontmatter(content)
		frontmatterKeys := 0.0
		if len(frontmatter) > 0 {
			var parsed map[string]any
			if err := yamlUnmarshalForB4(frontmatter, &parsed); err == nil {
				frontmatterKeys = float64(len(parsed))
			}
		}
		headings := explorer.extractHeadings(content)
		expected := map[string]float64{
			"frontmatter_keys":      frontmatterKeys,
			"heading_total":         float64(len(headings)),
			"inline_links":          float64(explorer.countInlineLinks(content)),
			"reference_links":       float64(explorer.countReferenceLinks(content)),
			"autolinks":             float64(explorer.countAutolinks(content)),
			"reference_definitions": float64(explorer.countReferenceDefinitions(content)),
		}
		required := []string{
			"markdown file",
			"frontmatter",
			"heading hierarchy",
			"links",
			"inline links (markdown style)",
			"reference definitions",
		}
		return ExploreInput{Path: fixtureName, Content: raw}, gateB4FeatureSpec{RequiredFields: required, ExpectedCounts: expected}, nil
	case "latex":
		content := string(raw)
		sections := extractLatexSections(content)
		sectionCounts := countSectionsByLevel(sections)
		envs := extractLatexEnvironments(content)
		envCounts := make(map[string]float64, len(envs))
		for _, env := range envs {
			envCounts[strings.ToLower(env.Name)] = float64(env.Count)
		}
		biblio := extractLatexBibliography(content)
		expected := map[string]float64{
			"section":       float64(sectionCounts[1]),
			"subsection":    float64(sectionCounts[2]),
			"subsubsection": float64(sectionCounts[3]),
			"citations":     float64(biblio.CiteCount),
			"env_figure":    envCounts["figure"],
			"env_table":     envCounts["table"],
			"env_equation":  envCounts["equation"],
		}
		required := []string{
			"latex file",
			"section structure",
			"environments",
			"citations",
			"style",
			"packages",
			"- \\bibliography",
		}
		if profile == OutputProfileEnhancement {
			required = append(required, "references", "citation keys")
		}
		return ExploreInput{Path: fixtureName, Content: raw}, gateB4FeatureSpec{RequiredFields: required, ExpectedCounts: expected}, nil
	case "logs":
		lineList := strings.Split(string(raw), "\n")
		levels := make(map[string]int)
		countLogLevels(lineList, levels)
		expected := map[string]float64{
			"total_lines": float64(len(lineList)),
			"level_error": float64(levels["ERROR"]),
			"level_warn":  float64(levels["WARN"]),
			"level_info":  float64(levels["INFO"]),
		}
		required := []string{
			"log file",
			"total lines",
			"level distribution",
			"timestamp patterns",
			"sample errors/warnings",
		}
		return ExploreInput{Path: fixtureName, Content: raw}, gateB4FeatureSpec{RequiredFields: required, ExpectedCounts: expected}, nil
	case "sqlite_seed":
		dbBytes, expected, err := buildSQLiteFixtureFromSeed(raw)
		if err != nil {
			return ExploreInput{}, gateB4FeatureSpec{}, err
		}
		if profile != OutputProfileEnhancement {
			delete(expected, "views")
			delete(expected, "triggers")
			delete(expected, "constraints")
			delete(expected, "unique_index")
		}
		required := []string{
			"sqlite database",
			"tables",
			"indexes",
			"table inventory",
			"index inventory",
		}
		if profile == OutputProfileEnhancement {
			required = append(required, "views", "triggers", "constraints", "unique index")
		}
		return ExploreInput{Path: "format_fixture.db", Content: dbBytes}, gateB4FeatureSpec{RequiredFields: required, ExpectedCounts: expected}, nil
	default:
		return ExploreInput{}, gateB4FeatureSpec{}, fmt.Errorf("unsupported B4 format key %q", formatKey)
	}
}

func scoreB4FormatMetrics(formatKey string, profile OutputProfile, summary string, spec gateB4FeatureSpec, actual map[string]float64) gateB4FormatScore {
	matched := 0
	for _, field := range spec.RequiredFields {
		if strings.Contains(strings.ToLower(summary), strings.ToLower(field)) {
			matched++
		}
	}
	requiredCount := maxInt(1, len(spec.RequiredFields))
	coverage := float64(matched) / float64(requiredCount)

	microF1 := 0.0
	if matched > 0 {
		microF1 = (2.0 * float64(matched)) / (2.0*float64(matched) + float64(requiredCount-matched))
	}

	macroSum := 0.0
	for _, field := range spec.RequiredFields {
		if strings.Contains(strings.ToLower(summary), strings.ToLower(field)) {
			macroSum += 1.0
		}
	}
	macroF1 := macroSum / float64(requiredCount)

	mape, _ := computeMAPE(spec.ExpectedCounts, actual)

	return gateB4FormatScore{
		Format:                formatKey,
		Profile:               profile,
		RequiredFieldCoverage: coverage,
		MicroF1:               microF1,
		MacroF1:               macroF1,
		MAPE:                  mape,
	}
}

func runGateB4ArtifactCoverageChecks(index *ParityFixtureIndex) error {
	phasePath := filepath.Join("testdata", "parity_volt", "phase_0c_gate_artifact.v1.json")
	phaseBytes, err := os.ReadFile(phasePath)
	if err != nil {
		return fmt.Errorf("B4 artifact coverage: read %s: %w", phasePath, err)
	}
	var phase struct {
		Evidence struct {
			TestedExplorers []string `json:"tested_explorers"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal(phaseBytes, &phase); err != nil {
		return fmt.Errorf("B4 artifact coverage: parse %s: %w", phasePath, err)
	}
	tested := make(map[string]struct{}, len(phase.Evidence.TestedExplorers))
	for _, exp := range phase.Evidence.TestedExplorers {
		tested[strings.ToLower(strings.TrimSpace(exp))] = struct{}{}
	}
	for _, req := range []string{"markdown", "latex", "sqlite", "logs"} {
		if _, ok := tested[req]; !ok {
			return fmt.Errorf("B4 artifact coverage: tested_explorers missing %q", req)
		}
	}

	matrixPath := filepath.Join("testdata", "parity_volt", "explorer_family_matrix.v1.json")
	matrixBytes, err := os.ReadFile(matrixPath)
	if err != nil {
		return fmt.Errorf("B4 artifact coverage: read %s: %w", matrixPath, err)
	}
	var matrix ExplorerFamilyMatrix
	if err := json.Unmarshal(matrixBytes, &matrix); err != nil {
		return fmt.Errorf("B4 artifact coverage: parse %s: %w", matrixPath, err)
	}
	requiredFamilies := map[string]string{
		"markdownexplorer": "markdown",
		"latexexplorer":    "latex",
		"sqliteexplorer":   "sqlite",
		"logsexplorer":     "logs",
	}
	present := map[string]bool{}
	for _, exp := range matrix.Explorers {
		id := strings.ToLower(strings.TrimSpace(exp.ExplorerID))
		if fam, ok := requiredFamilies[id]; ok {
			for _, language := range exp.LanguageFamilies {
				if strings.EqualFold(language, fam) {
					present[fam] = true
				}
			}
		}
	}
	for fam := range map[string]struct{}{"markdown": {}, "latex": {}, "sqlite": {}, "logs": {}} {
		if !present[fam] {
			return fmt.Errorf("B4 artifact coverage: explorer_family_matrix missing %q family mapping", fam)
		}
	}

	if _, ok := index.Markdown["readme"]; !ok {
		return fmt.Errorf("B4 artifact coverage: fixture index missing markdown.readme")
	}
	for _, key := range []string{"latex", "logs", "sqlite_seed"} {
		if _, ok := index.Format[key]; !ok {
			return fmt.Errorf("B4 artifact coverage: fixture index missing format.%s", key)
		}
	}

	return nil
}

func enforceB4Thresholds(profile OutputProfile, scores map[string]gateB4FormatScore) error {
	minPerFormatCoverage := 1.00
	minPerFormatMicroF1 := 0.90
	minPerFormatMacroF1 := 0.86
	minMacroCoverage := 1.00
	minMacroMicroF1 := 0.90
	minMacroMacroF1 := 0.86
	maxPerFormatMAPE := 0.10
	maxMacroMAPE := 0.10

	if profile == OutputProfileEnhancement {
		minPerFormatMicroF1 = 0.94
		minPerFormatMacroF1 = 0.90
		minMacroMicroF1 = 0.94
		minMacroMacroF1 = 0.90
		maxPerFormatMAPE = 0.05
		maxMacroMAPE = 0.05
	}

	if len(scores) == 0 {
		return fmt.Errorf("B4 threshold miss (%s): no scores computed", profile)
	}

	formats := make([]string, 0, len(scores))
	for format := range scores {
		formats = append(formats, format)
	}
	sort.Strings(formats)

	macroCoverage := 0.0
	macroMicroF1 := 0.0
	macroMacroF1 := 0.0
	macroMAPE := 0.0

	for _, format := range formats {
		s := scores[format]
		if s.RequiredFieldCoverage < minPerFormatCoverage {
			return fmt.Errorf("B4 threshold miss (%s): %s required-field coverage %.3f < %.3f", profile, format, s.RequiredFieldCoverage, minPerFormatCoverage)
		}
		if s.MicroF1 < minPerFormatMicroF1 {
			return fmt.Errorf("B4 threshold miss (%s): %s micro_f1 %.3f < %.3f", profile, format, s.MicroF1, minPerFormatMicroF1)
		}
		if s.MacroF1 < minPerFormatMacroF1 {
			return fmt.Errorf("B4 threshold miss (%s): %s macro_f1 %.3f < %.3f", profile, format, s.MacroF1, minPerFormatMacroF1)
		}
		if s.MAPE > maxPerFormatMAPE {
			return fmt.Errorf("B4 threshold miss (%s): %s mape %.3f > %.3f", profile, format, s.MAPE, maxPerFormatMAPE)
		}
		macroCoverage += s.RequiredFieldCoverage
		macroMicroF1 += s.MicroF1
		macroMacroF1 += s.MacroF1
		macroMAPE += s.MAPE
	}

	denom := float64(len(formats))
	macroCoverage /= denom
	macroMicroF1 /= denom
	macroMacroF1 /= denom
	macroMAPE /= denom

	if macroCoverage < minMacroCoverage {
		return fmt.Errorf("B4 threshold miss (%s): macro required-field coverage %.3f < %.3f", profile, macroCoverage, minMacroCoverage)
	}
	if macroMicroF1 < minMacroMicroF1 {
		return fmt.Errorf("B4 threshold miss (%s): macro micro_f1 %.3f < %.3f", profile, macroMicroF1, minMacroMicroF1)
	}
	if macroMacroF1 < minMacroMacroF1 {
		return fmt.Errorf("B4 threshold miss (%s): macro macro_f1 %.3f < %.3f", profile, macroMacroF1, minMacroMacroF1)
	}
	if macroMAPE > maxMacroMAPE {
		return fmt.Errorf("B4 threshold miss (%s): macro mape %.3f > %.3f", profile, macroMAPE, maxMacroMAPE)
	}

	return nil
}

func extractB4ActualCounts(formatKey, summary string) map[string]float64 {
	switch formatKey {
	case "markdown":
		return map[string]float64{
			"frontmatter_keys":      mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?Frontmatter keys:\s+(\d+)`),
			"heading_total":         mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?Total:\s+(\d+)`),
			"inline_links":          mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?Inline links \(markdown style\):\s+(\d+)`),
			"reference_links":       mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?Reference-style links:\s+(\d+)`),
			"autolinks":             mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?Autolinks \(http/https URLs\):\s+(\d+)`),
			"reference_definitions": mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?Reference definitions:\s+(\d+)`),
		}
	case "latex":
		return map[string]float64{
			"section":       mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?\\section:\s+(\d+)`),
			"subsection":    mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?\\subsection:\s+(\d+)`),
			"subsubsection": mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?\\subsubsection:\s+(\d+)`),
			"citations":     mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?Citations:\s+(\d+)`),
			"env_figure":    mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?figure:\s+(\d+)`),
			"env_table":     mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?table:\s+(\d+)`),
			"env_equation":  mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?equation:\s+(\d+)`),
		}
	case "logs":
		return map[string]float64{
			"total_lines": mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?Total lines:\s+(\d+)`),
			"level_error": mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?ERROR:\s+(\d+)`),
			"level_warn":  mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?WARN:\s+(\d+)`),
			"level_info":  mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?INFO:\s+(\d+)`),
		}
	case "sqlite_seed":
		return map[string]float64{
			"tables":       mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?Tables:\s+(\d+)`),
			"indexes":      mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?Indexes:\s+(\d+)`),
			"views":        mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?Views:\s+(\d+)`),
			"triggers":     mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?Triggers:\s+(\d+)`),
			"constraints":  mustExtractFloat(summary, `(?m)^\s*(?:-\s*)?Constraints:\s+(\d+)`),
			"unique_index": float64(strings.Count(strings.ToLower(summary), "unique index")),
		}
	default:
		return map[string]float64{}
	}
}

func mustExtractFloat(summary, pattern string) float64 {
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(summary)
	if len(match) != 2 {
		return 0
	}
	v, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0
	}
	return v
}

func computeMAPE(expected, actual map[string]float64) (float64, int) {
	totalAPE := 0.0
	samples := 0
	for key, exp := range expected {
		if exp <= 0 {
			continue
		}
		obs := actual[key]
		totalAPE += math.Abs(obs-exp) / exp
		samples++
	}
	if samples == 0 {
		return 0, 0
	}
	return totalAPE / float64(samples), samples
}

func buildSQLiteFixtureFromSeed(seedSQL []byte) ([]byte, map[string]float64, error) {
	tmpDir, err := os.MkdirTemp("", "crush-b4-sqlite-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "fixture.db")
	dsn := fmt.Sprintf("file:%s", url.QueryEscape(dbPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite db: %w", err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, string(seedSQL)); err != nil {
		return nil, nil, fmt.Errorf("apply sqlite seed: %w", err)
	}

	explorer := &SQLiteExplorer{}
	tables, err := explorer.getTables(ctx, db)
	if err != nil {
		return nil, nil, fmt.Errorf("query sqlite tables: %w", err)
	}
	indexes, err := explorer.getIndexes(ctx, db)
	if err != nil {
		return nil, nil, fmt.Errorf("query sqlite indexes: %w", err)
	}
	views, err := explorer.getViews(ctx, db)
	if err != nil {
		return nil, nil, fmt.Errorf("query sqlite views: %w", err)
	}
	triggers, err := explorer.getTriggers(ctx, db)
	if err != nil {
		return nil, nil, fmt.Errorf("query sqlite triggers: %w", err)
	}
	constraints, err := explorer.getConstraints(ctx, db, tables)
	if err != nil {
		return nil, nil, fmt.Errorf("query sqlite constraints: %w", err)
	}
	constraintCount := 0
	for _, cs := range constraints {
		constraintCount += len(cs)
	}

	content, err := os.ReadFile(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read sqlite fixture bytes: %w", err)
	}

	uniqueIndexCount := 0
	for _, cs := range constraints {
		for _, c := range cs {
			if strings.EqualFold(c.Type, "UNIQUE INDEX") {
				uniqueIndexCount++
			}
		}
	}

	expected := map[string]float64{
		"tables":       float64(len(tables)),
		"indexes":      float64(len(indexes)),
		"views":        float64(len(views)),
		"triggers":     float64(len(triggers)),
		"constraints":  float64(constraintCount),
		"unique_index": float64(uniqueIndexCount),
	}
	return content, expected, nil
}

func validateJSONFixtureDepthAndFields(content []byte) error {
	var payload map[string]any
	if err := json.Unmarshal(content, &payload); err != nil {
		return fmt.Errorf("json parse failed: %w", err)
	}

	requiredTop := []string{"data_format", "description", "provenance", "content"}
	for _, key := range requiredTop {
		if _, ok := payload[key]; !ok {
			return fmt.Errorf("missing top-level required field: %s", key)
		}
	}

	depth := maxJSONDepth(payload, 1)
	if depth < 4 {
		return fmt.Errorf("json depth too shallow: got %d want >=4", depth)
	}

	contentMap, ok := payload["content"].(map[string]any)
	if !ok || len(contentMap) < 4 {
		return fmt.Errorf("json content quality too low")
	}
	return nil
}

func maxJSONDepth(value any, depth int) int {
	maxDepth := depth
	switch v := value.(type) {
	case map[string]any:
		for _, child := range v {
			maxDepth = maxInt(maxDepth, maxJSONDepth(child, depth+1))
		}
	case []any:
		for _, child := range v {
			maxDepth = maxInt(maxDepth, maxJSONDepth(child, depth+1))
		}
	}
	return maxDepth
}

func yamlUnmarshalForB4(in []byte, out *map[string]any) error {
	parsed := make(map[string]any)
	if len(in) == 0 {
		*out = parsed
		return nil
	}
	if err := yaml.Unmarshal(in, &parsed); err != nil {
		return err
	}
	*out = parsed
	return nil
}

func runParityGateB5DeterministicE2EParityCheck() error {
	cfg := NewDefaultParityFixtureConfig(".")
	loader := NewParityFixtureLoader(cfg)
	index, err := loader.LoadIndex()
	if err != nil {
		return fmt.Errorf("load fixture index: %w", err)
	}
	if err := ValidateFixtureMetadata(index.Metadata); err != nil {
		return fmt.Errorf("metadata deterministic/parity check failed: %w", err)
	}

	bundle := validParityBundleForGateB()
	if err := RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: true,
		CorpusBasePath:         ".",
		Profile: &ParityPreflightProfile{
			ID:                      "parity-gate-b5",
			TokenBudget:             4096,
			RepeatRuns:              2,
			ParityMode:              true,
			DeterministicMode:       true,
			EnhancementTiersEnabled: "none",
			TokenCounterMode:        "tokenizer_backed",
			FixedSeed:               1337,
		},
	}); err != nil {
		return fmt.Errorf("preflight deterministic parity check failed: %w", err)
	}

	inv, err := LoadRuntimeInventory()
	if err != nil {
		return err
	}
	parityView := projectParityRuntimeInventory(inv)
	if err := ValidateInventory(parityView); err != nil {
		return fmt.Errorf("runtime parity fail-closed check failed: %w", err)
	}

	// Deterministic E2E: run real Explore() on fixture files, verify
	// determinism and no tier leakage.
	fixtureDir := filepath.Join("testdata", "parity_volt", "fixtures")
	fixtureFiles := []string{
		"binary_elf_header.bin",
		"negative_truncated.json",
		"negative_unsupported.xyz",
	}

	reg := NewRegistry()
	for _, fname := range fixtureFiles {
		fpath := filepath.Join(fixtureDir, fname)
		content, readErr := os.ReadFile(fpath)
		if readErr != nil {
			return fmt.Errorf("read fixture %s: %w", fname, readErr)
		}

		// Run twice to verify determinism.
		result1, err1 := reg.Explore(context.Background(), ExploreInput{
			Path:    fname,
			Content: content,
		})
		if err1 != nil {
			return fmt.Errorf("explore %s run 1: %w", fname, err1)
		}
		result2, err2 := reg.Explore(context.Background(), ExploreInput{
			Path:    fname,
			Content: content,
		})
		if err2 != nil {
			return fmt.Errorf("explore %s run 2: %w", fname, err2)
		}

		if result1.Summary != result2.Summary {
			return fmt.Errorf("explore %s: determinism violation, summaries differ", fname)
		}
		if result1.ExplorerUsed != result2.ExplorerUsed {
			return fmt.Errorf("explore %s: determinism violation, explorer mismatch %s vs %s",
				fname, result1.ExplorerUsed, result2.ExplorerUsed)
		}

		// No tier leakage: explorerUsed must not contain +llm or +agent.
		if strings.Contains(result1.ExplorerUsed, "+llm") {
			return fmt.Errorf("explore %s: tier leakage, explorerUsed contains +llm", fname)
		}
		if strings.Contains(result1.ExplorerUsed, "+agent") {
			return fmt.Errorf("explore %s: tier leakage, explorerUsed contains +agent", fname)
		}
	}

	return nil
}

func validParityBundleForGateB() ParityProvenanceBundle {
	return ParityProvenanceBundle{
		VoltCommitSHA:     strings.Repeat("a", 40),
		ComparatorPath:    "../volt/tree/" + strings.Repeat("a", 40),
		FixturesSHA256:    strings.Repeat("b", 64),
		GrepASTProvenance: "grep-ast@v1.2.3",
		TokenizerID:       "cl100k_base",
		TokenizerVersion:  "v0.1.0",
	}
}

func projectParityRuntimeInventory(src *RuntimeInventory) *RuntimeInventory {
	cloned := *src
	cloned.Profile = string(OutputProfileParity)
	cloned.DeterministicMode = true
	cloned.EnhancementTiersEnabled = "none"
	cloned.TokenCounterMode = "tokenizer_backed"
	cloned.FixedSeed = 1337

	filtered := make([]RuntimeIngestionPath, 0, len(src.Paths))
	for _, p := range src.Paths {
		if p.PathKind == "enhancement_tier2" || p.PathKind == "enhancement_tier3" {
			continue
		}
		p.LLMEnhancement = false
		filtered = append(filtered, p)
	}
	cloned.Paths = filtered
	return &cloned
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func TestExplorerFamilyMatrixFamiliesNonEmpty(t *testing.T) {
	t.Parallel()

	efm, err := LoadExplorerFamilyMatrix()
	require.NoError(t, err, "failed to load explorer family matrix")
	require.NotEmpty(t, efm.Families, "explorer family matrix families array must not be empty")

	for i, fam := range efm.Families {
		require.NotEmpty(t, fam.Family, "families[%d]: family name must not be empty", i)
		require.Greater(t, fam.ScoreWeight, 0.0, "families[%d]: score_weight must be positive", i)
		require.Greater(t, fam.Threshold, 0.0, "families[%d]: threshold must be positive", i)
	}
}

func TestParityFixtureDispatchBinaryAndNegative(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()

	tests := []struct {
		name             string
		path             string
		content          []byte
		expectedExplorer string
	}{
		{
			name:             "ELF header dispatches to executable",
			path:             "binary_elf_header.bin",
			content:          []byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			expectedExplorer: "executable",
		},
		{
			name: "PNG header dispatches to image",
			path: "binary_png_header.png",
			content: func() []byte {
				sig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
				ihdr := make([]byte, 25)
				ihdr[0], ihdr[1], ihdr[2], ihdr[3] = 0, 0, 0, 13
				copy(ihdr[4:8], []byte("IHDR"))
				ihdr[8], ihdr[9], ihdr[10], ihdr[11] = 0, 0, 0, 1
				ihdr[12], ihdr[13], ihdr[14], ihdr[15] = 0, 0, 0, 1
				ihdr[16] = 8
				ihdr[17] = 6
				return append(sig, ihdr...)
			}(),
			expectedExplorer: "image",
		},
		{
			name:             "truncated JSON dispatches to json",
			path:             "negative_truncated.json",
			content:          []byte(`{"key": "value", "incomplete":`),
			expectedExplorer: "json",
		},
		{
			// TextExplorer claims .xyz because content is valid UTF-8 text.
			// TASKS predicted FallbackExplorer, but TextExplorer precedes it
			// in the chain and accepts any valid text content.
			name:             "unsupported extension dispatches to text",
			path:             "negative_unsupported.xyz",
			content:          []byte("This is a file with an unsupported extension.\nIt should be handled by the fallback explorer.\n"),
			expectedExplorer: "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := reg.Explore(context.Background(), ExploreInput{
				Path:    tt.path,
				Content: tt.content,
			})
			require.NoError(t, err, "Explore should not return error")
			require.Equal(t, tt.expectedExplorer, result.ExplorerUsed)
		})
	}
}
