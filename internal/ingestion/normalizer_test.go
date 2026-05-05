package ingestion

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalize_H2H(t *testing.T) {
	event := RawEvent{
		ExternalID: "evt-1",
		HomeTeam:   "Arsenal",
		AwayTeam:   "Chelsea",
		Bookmakers: []RawBookmaker{
			{
				Key: "bet365",
				Markets: []RawMarket{
					{
						Key: "h2h",
						Outcomes: []RawOutcome{
							{Name: "Arsenal", Price: 2.10},
							{Name: "Chelsea", Price: 3.50},
							{Name: "Draw", Price: 3.20},
						},
					},
				},
			},
		},
	}

	result := Normalize("match-uuid", event)

	assert.Len(t, result, 1)
	assert.Equal(t, "bet365", result[0].Bookmaker)
	assert.Equal(t, "1x2", result[0].Market)
	assert.Equal(t, 2.10, result[0].OddsHome)
	assert.Equal(t, 3.20, result[0].OddsDraw)
	assert.Equal(t, 3.50, result[0].OddsAway)
}

func TestNormalize_SkipsUnknownMarkets(t *testing.T) {
	event := RawEvent{
		ExternalID: "evt-2",
		HomeTeam:   "Real Madrid",
		AwayTeam:   "Barcelona",
		Bookmakers: []RawBookmaker{
			{
				Key: "betfair",
				Markets: []RawMarket{
					{Key: "unknown_market", Outcomes: []RawOutcome{}},
				},
			},
		},
	}

	result := Normalize("match-uuid", event)
	assert.Empty(t, result)
}
