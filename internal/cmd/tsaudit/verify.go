package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var errVerifyFailed = errors.New("verification failed")

var inheritsDirectiveRE = regexp.MustCompile(`(?mi)^\s*;\s*inherits\s*:`)

type verifyOptions struct {
	repoRoot           string
	manifestPath       string
	queriesDir         string
	primaryQueriesDir  string
	fallbackQueriesDir string
}

type verifyReport struct {
	MissingVendored       []string
	MissingUpstreamSource []string
	ContentMismatches     []string
	InheritsDirectives    []string
	Uninterpretable       []string
}

func (r verifyReport) HasIssues() bool {
	return len(r.MissingVendored) > 0 ||
		len(r.MissingUpstreamSource) > 0 ||
		len(r.ContentMismatches) > 0 ||
		len(r.InheritsDirectives) > 0 ||
		len(r.Uninterpretable) > 0
}

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify vendored queries against upstream",
	Long:  "Verify vendored tags queries match upstream sources and satisfy parser-contract drift checks.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		report, err := runVerify(loadVerifyOptions(cmd))
		if err != nil {
			return err
		}

		if !report.HasIssues() {
			cmd.Println("tsaudit verify: ok")
			return nil
		}

		printVerify(cmd, report)
		return errVerifyFailed
	},
}

func init() {
	verifyCmd.Flags().String("repo-root", ".", "Repository root path")
	verifyCmd.Flags().String("manifest", defaultManifestRelPath, "Path to languages.json manifest")
	verifyCmd.Flags().String("queries-dir", defaultQueriesRelPath, "Path to vendored query directory")
	verifyCmd.Flags().String("primary-queries-dir", "", "Path to primary source directory containing *-tags.scm files")
	verifyCmd.Flags().String("fallback-queries-dir", "", "Path to fallback source directory containing *-tags.scm files")

	rootCmd.AddCommand(verifyCmd)
}

func loadVerifyOptions(cmd *cobra.Command) verifyOptions {
	repoRoot, _ := cmd.Flags().GetString("repo-root")
	manifestPath, _ := cmd.Flags().GetString("manifest")
	queriesDir, _ := cmd.Flags().GetString("queries-dir")
	primaryQueriesDir, _ := cmd.Flags().GetString("primary-queries-dir")
	fallbackQueriesDir, _ := cmd.Flags().GetString("fallback-queries-dir")

	return verifyOptions{
		repoRoot:           repoRoot,
		manifestPath:       manifestPath,
		queriesDir:         queriesDir,
		primaryQueriesDir:  primaryQueriesDir,
		fallbackQueriesDir: fallbackQueriesDir,
	}
}

func runVerify(opts verifyOptions) (verifyReport, error) {
	manifestPath := resolvePath(opts.repoRoot, opts.manifestPath)
	queriesDir := resolvePath(opts.repoRoot, opts.queriesDir)
	primaryQueriesDir := ""
	if strings.TrimSpace(opts.primaryQueriesDir) != "" {
		primaryQueriesDir = resolvePath(opts.repoRoot, opts.primaryQueriesDir)
	}
	fallbackQueriesDir := ""
	if strings.TrimSpace(opts.fallbackQueriesDir) != "" {
		fallbackQueriesDir = resolvePath(opts.repoRoot, opts.fallbackQueriesDir)
	}

	manifestNames, err := loadManifestNames(manifestPath)
	if err != nil {
		return verifyReport{}, err
	}

	sourceMap, err := collectQuerySources(primaryQueriesDir, fallbackQueriesDir)
	if err != nil {
		return verifyReport{}, err
	}

	report := verifyReport{}
	for _, name := range sortedSetKeys(manifestNames) {
		vendoredPath := filepath.Join(queriesDir, name+"-tags.scm")
		vendoredData, readErr := os.ReadFile(vendoredPath)
		vendoredExists := true
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				report.MissingVendored = append(report.MissingVendored, name)
				vendoredExists = false
			} else {
				return verifyReport{}, fmt.Errorf("read vendored query %q: %w", vendoredPath, readErr)
			}
		}

		if vendoredExists {
			if hasInheritsDirective(string(vendoredData)) {
				report.InheritsDirectives = append(report.InheritsDirectives, name)
			}
			if !isDualStyleInterpretable(string(vendoredData)) {
				report.Uninterpretable = append(report.Uninterpretable, name)
			}
		}

		if primaryQueriesDir == "" && fallbackQueriesDir == "" {
			continue
		}

		sourcePath, ok := sourceMap[name]
		if !ok {
			report.MissingUpstreamSource = append(report.MissingUpstreamSource, name)
			continue
		}

		if !vendoredExists {
			continue
		}

		changed, cmpErr := fileContentDiffers(sourcePath, vendoredPath)
		if cmpErr != nil {
			return verifyReport{}, cmpErr
		}
		if changed {
			report.ContentMismatches = append(report.ContentMismatches, name)
		}
	}

	return report, nil
}

func hasInheritsDirective(content string) bool {
	return inheritsDirectiveRE.MatchString(content)
}

func isDualStyleInterpretable(content string) bool {
	if strings.Contains(content, "@name.definition.") || strings.Contains(content, "@name.reference.") {
		return true
	}
	legacyName := strings.Contains(content, "@name")
	legacyPaired := strings.Contains(content, "@definition.") || strings.Contains(content, "@reference.")
	return legacyName && legacyPaired
}

func sortedSetKeys(items map[string]struct{}) []string {
	out := make([]string, 0, len(items))
	for item := range items {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func printVerify(cmd *cobra.Command, report verifyReport) {
	cmd.Println("tsaudit verify: failed")

	printList := func(header string, values []string) {
		if len(values) == 0 {
			return
		}
		cmd.Printf("\n%s:\n", header)
		for _, value := range values {
			cmd.Printf("  - %s\n", value)
		}
	}

	printList("Missing vendored query files", report.MissingVendored)
	printList("Missing upstream query sources", report.MissingUpstreamSource)
	printList("Vendored content mismatches", report.ContentMismatches)
	printList("Vendored queries with ; inherits: directives", report.InheritsDirectives)
	printList("Vendored queries not interpretable under dual-style contract", report.Uninterpretable)
}
