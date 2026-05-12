"""
Batch prediction service.
Loads the latest trained model and writes probability predictions
to ml_predictions table for all upcoming/live matches.
"""

import sys
import joblib
from pathlib import Path

import pandas as pd

from db import get_connection
from features import FEATURE_COLS, compute_elo, compute_rolling_stats, fetch_training_data

MODELS_DIR = Path("/app/models")

SPORT_GROUP_MAP = {
    "basketball_nba": "basketball",
    # basketball_nbb: add when key confirmed on Odds API
}

def get_sport_group(sport: str) -> str:
    """Map a sport key to its model group."""
    return SPORT_GROUP_MAP.get(sport, "soccer")


def load_model_for_group(sport_group: str):
    """Load the latest model for a sport group. Returns None if not found."""
    path = MODELS_DIR / f"model_{sport_group}_latest.pkl"
    if not path.exists():
        return None
    return joblib.load(path)


def fetch_upcoming_matches(conn) -> pd.DataFrame:
    """Return upcoming/live matches that need predictions, including sport."""
    query = """
        SELECT
            m.id AS match_id,
            m.home_team,
            m.away_team,
            m.starts_at,
            m.sport,
            AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_home, 0) END) AS implied_prob_home,
            AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_draw, 0) END) AS implied_prob_draw,
            AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_away, 0) END) AS implied_prob_away
        FROM matches m
        LEFT JOIN odds_normalized o ON o.match_id = m.id
        WHERE m.status IN ('upcoming', 'live')
          AND m.starts_at < NOW() + INTERVAL '48 hours'
        GROUP BY m.id, m.home_team, m.away_team, m.starts_at, m.sport
        HAVING AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_home, 0) END) IS NOT NULL
    """
    return pd.read_sql(query, conn)


def build_prediction_features(upcoming: pd.DataFrame, history: pd.DataFrame) -> pd.DataFrame:
    """
    Compute ELO and rolling stats for upcoming matches using historical data as base.
    Returns upcoming with all FEATURE_COLS filled.
    """
    combined = pd.concat([history, upcoming], ignore_index=True)
    combined = compute_elo(combined)
    combined = compute_rolling_stats(combined)
    result = combined.tail(len(upcoming)).copy()
    return result


def insert_predictions(conn, predictions: list):
    with conn.cursor() as cur:
        for p in predictions:
            cur.execute("""
                INSERT INTO ml_predictions (match_id, model_version, prob_home, prob_draw, prob_away)
                VALUES (%s, %s, %s, %s, %s)
                ON CONFLICT DO NOTHING
            """, (p["match_id"], p["model_version"], p["prob_home"], p["prob_draw"], p["prob_away"]))
    conn.commit()


def main():
    conn = get_connection()

    upcoming = fetch_upcoming_matches(conn)
    if upcoming.empty:
        print("No upcoming matches to predict.")
        conn.close()
        return

    from features import fetch_training_data as _fetch_history

    # Group matches by sport group and predict with the correct model
    all_predictions = []
    for sport_group, group_df in upcoming.groupby(upcoming["sport"].map(get_sport_group)):
        bundle = load_model_for_group(sport_group)
        if bundle is None:
            print(f"[{sport_group}] No trained model found. Run train.py first.")
            continue

        model = bundle["model"]
        model_version = bundle["version"]

        # Historical data for ELO/rolling context
        sport_keys = group_df["sport"].unique().tolist()
        history = _fetch_history(conn, sports=sport_keys)

        features_df = build_prediction_features(group_df, history)

        for col in FEATURE_COLS:
            if col not in features_df.columns:
                features_df[col] = 0.0
        features_df[FEATURE_COLS] = features_df[FEATURE_COLS].fillna(0.0)

        X = features_df[FEATURE_COLS]
        proba = model.predict_proba(X)
        classes = list(model.classes_)

        idx_home = classes.index(0) if 0 in classes else 0
        idx_draw = classes.index(1) if 1 in classes else -1
        idx_away = classes.index(2) if 2 in classes else 1

        for i, (_, row) in enumerate(group_df.iterrows()):
            all_predictions.append({
                "match_id": row["match_id"],
                "model_version": model_version,
                "prob_home": float(proba[i][idx_home]),
                "prob_draw": float(proba[i][idx_draw]) if idx_draw >= 0 and proba.shape[1] > 2 else 0.0,
                "prob_away": float(proba[i][idx_away]),
            })

        print(f"[{sport_group}] Predictions: {len(group_df)} matches (model={model_version})")

    if all_predictions:
        insert_predictions(conn, all_predictions)

    conn.close()
    print(f"Total predictions written: {len(all_predictions)}")


if __name__ == "__main__":
    main()
