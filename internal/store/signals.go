package store

import (
	"context"
	"encoding/json"
	"time"
)

type Signal struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenant_id"`
	MatchID    string          `json:"match_id"`
	Type       string          `json:"type"`
	Market     string          `json:"market"`
	Data       json.RawMessage `json:"data"`
	Confidence float64         `json:"confidence"`
	CreatedAt  time.Time       `json:"created_at"`
}

func (s *Store) InsertSignals(ctx context.Context, tenantID string, signals []Signal) error {
	for _, sig := range signals {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO signals (tenant_id, match_id, type, market, data, confidence)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, tenantID, sig.MatchID, sig.Type, sig.Market, sig.Data, sig.Confidence)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetSignals(ctx context.Context, tenantID, sigType string) ([]Signal, error) {
	if err := s.setTenant(ctx, tenantID); err != nil {
		return nil, err
	}

	query := `
		SELECT id, tenant_id, match_id, type, market, data, confidence, created_at
		FROM signals WHERE tenant_id = $1
	`
	args := []any{tenantID}

	if sigType != "" {
		query += " AND type = $2"
		args = append(args, sigType)
	}
	query += " ORDER BY created_at DESC LIMIT 100"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Signal{}
	for rows.Next() {
		var sig Signal
		if err := rows.Scan(&sig.ID, &sig.TenantID, &sig.MatchID, &sig.Type,
			&sig.Market, &sig.Data, &sig.Confidence, &sig.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, sig)
	}
	return result, rows.Err()
}

func (s *Store) GetSignalsByMatch(ctx context.Context, tenantID, matchID string) ([]Signal, error) {
	if err := s.setTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, match_id, type, market, data, confidence, created_at
		FROM signals WHERE tenant_id = $1 AND match_id = $2
		ORDER BY created_at DESC
	`, tenantID, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Signal{}
	for rows.Next() {
		var sig Signal
		if err := rows.Scan(&sig.ID, &sig.TenantID, &sig.MatchID, &sig.Type,
			&sig.Market, &sig.Data, &sig.Confidence, &sig.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, sig)
	}
	return result, rows.Err()
}
