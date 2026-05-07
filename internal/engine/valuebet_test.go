package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sportstips/internal/predictions"
	"sportstips/internal/store"
)

func TestDetectValueBets_Found(t *testing.T) {
	odds := []store.NormalizedOdds{
		{Bookmaker: "bet365", Market: "1x2", OddsHome: 2.30, OddsDraw: 3.50, OddsAway: 3.00},
	}
	pred := predictions.Prediction{
		ProbHome:     0.55,
		ProbDraw:     0.25,
		ProbAway:     0.20,
		ModelVersion: "lr_test",
	}

	signals := DetectValueBets("match-1", odds, pred, 0.05)
	assert.Len(t, signals, 1)
	assert.Equal(t, "value_bet", signals[0].Type)
	assert.Equal(t, "home", signals[0].Outcome)
	assert.InDelta(t, 0.115, signals[0].Edge, 0.001)
}

func TestDetectValueBets_NotFound(t *testing.T) {
	odds := []store.NormalizedOdds{
		{Bookmaker: "bet365", Market: "1x2", OddsHome: 2.00, OddsDraw: 3.50, OddsAway: 3.00},
	}
	pred := predictions.Prediction{
		ProbHome:     0.50,
		ProbDraw:     0.286,
		ProbAway:     0.214,
		ModelVersion: "lr_test",
	}

	signals := DetectValueBets("match-1", odds, pred, 0.05)
	assert.Empty(t, signals)
}

func TestDetectValueBets_BelowThreshold(t *testing.T) {
	odds := []store.NormalizedOdds{
		{Bookmaker: "bet365", Market: "1x2", OddsHome: 2.10, OddsDraw: 3.50, OddsAway: 3.00},
	}
	pred := predictions.Prediction{
		ProbHome:     0.50,
		ProbDraw:     0.28,
		ProbAway:     0.22,
		ModelVersion: "lr_test",
	}

	signals := DetectValueBets("match-1", odds, pred, 0.05)
	assert.Empty(t, signals)
}
