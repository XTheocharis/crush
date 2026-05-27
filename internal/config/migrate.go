package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MigrateJSONToYAML reads a crush.json file, converts it to the .xrush/config.yml
// YAML format, and writes it to yamlPath. Parent directories are created as
// needed. The original JSON file is not modified.
func MigrateJSONToYAML(jsonPath, yamlPath string) error {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON config %s: %w", jsonPath, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse JSON config %s: %w", jsonPath, err)
	}

	dc := fromConfig(&cfg)

	if err := os.MkdirAll(filepath.Dir(yamlPath), 0o755); err != nil {
		return fmt.Errorf("failed to create YAML config directory: %w", err)
	}

	return WriteYAMLConfig(yamlPath, dc)
}

// MigrateJSONBytesToYAML converts JSON config bytes directly to YAML bytes.
// This is useful for testing and in-memory conversion without touching the
// filesystem.
func MigrateJSONBytesToYAML(jsonData []byte) ([]byte, error) {
	var cfg Config
	if err := json.Unmarshal(jsonData, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse JSON config: %w", err)
	}

	dc := fromConfig(&cfg)
	return dc.MarshalYAML()
}
