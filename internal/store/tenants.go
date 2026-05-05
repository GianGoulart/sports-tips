package store

import (
	"context"
	"time"
)

type Tenant struct {
	ID        string
	Email     string
	Name      string
	Password  string
	Plan      string
	CreatedAt time.Time
}

type TenantPreferences struct {
	TenantID     string
	MinArbProfit float64
	MinValueEdge float64
	TelegramID   *string
	Email        *string
	Bookmakers   []string
}

func (s *Store) CreateTenant(ctx context.Context, t Tenant) (*Tenant, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tenants (email, name, password)
		VALUES ($1, $2, $3)
		RETURNING id, email, name, password, plan, created_at
	`, t.Email, t.Name, t.Password).Scan(
		&t.ID, &t.Email, &t.Name, &t.Password, &t.Plan, &t.CreatedAt)
	if err != nil {
		return nil, err
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO tenant_preferences (tenant_id) VALUES ($1)
	`, t.ID)
	if err != nil {
		return nil, err
	}

	return &t, nil
}

func (s *Store) GetTenantByEmail(ctx context.Context, email string) (*Tenant, error) {
	var t Tenant
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, name, password, plan, created_at
		FROM tenants WHERE email = $1
	`, email).Scan(&t.ID, &t.Email, &t.Name, &t.Password, &t.Plan, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) GetPreferences(ctx context.Context, tenantID string) (*TenantPreferences, error) {
	var p TenantPreferences
	err := s.pool.QueryRow(ctx, `
		SELECT tenant_id, min_arb_profit, min_value_edge,
		       alert_telegram_id, alert_email, bookmakers
		FROM tenant_preferences WHERE tenant_id = $1
	`, tenantID).Scan(&p.TenantID, &p.MinArbProfit, &p.MinValueEdge,
		&p.TelegramID, &p.Email, &p.Bookmakers)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) UpdatePreferences(ctx context.Context, p TenantPreferences) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE tenant_preferences SET
			min_arb_profit = $2,
			min_value_edge = $3,
			alert_telegram_id = $4,
			alert_email = $5,
			bookmakers = $6,
			updated_at = NOW()
		WHERE tenant_id = $1
	`, p.TenantID, p.MinArbProfit, p.MinValueEdge,
		p.TelegramID, p.Email, p.Bookmakers)
	return err
}
