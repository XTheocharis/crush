package repomap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFitToBudgetParityModeComparatorAcceptance(t *testing.T) {
	t.Parallel()

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "a.go", Ident: "A"},
		{Stage: stageRankedDefs, File: "b.go", Ident: "B"},
		{Stage: stageGraphNodes, File: "c.go"},
		{Stage: stageRemainingFiles, File: "d.go"},
	}

	res, err := FitToBudget(context.Background(), entries, BudgetProfile{
		ParityMode:   true,
		TokenBudget:  12,
		Model:        "m",
		LanguageHint: "default",
	}, fakeCounter{out: 12})
	require.NoError(t, err)
	require.True(t, res.ComparatorAccepted)
	require.InDelta(t, 12, res.ParityTokens, 1e-9)
}

func TestFitToBudgetEnhancementModeSafetyGuard(t *testing.T) {
	t.Parallel()

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "a.go", Ident: "A"},
		{Stage: stageRankedDefs, File: "b.go", Ident: "B"},
		{Stage: stageGraphNodes, File: "c.go"},
		{Stage: stageRemainingFiles, File: "d.go"},
	}

	res, err := FitToBudget(context.Background(), entries, BudgetProfile{
		ParityMode:   false,
		TokenBudget:  5,
		Model:        "m",
		LanguageHint: "default",
	}, nil)
	require.NoError(t, err)
	require.LessOrEqual(t, res.SafetyTokens, 5)
	require.NotEmpty(t, res.Entries)
}

func TestFitToBudgetTrimOrderFromTail(t *testing.T) {
	t.Parallel()

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "a.go", Ident: "A"},
		{Stage: stageGraphNodes, File: "c.go"},
		{Stage: stageRemainingFiles, File: "d.go"},
		{Stage: stageRemainingFiles, File: "e.go"},
	}

	res, err := FitToBudget(context.Background(), entries, BudgetProfile{
		ParityMode:   false,
		TokenBudget:  2,
		Model:        "m",
		LanguageHint: "default",
	}, nil)
	require.NoError(t, err)
	require.NotEmpty(t, res.TrimmedStages)
	// Tail-first trimming should start at stage3 entries.
	require.Equal(t, stageRemainingFiles, res.TrimmedStages[0])
}

func TestFitToBudgetInitialGuessAndEmptyInputs(t *testing.T) {
	t.Parallel()

	res, err := FitToBudget(context.Background(), nil, BudgetProfile{TokenBudget: 10}, nil)
	require.NoError(t, err)
	require.Empty(t, res.Entries)

	res, err = FitToBudget(context.Background(), []StageEntry{{Stage: stageSpecialPrelude, File: "README.md"}}, BudgetProfile{TokenBudget: 0}, nil)
	require.NoError(t, err)
	require.Empty(t, res.Entries)
}

func TestFitToBudgetPropagatesTokenCounterError(t *testing.T) {
	t.Parallel()

	_, err := FitToBudget(context.Background(), []StageEntry{{Stage: stageRankedDefs, File: "a.go", Ident: "A"}}, BudgetProfile{
		ParityMode:   true,
		TokenBudget:  10,
		Model:        "m",
		LanguageHint: "default",
	}, fakeCounter{err: context.Canceled})
	require.Error(t, err)
}
