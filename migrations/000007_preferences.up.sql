CREATE TABLE tenant_preferences (
    tenant_id         UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    min_arb_profit    NUMERIC(5,4) NOT NULL DEFAULT 0.01,
    min_value_edge    NUMERIC(5,4) NOT NULL DEFAULT 0.05,
    alert_telegram_id TEXT,
    alert_email       TEXT,
    bookmakers        TEXT[] NOT NULL DEFAULT '{}',
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
