//go:build treesitter
// +build treesitter

package repomap

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProximityScorer_Score(t *testing.T) {
	t.Parallel()

	scorer := NewProximityScorer()

	t.Run("returns nil for empty inputs", func(t *testing.T) {
		t.Parallel()

		result := scorer.Score(nil, []string{"foo_test.go"})
		require.Nil(t, result)

		result = scorer.Score([]string{"foo.go"}, nil)
		require.Nil(t, result)

		result = scorer.Score(nil, nil)
		require.Nil(t, result)
	})

	t.Run("test file scores 1.0", func(t *testing.T) {
		t.Parallel()

		files := []string{"handler.go", "handler_test.go"}
		tests := []string{"handler_test.go"}
		scores := scorer.Score(files, tests)

		require.InDelta(t, 1.0, scores["handler_test.go"], 1e-9)
	})

	t.Run("naming match same directory scores 0.8", func(t *testing.T) {
		t.Parallel()

		files := []string{"handler.go", "handler_test.go"}
		tests := []string{"handler_test.go"}
		scores := scorer.Score(files, tests)

		require.InDelta(t, 0.8, scores["handler.go"], 1e-9)
	})

	t.Run("naming match different directory scores 0.5", func(t *testing.T) {
		t.Parallel()

		files := []string{"pkg/handler.go"}
		tests := []string{"handler_test.go"}
		scores := scorer.Score(files, tests)

		require.InDelta(t, 0.5, scores["pkg/handler.go"], 1e-9)
	})

	t.Run("directory co-location without naming match", func(t *testing.T) {
		t.Parallel()

		files := []string{"pkg/utils.go", "pkg/handler_test.go"}
		tests := []string{"pkg/handler_test.go"}
		scores := scorer.Score(files, tests)

		// utils.go is in same dir as handler_test.go but no naming match.
		require.Equal(t, 0.4, scores["pkg/utils.go"])
	})

	t.Run("no proximity scores 0", func(t *testing.T) {
		t.Parallel()

		files := []string{"cmd/main.go"}
		tests := []string{"pkg/handler_test.go"}
		scores := scorer.Score(files, tests)

		_, exists := scores["cmd/main.go"]
		require.False(t, exists)
	})

	t.Run("multiple test files co-location density", func(t *testing.T) {
		t.Parallel()

		files := []string{
			"pkg/utils.go",
			"pkg/handler_test.go",
			"pkg/service_test.go",
			"pkg/repo_test.go",
		}
		tests := []string{
			"pkg/handler_test.go",
			"pkg/service_test.go",
			"pkg/repo_test.go",
		}
		scores := scorer.Score(files, tests)

		// 3 test files in same dir → 0.3 + 0.1*3 = 0.6.
		require.InDelta(t, 0.6, scores["pkg/utils.go"], 1e-9)
	})

	t.Run("typescript test patterns", func(t *testing.T) {
		t.Parallel()

		files := []string{"src/button.tsx", "src/button.test.tsx", "src/utils.ts"}
		tests := []string{"src/button.test.tsx"}
		scores := scorer.Score(files, tests)

		// button.tsx matches naming with button.test.tsx, same dir.
		require.InDelta(t, 0.8, scores["src/button.tsx"], 1e-9)
		// utils.ts is co-located but no naming match.
		require.Equal(t, 0.4, scores["src/utils.ts"])
	})

	t.Run("javascript spec pattern", func(t *testing.T) {
		t.Parallel()

		files := []string{"src/api.js", "src/api.spec.js"}
		tests := []string{"src/api.spec.js"}
		scores := scorer.Score(files, tests)

		require.InDelta(t, 0.8, scores["src/api.js"], 1e-9)
	})

	t.Run("python test pattern", func(t *testing.T) {
		t.Parallel()

		files := []string{"app/models.py", "app/models_test.py"}
		tests := []string{"app/models_test.py"}
		scores := scorer.Score(files, tests)

		require.InDelta(t, 0.8, scores["app/models.py"], 1e-9)
	})

	t.Run("rust test pattern", func(t *testing.T) {
		t.Parallel()

		files := []string{"src/lib.rs", "src/lib_test.rs"}
		tests := []string{"src/lib_test.rs"}
		scores := scorer.Score(files, tests)

		require.InDelta(t, 0.8, scores["src/lib.rs"], 1e-9)
	})
}

func TestProximityScorer_Score_SyntheticFileTree(t *testing.T) {
	t.Parallel()

	scorer := NewProximityScorer()
	tmpDir := t.TempDir()

	// Create a synthetic file tree structure.
	files := []string{
		filepath.Join(tmpDir, "internal/server/handler.go"),
		filepath.Join(tmpDir, "internal/server/handler_test.go"),
		filepath.Join(tmpDir, "internal/server/middleware.go"),
		filepath.Join(tmpDir, "internal/server/routes.go"),
		filepath.Join(tmpDir, "internal/server/routes_test.go"),
		filepath.Join(tmpDir, "internal/models/user.go"),
		filepath.Join(tmpDir, "internal/models/user_test.go"),
		filepath.Join(tmpDir, "cmd/main.go"),
	}

	testFiles := []string{
		filepath.Join(tmpDir, "internal/server/handler_test.go"),
		filepath.Join(tmpDir, "internal/server/routes_test.go"),
		filepath.Join(tmpDir, "internal/models/user_test.go"),
	}

	scores := scorer.Score(files, testFiles)

	// Test files score 1.0.
	require.InDelta(t, 1.0, scores[filepath.Join(tmpDir, "internal/server/handler_test.go")], 1e-9)
	require.InDelta(t, 1.0, scores[filepath.Join(tmpDir, "internal/server/routes_test.go")], 1e-9)

	// handler.go matches handler_test.go by naming AND co-location → 0.8.
	require.InDelta(t, 0.8, scores[filepath.Join(tmpDir, "internal/server/handler.go")], 1e-9)

	// middleware.go is co-located with 2 test files, no naming match → 0.5.
	require.InDelta(t, 0.5, scores[filepath.Join(tmpDir, "internal/server/middleware.go")], 1e-9)

	// cmd/main.go has no proximity to any test file.
	_, exists := scores[filepath.Join(tmpDir, "cmd/main.go")]
	require.False(t, exists)
}

func TestBlendProximityPersonalization(t *testing.T) {
	t.Parallel()

	t.Run("returns original when no proximity scores", func(t *testing.T) {
		t.Parallel()

		pers := map[string]float64{"foo.go": 0.5}
		result := BlendProximityPersonalization(pers, nil, 0.3)
		require.Equal(t, pers, result)
	})

	t.Run("returns original when blendFactor is zero", func(t *testing.T) {
		t.Parallel()

		pers := map[string]float64{"foo.go": 0.5}
		prox := map[string]float64{"foo.go": 0.8}
		result := BlendProximityPersonalization(pers, prox, 0.0)
		require.Equal(t, pers, result)
	})

	t.Run("blends proximity into existing personalization", func(t *testing.T) {
		t.Parallel()

		pers := map[string]float64{"foo.go": 1.0, "bar.go": 0.5}
		prox := map[string]float64{"foo.go": 0.8, "baz.go": 0.4}
		result := BlendProximityPersonalization(pers, prox, 0.5)

		// foo.go: (1-0.5)*1.0 + 0.5*0.8 = 0.5 + 0.4 = 0.9.
		require.InDelta(t, 0.9, result["foo.go"], 1e-9)
		// bar.go: untouched (no proximity score).
		require.InDelta(t, 0.5, result["bar.go"], 1e-9)
		// baz.go: (1-0.5)*0 + 0.5*0.4 = 0.2.
		require.InDelta(t, 0.2, result["baz.go"], 1e-9)
	})

	t.Run("does not mutate input map", func(t *testing.T) {
		t.Parallel()

		pers := map[string]float64{"foo.go": 1.0}
		prox := map[string]float64{"foo.go": 0.8}
		result := BlendProximityPersonalization(pers, prox, 0.5)

		require.InDelta(t, 1.0, pers["foo.go"], 1e-9)
		require.InDelta(t, 0.9, result["foo.go"], 1e-9)
	})
}

func TestIsTestFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		expected bool
	}{
		{"handler_test.go", true},
		{"handler.go", false},
		{"models_test.py", true},
		{"models.py", false},
		{"button.test.tsx", true},
		{"button.spec.ts", true},
		{"button.tsx", false},
		{"api.test.js", true},
		{"api.spec.js", true},
		{"api.js", false},
		{"lib_test.rs", true},
		{"lib.rs", false},
		{"UserTest.java", true},
		{"UserService.java", false},
		{"service_test.rb", true},
		{"service_spec.rb", true},
		{"service.rb", false},
		{"Makefile", false},
		{"README.md", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, IsTestFile(tc.path))
		})
	}
}

func TestFindTestFiles(t *testing.T) {
	t.Parallel()

	files := []string{
		"handler.go",
		"handler_test.go",
		"utils.ts",
		"utils.test.ts",
		"main.py",
		"README.md",
	}

	tests := FindTestFiles(files)
	require.Equal(t, []string{"handler_test.go", "utils.test.ts"}, tests)
}
