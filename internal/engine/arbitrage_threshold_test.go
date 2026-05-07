package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sportstips/internal/store"
)

func TestDetectArbitrage_RespectsThreshold(t *testing.T) {
	// arb_sum = 1/2.20 + 1/3.60 + 1/4.00 = 0.982 → profitPct ≈ 1.8%
	odds := []store.NormalizedOdds{
		{Bookmaker: "bet365", Market: "1x2", OddsHome: 2.20, OddsDraw: 3.60, OddsAway: 4.00},
	}

	// threshold 1% → should find
	signals1 := DetectArbitrage("match-1", odds, 0.01)
	assert.Len(t, signals1, 1)

	// threshold 5% → should NOT find (profit is only ~1.8%)
	signals5 := DetectArbitrage("match-1", odds, 0.05)
	assert.Empty(t, signals5)
}
