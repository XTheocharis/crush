package repomap

import "math"

func parityComparatorDelta(parityTokens float64, budget int) float64 {
	if budget <= 0 {
		return math.Inf(1)
	}
	return math.Abs(parityTokens-float64(budget)) / float64(budget)
}
