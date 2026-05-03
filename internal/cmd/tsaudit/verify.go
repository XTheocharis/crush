package main

import (
	"bufio"
	"encoding/json"
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
	MissingRuntimeGrammar []string
	RuntimeNotInManifest  []string
	MissingDependency     []string
}

func (r verifyReport) HasIssues() bool {
	return len(r.MissingVendored) > 0 ||
		len(r.MissingUpstreamSource) > 0 ||
		len(r.ContentMismatches) > 0 ||
		len(r.InheritsDirectives) > 0 ||
		len(r.Uninterpretable) > 0 ||
		len(r.MissingRuntimeGrammar) > 0 ||
		len(r.RuntimeNotInManifest) > 0 ||
		len(r.MissingDependency) > 0
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
	manifestEntries, err := loadManifestLanguageEntries(manifestPath)
	if err != nil {
		return verifyReport{}, err
	}

	queryNames, err := loadVendoredQueryNames(queriesDir)
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

	runtimeMissingManifest, runtimeNotInManifest, dependencyIssues, err := verifyRuntimeDependencyAlignment(opts.repoRoot, manifestNames, manifestEntries, queryNames)
	if err != nil {
		return verifyReport{}, err
	}
	report.MissingRuntimeGrammar = runtimeMissingManifest
	report.RuntimeNotInManifest = runtimeNotInManifest
	report.MissingDependency = dependencyIssues

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
	printList("Manifest languages with vendored queries but no runtime grammar registration", report.MissingRuntimeGrammar)
	printList("Runtime grammars registered in parser.go but missing from manifest", report.RuntimeNotInManifest)
	printList("Runtime grammar modules missing from go.mod dependencies", report.MissingDependency)
}

func loadManifestLanguageEntries(manifestPath string) (map[string]manifestLanguage, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	var m languagesManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %q: %w", manifestPath, err)
	}

	out := make(map[string]manifestLanguage, len(m.Languages))
	for _, lang := range m.Languages {
		name := strings.TrimSpace(lang.Name)
		if name == "" {
			continue
		}
		out[name] = lang
	}
	return out, nil
}

func verifyRuntimeDependencyAlignment(repoRoot string, manifestNames map[string]struct{}, manifestEntries map[string]manifestLanguage, queryNames map[string]struct{}) ([]string, []string, []string, error) {
	runtimeLanguages, err := loadRuntimeRegisteredLanguages(repoRoot)
	if err != nil {
		return nil, nil, nil, err
	}

	runtimeModules, err := loadRuntimeGrammarModules(repoRoot)
	if err != nil {
		return nil, nil, nil, err
	}

	goModModules, err := loadGoModRequiredModules(repoRoot)
	if err != nil {
		return nil, nil, nil, err
	}

	missingRuntime := make([]string, 0)
	runtimeNotInManifest := make([]string, 0)
	missingDependencySet := map[string]struct{}{}

	for name := range manifestNames {
		if _, hasQuery := queryNames[name]; !hasQuery {
			continue
		}

		manifestModule := strings.TrimSpace(manifestEntries[name].GrammarModule)
		if manifestModule != "" {
			if _, registered := runtimeLanguages[name]; !registered {
				missingRuntime = append(missingRuntime, name)
				continue
			}
		}

		if _, registered := runtimeLanguages[name]; !registered {
			continue
		}

		mod := strings.TrimSpace(runtimeModules[name])
		if mod == "" {
			mod = manifestModule
		}
		if mod == "" {
			continue
		}
		if _, ok := goModModules[mod]; !ok {
			missingDependencySet[name+" ("+mod+")"] = struct{}{}
		}
	}

	for runtimeLang := range runtimeLanguages {
		if _, inManifest := manifestNames[runtimeLang]; !inManifest {
			runtimeNotInManifest = append(runtimeNotInManifest, runtimeLang)
		}
	}

	sort.Strings(missingRuntime)
	sort.Strings(runtimeNotInManifest)
	missingDependency := sortedSetKeys(missingDependencySet)
	return missingRuntime, runtimeNotInManifest, missingDependency, nil
}

func loadRuntimeRegisteredLanguages(repoRoot string) (map[string]struct{}, error) {
	parserPath := filepath.Join(repoRoot, "internal", "treesitter", "parser.go")
	f, err := os.Open(parserPath)
	if err != nil {
		return nil, fmt.Errorf("open parser source %q: %w", parserPath, err)
	}
	defer f.Close()

	out := map[string]struct{}{}
	scanner := bufio.NewScanner(f)
	inSwitch := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !inSwitch {
			if strings.HasPrefix(line, "func languageForQueryKey(") {
				inSwitch = true
			}
			continue
		}
		if strings.HasPrefix(line, "default:") {
			break
		}
		if after, ok := strings.CutPrefix(line, "case "); ok {
			lang := after
			lang = strings.TrimSuffix(lang, ":")
			lang = strings.Trim(lang, "\"")
			lang = strings.TrimSpace(lang)
			if lang != "" {
				out[lang] = struct{}{}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan parser source %q: %w", parserPath, err)
	}
	return out, nil
}

func loadRuntimeGrammarModules(repoRoot string) (map[string]string, error) {
	parserPath := filepath.Join(repoRoot, "internal", "treesitter", "parser.go")
	f, err := os.Open(parserPath)
	if err != nil {
		return nil, fmt.Errorf("open parser source %q: %w", parserPath, err)
	}
	defer f.Close()

	moduleByAlias := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, "\"/bindings/go\"") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		alias := strings.TrimSpace(parts[0])
		module := strings.TrimSpace(parts[1])
		module = strings.Trim(module, "\"")
		module = strings.TrimSuffix(module, "/bindings/go")
		if alias != "" && module != "" {
			moduleByAlias[alias] = module
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan parser imports %q: %w", parserPath, err)
	}

	f2, err := os.Open(parserPath)
	if err != nil {
		return nil, fmt.Errorf("open parser source %q: %w", parserPath, err)
	}
	defer f2.Close()

	languageToModule := map[string]string{}
	scanner = bufio.NewScanner(f2)
	inSwitch := false
	currentCase := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !inSwitch {
			if strings.HasPrefix(line, "func languageForQueryKey(") {
				inSwitch = true
			}
			continue
		}
		if strings.HasPrefix(line, "default:") {
			break
		}
		if after, ok := strings.CutPrefix(line, "case "); ok {
			currentCase = after
			currentCase = strings.TrimSuffix(currentCase, ":")
			currentCase = strings.Trim(currentCase, "\"")
			currentCase = strings.TrimSpace(currentCase)
			continue
		}
		if currentCase == "" {
			continue
		}
		if !strings.Contains(line, "tree_sitter.NewLanguage(") {
			continue
		}
		_, after, ok := strings.Cut(line, "tree_sitter.NewLanguage(")
		if !ok {
			continue
		}
		expr := after
		before, _, ok := strings.Cut(expr, ".")
		if !ok {
			continue
		}
		alias := strings.TrimSpace(before)
		if mod, ok := moduleByAlias[alias]; ok {
			languageToModule[currentCase] = mod
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan parser switch %q: %w", parserPath, err)
	}
	return languageToModule, nil
}

func loadGoModRequiredModules(repoRoot string) (map[string]struct{}, error) {
	goModPath := filepath.Join(repoRoot, "go.mod")
	f, err := os.Open(goModPath)
	if err != nil {
		return nil, fmt.Errorf("open go.mod %q: %w", goModPath, err)
	}
	defer f.Close()

	out := map[string]struct{}{}
	scanner := bufio.NewScanner(f)
	inRequireBlock := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "require (") {
			inRequireBlock = true
			continue
		}
		if inRequireBlock && line == ")" {
			inRequireBlock = false
			continue
		}
		if after, ok := strings.CutPrefix(line, "require "); ok {
			mod := strings.TrimSpace(after)
			parts := strings.Fields(mod)
			if len(parts) >= 1 {
				out[parts[0]] = struct{}{}
			}
			continue
		}
		if inRequireBlock {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				out[parts[0]] = struct{}{}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan go.mod %q: %w", goModPath, err)
	}
	return out, nil
}
