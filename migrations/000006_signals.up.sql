CREATE TABLE signals (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    match_id   UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    type       TEXT NOT NULL,
    market     TEXT NOT NULL,
    data       JSONB NOT NULL,
    confidence NUMERIC(5,4),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_signals_tenant ON signals(tenant_id);
CREATE INDEX idx_signals_type ON signals(tenant_id, type);

ALTER TABLE signals ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON signals
    USING (tenant_id = current_setting('app.tenant_id', true)::UUID);
