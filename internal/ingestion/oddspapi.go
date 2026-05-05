package ingestion

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OddsPapiClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewOddsPapiClient(apiKey string) *OddsPapiClient {
	return &OddsPapiClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type oddsPapiResponse struct {
	Data []oddsPapiEvent `json:"data"`
}

type oddsPapiEvent struct {
	ID         string              `json:"id"`
	League     string              `json:"league"`
	HomeTeam   string              `json:"home_team"`
	AwayTeam   string              `json:"away_team"`
	StartTime  time.Time           `json:"start_time"`
	Bookmakers []oddsPapiBookmaker `json:"bookmakers"`
}

type oddsPapiBookmaker struct {
	Name    string           `json:"name"`
	Markets []oddsPapiMarket `json:"markets"`
}

type oddsPapiMarket struct {
	Name     string            `json:"name"`
	Outcomes []oddsPapiOutcome `json:"outcomes"`
}

type oddsPapiOutcome struct {
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

func (c *OddsPapiClient) GetOdds(sport string) ([]RawEvent, error) {
	url := fmt.Sprintf(
		"https://api.oddspapi.com/v1/odds?sport=%s&apiKey=%s",
		sport, c.apiKey,
	)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("oddspapi get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oddspapi status %d", resp.StatusCode)
	}

	var result oddsPapiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("oddspapi decode: %w", err)
	}

	return toRawEventsFromPapi(result.Data, sport), nil
}

func toRawEventsFromPapi(events []oddsPapiEvent, sport string) []RawEvent {
	result := make([]RawEvent, 0, len(events))
	for _, e := range events {
		raw := RawEvent{
			ExternalID:   e.ID,
			Sport:        sport,
			League:       e.League,
			HomeTeam:     e.HomeTeam,
			AwayTeam:     e.AwayTeam,
			CommenceTime: e.StartTime,
		}
		for _, bk := range e.Bookmakers {
			rb := RawBookmaker{Key: bk.Name}
			for _, m := range bk.Markets {
				rm := RawMarket{Key: m.Name}
				for _, o := range m.Outcomes {
					rm.Outcomes = append(rm.Outcomes, RawOutcome{
						Name:  o.Name,
						Price: o.Price,
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
