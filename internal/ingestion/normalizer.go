package ingestion

import (
	"fmt"

	"sportstips/internal/store"
)

func Normalize(matchID string, event RawEvent) []store.NormalizedOdds {
	var result []store.NormalizedOdds

	for _, bk := range event.Bookmakers {
		for _, m := range bk.Markets {
			switch m.Key {
			case "h2h":
				norm := normalizeH2H(matchID, bk.Key, event.HomeTeam, event.AwayTeam, m)
				if norm != nil {
					result = append(result, *norm)
				}
			case "totals":
				norms := normalizeTotals(matchID, bk.Key, m)
				result = append(result, norms...)
			}
		}
	}

	return result
}

func normalizeH2H(matchID, bookmaker, homeTeam, awayTeam string, m RawMarket) *store.NormalizedOdds {
	norm := &store.NormalizedOdds{
		MatchID:   matchID,
		Bookmaker: bookmaker,
		Market:    "1x2",
	}
	for _, o := range m.Outcomes {
		switch o.Name {
		case homeTeam:
			norm.OddsHome = o.Price
		case awayTeam:
			norm.OddsAway = o.Price
		case "Draw":
			norm.OddsDraw = o.Price
		}
	}
	if norm.OddsHome == 0 || norm.OddsAway == 0 {
		return nil
	}
	return norm
}

func normalizeTotals(matchID, bookmaker string, m RawMarket) []store.NormalizedOdds {
	type pointGroup struct{ over, under float64 }
	groups := map[float64]*pointGroup{}

	for _, o := range m.Outcomes {
		if o.Point == nil {
			continue
		}
		pt := *o.Point
		if groups[pt] == nil {
			groups[pt] = &pointGroup{}
		}
		switch o.Name {
		case "Over":
			groups[pt].over = o.Price
		case "Under":
			groups[pt].under = o.Price
		}
	}

	var result []store.NormalizedOdds
	for pt, g := range groups {
		if g.over == 0 || g.under == 0 {
			continue
		}
		result = append(result, store.NormalizedOdds{
			MatchID:   matchID,
			Bookmaker: bookmaker,
			Market:    fmt.Sprintf("over_under_%.1f", pt),
			OddsHome:  g.over,
			OddsAway:  g.under,
		})
	}
	return result
}
