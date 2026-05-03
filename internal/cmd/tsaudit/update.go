package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type updateOptions struct {
	repoRoot           string
	manifestPath       string
	queriesDir         string
	primaryFile        string
	fallbackFile       string
	primaryQueriesDir  string
	fallbackQueriesDir string
	dryRun             bool
}

type updateReport struct {
	ManifestAdded   []string
	ManifestRemoved []string
	QueriesAdded    []string
	QueriesRemoved  []string
	QueriesUpdated  []string
}

func (r updateReport) HasChanges() bool {
	return len(r.ManifestAdded) > 0 ||
		len(r.ManifestRemoved) > 0 ||
		len(r.QueriesAdded) > 0 ||
		len(r.QueriesRemoved) > 0 ||
		len(r.QueriesUpdated) > 0
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update manifest and vendored queries",
	Long:  "Update internal/treesitter/languages.json and vendored tags queries from primary/fallback sources.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		report, err := runUpdate(loadUpdateOptions(cmd))
		if err != nil {
			return err
		}

		if !report.HasChanges() {
			cmd.Println("tsaudit update: no changes")
			return nil
		}

		printUpdate(cmd, report, true)
		return nil
	},
}

func init() {
	updateCmd.Flags().String("repo-root", ".", "Repository root path")
	updateCmd.Flags().String("manifest", defaultManifestRelPath, "Path to languages.json manifest")
	updateCmd.Flags().String("queries-dir", defaultQueriesRelPath, "Path to vendored query directory")
	updateCmd.Flags().String("primary-file", "", "Path to JSON file with primary (pack) language names")
	updateCmd.Flags().String("fallback-file", "", "Path to JSON file with fallback (langs) language names")
	updateCmd.Flags().String("primary-queries-dir", "", "Path to primary source directory containing *-tags.scm files")
	updateCmd.Flags().String("fallback-queries-dir", "", "Path to fallback source directory containing *-tags.scm files")
	updateCmd.Flags().Bool("dry-run", false, "Show planned changes without writing files")

	rootCmd.AddCommand(updateCmd)
}

func loadUpdateOptions(cmd *cobra.Command) updateOptions {
	repoRoot, _ := cmd.Flags().GetString("repo-root")
	manifestPath, _ := cmd.Flags().GetString("manifest")
	queriesDir, _ := cmd.Flags().GetString("queries-dir")
	primaryFile, _ := cmd.Flags().GetString("primary-file")
	fallbackFile, _ := cmd.Flags().GetString("fallback-file")
	primaryQueriesDir, _ := cmd.Flags().GetString("primary-queries-dir")
	fallbackQueriesDir, _ := cmd.Flags().GetString("fallback-queries-dir")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	return updateOptions{
		repoRoot:           repoRoot,
		manifestPath:       manifestPath,
		queriesDir:         queriesDir,
		primaryFile:        primaryFile,
		fallbackFile:       fallbackFile,
		primaryQueriesDir:  primaryQueriesDir,
		fallbackQueriesDir: fallbackQueriesDir,
		dryRun:             dryRun,
	}
}

func runUpdate(opts updateOptions) (updateReport, error) {
	targetNames, err := loadExpectedNames(opts)
	if err != nil {
		return updateReport{}, err
	}
	if len(targetNames) == 0 {
		return updateReport{}, errors.New("no languages loaded from primary/fallback files")
	}

	manifestPath := resolvePath(opts.repoRoot, opts.manifestPath)
	queriesDir := resolvePath(opts.repoRoot, opts.queriesDir)
	primaryQueriesDir := resolvePath(opts.repoRoot, opts.primaryQueriesDir)
	fallbackQueriesDir := resolvePath(opts.repoRoot, opts.fallbackQueriesDir)

	existingManifestNames, err := loadManifestNamesIfExists(manifestPath)
	if err != nil {
		return updateReport{}, err
	}
	existingQueryNames, err := loadVendoredQueryNamesIfExists(queriesDir)
	if err != nil {
		return updateReport{}, err
	}

	report := updateReport{
		ManifestAdded:   sortedSetDiff(targetNames, existingManifestNames),
		ManifestRemoved: sortedSetDiff(existingManifestNames, targetNames),
		QueriesAdded:    sortedSetDiff(targetNames, existingQueryNames),
		QueriesRemoved:  sortedSetDiff(existingQueryNames, targetNames),
	}

	sources, err := collectQuerySources(primaryQueriesDir, fallbackQueriesDir)
	if err != nil {
		return updateReport{}, err
	}

	for name := range targetNames {
		if _, existed := existingQueryNames[name]; !existed {
			continue
		}
		sourcePath, hasSource := sources[name]
		if !hasSource {
			continue
		}
		dst := filepath.Join(queriesDir, name+"-tags.scm")
		changed, cmpErr := fileContentDiffers(sourcePath, dst)
		if cmpErr != nil {
			return updateReport{}, cmpErr
		}
		if changed {
			report.QueriesUpdated = append(report.QueriesUpdated, name)
		}
	}
	sort.Strings(report.QueriesUpdated)

	if opts.dryRun {
		return report, nil
	}

	if err := writeManifestWithTargetNames(manifestPath, targetNames); err != nil {
		return updateReport{}, err
	}

	if err := os.MkdirAll(queriesDir, 0o755); err != nil {
		return updateReport{}, fmt.Errorf("create queries directory %q: %w", queriesDir, err)
	}

	for _, name := range report.QueriesRemoved {
		path := filepath.Join(queriesDir, name+"-tags.scm")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return updateReport{}, fmt.Errorf("remove query file %q: %w", path, err)
		}
	}

	for name := range targetNames {
		sourcePath, ok := sources[name]
		if !ok {
			continue
		}
		dst := filepath.Join(queriesDir, name+"-tags.scm")
		if err := copyFile(sourcePath, dst); err != nil {
			return updateReport{}, err
		}
	}

	return report, nil
}

func loadExpectedNames(opts updateOptions) (map[string]struct{}, error) {
	primary := map[string]struct{}{}
	fallback := map[string]struct{}{}
	var err error

	if strings.TrimSpace(opts.primaryFile) != "" {
		primaryPath := resolvePath(opts.repoRoot, opts.primaryFile)
		primary, err = loadLanguageSet(primaryPath)
		if err != nil {
			return nil, fmt.Errorf("load primary list: %w", err)
		}
	}

	if strings.TrimSpace(opts.fallbackFile) != "" {
		fallbackPath := resolvePath(opts.repoRoot, opts.fallbackFile)
		fallback, err = loadLanguageSet(fallbackPath)
		if err != nil {
			return nil, fmt.Errorf("load fallback list: %w", err)
		}
	}

	return expectedNames(primary, fallback), nil
}

func loadManifestNamesIfExists(manifestPath string) (map[string]struct{}, error) {
	if _, err := os.Stat(manifestPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]struct{}{}, nil
		}
		return nil, fmt.Errorf("stat manifest %q: %w", manifestPath, err)
	}
	return loadManifestNames(manifestPath)
}

func loadVendoredQueryNamesIfExists(queriesDir string) (map[string]struct{}, error) {
	if _, err := os.Stat(queriesDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]struct{}{}, nil
		}
		return nil, fmt.Errorf("stat queries dir %q: %w", queriesDir, err)
	}
	return loadVendoredQueryNames(queriesDir)
}

func collectQuerySources(primaryDir, fallbackDir string) (map[string]string, error) {
	sources := map[string]string{}

	if strings.TrimSpace(primaryDir) != "" {
		primary, err := loadQuerySourceMap(primaryDir)
		if err != nil {
			return nil, err
		}
		maps.Copy(sources, primary)
	}

	if strings.TrimSpace(fallbackDir) != "" {
		fallback, err := loadQuerySourceMap(fallbackDir)
		if err != nil {
			return nil, err
		}
		for name, path := range fallback {
			if _, exists := sources[name]; exists {
				continue
			}
			sources[name] = path
		}
	}

	return sources, nil
}

func loadQuerySourceMap(dir string) (map[string]string, error) {
	pattern := filepath.Join(dir, "*-tags.scm")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob query source files %q: %w", pattern, err)
	}
	out := make(map[string]string, len(matches))
	for _, match := range matches {
		base := filepath.Base(match)
		name := strings.TrimSuffix(base, "-tags.scm")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out[name] = match
	}
	return out, nil
}

func writeManifestWithTargetNames(path string, targetNames map[string]struct{}) error {
	doc := map[string]any{}
	existingByName := map[string]map[string]any{}

	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parse manifest %q: %w", path, err)
		}
		maps.Copy(existingByName, languageEntriesByName(doc["languages"]))
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read manifest %q: %w", path, err)
	}

	names := make([]string, 0, len(targetNames))
	for name := range targetNames {
		names = append(names, name)
	}
	sort.Strings(names)

	languages := make([]any, 0, len(names))
	for _, name := range names {
		if existing, ok := existingByName[name]; ok {
			entry := cloneStringAnyMap(existing)
			entry["name"] = name
			languages = append(languages, entry)
			continue
		}
		languages = append(languages, map[string]any{"name": name})
	}
	doc["languages"] = languages

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest %q: %w", path, err)
	}
	out = append(out, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create manifest directory for %q: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("write manifest %q: %w", path, err)
	}
	return nil
}

func languageEntriesByName(raw any) map[string]map[string]any {
	entries := map[string]map[string]any{}
	items, ok := raw.([]any)
	if !ok {
		return entries
	}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := entry["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		entries[name] = entry
	}
	return entries
}

func cloneStringAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

func fileContentDiffers(src, dst string) (bool, error) {
	srcData, err := os.ReadFile(src)
	if err != nil {
		return false, fmt.Errorf("read query source %q: %w", src, err)
	}
	dstData, err := os.ReadFile(dst)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("read vendored query file %q: %w", dst, err)
	}
	return !bytes.Equal(srcData, dstData), nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read query source %q: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write vendored query %q: %w", dst, err)
	}
	return nil
}

func printUpdate(cmd *cobra.Command, report updateReport, includeHeader bool) {
	if includeHeader {
		cmd.Println("tsaudit update: applied changes")
	}

	printList := func(header string, values []string) {
		if len(values) == 0 {
			return
		}
		cmd.Printf("\n%s:\n", header)
		for _, value := range values {
			cmd.Printf("  - %s\n", value)
		}
	}

	printList("Manifest added", report.ManifestAdded)
	printList("Manifest removed", report.ManifestRemoved)
	printList("Queries added", report.QueriesAdded)
	printList("Queries removed", report.QueriesRemoved)
	printList("Queries updated", report.QueriesUpdated)
}
