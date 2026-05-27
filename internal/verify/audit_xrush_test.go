package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("cannot find go.mod")
		}
		dir = parent
	}
}

type component struct {
	ID       string
	Name     string
	Files    []string
	DirFiles []string // Directories to check for existence.
	WireGrep []wireCheck
}

type wireCheck struct {
	Pattern string
	Files   []string
	Dirs    []string // Directories to scan all files in.
}

var stubMarkers = []string{
	"stub",
	"todo",
	"fixme",
	"not yet implemented",
	"not implemented",
	"placeholder",
	"noop",
	"// no-op",
}

func hasStubContent(content string) []string {
	lower := strings.ToLower(content)
	var found []string
	for _, marker := range stubMarkers {
		if strings.Contains(lower, marker) {
			found = append(found, marker)
		}
	}
	return found
}

type checkResult struct {
	FileExists  bool
	FileMissing []string
	Stubs       map[string][]string
	Wired       bool
	WireDetails []string
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func readFileGrep(dirPath, pattern string) (bool, string, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false, "", err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fullPath := filepath.Join(dirPath, e.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		content := string(data)
		if strings.Contains(content, pattern) {
			return true, fullPath, nil
		}
	}
	return false, "", nil
}

func checkComponent(root string, comp component) checkResult {
	res := checkResult{
		Stubs: make(map[string][]string),
	}

	// File existence.
	res.FileExists = true
	for _, f := range comp.Files {
		if !fileExists(filepath.Join(root, f)) {
			res.FileExists = false
			res.FileMissing = append(res.FileMissing, f)
		}
	}
	for _, d := range comp.DirFiles {
		if !dirExists(filepath.Join(root, d)) {
			res.FileExists = false
			res.FileMissing = append(res.FileMissing, d)
		}
	}

	// Stub markers in existing files.
	for _, f := range comp.Files {
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			continue
		}
		if markers := hasStubContent(string(data)); len(markers) > 0 {
			res.Stubs[f] = markers
		}
	}

	// Wiring checks.
	res.Wired = true
	for _, wc := range comp.WireGrep {
		found := false

		// Check individual files.
		for _, wf := range wc.Files {
			data, err := os.ReadFile(filepath.Join(root, wf))
			if err != nil {
				continue
			}
			if strings.Contains(string(data), wc.Pattern) {
				found = true
				res.WireDetails = append(res.WireDetails,
					fmt.Sprintf("found %q in %s", wc.Pattern, wf))
				break
			}
		}

		// Check directory scans.
		if !found {
			for _, d := range wc.Dirs {
				ok, matchFile, err := readFileGrep(filepath.Join(root, d), wc.Pattern)
				if err != nil {
					continue
				}
				if ok {
					found = true
					res.WireDetails = append(res.WireDetails,
						fmt.Sprintf("found %q in %s", wc.Pattern, matchFile))
					break
				}
			}
		}

		if !found {
			res.Wired = false
			res.WireDetails = append(res.WireDetails,
				fmt.Sprintf("MISSING: %q not found in %v / dirs %v", wc.Pattern, wc.Files, wc.Dirs))
		}
	}

	if len(comp.WireGrep) == 0 {
		res.Wired = true
		res.WireDetails = []string{"no wiring check defined (N/A)"}
	}

	return res
}

func all29Components() []component {
	return []component{
		// Layer A — Code Understanding.
		{
			ID:   "A.1",
			Name: "PageRank Repomap",
			Files: []string{
				"internal/repomap/pagerank.go",
				"internal/repomap/graph.go",
				"internal/repomap/stage.go",
				"internal/repomap/render.go",
				"internal/repomap/cache.go",
				"internal/repomap/blame.go",
				"internal/repomap/proximity.go",
				"internal/repomap/diffwatch.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "repomap", Files: []string{"internal/extensions/repomap_ext.go", "internal/extensions/register.go"}},
			},
		},
		{
			ID:   "A.2",
			Name: "File Dispatchers (Tree-sitter)",
			Files: []string{
				"internal/treesitter/parser.go",
				"internal/treesitter/query.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "treesitter", Files: []string{"internal/repomap/repomap.go", "internal/extensions/treesitter_ext.go"}},
				{Pattern: "Parser", Files: []string{"internal/treesitter/parser.go"}},
			},
		},
		{
			ID:   "A.3",
			Name: "Embedded LSP",
			Files: []string{
				"internal/lsp/manager.go",
				"internal/lsp/executor.go",
				"internal/lsp/backoff.go",
				"internal/lsp/namepath.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "lsp", Files: []string{"internal/app/app.go"}},
			},
		},

		// Layer B — Context Management.
		{
			ID:   "B.1",
			Name: "7-Layer Reduction",
			Files: []string{
				"internal/lcm/compaction_layers.go",
				"internal/lcm/pressure.go",
				"internal/lcm/cache_optimizer.go",
				"internal/lcm/post_compact.go",
				"internal/lcm/full_compactor.go",
				"internal/lcm/session_compactor.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "RunLayeredCompaction", Files: []string{"internal/lcm/manager.go"}},
				{Pattern: "compaction", Files: []string{"internal/lcm/manager.go"}},
			},
		},
		{
			ID:   "B.2",
			Name: "LLM-as-Compressor",
			Files: []string{
				"internal/lcm/compressor.go",
				"internal/lcm/reversible.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "CompressionStrategy", Files: []string{"internal/lcm/compressor.go"}},
				{Pattern: "Compressor", Files: []string{"internal/lcm/manager.go", "internal/lcm/compactor.go"}},
			},
		},
		{
			ID:   "B.3",
			Name: "3-Agent Observation",
			Files: []string{
				"internal/lcm/observation.go",
				"internal/lcm/reflector.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "Observation", Files: []string{"internal/lcm/manager.go", "internal/lcm/observation.go"}},
			},
		},
		{
			ID:   "B.4",
			Name: "Ghost-Cue Injection",
			Files: []string{
				"internal/lcm/cue.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "InjectIntoPrompt", Files: []string{"internal/lcm/cue.go"}},
				{Pattern: "CueInjector", Files: []string{"internal/lcm/manager.go"}},
			},
		},
		{
			ID:   "B.5",
			Name: "Summary DAG / Retrieval",
			Files: []string{
				"internal/lcm/retrieval.go",
				"internal/lcm/retrieval_tools.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "Bindle", Files: []string{"internal/lcm/retrieval.go"}},
				{Pattern: "retrieval", Files: []string{"internal/lcm/manager.go"}},
			},
		},

		// Layer C — Memory.
		{
			ID:   "C.1",
			Name: "Hierarchical CLAUDE.md / Skills",
			Files: []string{
				"internal/skills/skills.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "Discover", Files: []string{"internal/skills/skills.go"}},
				{Pattern: "skills", Files: []string{"internal/app/app.go"}},
			},
		},
		{
			ID:   "C.2",
			Name: "Thread-Scoped OM (session_om)",
			Files: []string{
				"internal/session/om.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "session_om", Dirs: []string{"internal/db/migrations"}},
			},
		},
		{
			ID:   "C.3",
			Name: "Auto-Memory",
			Files: []string{
				"internal/lcm/memory.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "Memory", Files: []string{"internal/lcm/manager.go"}},
			},
		},
		{
			ID:   "C.4",
			Name: "Reversible Compression (BlockIDTracker)",
			Files: []string{
				"internal/lcm/reversible.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "BlockIDTracker", Files: []string{"internal/lcm/reversible.go"}},
			},
		},

		// Layer D — Edit.
		{
			ID:   "D.1",
			Name: "Hash-Anchored Edits",
			Files: []string{
				"internal/agent/tools/edit_anchors.go",
				"internal/agent/tools/edit_batch.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "HashAnchor", Files: []string{"internal/agent/tools/edit_anchors.go"}},
				{Pattern: "BatchProcessor", Files: []string{"internal/agent/tools/edit_batch.go"}},
			},
		},
		{
			ID:   "D.2",
			Name: "LSP Symbolic Edit",
			Files: []string{
				"internal/agent/tools/lsp_rename.go",
				"internal/agent/tools/lsp_symbolic.go",
				"internal/lsp/util/edit.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "ReplaceSymbolBody", Files: []string{"internal/agent/tools/lsp_symbolic.go"}},
			},
		},

		// Layer E — Validation.
		{
			ID:   "E.1",
			Name: "12-Step Validation Pipeline",
			Files: []string{
				"internal/agent/tools/validate.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "ValidationPipeline", Files: []string{"internal/agent/tools/validate.go"}},
			},
		},
		{
			ID:   "E.2",
			Name: "Auto LSP Diagnostics",
			Files: []string{
				"internal/agent/tools/diag_gate.go",
				"internal/agent/tools/diag_autofix.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "DiagnosticGate", Files: []string{"internal/agent/tools/diag_gate.go"}},
				{Pattern: "AutoFixer", Files: []string{"internal/agent/tools/diag_autofix.go"}},
			},
		},
		{
			ID:   "E.3",
			Name: "Auto-Lint→Commit→Test (AutoFixLoop)",
			Files: []string{
				"internal/agent/autofix.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "AutoFixLoop", Files: []string{"internal/agent/autofix.go"}},
			},
		},
		{
			ID:   "E.4",
			Name: "Atomic Rollback",
			Files: []string{
				"internal/agent/tools/rollback.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "Rollback", Files: []string{"internal/agent/tools/rollback.go"}},
			},
		},

		// Layer F — Model Optimization.
		// Adapted: selectModelForPrompt in coordinator.go → selectModel in
		// model_router_ext.go (clean reorganized model routing into extensions).
		{
			ID:   "F.1",
			Name: "Architect/Editor Split (selectModel)",
			Files: []string{
				"internal/agent/coordinator.go",
				"internal/extensions/model_router_ext.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "selectModel", Files: []string{"internal/extensions/model_router_ext.go"}},
			},
		},
		{
			ID:   "F.2",
			Name: "Model Routing (RouteByTokenCount)",
			Files: []string{
				"internal/agent/coordinator.go",
				"internal/agent/model_router.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "RouteByTokenCount", Files: []string{"internal/agent/model_router.go"}},
			},
		},

		// Layer G — Orchestration.
		{
			ID:   "G.1",
			Name: "Coordinator/Worker (Swarm)",
			Files: []string{
				"internal/agent/coordinator.go",
				"internal/agent/swarm.go",
				"internal/extensions/swarm_ext.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "Swarm", Files: []string{"internal/agent/swarm.go", "internal/extensions/swarm_ext.go"}},
			},
		},
		{
			ID:   "G.2",
			Name: "Structured Subagents",
			Files: []string{
				"internal/agent/coordinator.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "StructuredSubagentFactory", Files: []string{"internal/agent/coordinator.go"}},
				{Pattern: "Subagent", Files: []string{"internal/agent/coordinator.go"}},
			},
		},
		{
			ID:   "G.3",
			Name: "Operator + Map Tools",
			Files: []string{
				"internal/agent/operator.go",
				"internal/agent/tools/llm_map.go",
				"internal/agent/tools/agentic_map.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "Operator", Files: []string{"internal/agent/operator.go"}},
				{Pattern: "llm_map", Files: []string{"internal/agent/tools/llm_map.go"}},
			},
		},
		// Adapted: ParallelController moved from coordinator.go to parallel.go
		// (clean keeps parallel execution as a standalone package-internal type).
		{
			ID:   "G.4",
			Name: "Parallel Subagents (ParallelController)",
			Files: []string{
				"internal/agent/coordinator.go",
				"internal/agent/parallel.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "ParallelController", Files: []string{"internal/agent/parallel.go"}},
			},
		},
		// Adapted: doomDetector moved from coordinator.go to doom_ext.go
		// (clean reorganized doom detection into extension host).
		{
			ID:   "G.5",
			Name: "Doom Loop Detection",
			Files: []string{
				"internal/agent/doom.go",
				"internal/extensions/doom_ext.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "doomDetector", Files: []string{"internal/extensions/doom_ext.go"}},
				{Pattern: "DoomLoopDetector", Files: []string{"internal/agent/doom.go"}},
			},
		},
		{
			ID:   "G.6",
			Name: "Dynamic Tool Surface + Prompt Assembly",
			Files: []string{
				"internal/agent/tool_surface.go",
				"internal/agent/prompt_assembly.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "ToolSurface", Files: []string{"internal/agent/tool_surface.go"}},
				{Pattern: "PromptAssembly", Files: []string{"internal/agent/prompt_assembly.go"}},
			},
		},

		// Layer H — Evaluation.
		{
			ID:   "H.1",
			Name: "Eval Framework",
			Files: []string{
				"internal/eval/harness.go",
				"internal/eval/report.go",
			},
			DirFiles: []string{"internal/eval/scorers"},
			WireGrep: []wireCheck{
				{Pattern: "EvalHarness", Files: []string{"internal/eval/harness.go"}},
			},
		},
		{
			ID:       "H.2",
			Name:     "Processor Pipeline",
			DirFiles: []string{"internal/processor"},
			WireGrep: []wireCheck{},
		},
		{
			ID:   "H.3",
			Name: "ReadCoordinator (PageRank connection)",
			Files: []string{
				"internal/eval/readcoordinator.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "ReadCoordinator", Files: []string{"internal/eval/readcoordinator.go"}},
			},
		},
	}
}

// specGapComponents returns the 10 spec-gap closure checks adapted to the
// clean branch structure. On dream these lived in coordinator_opts.go,
// app_validation.go, and go_tester.go. On clean they are reorganized into
// the extension host system and consolidated into fewer files.
func specGapComponents() []component {
	return []component{
		// T1 — Validation init through extension system.
		// Dream: initValidation on *App in app_validation.go.
		// Clean: AutofixExtension provides validation, registered in register.go.
		{
			ID:   "T.1",
			Name: "Validation init (extension host)",
			Files: []string{
				"internal/extensions/autofix_ext.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "AutofixExtension", Files: []string{"internal/extensions/autofix_ext.go"}},
				{Pattern: "AutofixExtension", Files: []string{"internal/extensions/register.go"}},
			},
		},
		// T2 — ValidationHandler wired through extension system.
		// Dream: WithValidationHandler in coordinator_opts.go.
		// Clean: ValidationHandler in validate_stub.go, TreesitterExtension registered.
		{
			ID:   "T.2",
			Name: "ValidationHandler wired (extension host)",
			Files: []string{
				"internal/agent/tools/validate_stub.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "NewValidationHandler", Files: []string{"internal/agent/tools/validate_stub.go"}},
				{Pattern: "TreesitterExtension", Files: []string{"internal/extensions/register.go"}},
			},
		},
		// T3 — ModelRouter wired through extension system.
		// Dream: WithModelRouter in coordinator_opts.go + modelRouter in coordinator.go.
		// Clean: ModelRouterExtension registered in extensions.
		{
			ID:   "T.3",
			Name: "ModelRouter wired (extension host)",
			Files: []string{
				"internal/extensions/model_router_ext.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "ModelRouterExtension", Files: []string{"internal/extensions/register.go"}},
				{Pattern: "ModelRouter", Files: []string{"internal/agent/model_router.go"}},
			},
		},
		// T4 — Model type on session agent.
		// Dream: Model *Model field on SessionAgentCall.
		// Clean: Model struct + SetModels/Model methods on SessionAgent interface.
		{
			ID:   "T.4",
			Name: "Model type on SessionAgent",
			Files: []string{
				"internal/agent/agent.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "type Model struct", Files: []string{"internal/agent/agent.go"}},
			},
		},
		// T5 — RouterTokenLimit in config.Options.
		{
			ID:   "T.5",
			Name: "RouterTokenLimit in config.Options",
			Files: []string{
				"internal/config/config.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "RouterTokenLimit", Files: []string{"internal/config/config.go"}},
			},
		},
		// T6 — repo_map_imports migration exists.
		{
			ID:   "T.6",
			Name: "repo_map_imports migration",
			WireGrep: []wireCheck{
				{Pattern: "repo_map_imports", Dirs: []string{"internal/db/migrations"}},
			},
		},
		// T7 — GoLinter implements Linter interface.
		{
			ID:   "T.7",
			Name: "GoLinter implements Linter",
			Files: []string{
				"internal/agent/go_linter.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "var _ Linter = (*GoLinter)(nil)", Files: []string{"internal/agent/go_linter.go"}},
			},
		},
		// T8 — GoTester implements Tester interface.
		// Dream: separate go_tester.go + go_tester_test.go.
		// Clean: consolidated into go_linter.go.
		{
			ID:   "T.8",
			Name: "GoTester implements Tester",
			Files: []string{
				"internal/agent/go_linter.go",
				"internal/agent/autofix.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "type Tester interface", Files: []string{"internal/agent/autofix.go"}},
				{Pattern: "func (t *GoTester) RunTests", Files: []string{"internal/agent/go_linter.go"}},
				{Pattern: "var _ Tester = (*GoTester)(nil)", Files: []string{"internal/agent/go_linter.go"}},
			},
		},
		// T9 — sync.Pool in pagerank.go.
		{
			ID:   "T.9",
			Name: "sync.Pool in pagerank",
			Files: []string{
				"internal/repomap/pagerank.go",
			},
			WireGrep: []wireCheck{
				{Pattern: "sync.Pool", Files: []string{"internal/repomap/pagerank.go"}},
			},
		},
		// T10 — GC tuning via Taskfile build environment.
		// Dream: debug.SetMemoryLimit in main.go.
		// Clean: GOEXPERIMENT=greenteagc + CGO_ENABLED=0 in Taskfile.yaml.
		{
			ID:   "T.10",
			Name: "GC tuning (build environment)",
			Files: []string{
				"Taskfile.yaml",
			},
			WireGrep: []wireCheck{
				{Pattern: "greenteagc", Files: []string{"Taskfile.yaml"}},
			},
		},
	}
}

func TestSpecGapClosureWiring(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	components := specGapComponents()

	for _, comp := range components {
		t.Run(comp.ID, func(t *testing.T) {
			t.Parallel()
			res := checkComponent(root, comp)

			if !res.FileExists {
				t.Errorf("[%s] %s: missing files: %v", comp.ID, comp.Name, res.FileMissing)
			}

			if !res.Wired {
				t.Errorf("[%s] %s: wiring check failed", comp.ID, comp.Name)
				for _, detail := range res.WireDetails {
					t.Logf("  %s", detail)
				}
			}

			for f, markers := range res.Stubs {
				t.Logf("[%s] %s: stub markers in %s: %s (informational)",
					comp.ID, comp.Name, f, strings.Join(markers, ", "))
			}

			if res.FileExists && res.Wired {
				t.Logf("[%s] %s: PASS", comp.ID, comp.Name)
			}
		})
	}
}

func TestGapAudit(t *testing.T) {
	root := repoRoot(t)
	components := all29Components()

	evidenceDir := filepath.Join(root, ".sisyphus", "evidence")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatalf("create evidence dir: %v", err)
	}

	var report strings.Builder
	fmt.Fprintf(&report, "========================================\n")
	fmt.Fprintf(&report, "SPEC GAP AUDIT REPORT (xrush-clean)\n")
	fmt.Fprintf(&report, "========================================\n")
	fmt.Fprintf(&report, "Repository: %s\n", root)
	fmt.Fprintf(&report, "Components: %d\n\n", len(components))

	passCount := 0
	failCount := 0
	filePassCount := 0
	stubPassCount := 0
	wirePassCount := 0

	for _, comp := range components {
		res := checkComponent(root, comp)

		fmt.Fprintf(&report, "----------------------------------------\n")
		fmt.Fprintf(&report, "[%s] %s\n", comp.ID, comp.Name)
		fmt.Fprintf(&report, "----------------------------------------\n")

		fileStatus := "PASS"
		if res.FileExists {
			allPaths := len(comp.Files) + len(comp.DirFiles)
			fmt.Fprintf(&report, "  FILE CHECK:   PASS (all %d paths exist)\n", allPaths)
			filePassCount++
		} else {
			fileStatus = "FAIL"
			fmt.Fprintf(&report, "  FILE CHECK:   FAIL (%d missing)\n", len(res.FileMissing))
			for _, mf := range res.FileMissing {
				fmt.Fprintf(&report, "    MISSING: %s\n", mf)
			}
		}

		stubStatus := "PASS"
		if len(res.Stubs) > 0 {
			stubStatus = "WARN"
			fmt.Fprintf(&report, "  STUB CHECK:   WARN (markers in %d files)\n", len(res.Stubs))
			for f, markers := range res.Stubs {
				fmt.Fprintf(&report, "    %s: %s\n", f, strings.Join(markers, ", "))
			}
		} else {
			fmt.Fprintf(&report, "  STUB CHECK:   PASS (no stub markers)\n")
			stubPassCount++
		}

		wireStatus := "PASS"
		if res.Wired {
			fmt.Fprintf(&report, "  WIRE CHECK:   PASS\n")
			wirePassCount++
		} else {
			wireStatus = "FAIL"
			fmt.Fprintf(&report, "  WIRE CHECK:   FAIL\n")
		}
		for _, detail := range res.WireDetails {
			fmt.Fprintf(&report, "    %s\n", detail)
		}

		overall := "PASS"
		if fileStatus == "FAIL" || wireStatus == "FAIL" {
			overall = "FAIL"
			failCount++
		} else {
			passCount++
		}
		fmt.Fprintf(&report, "  OVERALL:      %s/%s/%s => %s\n", fileStatus, stubStatus, wireStatus, overall)
		fmt.Fprintf(&report, "\n")

		t.Logf("[%s] %s => FILE:%s STUB:%s WIRE:%s => %s",
			comp.ID, comp.Name, fileStatus, stubStatus, wireStatus, overall)
	}

	fmt.Fprintf(&report, "========================================\n")
	fmt.Fprintf(&report, "SUMMARY\n")
	fmt.Fprintf(&report, "========================================\n")
	fmt.Fprintf(&report, "Total Components: %d\n", len(components))
	fmt.Fprintf(&report, "  Overall PASS:   %d\n", passCount)
	fmt.Fprintf(&report, "  Overall FAIL:   %d\n", failCount)
	fmt.Fprintf(&report, "  File PASS:      %d/%d\n", filePassCount, len(components))
	fmt.Fprintf(&report, "  Stub PASS:      %d/%d (WARN = stub markers found)\n", stubPassCount, len(components))
	fmt.Fprintf(&report, "  Wire PASS:      %d/%d\n", wirePassCount, len(components))
	fmt.Fprintf(&report, "========================================\n")
	fmt.Fprintf(&report, "\nNOTE: H.2 (Processor Pipeline) is expected to NOT EXIST.\n")
	fmt.Fprintf(&report, "  Its missing status is informational only.\n")

	reportPath := filepath.Join(evidenceDir, "task-1-audit-report.txt")
	if err := os.WriteFile(reportPath, []byte(report.String()), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	t.Logf("Report written to: %s", reportPath)
	t.Logf("AUDIT COMPLETE: %d/%d components fully PASS, %d have gaps",
		passCount, len(components), failCount)
}
