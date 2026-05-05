CREATE TABLE odds_raw (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id   UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    source     TEXT NOT NULL,
    payload    JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE odds_normalized (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id   UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    bookmaker  TEXT NOT NULL,
    market     TEXT NOT NULL,
    odds_home  NUMERIC(8,4),
    odds_draw  NUMERIC(8,4),
    odds_away  NUMERIC(8,4),
    timestamp  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_odds_normalized_match ON odds_normalized(match_id);
CREATE INDEX idx_odds_normalized_market ON odds_normalized(match_id, market);
