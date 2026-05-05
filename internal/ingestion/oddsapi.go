package ingestion

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const oddsAPIBase = "https://api.the-odds-api.com/v4"

type OddsAPIClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewOddsAPIClient(apiKey string) *OddsAPIClient {
	return &OddsAPIClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type oddsAPIEvent struct {
	ID           string             `json:"id"`
	SportKey     string             `json:"sport_key"`
	SportTitle   string             `json:"sport_title"`
	CommenceTime time.Time          `json:"commence_time"`
	HomeTeam     string             `json:"home_team"`
	AwayTeam     string             `json:"away_team"`
	Bookmakers   []oddsAPIBookmaker `json:"bookmakers"`
}

type oddsAPIBookmaker struct {
	Key     string          `json:"key"`
	Title   string          `json:"title"`
	Markets []oddsAPIMarket `json:"markets"`
}

type oddsAPIMarket struct {
	Key      string           `json:"key"`
	Outcomes []oddsAPIOutcome `json:"outcomes"`
}

type oddsAPIOutcome struct {
	Name  string   `json:"name"`
	Price float64  `json:"price"`
	Point *float64 `json:"point,omitempty"`
}

func (c *OddsAPIClient) GetOdds(sport string) ([]RawEvent, error) {
	url := fmt.Sprintf(
		"%s/sports/%s/odds/?apiKey=%s&regions=eu&markets=h2h,totals&dateFormat=iso&oddsFormat=decimal",
		oddsAPIBase, sport, c.apiKey,
	)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("oddsapi get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oddsapi status %d", resp.StatusCode)
	}

	var events []oddsAPIEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, fmt.Errorf("oddsapi decode: %w", err)
	}

	return toRawEvents(events), nil
}

func toRawEvents(events []oddsAPIEvent) []RawEvent {
	result := make([]RawEvent, 0, len(events))
	for _, e := range events {
		raw := RawEvent{
			ExternalID:   e.ID,
			Sport:        e.SportKey,
			League:       e.SportTitle,
			HomeTeam:     e.HomeTeam,
			AwayTeam:     e.AwayTeam,
			CommenceTime: e.CommenceTime,
		}
		for _, bk := range e.Bookmakers {
			rb := RawBookmaker{Key: bk.Key}
			for _, m := range bk.Markets {
				rm := RawMarket{Key: m.Key}
				for _, o := range m.Outcomes {
					rm.Outcomes = append(rm.Outcomes, RawOutcome{
						Name:  o.Name,
						Price: o.Price,
						Point: o.Point,
					})
				}
				rb.Markets = append(rb.Markets, rm)
			}
			raw.Bookmakers = append(raw.Bookmakers, rb)
		}
		result = append(result, raw)
	}
	return result
}
