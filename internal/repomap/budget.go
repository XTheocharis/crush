package repomap

import (
	"context"
	"math"
	"strings"
)

// BudgetProfile controls fit-mode behavior.
type BudgetProfile struct {
	ParityMode   bool
	TokenBudget  int
	Model        string
	LanguageHint string
}

// BudgetFitResult is the selected best-fit candidate.
type BudgetFitResult struct {
	Entries            []StageEntry
	TrimmedStages      []int
	ParityTokens       float64
	SafetyTokens       int
	ComparatorAccepted bool
	ComparatorDelta    float64
}

type budgetCandidate struct {
	entries            []StageEntry
	trimmedStages      []int
	parityTokens       float64
	safetyTokens       int
	comparatorAccepted bool
	comparatorDelta    float64
}

// FitToBudget performs binary-search-like fitting with profile-specific
// acceptance behavior and best-candidate tracking.
func FitToBudget(
	ctx context.Context,
	entries []StageEntry,
	profile BudgetProfile,
	counter TokenCounter,
) (BudgetFitResult, error) {
	rendered := func(items []StageEntry) string {
		return renderStageEntries(items)
	}

	if profile.TokenBudget <= 0 {
		return BudgetFitResult{}, nil
	}

	n := len(entries)
	if n == 0 {
		return BudgetFitResult{}, nil
	}

	bestValid := budgetCandidate{}
	haveBestValid := false
	bestAny := budgetCandidate{comparatorDelta: math.Inf(1)}
	trimmedByPrefix := make(map[int][]int)

	initialGuess := minInt(profile.TokenBudget/25, n)
	if initialGuess <= 0 {
		initialGuess = minInt(1, n)
	}
	lo, hi := 0, n
	mid := initialGuess

	seenMid := make(map[int]struct{})
	for lo <= hi {
		if mid < 0 {
			mid = 0
		}
		if mid > n {
			mid = n
		}

		if _, seen := seenMid[mid]; seen {
			break
		}
		seenMid[mid] = struct{}{}

		prefix := append([]StageEntry(nil), entries[:mid]...)
		trimmedStages := trimmedStagesForPrefix(entries, mid, trimmedByPrefix)

		text := rendered(prefix)
		metrics, err := CountParityAndSafetyTokens(ctx, counter, profile.Model, text, profile.LanguageHint)
		if err != nil {
			return BudgetFitResult{}, err
		}
		delta := parityComparatorDelta(metrics.ParityTokens, profile.TokenBudget)

		cand := budgetCandidate{
			entries:            prefix,
			trimmedStages:      trimmedStages,
			parityTokens:       metrics.ParityTokens,
			safetyTokens:       metrics.SafetyTokens,
			comparatorAccepted: delta <= 0.15,
			comparatorDelta:    delta,
		}

		if cand.comparatorDelta < bestAny.comparatorDelta {
			bestAny = cand
		}

		isValid := false
		if profile.ParityMode {
			isValid = cand.comparatorAccepted
		} else {
			isValid = cand.safetyTokens <= profile.TokenBudget
			if isValid && delta <= 0.15 {
				haveBestValid = true
				bestValid = cand
				break
			}
		}
		if isValid {
			if !haveBestValid || betterCandidate(cand, bestValid, profile.ParityMode) {
				haveBestValid = true
				bestValid = cand
			}
		}

		tooBig := false
		if profile.ParityMode {
			tooBig = metrics.ParityTokens > float64(profile.TokenBudget)
		} else {
			tooBig = metrics.SafetyTokens > profile.TokenBudget
		}

		if tooBig {
			hi = mid - 1
		} else {
			lo = mid + 1
		}

		if lo > hi {
			break
		}
		mid = lo + (hi-lo)/2
	}

	picked := bestAny
	if haveBestValid {
		picked = bestValid
	}

	return BudgetFitResult{
		Entries:            picked.entries,
		TrimmedStages:      picked.trimmedStages,
		ParityTokens:       picked.parityTokens,
		SafetyTokens:       picked.safetyTokens,
		ComparatorAccepted: picked.comparatorAccepted,
		ComparatorDelta:    picked.comparatorDelta,
	}, nil
}

func betterCandidate(a, b budgetCandidate, parityMode bool) bool {
	if parityMode {
		if a.comparatorAccepted != b.comparatorAccepted {
			return a.comparatorAccepted
		}
		if a.comparatorDelta != b.comparatorDelta {
			return a.comparatorDelta < b.comparatorDelta
		}
		return len(a.entries) > len(b.entries)
	}

	if a.safetyTokens != b.safetyTokens {
		return a.safetyTokens > b.safetyTokens
	}
	if a.comparatorDelta != b.comparatorDelta {
		return a.comparatorDelta < b.comparatorDelta
	}
	return len(a.entries) > len(b.entries)
}

func trimmedStagesForPrefix(entries []StageEntry, keep int, cache map[int][]int) []int {
	if got, ok := cache[keep]; ok {
		return append([]int(nil), got...)
	}
	if keep >= len(entries) {
		cache[keep] = nil
		return nil
	}
	trimmed := make([]int, 0, len(entries)-keep)
	for i := len(entries) - 1; i >= keep; i-- {
		trimmed = append(trimmed, entries[i].Stage)
	}
	cache[keep] = append([]int(nil), trimmed...)
	return trimmed
}

func renderStageEntries(entries []StageEntry) string {
	if len(entries) == 0 {
		return ""
	}
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		switch e.Stage {
		case stageSpecialPrelude:
			lines = append(lines, "S0|"+e.File)
		case stageRankedDefs:
			lines = append(lines, "S1|"+e.File+"|"+e.Ident)
		case stageGraphNodes:
			lines = append(lines, "S2|"+e.File)
		case stageRemainingFiles:
			lines = append(lines, "S3|"+e.File)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	var out strings.Builder
	for _, l := range lines {
		out.WriteString(l + "\n")
	}
	return out.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
