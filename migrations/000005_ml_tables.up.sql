CREATE TABLE ml_predictions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id      UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    model_version TEXT NOT NULL,
    prob_home     NUMERIC(5,4),
    prob_draw     NUMERIC(5,4),
    prob_away     NUMERIC(5,4),
    predicted_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE ml_model_metrics (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model_version TEXT NOT NULL,
    brier_score   NUMERIC(6,5),
    log_loss      NUMERIC(6,5),
    sample_size   INT,
    trained_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ml_predictions_match ON ml_predictions(match_id);
