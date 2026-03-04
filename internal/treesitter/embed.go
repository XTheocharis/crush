package treesitter

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed queries/* languages.json
var queriesFS embed.FS

// LanguagesManifest is the source-of-truth language manifest structure.
type LanguagesManifest struct {
	Generated           string             `json:"generated,omitempty"`
	LanguagePackVersion string             `json:"language_pack_version,omitempty"`
	AiderCommit         string             `json:"aider_commit,omitempty"`
	Languages           []ManifestLanguage `json:"languages"`
}

// ManifestLanguage describes one supported tree-sitter language.
type ManifestLanguage struct {
	Name          string `json:"name"`
	GrammarModule string `json:"grammar_module,omitempty"`
	GrammarRev    string `json:"grammar_rev,omitempty"`
	GrammarDir    string `json:"grammar_dir,omitempty"`
	QuerySource   string `json:"query_source,omitempty"`
}

// LoadLanguagesManifest loads the embedded languages manifest.
func LoadLanguagesManifest() (LanguagesManifest, error) {
	data, err := queriesFS.ReadFile("languages.json")
	if err != nil {
		return LanguagesManifest{}, fmt.Errorf("read embedded languages manifest: %w", err)
	}

	var m LanguagesManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return LanguagesManifest{}, fmt.Errorf("parse embedded languages manifest: %w", err)
	}

	return m, nil
}

// LoadTagsQuery returns embedded tags query content for a language key.
func LoadTagsQuery(lang string) ([]byte, error) {
	name := strings.TrimSpace(lang)
	if name == "" {
		return nil, fmt.Errorf("language key is empty")
	}
	return queriesFS.ReadFile("queries/" + name + "-tags.scm")
}

// HasTagsQuery reports whether an embedded tags query exists for a language key.
func HasTagsQuery(lang string) bool {
	_, err := LoadTagsQuery(lang)
	return err == nil
}
