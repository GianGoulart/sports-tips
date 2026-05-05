CREATE TABLE matches (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id  TEXT UNIQUE NOT NULL,
    sport        TEXT NOT NULL DEFAULT 'soccer',
    league       TEXT NOT NULL,
    home_team    TEXT NOT NULL,
    away_team    TEXT NOT NULL,
    starts_at    TIMESTAMPTZ NOT NULL,
    status       TEXT NOT NULL DEFAULT 'upcoming',
    last_fetched TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_matches_status ON matches(status);
CREATE INDEX idx_matches_starts_at ON matches(starts_at);
