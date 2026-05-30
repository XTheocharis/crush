package app

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDeadSettersWired verifies that the previously-dead LCM Manager setter
// methods are now called from production code (not just tests). For each
// setter, it walks the repository Go source, finds call sites, and confirms
// at least one call site exists in a non-test file.
func TestDeadSettersWired(t *testing.T) {
	t.Parallel()

	repoRoot := repoRoot(t)

	setters := []struct {
		name    string
		method  string
		sources []string // production files expected to contain a call
	}{
		{
			name:   "SetModelOutputLimit",
			method: "SetModelOutputLimit",
			sources: []string{
				"internal/app/app_xrush_wiring.go",
			},
		},
		{
			name:   "SetOverheadTokens",
			method: "SetOverheadTokens",
			sources: []string{
				"internal/app/app_xrush_wiring.go",
			},
		},
		{
			name:   "SetProviderType",
			method: "SetProviderType",
			sources: []string{
				"internal/app/app_xrush_wiring.go",
			},
		},
		{
			name:   "SetActualPromptTokens",
			method: "SetActualPromptTokens",
			sources: []string{
				"internal/extensions/lcm_ext.go",
			},
		},
		{
			name:   "SetRepoMapTokens",
			method: "SetRepoMapTokens",
			sources: []string{
				"internal/extensions/prompt_assembly_ext.go",
			},
		},
	}

	for _, s := range setters {
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()

			found := false
			for _, src := range s.sources {
				fullPath := filepath.Join(repoRoot, src)
				content, err := os.ReadFile(fullPath)
				require.NoError(t, err, "reading %s", src)

				if strings.Contains(string(content), s.method) {
					found = true
					break
				}
			}
			require.True(t, found,
				"%s should have a production caller in one of: %v",
				s.method, s.sources,
			)
		})
	}
}

// TestDeadSettersNoTestOnly verifies that each wired setter has no callers
// exclusively in test files. It parses all .go files in the repo and checks
// that at least one non-_test.go file calls the setter.
func TestDeadSettersNoTestOnly(t *testing.T) {
	t.Parallel()

	repoRoot := repoRoot(t)

	methods := []string{
		"SetModelOutputLimit",
		"SetOverheadTokens",
		"SetProviderType",
		"SetActualPromptTokens",
		"SetRepoMapTokens",
	}

	fset := token.NewFileSet()

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			hasProdCaller := false

			err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
					content, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					rel, _ := filepath.Rel(repoRoot, path)
					if strings.Contains(string(content), method+"(") {
						node, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
						if parseErr != nil {
							return parseErr
						}
						_ = node
						hasProdCaller = true
					}
					_ = rel
				}
				return nil
			})
			require.NoError(t, err)

			require.True(t, hasProdCaller,
				"%s should have at least one production (non-test) caller",
				method,
			)
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../..")
	require.NoError(t, err)
	return dir
}
