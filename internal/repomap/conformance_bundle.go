package repomap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	explorer "github.com/charmbracelet/crush/internal/lcm/explorer"
)

// SignOffBundle captures an atomic Phase 5 parity sign-off snapshot from one run.
type SignOffBundle struct {
	Version      string                       `json:"version"`
	GeneratedAt  string                       `json:"generated_at"`
	RepoMap      ConformanceSnapshot          `json:"repo_map"`
	Explorer     explorer.ConformanceSnapshot `json:"explorer"`
	GateAPassed  bool                         `json:"gate_a_passed"`
	GateBPassed  bool                         `json:"gate_b_passed"`
	Phase5Passed bool                         `json:"phase5_passed"`
}

// BuildSignOffBundle builds a single-run atomic sign-off bundle from both gate
// families (A/B) with provenance, protocol versions, toggles, and outcomes.
func BuildSignOffBundle(basePath string) (*SignOffBundle, error) {
	if strings.TrimSpace(basePath) == "" {
		basePath = "."
	}

	repoSnap, err := BuildConformanceSnapshot(basePath)
	if err != nil {
		return nil, fmt.Errorf("build repomap conformance snapshot: %w", err)
	}

	explorerBasePath := resolveExplorerConformanceBasePath(basePath)
	explorerSnap, err := explorer.BuildConformanceSnapshot(explorerBasePath)
	if err != nil {
		return nil, fmt.Errorf("build explorer conformance snapshot: %w", err)
	}

	bundle := &SignOffBundle{
		Version:      "1",
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		RepoMap:      *repoSnap,
		Explorer:     *explorerSnap,
		GateAPassed:  repoSnap.GateAPassed,
		GateBPassed:  explorerSnap.GateBPassed,
		Phase5Passed: repoSnap.GateAPassed && explorerSnap.GateBPassed,
	}
	if err := ValidateSignOffBundle(bundle); err != nil {
		return nil, err
	}
	return bundle, nil
}

// ValidateSignOffBundle validates required fields and deterministic parity
// toggles for an atomic sign-off bundle.
func ValidateSignOffBundle(bundle *SignOffBundle) error {
	if bundle == nil {
		return fmt.Errorf("sign-off bundle is nil")
	}
	if strings.TrimSpace(bundle.Version) == "" {
		return fmt.Errorf("sign-off bundle missing version")
	}
	if strings.TrimSpace(bundle.GeneratedAt) == "" {
		return fmt.Errorf("sign-off bundle missing generated_at")
	}

	if !bundle.GateAPassed || !bundle.RepoMap.GateAPassed {
		return fmt.Errorf("sign-off bundle Gate A failed")
	}
	if !bundle.GateBPassed || !bundle.Explorer.GateBPassed {
		return fmt.Errorf("sign-off bundle Gate B failed")
	}
	if !bundle.Phase5Passed {
		return fmt.Errorf("sign-off bundle phase5_passed must be true")
	}

	if strings.TrimSpace(bundle.RepoMap.AiderCommitSHA) == "" || strings.TrimSpace(bundle.Explorer.VoltCommitSHA) == "" {
		return fmt.Errorf("sign-off bundle missing comparator commit shas")
	}
	if strings.TrimSpace(bundle.RepoMap.FixturesSHA256) == "" || strings.TrimSpace(bundle.Explorer.FixturesSHA256) == "" {
		return fmt.Errorf("sign-off bundle missing fixture hashes")
	}
	if strings.TrimSpace(bundle.RepoMap.ComparatorPath) == "" || strings.TrimSpace(bundle.Explorer.ComparatorPath) == "" {
		return fmt.Errorf("sign-off bundle missing comparator paths")
	}

	if !bundle.RepoMap.DeterministicMode || strings.ToLower(strings.TrimSpace(bundle.RepoMap.EnhancementTiersEnabled)) != "none" {
		return fmt.Errorf("sign-off bundle invalid repomap deterministic toggles")
	}
	if !bundle.Explorer.DeterministicMode || strings.ToLower(strings.TrimSpace(bundle.Explorer.EnhancementTiersEnabled)) != "none" {
		return fmt.Errorf("sign-off bundle invalid explorer deterministic toggles")
	}
	if !isAllowedCounterMode(bundle.RepoMap.TokenCounterMode) || !isAllowedCounterMode(bundle.Explorer.TokenCounterMode) {
		return fmt.Errorf("sign-off bundle invalid token counter mode")
	}
	if bundle.RepoMap.FixedSeed <= 0 || bundle.Explorer.FixedSeed <= 0 {
		return fmt.Errorf("sign-off bundle fixed_seed must be positive")
	}

	return nil
}

func isAllowedCounterMode(mode string) bool {
	mode = strings.ToLower(strings.TrimSpace(mode))
	return mode == "tokenizer_backed" || mode == "heuristic"
}

func resolveExplorerConformanceBasePath(basePath string) string {
	candidates := []string{
		basePath,
		filepath.Join(basePath, "internal", "lcm", "explorer"),
		filepath.Join(basePath, "..", "lcm", "explorer"),
		filepath.Join(basePath, "..", "..", "internal", "lcm", "explorer"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "testdata", "parity_volt", "fixture_index.v1.json")); err == nil {
			return candidate
		}
	}
	return basePath
}

// WriteSignOffBundleManifest writes the sign-off bundle to a JSON file.
func WriteSignOffBundleManifest(filePath string, bundle *SignOffBundle) error {
	if err := ValidateSignOffBundle(bundle); err != nil {
		return err
	}
	content, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sign-off bundle: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return fmt.Errorf("create sign-off bundle directory: %w", err)
	}
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		return fmt.Errorf("write sign-off bundle manifest: %w", err)
	}
	return nil
}

// LoadSignOffBundleManifest loads and validates a sign-off bundle manifest.
func LoadSignOffBundleManifest(filePath string) (*SignOffBundle, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read sign-off bundle manifest: %w", err)
	}
	var bundle SignOffBundle
	if err := json.Unmarshal(content, &bundle); err != nil {
		return nil, fmt.Errorf("unmarshal sign-off bundle manifest: %w", err)
	}
	if err := ValidateSignOffBundle(&bundle); err != nil {
		return nil, err
	}
	return &bundle, nil
}
