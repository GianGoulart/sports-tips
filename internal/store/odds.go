package store

import (
	"context"
	"encoding/json"
	"time"
)

type NormalizedOdds struct {
	ID        string
	MatchID   string
	Bookmaker string
	Market    string
	OddsHome  float64
	OddsDraw  float64
	OddsAway  float64
	Timestamp time.Time
}

func (s *Store) InsertOddsRaw(ctx context.Context, matchID, source string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO odds_raw (match_id, source, payload) VALUES ($1, $2, $3)
	`, matchID, source, b)
	return err
}

func (s *Store) UpsertOddsNormalized(ctx context.Context, odds []NormalizedOdds) error {
	for _, o := range odds {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO odds_normalized (match_id, bookmaker, market, odds_home, odds_draw, odds_away)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, o.MatchID, o.Bookmaker, o.Market, o.OddsHome, o.OddsDraw, o.OddsAway)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetLatestOddsByMatch(ctx context.Context, matchID string) ([]NormalizedOdds, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (bookmaker, market)
			id, match_id, bookmaker, market, odds_home, odds_draw, odds_away, timestamp
		FROM odds_normalized
		WHERE match_id = $1
		ORDER BY bookmaker, market, timestamp DESC
	`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []NormalizedOdds
	for rows.Next() {
		var o NormalizedOdds
		if err := rows.Scan(&o.ID, &o.MatchID, &o.Bookmaker, &o.Market,
			&o.OddsHome, &o.OddsDraw, &o.OddsAway, &o.Timestamp); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}
