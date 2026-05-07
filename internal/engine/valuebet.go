package engine

import (
	"encoding/json"
	"fmt"

	"sportstips/internal/predictions"
	"sportstips/internal/store"
)

// ValueBetSignal represents a detected value bet opportunity.
type ValueBetSignal struct {
	MatchID      string
	Type         string
	Market       string
	Outcome      string
	Bookmaker    string
	Odds         float64
	ImpliedProb  float64
	ModelProb    float64
	Edge         float64
	KellyFull    float64
	KellyHalf    float64
	ModelVersion string
}

// DetectValueBets finds value bets for a match given current odds and ML prediction.
// Only checks 1x2 market. minEdge: minimum model_prob - implied_prob (e.g. 0.05 = 5%).
func DetectValueBets(
	matchID string,
	odds []store.NormalizedOdds,
	pred predictions.Prediction,
	minEdge float64,
) []ValueBetSignal {
	if pred.ModelVersion == "" {
		return nil
	}

	var signals []ValueBetSignal

	for _, o := range odds {
		if o.Market != "1x2" {
			continue
		}

		type candidate struct {
			outcome   string
			bookmaker string
			oddsVal   float64
			modelProb float64
		}

		candidates := []candidate{
			{"home", o.Bookmaker, o.OddsHome, pred.ProbHome},
			{"draw", o.Bookmaker, o.OddsDraw, pred.ProbDraw},
			{"away", o.Bookmaker, o.OddsAway, pred.ProbAway},
		}

		for _, c := range candidates {
			if c.oddsVal <= 1.0 || c.modelProb <= 0 {
				continue
			}
			k := Kelly(c.oddsVal, c.modelProb)
			if !k.IsValueBet || k.Edge < minEdge {
				continue
			}
			signals = append(signals, ValueBetSignal{
				MatchID:      matchID,
				Type:         "value_bet",
				Market:       "1x2",
				Outcome:      c.outcome,
				Bookmaker:    c.bookmaker,
				Odds:         c.oddsVal,
				ImpliedProb:  k.ImpliedProb,
				ModelProb:    c.modelProb,
				Edge:         k.Edge,
				KellyFull:    k.KellyFull,
				KellyHalf:    k.KellyHalf,
				ModelVersion: pred.ModelVersion,
			})
		}
	}

	return signals
}

// ToStoreSignal converts ValueBetSignal to store.Signal for persistence.
func (v ValueBetSignal) ToStoreSignal() (store.Signal, error) {
	data := map[string]any{
		"outcome":               v.Outcome,
		"bookmaker":             v.Bookmaker,
		"odds":                  v.Odds,
		"implied_prob":          v.ImpliedProb,
		"model_prob":            v.ModelProb,
		"edge":                  fmt.Sprintf("%.2f%%", v.Edge*100),
		"kelly_full":            v.KellyFull,
		"kelly_half":            v.KellyHalf,
		"recommended_stake_pct": v.KellyHalf * 100,
		"model_version":         v.ModelVersion,
	}
	b, err := json.Marshal(data)
	if err != nil {
		return store.Signal{}, err
	}
	return store.Signal{
		MatchID:    v.MatchID,
		Type:       "value_bet",
		Market:     v.Market,
		Data:       b,
		Confidence: v.Edge,
	}, nil
}
