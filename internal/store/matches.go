package store

import (
	"context"
	"time"
)

type Match struct {
	ID          string     `json:"id"`
	ExternalID  string     `json:"external_id"`
	Sport       string     `json:"sport"`
	League      string     `json:"league"`
	HomeTeam    string     `json:"home_team"`
	AwayTeam    string     `json:"away_team"`
	StartsAt    time.Time  `json:"starts_at"`
	Status      string     `json:"status"`
	LastFetched *time.Time `json:"last_fetched,omitempty"`
}

func (s *Store) UpsertMatch(ctx context.Context, m Match) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO matches (external_id, sport, league, home_team, away_team, starts_at, status, last_fetched)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (external_id) DO UPDATE SET
			status = EXCLUDED.status,
			starts_at = EXCLUDED.starts_at,
			last_fetched = NOW()
	`, m.ExternalID, m.Sport, m.League, m.HomeTeam, m.AwayTeam, m.StartsAt, m.Status)
	return err
}

func (s *Store) GetActiveMatches(ctx context.Context) ([]Match, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, external_id, sport, league, home_team, away_team, starts_at, status, last_fetched
		FROM matches
		WHERE status IN ('upcoming', 'live')
		ORDER BY starts_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	matches := []Match{}
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.ID, &m.ExternalID, &m.Sport, &m.League,
			&m.HomeTeam, &m.AwayTeam, &m.StartsAt, &m.Status, &m.LastFetched); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

func (s *Store) GetMatchByExternalID(ctx context.Context, externalID string) (*Match, error) {
	var m Match
	err := s.pool.QueryRow(ctx, `
		SELECT id, external_id, sport, league, home_team, away_team, starts_at, status, last_fetched
		FROM matches WHERE external_id = $1
	`, externalID).Scan(&m.ID, &m.ExternalID, &m.Sport, &m.League,
		&m.HomeTeam, &m.AwayTeam, &m.StartsAt, &m.Status, &m.LastFetched)
	if err != nil {
		return nil, err
	}
	return &m, nil
}
