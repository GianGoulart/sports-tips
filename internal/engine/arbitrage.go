package engine

import (
	"encoding/json"
	"fmt"

	"sportstips/internal/store"
)

type ArbSignal struct {
	MatchID   string
	Type      string
	Market    string
	ProfitPct float64
	ArbSum    float64
	Best      BestOdds
	Stakes    map[string]float64
}

type BestOdds struct {
	HomeBookmaker string
	HomeOdds      float64
	DrawBookmaker string
	DrawOdds      float64
	AwayBookmaker string
	AwayOdds      float64
}

func DetectArbitrage(matchID string, odds []store.NormalizedOdds, minProfitPct float64) []ArbSignal {
	byMarket := map[string][]store.NormalizedOdds{}
	for _, o := range odds {
		byMarket[o.Market] = append(byMarket[o.Market], o)
	}

	var signals []ArbSignal
	for market, marketOdds := range byMarket {
		sig := checkArb(matchID, market, marketOdds, minProfitPct)
		if sig != nil {
			signals = append(signals, *sig)
		}
	}
	return signals
}

func checkArb(matchID, market string, odds []store.NormalizedOdds, minProfitPct float64) *ArbSignal {
	best := findBestOdds(odds)
	if best == nil {
		return nil
	}

	// Two-outcome market (e.g. over/under): no draw, use only home+away
	isTwoOutcome := best.DrawOdds == 0
	var arbSum float64
	if isTwoOutcome {
		arbSum = (1 / best.HomeOdds) + (1 / best.AwayOdds)
	} else {
		arbSum = (1 / best.HomeOdds) + (1 / best.DrawOdds) + (1 / best.AwayOdds)
	}

	if arbSum >= 1.0 {
		return nil
	}

	profitPct := (1 - arbSum) / arbSum
	if profitPct < minProfitPct {
		return nil
	}

	stakes := map[string]float64{
		"home": (1 / best.HomeOdds) / arbSum * 100,
		"away": (1 / best.AwayOdds) / arbSum * 100,
	}
	if !isTwoOutcome {
		stakes["draw"] = (1 / best.DrawOdds) / arbSum * 100
	}

	return &ArbSignal{
		MatchID:   matchID,
		Type:      "arbitrage",
		Market:    market,
		ProfitPct: profitPct,
		ArbSum:    arbSum,
		Best:      *best,
		Stakes:    stakes,
	}
}

func findBestOdds(odds []store.NormalizedOdds) *BestOdds {
	if len(odds) == 0 {
		return nil
	}
	best := &BestOdds{}
	for _, o := range odds {
		if o.OddsHome > best.HomeOdds {
			best.HomeOdds = o.OddsHome
			best.HomeBookmaker = o.Bookmaker
		}
		if o.OddsDraw > best.DrawOdds && o.OddsDraw > 0 {
			best.DrawOdds = o.OddsDraw
			best.DrawBookmaker = o.Bookmaker
		}
		if o.OddsAway > best.AwayOdds {
			best.AwayOdds = o.OddsAway
			best.AwayBookmaker = o.Bookmaker
		}
	}
	if best.HomeOdds == 0 || best.AwayOdds == 0 {
		return nil
	}
	// DrawOdds stays 0 for two-outcome markets — handled in checkArb
	return best
}

func (a ArbSignal) ToStoreSignal() (store.Signal, error) {
	data := map[string]any{
		"arb_sum":        a.ArbSum,
		"profit_pct":     fmt.Sprintf("%.2f%%", a.ProfitPct*100),
		"home_bookmaker": a.Best.HomeBookmaker,
		"home_odds":      a.Best.HomeOdds,
		"draw_bookmaker": a.Best.DrawBookmaker,
		"draw_odds":      a.Best.DrawOdds,
		"away_bookmaker": a.Best.AwayBookmaker,
		"away_odds":      a.Best.AwayOdds,
		"stakes":         a.Stakes,
	}
	b, err := json.Marshal(data)
	if err != nil {
		return store.Signal{}, err
	}
	return store.Signal{
		MatchID:    a.MatchID,
		Type:       "arbitrage",
		Market:     a.Market,
		Data:       b,
		Confidence: 1.0,
	}, nil
}
