package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sportstips/internal/store"
)

func TestDetectArbitrage_Found(t *testing.T) {
	odds := []store.NormalizedOdds{
		{Bookmaker: "bet365", Market: "1x2", OddsHome: 2.20, OddsDraw: 3.60, OddsAway: 4.00},
		{Bookmaker: "betway", Market: "1x2", OddsHome: 2.10, OddsDraw: 3.50, OddsAway: 3.80},
	}

	signals := DetectArbitrage("match-1", odds, 0.01)

	assert.Len(t, signals, 1)
	assert.Equal(t, "arbitrage", signals[0].Type)
	assert.Equal(t, "1x2", signals[0].Market)
	assert.Greater(t, signals[0].ProfitPct, 0.0)
}

func TestDetectArbitrage_NotFound(t *testing.T) {
	odds := []store.NormalizedOdds{
		{Bookmaker: "bet365", Market: "1x2", OddsHome: 1.80, OddsDraw: 3.20, OddsAway: 4.00},
	}

	signals := DetectArbitrage("match-1", odds, 0.01)
	assert.Empty(t, signals)
}

func TestDetectArbitrage_BelowThreshold(t *testing.T) {
	odds := []store.NormalizedOdds{
		{Bookmaker: "bet365", Market: "1x2", OddsHome: 2.10, OddsDraw: 3.50, OddsAway: 4.20},
	}
	signals := DetectArbitrage("match-1", odds, 0.01)
	assert.Empty(t, signals)
}
