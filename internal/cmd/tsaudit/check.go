package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

const (
	defaultManifestRelPath = "internal/treesitter/languages.json"
	defaultQueriesRelPath  = "internal/treesitter/queries"
)

var errDriftDetected = errors.New("drift detected")

type checkOptions struct {
	repoRoot     string
	manifestPath string
	queriesDir   string
	primaryFile  string
	fallbackFile string
}

type languagesManifest struct {
	AiderCommit string             `json:"aider_commit"`
	Languages   []manifestLanguage `json:"languages"`
}

type manifestLanguage struct {
	Name          string `json:"name"`
	GrammarModule string `json:"grammar_module,omitempty"`
	GrammarRev    string `json:"grammar_rev,omitempty"`
	GrammarDir    string `json:"grammar_dir,omitempty"`
	QuerySource   string `json:"query_source,omitempty"`
}

type languageListFile struct {
	Languages []string `json:"languages"`
}

type driftReport struct {
	MissingInManifest    []string
	UnexpectedInManifest []string
	MissingQueryFiles    []string
	UnexpectedQueryFiles []string
	InvalidAiderCommit   bool
}

func (r driftReport) HasDrift() bool {
	return len(r.MissingInManifest) > 0 ||
		len(r.UnexpectedInManifest) > 0 ||
		len(r.MissingQueryFiles) > 0 ||
		len(r.UnexpectedQueryFiles) > 0 ||
		r.InvalidAiderCommit
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check manifest/query drift",
	Long:  "Check drift between Aider source lists, internal/treesitter/languages.json, and vendored query files.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		report, err := runCheck(loadCheckOptions(cmd))
		if err != nil {
			return err
		}

		if !report.HasDrift() {
			cmd.Println("tsaudit check: no drift detected")
			return nil
		}

		printDrift(cmd, report)
		return errDriftDetected
	},
}

func init() {
	checkCmd.Flags().String("repo-root", ".", "Repository root path")
	checkCmd.Flags().String("manifest", defaultManifestRelPath, "Path to languages.json manifest")
	checkCmd.Flags().String("queries-dir", defaultQueriesRelPath, "Path to vendored query directory")
	checkCmd.Flags().String("primary-file", "", "Path to JSON file with primary (pack) language names")
	checkCmd.Flags().String("fallback-file", "", "Path to JSON file with fallback (langs) language names")

	rootCmd.AddCommand(checkCmd)
}

func loadCheckOptions(cmd *cobra.Command) checkOptions {
	repoRoot, _ := cmd.Flags().GetString("repo-root")
	manifestPath, _ := cmd.Flags().GetString("manifest")
	queriesDir, _ := cmd.Flags().GetString("queries-dir")
	primaryFile, _ := cmd.Flags().GetString("primary-file")
	fallbackFile, _ := cmd.Flags().GetString("fallback-file")

	return checkOptions{
		repoRoot:     repoRoot,
		manifestPath: manifestPath,
		queriesDir:   queriesDir,
		primaryFile:  primaryFile,
		fallbackFile: fallbackFile,
	}
}

func runCheck(opts checkOptions) (driftReport, error) {
	manifestPath := resolvePath(opts.repoRoot, opts.manifestPath)
	queriesDir := resolvePath(opts.repoRoot, opts.queriesDir)

	manifest, err := loadFullManifest(manifestPath)
	if err != nil {
		return driftReport{}, err
	}

	manifestNames, err := loadManifestNames(manifestPath)
	if err != nil {
		return driftReport{}, err
	}

	queryNames, err := loadVendoredQueryNames(queriesDir)
	if err != nil {
		return driftReport{}, err
	}

	primary := map[string]struct{}{}
	fallback := map[string]struct{}{}

	if strings.TrimSpace(opts.primaryFile) != "" {
		primaryPath := resolvePath(opts.repoRoot, opts.primaryFile)
		primary, err = loadLanguageSet(primaryPath)
		if err != nil {
			return driftReport{}, fmt.Errorf("load primary list: %w", err)
		}
	}

	if strings.TrimSpace(opts.fallbackFile) != "" {
		fallbackPath := resolvePath(opts.repoRoot, opts.fallbackFile)
		fallback, err = loadLanguageSet(fallbackPath)
		if err != nil {
			return driftReport{}, fmt.Errorf("load fallback list: %w", err)
		}
	}

	report := compareDrift(manifestNames, queryNames, primary, fallback)

	commit := strings.TrimSpace(manifest.AiderCommit)
	if commit == "" || commit == "unknown" {
		report.InvalidAiderCommit = true
	}

	return report, nil
}

func loadFullManifest(manifestPath string) (languagesManifest, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return languagesManifest{}, fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	var m languagesManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return languagesManifest{}, fmt.Errorf("parse manifest %q: %w", manifestPath, err)
	}

	return m, nil
}

func resolvePath(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	if root == "" {
		return p
	}
	return filepath.Join(root, p)
}

func loadManifestNames(manifestPath string) (map[string]struct{}, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	var m languagesManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %q: %w", manifestPath, err)
	}

	out := make(map[string]struct{}, len(m.Languages))
	for _, lang := range m.Languages {
		name := strings.TrimSpace(lang.Name)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	return out, nil
}

func loadLanguageSet(path string) (map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", path, err)
	}

	var rawList []string
	if err := json.Unmarshal(data, &rawList); err == nil {
		return sliceToSet(rawList), nil
	}

	var wrapper languageListFile
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("parse language list %q: %w", path, err)
	}
	return sliceToSet(wrapper.Languages), nil
}

func loadVendoredQueryNames(queriesDir string) (map[string]struct{}, error) {
	pattern := filepath.Join(queriesDir, "*-tags.scm")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob vendored queries %q: %w", pattern, err)
	}

	out := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		base := filepath.Base(match)
		name := strings.TrimSuffix(base, "-tags.scm")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}

	return out, nil
}

func compareDrift(manifestNames, queryNames, primaryNames, fallbackNames map[string]struct{}) driftReport {
	report := driftReport{}

	expected := expectedNames(primaryNames, fallbackNames)
	if len(expected) > 0 {
		report.MissingInManifest = sortedSetDiff(expected, manifestNames)
		report.UnexpectedInManifest = sortedSetDiff(manifestNames, expected)
	}

	report.MissingQueryFiles = sortedSetDiff(manifestNames, queryNames)
	report.UnexpectedQueryFiles = sortedSetDiff(queryNames, manifestNames)

	return report
}

func expectedNames(primaryNames, fallbackNames map[string]struct{}) map[string]struct{} {
	if len(primaryNames) == 0 && len(fallbackNames) == 0 {
		return nil
	}

	out := make(map[string]struct{}, len(primaryNames)+len(fallbackNames))
	for name := range primaryNames {
		out[name] = struct{}{}
	}
	for name := range fallbackNames {
		if _, exists := primaryNames[name]; exists {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}

func sortedSetDiff(a, b map[string]struct{}) []string {
	if len(a) == 0 {
		return nil
	}
	out := make([]string, 0)
	for name := range a {
		if _, ok := b[name]; !ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func sliceToSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}

func printDrift(cmd *cobra.Command, report driftReport) {
	cmd.Println("tsaudit check: drift detected")

	printList := func(header string, values []string) {
		if len(values) == 0 {
			return
		}
		cmd.Printf("\n%s:\n", header)
		for _, value := range values {
			cmd.Printf("  - %s\n", value)
		}
	}

	if report.InvalidAiderCommit {
		cmd.Println("\naider_commit is empty or \"unknown\" (must be a valid SHA)")
	}

	printList("Missing in manifest (expected from Aider sources)", report.MissingInManifest)
	printList("Unexpected in manifest", report.UnexpectedInManifest)
	printList("Missing vendored query files", report.MissingQueryFiles)
	printList("Unexpected vendored query files", report.UnexpectedQueryFiles)
}
