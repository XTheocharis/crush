package tools

import (
	"context"
	"testing"

	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
	"github.com/stretchr/testify/require"
)

func TestConvertSeverity(t *testing.T) {
	tests := []struct {
		name     string
		input    protocol.DiagnosticSeverity
		expected DiagnosticSeverity
	}{
		{"error", protocol.SeverityError, SeverityError},
		{"warning", protocol.SeverityWarning, SeverityWarning},
		{"info", protocol.SeverityInformation, SeverityInfo},
		{"hint", protocol.SeverityHint, SeverityHint},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertSeverity(tt.input)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestDiagnosticInfoKey(t *testing.T) {
	d1 := DiagnosticInfo{
		FilePath:  "foo.go",
		Line:      10,
		Character: 5,
		Severity:  SeverityError,
		Message:   "undefined: x",
	}
	d2 := DiagnosticInfo{
		FilePath:  "foo.go",
		Line:      10,
		Character: 5,
		Severity:  SeverityWarning,
		Message:   "undefined: x",
	}

	require.Equal(t, d1.Key(), d2.Key())

	d3 := DiagnosticInfo{
		FilePath:  "foo.go",
		Line:      11,
		Character: 5,
		Severity:  SeverityError,
		Message:   "undefined: x",
	}
	require.NotEqual(t, d1.Key(), d3.Key())
}

func TestComputeDiff(t *testing.T) {
	errA := DiagnosticInfo{
		FilePath: "a.go", Line: 1, Character: 0,
		Severity: SeverityError, Message: "err A",
	}
	errB := DiagnosticInfo{
		FilePath: "a.go", Line: 2, Character: 0,
		Severity: SeverityError, Message: "err B",
	}
	warnA := DiagnosticInfo{
		FilePath: "a.go", Line: 3, Character: 0,
		Severity: SeverityWarning, Message: "warn A",
	}

	t.Run("no change", func(t *testing.T) {
		baseline := map[diagnosticKey]DiagnosticInfo{
			errA.Key(): errA,
		}
		post := map[diagnosticKey]DiagnosticInfo{
			errA.Key(): errA,
		}
		diff := computeDiff(baseline, post)
		require.Len(t, diff.Unchanged, 1)
		require.Empty(t, diff.Added)
		require.Empty(t, diff.Removed)
	})

	t.Run("new error added", func(t *testing.T) {
		baseline := map[diagnosticKey]DiagnosticInfo{
			errA.Key(): errA,
		}
		post := map[diagnosticKey]DiagnosticInfo{
			errA.Key(): errA,
			errB.Key(): errB,
		}
		diff := computeDiff(baseline, post)
		require.Len(t, diff.Unchanged, 1)
		require.Len(t, diff.Added, 1)
		require.Empty(t, diff.Removed)
		require.Equal(t, errB, diff.Added[0])
	})

	t.Run("error removed", func(t *testing.T) {
		baseline := map[diagnosticKey]DiagnosticInfo{
			errA.Key(): errA,
			errB.Key(): errB,
		}
		post := map[diagnosticKey]DiagnosticInfo{
			errA.Key(): errA,
		}
		diff := computeDiff(baseline, post)
		require.Len(t, diff.Unchanged, 1)
		require.Empty(t, diff.Added)
		require.Len(t, diff.Removed, 1)
		require.Equal(t, errB, diff.Removed[0])
	})

	t.Run("warning added", func(t *testing.T) {
		baseline := map[diagnosticKey]DiagnosticInfo{
			errA.Key(): errA,
		}
		post := map[diagnosticKey]DiagnosticInfo{
			errA.Key():  errA,
			warnA.Key(): warnA,
		}
		diff := computeDiff(baseline, post)
		require.Len(t, diff.Unchanged, 1)
		require.Len(t, diff.Added, 1)
		require.Empty(t, diff.Removed)
	})
}

func TestNewDiagnosticGate_NilManager(t *testing.T) {
	gate := NewDiagnosticGate(nil)
	require.NotNil(t, gate)
	require.Empty(t, gate.baseline)
}

func TestDiagnosticGate_NilManager_CaptureAndCompare(t *testing.T) {
	gate := NewDiagnosticGate(nil)

	gate.CaptureBaseline(context.TODO(), []string{"foo.go"})

	result := gate.Compare(context.TODO(), []string{"foo.go"})
	require.True(t, result.Pass)
	require.True(t, result.NoLSP)
	require.Empty(t, result.NewErrors)
	require.Empty(t, result.Warnings)
	require.Empty(t, result.Diff.Added)
	require.Empty(t, result.Diff.Removed)
}

func TestGateResult_Pass_NoChange(t *testing.T) {
	result := GateResult{
		Pass: true,
		Diff: DiagnosticDiff{},
	}
	require.Contains(t, result.Message(), "no diagnostic changes")
}

func TestGateResult_Pass_WithWarnings(t *testing.T) {
	result := GateResult{
		Pass: true,
		Warnings: []DiagnosticInfo{
			{FilePath: "a.go", Severity: SeverityWarning, Message: "unused var"},
		},
		Diff: DiagnosticDiff{
			Added: []DiagnosticInfo{
				{FilePath: "a.go", Severity: SeverityWarning, Message: "unused var"},
			},
		},
	}
	require.Contains(t, result.Message(), "passed with 1 new warning")
}

func TestGateResult_Fail_NewErrors(t *testing.T) {
	result := GateResult{
		Pass: false,
		NewErrors: []DiagnosticInfo{
			{FilePath: "a.go", Severity: SeverityError, Message: "undefined: x"},
		},
	}
	require.Contains(t, result.Message(), "FAILED")
	require.Contains(t, result.Message(), "1 new error")
}

func TestGateResult_NoLSP(t *testing.T) {
	result := GateResult{Pass: true, NoLSP: true}
	require.Contains(t, result.Message(), "no LSP servers available")
}

func TestGateResult_Pass_WithAddedAndRemoved(t *testing.T) {
	result := GateResult{
		Pass: true,
		Diff: DiagnosticDiff{
			Added: []DiagnosticInfo{
				{FilePath: "a.go", Severity: SeverityInfo, Message: "info"},
			},
			Removed: []DiagnosticInfo{
				{FilePath: "a.go", Severity: SeverityWarning, Message: "old warn"},
			},
		},
	}
	require.Contains(t, result.Message(), "1 added, 1 removed")
}

func TestClassifyAddedDiagnostics(t *testing.T) {
	baseline := map[diagnosticKey]DiagnosticInfo{
		{FilePath: "a.go", Line: 1, Message: "pre-existing error"}: {
			FilePath: "a.go", Line: 1, Severity: SeverityError, Message: "pre-existing error",
		},
	}
	post := map[diagnosticKey]DiagnosticInfo{
		{FilePath: "a.go", Line: 1, Message: "pre-existing error"}: {
			FilePath: "a.go", Line: 1, Severity: SeverityError, Message: "pre-existing error",
		},
		{FilePath: "a.go", Line: 5, Message: "new error"}: {
			FilePath: "a.go", Line: 5, Severity: SeverityError, Message: "new error",
		},
		{FilePath: "a.go", Line: 8, Message: "new warning"}: {
			FilePath: "a.go", Line: 8, Severity: SeverityWarning, Message: "new warning",
		},
		{FilePath: "a.go", Line: 12, Message: "new info"}: {
			FilePath: "a.go", Line: 12, Severity: SeverityInfo, Message: "new info",
		},
	}

	diff := computeDiff(baseline, post)

	result := GateResult{Pass: true}
	for _, di := range diff.Added {
		switch di.Severity {
		case SeverityError:
			result.NewErrors = append(result.NewErrors, di)
			result.Pass = false
		case SeverityWarning:
			result.Warnings = append(result.Warnings, di)
		}
	}

	require.False(t, result.Pass, "gate should fail when new errors are added")
	require.Len(t, result.NewErrors, 1)
	require.Equal(t, "new error", result.NewErrors[0].Message)
	require.Len(t, result.Warnings, 1)
	require.Equal(t, "new warning", result.Warnings[0].Message)
	require.Len(t, diff.Unchanged, 1, "pre-existing error should be unchanged")
}

func TestPreExistingErrors_Pass(t *testing.T) {
	baseline := map[diagnosticKey]DiagnosticInfo{
		{FilePath: "a.go", Line: 1, Message: "pre-existing"}: {
			FilePath: "a.go", Line: 1, Severity: SeverityError, Message: "pre-existing",
		},
	}
	post := map[diagnosticKey]DiagnosticInfo{
		{FilePath: "a.go", Line: 1, Message: "pre-existing"}: {
			FilePath: "a.go", Line: 1, Severity: SeverityError, Message: "pre-existing",
		},
	}

	diff := computeDiff(baseline, post)
	result := GateResult{Pass: true}
	for _, di := range diff.Added {
		if di.Severity == SeverityError {
			result.NewErrors = append(result.NewErrors, di)
			result.Pass = false
		}
	}
	require.True(t, result.Pass, "pre-existing errors should not fail the gate")
	require.Empty(t, result.NewErrors)
	require.Len(t, diff.Unchanged, 1)
}

func TestNewWarningsOnly_PassWithWarning(t *testing.T) {
	baseline := map[diagnosticKey]DiagnosticInfo{}
	post := map[diagnosticKey]DiagnosticInfo{
		{FilePath: "a.go", Line: 1, Message: "warn"}: {
			FilePath: "a.go", Line: 1, Severity: SeverityWarning, Message: "warn",
		},
	}

	diff := computeDiff(baseline, post)
	result := GateResult{Pass: true}
	for _, di := range diff.Added {
		switch di.Severity {
		case SeverityError:
			result.NewErrors = append(result.NewErrors, di)
			result.Pass = false
		case SeverityWarning:
			result.Warnings = append(result.Warnings, di)
		}
	}

	require.True(t, result.Pass, "new warnings should not fail the gate")
	require.Empty(t, result.NewErrors)
	require.Len(t, result.Warnings, 1)
}

func TestProtocolDiagToInfo(t *testing.T) {
	pd := protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{Line: 4, Character: 2},
		},
		Severity: protocol.SeverityError,
		Message:  "something broke",
	}

	info := protocolDiagToInfo("test.go", pd)
	require.Equal(t, "test.go", info.FilePath)
	require.Equal(t, uint32(4), info.Line)
	require.Equal(t, uint32(2), info.Character)
	require.Equal(t, SeverityError, info.Severity)
	require.Equal(t, "something broke", info.Message)
}

func TestImportCascade_DirectImporter(t *testing.T) {
	editedFile := "/project/pkg/foo/foo.go"
	importerFile := "/project/cmd/main.go"

	findImporters := func(packagePath string) ([]string, error) {
		require.Equal(t, "/project/pkg/foo", packagePath)
		return []string{importerFile}, nil
	}

	getDiags := func(filePath string) ([]DiagnosticInfo, error) {
		return []DiagnosticInfo{
			{FilePath: filePath, Line: 5, Severity: SeverityError, Message: "cannot use x as y"},
		}, nil
	}

	result, err := CascadeDiagnostics(
		context.Background(),
		editedFile,
		getDiags,
		findImporters,
		ImportCascadeConfig{MaxDepth: 1},
	)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Contains(t, result, importerFile)
	require.Len(t, result[importerFile], 1)
	require.Equal(t, "cannot use x as y", result[importerFile][0].Message)
}

func TestImportCascade_NoImporters(t *testing.T) {
	findImporters := func(string) ([]string, error) {
		return nil, nil
	}

	getDiags := func(string) ([]DiagnosticInfo, error) {
		t.Fatal("getDiagnostics should not be called with no importers")
		return nil, nil
	}

	result, err := CascadeDiagnostics(
		context.Background(),
		"/project/pkg/foo/foo.go",
		getDiags,
		findImporters,
		ImportCascadeConfig{MaxDepth: 1},
	)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestImportCascade_DepthLimited(t *testing.T) {
	directImporter := "/project/cmd/main.go"
	indirectImporter := "/project/cmd/app.go"
	checked := map[string]bool{}

	findImporters := func(packagePath string) ([]string, error) {
		switch packagePath {
		case "/project/pkg/foo":
			return []string{directImporter}, nil
		case "/project/cmd":
			return []string{indirectImporter}, nil
		default:
			return nil, nil
		}
	}

	getDiags := func(filePath string) ([]DiagnosticInfo, error) {
		checked[filePath] = true
		return []DiagnosticInfo{
			{FilePath: filePath, Severity: SeverityError, Message: "err"},
		}, nil
	}

	result, err := CascadeDiagnostics(
		context.Background(),
		"/project/pkg/foo/foo.go",
		getDiags,
		findImporters,
		ImportCascadeConfig{MaxDepth: 1},
	)
	require.NoError(t, err)
	require.Contains(t, result, directImporter)
	require.NotContains(t, result, indirectImporter)
	require.True(t, checked[directImporter])
	require.False(t, checked[indirectImporter])
}

func TestImportCascade_DefaultConfig(t *testing.T) {
	findImportersCalled := false
	importerFile := "/project/cmd/main.go"

	findImporters := func(string) ([]string, error) {
		findImportersCalled = true
		return []string{importerFile}, nil
	}

	getDiags := func(filePath string) ([]DiagnosticInfo, error) {
		return []DiagnosticInfo{
			{FilePath: filePath, Severity: SeverityError, Message: "err"},
		}, nil
	}

	result, err := CascadeDiagnostics(
		context.Background(),
		"/project/pkg/foo/foo.go",
		getDiags,
		findImporters,
		ImportCascadeConfig{},
	)
	require.NoError(t, err)
	require.True(t, findImportersCalled)
	require.Len(t, result, 1)
	require.Contains(t, result, importerFile)
}

func TestSeverityFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		filter        DiagnosticSeverity
		addedDiags    []DiagnosticInfo
		wantPass      bool
		wantNewErrors int
		wantWarnings  int
	}{
		{
			name:   "error_filter_only_errors",
			filter: SeverityError,
			addedDiags: []DiagnosticInfo{
				{FilePath: "a.go", Line: 1, Severity: SeverityError, Message: "err"},
				{FilePath: "a.go", Line: 2, Severity: SeverityWarning, Message: "warn"},
				{FilePath: "a.go", Line: 3, Severity: SeverityInfo, Message: "info"},
				{FilePath: "a.go", Line: 4, Severity: SeverityHint, Message: "hint"},
			},
			wantPass:      false,
			wantNewErrors: 1,
			wantWarnings:  0,
		},
		{
			name:   "warning_filter_errors_and_warnings",
			filter: SeverityWarning,
			addedDiags: []DiagnosticInfo{
				{FilePath: "a.go", Line: 1, Severity: SeverityError, Message: "err"},
				{FilePath: "a.go", Line: 2, Severity: SeverityWarning, Message: "warn"},
				{FilePath: "a.go", Line: 3, Severity: SeverityInfo, Message: "info"},
				{FilePath: "a.go", Line: 4, Severity: SeverityHint, Message: "hint"},
			},
			wantPass:      false,
			wantNewErrors: 1,
			wantWarnings:  1,
		},
		{
			name:   "info_filter_up_to_info",
			filter: SeverityInfo,
			addedDiags: []DiagnosticInfo{
				{FilePath: "a.go", Line: 1, Severity: SeverityError, Message: "err"},
				{FilePath: "a.go", Line: 2, Severity: SeverityWarning, Message: "warn"},
				{FilePath: "a.go", Line: 3, Severity: SeverityInfo, Message: "info"},
				{FilePath: "a.go", Line: 4, Severity: SeverityHint, Message: "hint"},
			},
			wantPass:      false,
			wantNewErrors: 1,
			wantWarnings:  2,
		},
		{
			name:   "hint_filter_all_reported",
			filter: SeverityHint,
			addedDiags: []DiagnosticInfo{
				{FilePath: "a.go", Line: 1, Severity: SeverityError, Message: "err"},
				{FilePath: "a.go", Line: 2, Severity: SeverityWarning, Message: "warn"},
				{FilePath: "a.go", Line: 3, Severity: SeverityInfo, Message: "info"},
				{FilePath: "a.go", Line: 4, Severity: SeverityHint, Message: "hint"},
			},
			wantPass:      false,
			wantNewErrors: 1,
			wantWarnings:  3,
		},
		{
			name:   "default_filter_warn_only",
			filter: 0,
			addedDiags: []DiagnosticInfo{
				{FilePath: "a.go", Line: 1, Severity: SeverityError, Message: "err"},
				{FilePath: "a.go", Line: 2, Severity: SeverityWarning, Message: "warn"},
				{FilePath: "a.go", Line: 3, Severity: SeverityInfo, Message: "info"},
				{FilePath: "a.go", Line: 4, Severity: SeverityHint, Message: "hint"},
			},
			wantPass:      false,
			wantNewErrors: 1,
			wantWarnings:  1,
		},
		{
			name:   "error_filter_warnings_only_passes",
			filter: SeverityError,
			addedDiags: []DiagnosticInfo{
				{FilePath: "a.go", Line: 1, Severity: SeverityWarning, Message: "warn"},
				{FilePath: "a.go", Line: 2, Severity: SeverityInfo, Message: "info"},
			},
			wantPass:      true,
			wantNewErrors: 0,
			wantWarnings:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gate *DiagnosticGate
			if tt.name == "default_filter_warn_only" {
				gate = NewDiagnosticGate(nil)
			} else {
				gate = NewDiagnosticGate(nil, WithSeverityFilter(tt.filter))
			}

			baseline := map[diagnosticKey]DiagnosticInfo{}
			post := map[diagnosticKey]DiagnosticInfo{}
			for _, di := range tt.addedDiags {
				post[di.Key()] = di
			}

			diff := computeDiff(baseline, post)
			result := GateResult{Pass: true}
			for _, di := range diff.Added {
				if di.Severity > gate.SeverityFilter() {
					continue
				}
				if di.Severity == SeverityError {
					result.NewErrors = append(result.NewErrors, di)
					result.Pass = false
				} else {
					result.Warnings = append(result.Warnings, di)
				}
			}

			require.Equal(t, tt.wantPass, result.Pass)
			require.Len(t, result.NewErrors, tt.wantNewErrors)
			require.Len(t, result.Warnings, tt.wantWarnings)
		})
	}
}

func TestParseSeverityFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected DiagnosticSeverity
	}{
		{"error", SeverityError},
		{"warning", SeverityWarning},
		{"info", SeverityInfo},
		{"hint", SeverityHint},
		{"", SeverityWarning},
		{"unknown", SeverityWarning},
		{"ERROR", SeverityWarning},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, ParseSeverityFilter(tt.input))
		})
	}
}

func TestSeverityFilterDefault(t *testing.T) {
	t.Parallel()

	gate := NewDiagnosticGate(nil)
	require.Equal(t, SeverityWarning, gate.SeverityFilter())

	gateExplicit := NewDiagnosticGate(nil, WithSeverityFilter(SeverityError))
	require.Equal(t, SeverityError, gateExplicit.SeverityFilter())
}
