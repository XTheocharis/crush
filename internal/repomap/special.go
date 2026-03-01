package repomap

import (
	"path"
	"sort"
	"strings"
)

// specialRootFiles contains root-scoped file patterns that are always
// included in the stage-0 special prelude when present in the repository.
// Sourced from Aider's ROOT_IMPORTANT_FILES (commit 235b83d0).
// Deduplicated to 153 unique entries; Makefile and CMakeLists.txt are
// intentionally excluded as they are not in Aider's list.
var specialRootFiles = map[string]struct{}{
	// Version control.
	".gitattributes": {},
	".gitignore":     {},
	".gitkeep":       {},

	// Editor and IDE configuration.
	".editorconfig": {},
	".nvmrc":        {},

	// Documentation.
	"CHANGELOG":        {},
	"CHANGELOG.md":     {},
	"CHANGELOG.rst":    {},
	"CHANGELOG.txt":    {},
	"CODEOWNERS":       {},
	"CONTRIBUTING":     {},
	"CONTRIBUTING.md":  {},
	"CONTRIBUTING.rst": {},
	"CONTRIBUTING.txt": {},
	"LICENSE":          {},
	"LICENSE.md":       {},
	"LICENSE.txt":      {},
	"README":           {},
	"README.md":        {},
	"README.rst":       {},
	"README.txt":       {},
	"SECURITY":         {},
	"SECURITY.md":      {},
	"SECURITY.txt":     {},

	// Environment and secrets.
	".env":              {},
	".env.example":      {},
	".secrets.baseline": {},

	// CI/CD configuration.
	".circleci/config.yml":    {},
	".github/dependabot.yml":  {},
	".gitlab-ci.yml":          {},
	".gitpod.yml":             {},
	".travis.yml":             {},
	"Jenkinsfile":             {},
	"appveyor.yml":            {},
	"azure-pipelines.yml":     {},
	"bitbucket-pipelines.yml": {},
	"circle.yml":              {},
	"dependabot.yml":          {},

	// Container and orchestration.
	"Dockerfile":                  {},
	".dockerignore":               {},
	"Vagrantfile":                 {},
	"docker-compose.override.yml": {},
	"docker-compose.yml":          {},

	// Cloud and infrastructure.
	"ansible.cfg":         {},
	"app.yaml":            {},
	"cloudformation.json": {},
	"cloudformation.yaml": {},
	"firebase.json":       {},
	"flyway.conf":         {},
	"k8s.yaml":            {},
	"kubernetes.yaml":     {},
	"main.tf":             {},
	"netlify.toml":        {},
	"now.json":            {},
	"serverless.yml":      {},
	"terraform.tf":        {},
	"vercel.json":         {},

	// Go.
	"go.mod": {},
	"go.sum": {},

	// JavaScript and TypeScript.
	".babelrc":            {},
	".npmignore":          {},
	".npmrc":              {},
	".nycrc":              {},
	".nycrc.json":         {},
	".yarnrc":             {},
	"Gruntfile.js":        {},
	"angular.json":        {},
	"babel.config.js":     {},
	"cypress.json":        {},
	"gatsby-config.js":    {},
	"gridsome.config.js":  {},
	"gulpfile.js":         {},
	"jest.config.js":      {},
	"jsconfig.json":       {},
	"karma.conf.js":       {},
	"next.config.js":      {},
	"npm-shrinkwrap.json": {},
	"nuxt.config.js":      {},
	"package-lock.json":   {},
	"package.json":        {},
	"parcel.config.js":    {},
	"rollup.config.js":    {},
	"tsconfig.json":       {},
	"tslint.json":         {},
	"vue.config.js":       {},
	"webpack.config.js":   {},
	"yarn.lock":           {},

	// Python.
	".coveragerc":        {},
	".flake8":            {},
	".isort.cfg":         {},
	".pylintrc":          {},
	".pypirc":            {},
	".python-version":    {},
	"MANIFEST.in":        {},
	"Pipfile":            {},
	"Pipfile.lock":       {},
	"mypy.ini":           {},
	"pyproject.toml":     {},
	"pyrightconfig.json": {},
	"pytest.ini":         {},
	"requirements.txt":   {},
	"setup.cfg":          {},
	"setup.py":           {},
	"tox.ini":            {},

	// Ruby.
	".rubocop.yml":  {},
	".ruby-version": {},
	"Gemfile":       {},
	"Gemfile.lock":  {},
	"Podfile":       {},

	// Rust.
	"Cargo.lock": {},
	"Cargo.toml": {},

	// Java and JVM.
	"build.gradle":     {},
	"build.gradle.kts": {},
	"build.sbt":        {},
	"build.xml":        {},
	"pom.xml":          {},

	// .NET and C#.
	"build.cake":   {},
	"project.json": {},

	// Scala.
	".scalafmt.conf": {},

	// Clojure.
	"project.clj": {},

	// Elixir.
	"mix.exs": {},

	// Erlang.
	"rebar.config": {},

	// D language.
	"dub.json": {},
	"dub.sdl":  {},

	// PHP.
	"composer.json": {},
	"composer.lock": {},
	"phpunit.xml":   {},

	// Static site generators and documentation tools.
	"_config.yml": {},
	"book.toml":   {},
	"mkdocs.yml":  {},

	// Linting and formatting.
	".eslintignore":      {},
	".eslintrc":          {},
	".markdownlint.json": {},
	".markdownlint.yaml": {},
	".prettierrc":        {},
	".stylelintrc":       {},
	".yamllint":          {},

	// Code quality and analysis.
	".bandit":                  {},
	".codeclimate.yml":         {},
	".pre-commit-config.yaml":  {},
	"codecov.yml":              {},
	"renovate.json":            {},
	"sonar-project.properties": {},

	// API specification.
	"openapi.json": {},
	"openapi.yaml": {},
	"swagger.json": {},
	"swagger.yaml": {},

	// Database.
	"liquibase.properties": {},
	"schema.sql":           {},

	// Documentation hosting.
	".readthedocs.yaml": {},
	"readthedocs.yml":   {},

	// Clojure build.
	"build.boot": {},

	// iOS and macOS.
	"Cartfile": {},
}

// IsSpecialFile reports whether relPath is part of the special-file
// prelude set. Root-scoped files match only at repo root, except
// .github/workflows/*.yml|yaml.
func IsSpecialFile(relPath string) bool {
	relPath = normalizeGraphRelPath(relPath)
	if relPath == "" {
		return false
	}

	if strings.HasPrefix(relPath, ".github/workflows/") {
		base := strings.ToLower(path.Base(relPath))
		if strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml") {
			return true
		}
	}

	if strings.Contains(relPath, "/") {
		// The only multi-segment paths in specialRootFiles are fixed
		// directory-scoped entries (e.g. .circleci/config.yml,
		// .github/dependabot.yml). Check them directly.
		_, ok := specialRootFiles[relPath]
		return ok
	}
	_, ok := specialRootFiles[relPath]
	return ok
}

// BuildSpecialPrelude selects stage-0 special files from otherFnames
// only, excluding files already represented in ranked output.
func BuildSpecialPrelude(otherFnames []string, rankedFiles []string, parityMode bool) []string {
	other := normalizeUniqueGraphPaths(otherFnames)
	if !parityMode {
		// Enhancement mode allows deterministic behavior; keep lexical
		// order.
		sort.Strings(other)
	}

	rankedSet := make(map[string]struct{})
	for _, f := range normalizeUniqueGraphPaths(rankedFiles) {
		rankedSet[f] = struct{}{}
	}

	out := make([]string, 0)
	for _, f := range other {
		if !IsSpecialFile(f) {
			continue
		}
		if _, inRanked := rankedSet[f]; inRanked {
			continue
		}
		out = append(out, f)
	}
	return out
}
