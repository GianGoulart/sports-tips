package results

import "time"

// ScoreEvent is a match with its final score from any source.
type ScoreEvent struct {
	ExternalID   string
	HomeScore    int
	AwayScore    int
	Completed    bool
	CommenceTime time.Time
}

// ResultsClient fetches scores from a data source.
type ResultsClient interface {
	GetScores(sport string, daysFrom int) ([]ScoreEvent, error)
}

// DeriveOutcome converts home/away scores to "home"/"draw"/"away".
func DeriveOutcome(homeScore, awayScore int) string {
	switch {
	case homeScore > awayScore:
		return "home"
	case awayScore > homeScore:
		return "away"
	default:
		return "draw"
	}
}
