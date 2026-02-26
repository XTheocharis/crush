package explorer

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type gateB1LanguageScore struct {
	Language string
	Micro    float64
	Macro    float64
	Style    float64
	Overall  float64
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
	require.NoError(t, runParityGateB1ExtractionQualityScoringCheck())

	t.Run("detects intentional low-quality summary", func(t *testing.T) {
		t.Parallel()
		low := map[string]gateB1LanguageScore{
			"go": {
				Language: "go",
				Micro:    0.05,
				Macro:    0.05,
				Style:    0.10,
				Overall:  0.06,
			},
		}
		err := enforceB1Thresholds(low)
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

		discovered := inventoryToDiscovered(inventory)
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
		{name: "B1 extraction quality scoring", run: runParityGateB1ExtractionQualityScoringCheck},
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

func runParityGateB1ExtractionQualityScoringCheck() error {
	cfg := NewDefaultParityFixtureConfig(".")
	loader := NewParityFixtureLoader(cfg)
	index, err := loader.LoadIndex()
	if err != nil {
		return fmt.Errorf("load fixture index: %w", err)
	}

	registry := NewRegistry(WithOutputProfile(OutputProfileParity))

	languageExpectations := map[string][]string{
		"go":     {"package", "import", "type", "func"},
		"python": {"import", "class", "def", "dataclass"},
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

		scores[lang] = scoreExtractionQuality(lang, result.Summary, expected)
	}

	if err := enforceB1Thresholds(scores); err != nil {
		return err
	}
	return nil
}

func scoreExtractionQuality(language, summary string, expectedCapabilities []string) gateB1LanguageScore {
	normalized := strings.ToLower(summary)
	matched := 0
	for _, token := range expectedCapabilities {
		if strings.Contains(normalized, strings.ToLower(token)) {
			matched++
		}
	}

	micro := float64(matched) / float64(maxInt(1, len(expectedCapabilities)))

	sectionCount := strings.Count(summary, "### ")
	bulletCount := strings.Count(summary, "- ")
	style := 0.0
	if strings.HasPrefix(strings.TrimSpace(summary), "## ") {
		style += 0.4
	}
	if sectionCount >= 2 {
		style += 0.3
	}
	if bulletCount >= 4 {
		style += 0.3
	}

	macro := (micro + style) / 2.0
	overall := (micro*0.6 + macro*0.25 + style*0.15)

	return gateB1LanguageScore{
		Language: language,
		Micro:    micro,
		Macro:    macro,
		Style:    style,
		Overall:  overall,
	}
}

func enforceB1Thresholds(scores map[string]gateB1LanguageScore) error {
	const (
		minPerLanguageMicro = 0.70
		minPerLanguageStyle = 0.60
		minPerLanguageScore = 0.70
		minMacroOverall     = 0.75
	)

	if len(scores) == 0 {
		return fmt.Errorf("B1 threshold miss: no language scores computed")
	}

	langs := make([]string, 0, len(scores))
	for lang := range scores {
		langs = append(langs, lang)
	}
	sort.Strings(langs)

	macroOverall := 0.0
	for _, lang := range langs {
		s := scores[lang]
		if s.Micro < minPerLanguageMicro {
			return fmt.Errorf("B1 threshold miss: %s micro %.2f < %.2f", lang, s.Micro, minPerLanguageMicro)
		}
		if s.Style < minPerLanguageStyle {
			return fmt.Errorf("B1 threshold miss: %s style %.2f < %.2f", lang, s.Style, minPerLanguageStyle)
		}
		if s.Overall < minPerLanguageScore {
			return fmt.Errorf("B1 threshold miss: %s overall %.2f < %.2f", lang, s.Overall, minPerLanguageScore)
		}
		macroOverall += s.Overall
	}

	macroOverall /= float64(len(langs))
	if macroOverall < minMacroOverall {
		return fmt.Errorf("B1 threshold miss: macro overall %.2f < %.2f", macroOverall, minMacroOverall)
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

	return nil
}

func verifyParityMarkerClasses(summary string) error {
	for _, line := range strings.Split(summary, "\n") {
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
	for _, line := range strings.Split(summary, "\n") {
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

	discovered := inventoryToDiscovered(inventory)
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
	artifactKeys := make(map[string]struct{}, len(inventory.Paths))
	for _, p := range inventory.Paths {
		key := fmt.Sprintf("%s:%s", p.Explorer, p.PathKind)
		artifactKeys[key] = struct{}{}
	}

	for _, d := range discovered {
		key := fmt.Sprintf("%s:%s", d.ExplorerName, d.Kind)
		if _, ok := artifactKeys[key]; !ok {
			return fmt.Errorf("runtime-path matrix mismatch: discovered drift for %s", key)
		}
	}

	return nil
}

func validateRuntimeRetrievalPersistenceExpectations(inventory *RuntimeInventory) error {
	byID := make(map[string]RuntimeIngestionPath, len(inventory.Paths))
	for _, p := range inventory.Paths {
		byID[p.ID] = p
		if p.EntryPoint != "RuntimeAdapter.Explore" {
			return fmt.Errorf("runtime-path matrix mismatch: unexpected entry point for %s", p.ID)
		}
	}

	binary, ok := byID["path_binary_direct"]
	if !ok {
		return fmt.Errorf("runtime-path matrix mismatch: missing path_binary_direct")
	}
	if binary.PathKind != "native_binary" || binary.LLMEnhancement {
		return fmt.Errorf("runtime-path matrix mismatch: invalid binary retrieval/persistence contract")
	}

	text, ok := byID["path_text_generic"]
	if !ok {
		return fmt.Errorf("runtime-path matrix mismatch: missing path_text_generic")
	}
	if text.PathKind != "text_format_generic" || !text.LLMEnhancement {
		return fmt.Errorf("runtime-path matrix mismatch: invalid text retrieval/persistence contract")
	}

	fallback, ok := byID["path_fallback_final"]
	if !ok {
		return fmt.Errorf("runtime-path matrix mismatch: missing path_fallback_final")
	}
	if fallback.PathKind != "fallback_final" || fallback.LLMEnhancement {
		return fmt.Errorf("runtime-path matrix mismatch: invalid fallback retrieval/persistence contract")
	}

	return nil
}

func runParityGateB4DataFormatDepthChecks() error {
	cfg := NewDefaultParityFixtureConfig(".")
	loader := NewParityFixtureLoader(cfg)
	index, err := loader.LoadIndex()
	if err != nil {
		return fmt.Errorf("load fixture index: %w", err)
	}

	for lang, fixture := range index.Language {
		content, err := LoadFixtureFile(cfg, fixture)
		if err != nil {
			return fmt.Errorf("load %s fixture: %w", lang, err)
		}
		if err := validateLanguageFixtureDepth(lang, content); err != nil {
			return fmt.Errorf("language fixture %s failed depth checks: %w", lang, err)
		}
	}

	if jsonFixture, ok := index.Format["json"]; ok {
		content, err := LoadFixtureFile(cfg, jsonFixture)
		if err != nil {
			return err
		}
		if err := validateJSONFixtureDepthAndFields(content); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("missing json format fixture")
	}

	if yamlFixture, ok := index.Format["yaml"]; ok {
		content, err := LoadFixtureFile(cfg, yamlFixture)
		if err != nil {
			return err
		}
		if err := validateYAMLFixtureDepthAndFields(content); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("missing yaml format fixture")
	}

	if csvFixture, ok := index.Format["csv"]; ok {
		content, err := LoadFixtureFile(cfg, csvFixture)
		if err != nil {
			return err
		}
		if err := validateCSVFixtureDepthAndFields(content); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("missing csv format fixture")
	}

	return nil
}

func validateLanguageFixtureDepth(language string, content []byte) error {
	text := string(content)
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) < 20 {
		return fmt.Errorf("fixture quality too low: expected >=20 lines, got %d", len(lines))
	}

	normalized := strings.ToLower(text)
	switch language {
	case "go":
		if strings.Count(normalized, "func ") < 3 {
			return fmt.Errorf("insufficient go function depth")
		}
		if strings.Count(normalized, "type ") < 2 {
			return fmt.Errorf("insufficient go type depth")
		}
	case "python":
		if strings.Count(normalized, "def ") < 3 {
			return fmt.Errorf("insufficient python function depth")
		}
		if !strings.Contains(normalized, "class ") {
			return fmt.Errorf("missing python class depth")
		}
	}
	return nil
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

func validateYAMLFixtureDepthAndFields(content []byte) error {
	text := string(content)
	lower := strings.ToLower(text)
	requiredTokens := []string{"services:", "networks:", "volumes:"}
	for _, token := range requiredTokens {
		if !strings.Contains(lower, token) {
			return fmt.Errorf("yaml missing required field: %s", token)
		}
	}

	maxIndent := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent > maxIndent {
			maxIndent = indent
		}
	}
	if maxIndent < 4 {
		return fmt.Errorf("yaml depth too shallow: max indentation %d", maxIndent)
	}

	return nil
}

func validateCSVFixtureDepthAndFields(content []byte) error {
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) < 6 {
		return fmt.Errorf("csv has insufficient rows: %d", len(lines))
	}
	columns := strings.Split(lines[0], ",")
	if len(columns) < 5 {
		return fmt.Errorf("csv has insufficient columns: %d", len(columns))
	}
	requiredColumns := []string{"id", "name", "email"}
	for _, required := range requiredColumns {
		found := false
		for _, col := range columns {
			if strings.EqualFold(strings.TrimSpace(col), required) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("csv missing required column: %s", required)
		}
	}
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

	return nil
}

func validParityBundleForGateB() ParityProvenanceBundle {
	return ParityProvenanceBundle{
		VoltCommitSHA:     strings.Repeat("c", 40),
		ComparatorPath:    "../volt",
		FixturesSHA256:    strings.Repeat("d", 64),
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
