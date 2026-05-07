package results

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type OddsAPIResultsClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewOddsAPIResultsClient(apiKey string) *OddsAPIResultsClient {
	return &OddsAPIResultsClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type oddsAPIScore struct {
	ID           string             `json:"id"`
	CommenceTime time.Time          `json:"commence_time"`
	Completed    bool               `json:"completed"`
	HomeTeam     string             `json:"home_team"`
	AwayTeam     string             `json:"away_team"`
	Scores       []oddsAPITeamScore `json:"scores"`
}

type oddsAPITeamScore struct {
	Name  string `json:"name"`
	Score string `json:"score"`
}

func (c *OddsAPIResultsClient) GetScores(sport string, daysFrom int) ([]ScoreEvent, error) {
	url := fmt.Sprintf(
		"https://api.the-odds-api.com/v4/sports/%s/scores/?apiKey=%s&daysFrom=%d",
		sport, c.apiKey, daysFrom,
	)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("oddsapi scores get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oddsapi scores status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("oddsapi scores read: %w", err)
	}

	var raw []oddsAPIScore
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("oddsapi scores decode: %w", err)
	}

	return toScoreEvents(raw), nil
}

func toScoreEvents(raw []oddsAPIScore) []ScoreEvent {
	result := make([]ScoreEvent, 0, len(raw))
	for _, r := range raw {
		if !r.Completed || len(r.Scores) < 2 {
			continue
		}

		var homeScore, awayScore int
		for _, s := range r.Scores {
			var score int
			fmt.Sscanf(s.Score, "%d", &score)
			if s.Name == r.HomeTeam {
				homeScore = score
			} else {
				awayScore = score
			}
		}

		result = append(result, ScoreEvent{
			ExternalID:   r.ID,
			HomeScore:    homeScore,
			AwayScore:    awayScore,
			Completed:    true,
			CommenceTime: r.CommenceTime,
		})
	}
	return result
}
