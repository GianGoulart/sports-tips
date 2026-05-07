package alerts

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatSignal returns a human-readable Telegram message for a signal.
// signalType: "arbitrage" or "value_bet"
// matchLabel: "Arsenal vs Chelsea" or a match ID when no label is available
// data: the signal's JSON data field
func FormatSignal(signalType, market, matchLabel string, data json.RawMessage) string {
	switch signalType {
	case "arbitrage":
		return formatArb(market, matchLabel, data)
	case "value_bet":
		return formatValueBet(market, matchLabel, data)
	default:
		return fmt.Sprintf("Signal: %s — %s", signalType, matchLabel)
	}
}

func formatArb(market, matchLabel string, data json.RawMessage) string {
	var d map[string]any
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Sprintf("ARBITRAGE — %s", matchLabel)
	}

	profit := d["profit_pct"]
	home := fmt.Sprintf("%v @ %v", d["home_bookmaker"], d["home_odds"])
	draw := fmt.Sprintf("%v @ %v", d["draw_bookmaker"], d["draw_odds"])
	away := fmt.Sprintf("%v @ %v", d["away_bookmaker"], d["away_odds"])

	stakes := ""
	if s, ok := d["stakes"].(map[string]any); ok {
		parts := []string{}
		for outcome, pct := range s {
			parts = append(parts, fmt.Sprintf("%s: %.1f%%", outcome, pct))
		}
		stakes = strings.Join(parts, " | ")
	}

	return fmt.Sprintf(
		"<b>ARBITRAGE</b> — %s\nMarket: %s | Profit: %v\nHome: %s\nDraw: %s\nAway: %s\nStakes: %s",
		matchLabel, market, profit, home, draw, away, stakes,
	)
}

func formatValueBet(market, matchLabel string, data json.RawMessage) string {
	var d map[string]any
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Sprintf("VALUE BET — %s", matchLabel)
	}

	return fmt.Sprintf(
		"<b>VALUE BET</b> — %s\nMarket: %s | Outcome: %v @ %v\nEdge: %v | Kelly: %.1f%% bankroll\nModel: %v — Implied: %.3f",
		matchLabel, market,
		d["outcome"], d["odds"],
		d["edge"], d["kelly_half"],
		d["model_prob"], d["implied_prob"],
	)
}
