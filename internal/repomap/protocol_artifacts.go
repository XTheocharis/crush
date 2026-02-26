package repomap

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	// TokenizerSupportPath is the path to the tokenizer support artifact.
	TokenizerSupportPath = "testdata/parity_aider/tokenizer_support.v1.json"

	// ExplorerFamilyMatrixPath is the path to the explorer family matrix artifact.
	ExplorerFamilyMatrixPath = "testdata/parity_aider/explorer_family_matrix.v1.json"
)

// TokenizerFamily defines a model family and its tokenizer configuration.
type TokenizerFamily struct {
	ModelFamily      string   `json:"model_family"`
	TokenizerID      string   `json:"tokenizer_id"`
	TokenizerVersion string   `json:"tokenizer_version"`
	Supported        bool     `json:"supported"`
	Models           []string `json:"models"`
}

// TokenizerSupport is the tokenizer support matrix artifact.
type TokenizerSupport struct {
	Version           string            `json:"version"`
	Description       string            `json:"description"`
	GeneratedAt       string            `json:"generated_at"`
	SupportedFamilies []TokenizerFamily `json:"supported_families"`
}

// ExplorerInfo defines information about an explorer in the family matrix.
type ExplorerInfo struct {
	ExplorerID          string   `json:"explorer_id"`
	ExplorerType        string   `json:"explorer_type"`
	SupportedExtensions []string `json:"supported_extensions"`
	LanguageFamilies    []string `json:"language_families"`
	ModelSupport        string   `json:"model_support"`
	Description         string   `json:"description"`
}

// ExplorerFamilyMatrix is the explorer family matrix artifact.
type ExplorerFamilyMatrix struct {
	Version     string         `json:"version"`
	Description string         `json:"description"`
	GeneratedAt string         `json:"generated_at"`
	Explorers   []ExplorerInfo `json:"explorers"`
}

// ProtocolArtifact encapsulates a versioned protocol artifact.
type ProtocolArtifact interface {
	ValidateVersion() error
	ValidateRequiredFields() error
}

// ValidateVersion ensures the artifact has a valid version field.
func (ts *TokenizerSupport) ValidateVersion() error {
	if ts.Version == "" {
		return fmt.Errorf("tokenizer support: missing version field")
	}
	if !strings.HasPrefix(ts.Version, "1") {
		return fmt.Errorf("tokenizer support: unsupported version %s, expected version 1", ts.Version)
	}
	return nil
}

// ValidateRequiredFields ensures the tokenizer support has all required fields.
func (ts *TokenizerSupport) ValidateRequiredFields() error {
	if err := ts.ValidateVersion(); err != nil {
		return err
	}
	if ts.Description == "" {
		return fmt.Errorf("tokenizer support: missing description field")
	}
	if ts.GeneratedAt == "" {
		return fmt.Errorf("tokenizer support: missing generated_at field")
	}
	if len(ts.SupportedFamilies) == 0 {
		return fmt.Errorf("tokenizer support: supported_families array must not be empty")
	}
	for i, fam := range ts.SupportedFamilies {
		if fam.ModelFamily == "" {
			return fmt.Errorf("tokenizer support: supported_families[%d]: missing model_family", i)
		}
		if fam.TokenizerID == "" {
			return fmt.Errorf("tokenizer support: supported_families[%d]: missing tokenizer_id", i)
		}
		if fam.TokenizerVersion == "" {
			return fmt.Errorf("tokenizer support: supported_families[%d]: missing tokenizer_version", i)
		}
	}
	return nil
}

// ValidateVersion ensures the explorer family matrix has a valid version field.
func (efm *ExplorerFamilyMatrix) ValidateVersion() error {
	if efm.Version == "" {
		return fmt.Errorf("explorer family matrix: missing version field")
	}
	if !strings.HasPrefix(efm.Version, "1") {
		return fmt.Errorf("explorer family matrix: unsupported version %s, expected version 1", efm.Version)
	}
	return nil
}

// ValidateRequiredFields ensures the explorer family matrix has all required fields.
func (efm *ExplorerFamilyMatrix) ValidateRequiredFields() error {
	if err := efm.ValidateVersion(); err != nil {
		return err
	}
	if efm.Description == "" {
		return fmt.Errorf("explorer family matrix: missing description field")
	}
	if efm.GeneratedAt == "" {
		return fmt.Errorf("explorer family matrix: missing generated_at field")
	}
	if len(efm.Explorers) == 0 {
		return fmt.Errorf("explorer family matrix: explorers array must not be empty")
	}
	for i, exp := range efm.Explorers {
		if exp.ExplorerID == "" {
			return fmt.Errorf("explorer family matrix: explorers[%d]: missing explorer_id", i)
		}
		if exp.ExplorerType == "" {
			return fmt.Errorf("explorer family matrix: explorers[%d]: missing explorer_type", i)
		}
		if exp.Description == "" {
			return fmt.Errorf("explorer family matrix: explorers[%d]: missing description", i)
		}
	}
	return nil
}

// LoadTokenizerSupport loads the tokenizer support artifact from disk.
func LoadTokenizerSupport() (*TokenizerSupport, error) {
	content, err := os.ReadFile(TokenizerSupportPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read tokenizer support artifact: %w", err)
	}

	var ts TokenizerSupport
	if err := json.Unmarshal(content, &ts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tokenizer support artifact: %w", err)
	}

	return &ts, nil
}

// LoadExplorerFamilyMatrix loads the explorer family matrix artifact from disk.
func LoadExplorerFamilyMatrix() (*ExplorerFamilyMatrix, error) {
	content, err := os.ReadFile(ExplorerFamilyMatrixPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read explorer family matrix artifact: %w", err)
	}

	var efm ExplorerFamilyMatrix
	if err := json.Unmarshal(content, &efm); err != nil {
		return nil, fmt.Errorf("failed to unmarshal explorer family matrix artifact: %w", err)
	}

	return &efm, nil
}

// ValidateProtocolArtifact validates any protocol artifact.
func ValidateProtocolArtifact(artifact ProtocolArtifact) error {
	if err := artifact.ValidateVersion(); err != nil {
		return err
	}
	if err := artifact.ValidateRequiredFields(); err != nil {
		return err
	}
	return nil
}

// ValidateTokenizerSupportArtifact validates the tokenizer support artifact.
func ValidateTokenizerSupportArtifact() error {
	ts, err := LoadTokenizerSupport()
	if err != nil {
		return fmt.Errorf("failed to load tokenizer support: %w", err)
	}
	return ts.ValidateRequiredFields()
}

// ValidateExplorerFamilyMatrixArtifact validates the explorer family matrix artifact.
func ValidateExplorerFamilyMatrixArtifact() error {
	efm, err := LoadExplorerFamilyMatrix()
	if err != nil {
		return fmt.Errorf("failed to load explorer family matrix: %w", err)
	}
	return efm.ValidateRequiredFields()
}

// ValidateAllProtocolArtifacts validates all protocol artifacts in the parity testdata.
func ValidateAllProtocolArtifacts() error {
	if err := ValidateTokenizerSupportArtifact(); err != nil {
		return fmt.Errorf("tokenizer support validation failed: %w", err)
	}
	if err := ValidateExplorerFamilyMatrixArtifact(); err != nil {
		return fmt.Errorf("explorer family matrix validation failed: %w", err)
	}
	return nil
}

// GetSupportedTokenizers returns a map of model family to tokenizer ID.
func (ts *TokenizerSupport) GetSupportedTokenizers() map[string]string {
	result := make(map[string]string)
	for _, fam := range ts.SupportedFamilies {
		if fam.Supported {
			result[fam.ModelFamily] = fam.TokenizerID
		}
	}
	return result
}

// GetModelsForFamily returns all models in a given family.
func (ts *TokenizerSupport) GetModelsForFamily(family string) []string {
	for _, fam := range ts.SupportedFamilies {
		if fam.ModelFamily == family {
			return fam.Models
		}
	}
	return nil
}

// GetExplorerByID finds an explorer by its ID.
func (efm *ExplorerFamilyMatrix) GetExplorerByID(id string) *ExplorerInfo {
	for i := range efm.Explorers {
		if efm.Explorers[i].ExplorerID == id {
			return &efm.Explorers[i]
		}
	}
	return nil
}

// GetExplorersByType returns all explorers of a given type.
func (efm *ExplorerFamilyMatrix) GetExplorersByType(expType string) []ExplorerInfo {
	result := []ExplorerInfo{}
	for _, exp := range efm.Explorers {
		if exp.ExplorerType == expType {
			result = append(result, exp)
		}
	}
	return result
}

var rfc3339Pattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`)

// ValidateGeneratedAt validates that generated_at is a valid timestamp.
func ValidateGeneratedAt(value string) error {
	if value == "" {
		return fmt.Errorf("generated_at cannot be empty")
	}
	if !rfc3339Pattern.MatchString(value) {
		return fmt.Errorf("generated_at must be in RFC3339 format, got: %s", value)
	}
	return nil
}
