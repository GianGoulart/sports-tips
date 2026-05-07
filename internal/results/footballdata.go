package results

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FootballDataClient calls football-data.org as a fallback source.
type FootballDataClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewFootballDataClient(apiKey string) *FootballDataClient {
	return &FootballDataClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type fdResponse struct {
	Matches []fdMatch `json:"matches"`
}

type fdMatch struct {
	ID       int       `json:"id"`
	UtcDate  time.Time `json:"utcDate"`
	Status   string    `json:"status"`
	HomeTeam fdTeam    `json:"homeTeam"`
	AwayTeam fdTeam    `json:"awayTeam"`
	Score    fdScore   `json:"score"`
}

type fdTeam struct {
	Name string `json:"name"`
}

type fdScore struct {
	FullTime fdHalfScore `json:"fullTime"`
}

type fdHalfScore struct {
	Home *int `json:"home"`
	Away *int `json:"away"`
}

func (c *FootballDataClient) GetScores(sport string, daysFrom int) ([]ScoreEvent, error) {
	competitions := []string{"PL", "PD", "SA", "BL1", "CL"}
	var all []ScoreEvent

	for _, comp := range competitions {
		url := fmt.Sprintf(
			"https://api.football-data.org/v4/competitions/%s/matches?status=FINISHED",
			comp,
		)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("X-Auth-Token", c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
		resp.Body.Close()

		var result fdResponse
		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}

		for _, m := range result.Matches {
			if m.Status != "FINISHED" {
				continue
			}
			if m.Score.FullTime.Home == nil || m.Score.FullTime.Away == nil {
				continue
			}
			all = append(all, ScoreEvent{
				ExternalID:   fmt.Sprintf("fd_%d", m.ID),
				HomeScore:    *m.Score.FullTime.Home,
				AwayScore:    *m.Score.FullTime.Away,
				Completed:    true,
				CommenceTime: m.UtcDate,
			})
		}
	}

	return all, nil
}

// fuzzyMatchTeam returns true if nameA contains nameB or vice versa (case-insensitive),
// or if all words of one name appear (as substrings) in the other name.
func fuzzyMatchTeam(nameA, nameB string) bool {
	a := strings.ToLower(strings.TrimSpace(nameA))
	b := strings.ToLower(strings.TrimSpace(nameB))
	if a == b {
		return true
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return true
	}
	// Check if all words of B appear in A (handles abbreviations like "Man" -> "Manchester").
	allMatch := func(words []string, target string) bool {
		for _, w := range words {
			if !strings.Contains(target, w) {
				return false
			}
		}
		return true
	}
	return allMatch(strings.Fields(b), a) || allMatch(strings.Fields(a), b)
}
