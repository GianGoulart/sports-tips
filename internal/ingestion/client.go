package ingestion

import "time"

type OddsClient interface {
	GetOdds(sport string) ([]RawEvent, error)
}

type RawEvent struct {
	ExternalID   string
	Sport        string
	League       string
	HomeTeam     string
	AwayTeam     string
	CommenceTime time.Time
	Bookmakers   []RawBookmaker
}

type RawBookmaker struct {
	Key     string
	Markets []RawMarket
}

type RawMarket struct {
	Key      string
	Outcomes []RawOutcome
}

type RawOutcome struct {
	Name  string
	Price float64
	Point *float64
}
