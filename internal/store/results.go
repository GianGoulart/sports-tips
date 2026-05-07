package store

import (
	"context"
	"time"
)

type Result struct {
	ID         string
	MatchID    string
	ScoreHome  int
	ScoreAway  int
	Outcome    string
	Source     string
	RecordedAt time.Time
}

func (s *Store) UpsertResult(ctx context.Context, r Result) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO results (match_id, score_home, score_away, outcome, source)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (match_id) DO UPDATE SET
			score_home  = EXCLUDED.score_home,
			score_away  = EXCLUDED.score_away,
			outcome     = EXCLUDED.outcome,
			source      = EXCLUDED.source,
			recorded_at = NOW()
	`, r.MatchID, r.ScoreHome, r.ScoreAway, r.Outcome, r.Source)
	return err
}

func (s *Store) GetFinishedWithoutResult(ctx context.Context) ([]Match, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.external_id, m.sport, m.league, m.home_team, m.away_team,
		       m.starts_at, m.status, m.last_fetched
		FROM matches m
		LEFT JOIN results r ON r.match_id = m.id
		WHERE m.status = 'finished'
		  AND r.id IS NULL
		  AND m.starts_at < NOW() - INTERVAL '3 hours'
		ORDER BY m.starts_at DESC
		LIMIT 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.ID, &m.ExternalID, &m.Sport, &m.League,
			&m.HomeTeam, &m.AwayTeam, &m.StartsAt, &m.Status, &m.LastFetched); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	if matches == nil {
		matches = []Match{}
	}
	return matches, rows.Err()
}
