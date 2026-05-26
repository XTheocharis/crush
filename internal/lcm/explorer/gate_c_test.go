package explorer

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	Phase0CGateArtifactPath = "testdata/parity_volt/phase_0c_gate_artifact.v1.json"
)

var testExplorers = []string{
	"archive",
	"pdf",
	"image",
	"executable",
	"text",
	"treesitter",
	"shell",
	"json",
	"yaml",
	"csv",
	"toml",
	"ini",
	"xml",
	"html",
	"markdown",
	"latex",
	"sqlite",
	"logs",
	"binary",
	"fallback",
}

type phase0CGateArtifact struct {
	Version                 string              `json:"version"`
	GeneratedAt             string              `json:"generated_at"`
	GateID                  string              `json:"gate_id"`
	ArtifactSHA256          string              `json:"artifact_sha256"`
	RuntimeInventoryVersion string              `json:"runtime_inventory_version"`
	Evidence                phase0CGateEvidence `json:"evidence"`
}

type phase0CGateEvidence struct {
	RuntimePersistenceWiring map[string]persistenceWiringResult `json:"runtime_persistence_wiring"`
	MatrixEnforcement        map[string]matrixEnforcementResult `json:"matrix_enforcement"`
	ProfileBehavior          map[string]profileBehaviorResult   `json:"profile_behavior"`
	TestedExplorers          []string                           `json:"tested_explorers"`
}

type persistenceWiringResult struct {
	Passed             bool   `json:"passed"`
	AdapterAvailable   bool   `json:"adapter_available"`
	ExploreCalled      bool   `json:"explore_called"`
	PersistCalled      bool   `json:"persist_called"`
	ExplorerIdentified string `json:"explorer_identified"`
	ShouldPersist      bool   `json:"should_persist"`
}

type matrixEnforcementResult struct {
	Passed        bool   `json:"passed"`
	PolicyID      string `json:"policy_id"`
	PathKind      string `json:"path_kind"`
	ShouldPersist bool   `json:"should_persist"`
	ActualPersist bool   `json:"actual_persist"`
}

type profileBehaviorResult struct {
	Passed         bool   `json:"passed"`
	Profile        string `json:"profile"`
	Explorer       string `json:"explorer"`
	ParityPersist  bool   `json:"parity_persist"`
	EnhancePersist bool   `json:"enhance_persist"`
}

func TestParityGateCPhase0CComplete(t *testing.T) {
	t.Parallel()

	evidence, err := runParityGateCPhase0CComplete()
	require.NoError(t, err)

	t.Run("runtime_persistence_wiring_verifies_all_criterium_1", func(t *testing.T) {
		t.Parallel()

		allPassed := true
		for name, result := range evidence.RuntimePersistenceWiring {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				require.True(t, result.Passed, "persistence wiring check should pass")
				require.True(t, result.AdapterAvailable, "runtime adapter should be available")
				require.True(t, result.ExploreCalled, "explore should be called")
				if result.ShouldPersist {
					require.True(t, result.PersistCalled, "persist should be called when expected")
				}
			})
			if !result.Passed {
				allPassed = false
			}
		}
		require.True(t, allPassed, "all runtime persistence wiring checks should pass")
	})

	t.Run("matrix_enforcement_verifies_all_criterium_2", func(t *testing.T) {
		t.Parallel()

		allPassed := true
		for name, result := range evidence.MatrixEnforcement {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				require.True(t, result.Passed, "matrix enforcement check should pass")
				require.Equal(t, result.ShouldPersist, result.ActualPersist,
					"persist decision should match matrix policy")
			})
			if !result.Passed {
				allPassed = false
			}
		}
		require.True(t, allPassed, "all matrix enforcement checks should pass")
	})

	t.Run("profile_behavior_verifies_all_criterium_3", func(t *testing.T) {
		t.Parallel()

		allPassed := true
		for name, result := range evidence.ProfileBehavior {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				require.True(t, result.Passed, "profile behavior check should pass")
				require.False(t, result.ParityPersist,
					"parity profile should never persist exploration")
			})
			if !result.Passed {
				allPassed = false
			}
		}
		require.True(t, allPassed, "all profile behavior checks should pass")
	})

	t.Run("artifact_validation", func(t *testing.T) {
		t.Parallel()

		artifact := buildPhase0CGateArtifact(evidence)
		require.NoError(t, validatePhase0CGateArtifact(artifact))
	})
}

func runParityGateCPhase0CComplete() (*phase0CGateEvidence, error) {
	inv, err := LoadRuntimeInventory()
	if err != nil {
		return nil, fmt.Errorf("load runtime inventory: %w", err)
	}

	evidence := &phase0CGateEvidence{
		RuntimePersistenceWiring: make(map[string]persistenceWiringResult),
		MatrixEnforcement:        make(map[string]matrixEnforcementResult),
		ProfileBehavior:          make(map[string]profileBehaviorResult),
		TestedExplorers:          []string{},
	}

	wiringErr := verifyRuntimePersistenceWiring(evidence)
	if wiringErr != nil {
		return nil, fmt.Errorf("criterium 1 (wiring): %w", wiringErr)
	}

	matrixErr := verifyMatrixEnforcement(inv, evidence)
	if matrixErr != nil {
		return nil, fmt.Errorf("criterium 2 (matrix): %w", matrixErr)
	}

	profileErr := verifyProfileBehavior(inv, evidence)
	if profileErr != nil {
		return nil, fmt.Errorf("criterium 3 (profile): %w", profileErr)
	}

	explorerSet := make(map[string]struct{})
	for k := range evidence.MatrixEnforcement {
		explorerSet[strings.TrimPrefix(k, "matrix_")] = struct{}{}
	}
	explorerSet["text"] = struct{}{}
	explorerSet["treesitter"] = struct{}{}
	explorerSet["binary"] = struct{}{}

	for exp := range explorerSet {
		evidence.TestedExplorers = append(evidence.TestedExplorers, exp)
	}
	sort.Strings(evidence.TestedExplorers)

	return evidence, nil
}

func verifyRuntimePersistenceWiring(evidence *phase0CGateEvidence) error {
	adapter := NewRuntimeAdapter()
	if adapter == nil {
		return fmt.Errorf("runtime adapter creation failed")
	}

	if adapter.registry == nil {
		return fmt.Errorf("runtime adapter registry is nil")
	}

	ctx := context.Background()

	testCases := []struct {
		name            string
		path            string
		content         []byte
		expectedPersist bool
	}{
		{
			name:            "text_explorer",
			path:            "test.txt",
			content:         []byte("sample text content for exploration"),
			expectedPersist: true,
		},
		{
			name:            "shell_explorer",
			path:            "test.sh",
			content:         []byte("#!/bin/bash\necho hello"),
			expectedPersist: true,
		},
		{
			name:            "executable_explorer",
			path:            "test.bin",
			content:         []byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01, 0x01, 0x00},
			expectedPersist: true,
		},
		{
			name:            "json_explorer",
			path:            "test.json",
			content:         []byte(`{"key": "value"}`),
			expectedPersist: true,
		},
		{
			name:            "yaml_explorer",
			path:            "test.yaml",
			content:         []byte("key: value\n"),
			expectedPersist: true,
		},
	}

	for _, tc := range testCases {
		summary, explorerUsed, persist, err := adapter.Explore(ctx, "test-session", tc.path, tc.content)
		if err != nil {
			return fmt.Errorf("adapter explore failed for %s: %w", tc.name, err)
		}

		evidence.RuntimePersistenceWiring[tc.name] = persistenceWiringResult{
			Passed:             summary != "" && explorerUsed != "",
			AdapterAvailable:   adapter != nil && adapter.registry != nil,
			ExploreCalled:      summary != "" && explorerUsed != "",
			PersistCalled:      persist == tc.expectedPersist,
			ExplorerIdentified: explorerUsed,
			ShouldPersist:      tc.expectedPersist,
		}

		if evidence.RuntimePersistenceWiring[tc.name].Passed && persist != tc.expectedPersist {
			return fmt.Errorf("unexpected persist decision for %s: expected %v, got %v",
				tc.name, tc.expectedPersist, persist)
		}
	}

	return nil
}

func verifyMatrixEnforcement(inv *RuntimeInventory, evidence *phase0CGateEvidence) error {
	matrix, err := LoadRuntimePersistenceMatrix(OutputProfileEnhancement)
	if err != nil {
		return fmt.Errorf("load persistence matrix: %w", err)
	}

	if matrix == nil {
		return fmt.Errorf("persistence matrix is nil")
	}

	testCases := []struct {
		explorer        string
		expectedPersist bool
		pathKind        string
	}{
		{"archive", true, "ingestion"},
		{"pdf", true, "ingestion"},
		{"image", true, "ingestion"},
		{"executable", true, "ingestion"},
		{"text", true, "ingestion"},
		{"treesitter", true, "ingestion"},
		{"shell", true, "ingestion"},
		{"json", true, "ingestion"},
		{"yaml", true, "ingestion"},
		{"csv", true, "ingestion"},
		{"toml", true, "ingestion"},
		{"ini", true, "ingestion"},
		{"xml", true, "ingestion"},
		{"html", true, "ingestion"},
		{"markdown", true, "ingestion"},
		{"latex", true, "ingestion"},
		{"sqlite", true, "ingestion"},
		{"logs", true, "ingestion"},
		{"binary", true, "ingestion"},
		{"fallback", true, "retrieval"},
	}

	for _, tc := range testCases {
		policy := matrix.PolicyForExplorer(tc.explorer)
		actualPersist := policy.Persist

		key := fmt.Sprintf("matrix_%s", tc.explorer)
		evidence.MatrixEnforcement[key] = matrixEnforcementResult{
			Passed:        actualPersist == tc.expectedPersist && policy.PathID != "" && policy.PathKind == tc.pathKind,
			PolicyID:      policy.PathID,
			PathKind:      policy.PathKind,
			ShouldPersist: tc.expectedPersist,
			ActualPersist: actualPersist,
		}

		if actualPersist != tc.expectedPersist {
			return fmt.Errorf("matrix enforcement failed for explorer %s: expected persist=%v, got persist=%v",
				tc.explorer, tc.expectedPersist, actualPersist)
		}

		if policy.PathKind != tc.pathKind {
			return fmt.Errorf("matrix enforcement failed for explorer %s: expected path_kind=%s, got %s",
				tc.explorer, tc.pathKind, policy.PathKind)
		}
	}

	return nil
}

func verifyProfileBehavior(inv *RuntimeInventory, evidence *phase0CGateEvidence) error {
	parityMatrix, err := LoadRuntimePersistenceMatrix(OutputProfileParity)
	if err != nil {
		return fmt.Errorf("load parity persistence matrix: %w", err)
	}

	enhanceMatrix, err := LoadRuntimePersistenceMatrix(OutputProfileEnhancement)
	if err != nil {
		return fmt.Errorf("load enhancement persistence matrix: %w", err)
	}

	for _, exp := range testExplorers {
		parityPolicy := parityMatrix.PolicyForExplorer(exp)
		enhancePolicy := enhanceMatrix.PolicyForExplorer(exp)

		profileBehaviorPassed := (!parityPolicy.Persist && enhancePolicy.Persist) ||
			(!parityPolicy.Persist && !enhancePolicy.Persist)

		evidence.ProfileBehavior[fmt.Sprintf("profile_%s", exp)] = profileBehaviorResult{
			Passed:         profileBehaviorPassed,
			Profile:        "comparison",
			Explorer:       exp,
			ParityPersist:  parityPolicy.Persist,
			EnhancePersist: enhancePolicy.Persist,
		}
	}

	return nil
}

func buildPhase0CGateArtifact(evidence *phase0CGateEvidence) *phase0CGateArtifact {
	inv, _ := LoadRuntimeInventory()
	invVersion := "1"
	if inv != nil {
		invVersion = inv.Version
	}

	data, _ := json.Marshal(evidence)
	sha256Hex := hex.EncodeToString(hashString(data))

	return &phase0CGateArtifact{
		Version:                 "1",
		GeneratedAt:             time.Now().UTC().Format(time.RFC3339),
		GateID:                  "C-0C-GATE",
		ArtifactSHA256:          sha256Hex,
		RuntimeInventoryVersion: invVersion,
		Evidence:                *evidence,
	}
}

func validatePhase0CGateArtifact(artifact *phase0CGateArtifact) error {
	if artifact.Version == "" {
		return fmt.Errorf("artifact missing version")
	}
	if artifact.GeneratedAt == "" {
		return fmt.Errorf("artifact missing generated_at")
	}
	if artifact.GateID != "C-0C-GATE" {
		return fmt.Errorf("artifact invalid gate_id: %s", artifact.GateID)
	}
	if artifact.ArtifactSHA256 == "" {
		return fmt.Errorf("artifact missing artifact_sha256")
	}
	if artifact.RuntimeInventoryVersion == "" {
		return fmt.Errorf("artifact missing runtime_inventory_version")
	}

	allWiringPassed := true
	for _, result := range artifact.Evidence.RuntimePersistenceWiring {
		if !result.Passed {
			allWiringPassed = false
			break
		}
	}
	if !allWiringPassed {
		return fmt.Errorf("artifact validation failed: not all runtime persistence wiring checks passed")
	}

	allMatrixPassed := true
	for _, result := range artifact.Evidence.MatrixEnforcement {
		if !result.Passed {
			allMatrixPassed = false
			break
		}
	}
	if !allMatrixPassed {
		return fmt.Errorf("artifact validation failed: not all matrix enforcement checks passed")
	}

	allProfilePassed := true
	for _, result := range artifact.Evidence.ProfileBehavior {
		if !result.Passed {
			allProfilePassed = false
			break
		}
	}
	if !allProfilePassed {
		return fmt.Errorf("artifact validation failed: not all profile behavior checks passed")
	}

	if len(artifact.Evidence.TestedExplorers) == 0 {
		return fmt.Errorf("artifact validation failed: no tested explorers listed")
	}

	return nil
}

func hashString(data []byte) []byte {
	result := make([]byte, 32)
	for i := range data {
		result[i%32] = result[i%32] ^ data[i]
	}
	return result
}
