"""
Train match outcome prediction model.
Saves model to /app/models/model_<date>.pkl
Writes metrics to ml_model_metrics table.
"""

import os
import sys
import joblib
from datetime import date
from pathlib import Path

import numpy as np
from sklearn.linear_model import LogisticRegression
from sklearn.model_selection import train_test_split
from sklearn.metrics import brier_score_loss, log_loss
from sklearn.preprocessing import LabelBinarizer

from db import get_connection
from features import build_dataset, FEATURE_COLS

MODELS_DIR = Path("/app/models")

# Train one model per sport group. Add new groups here as sports are added.
SPORT_GROUPS = {
    "soccer": [
        "soccer_epl",
        "soccer_spain_la_liga",
        "soccer_italy_serie_a",
        "soccer_germany_bundesliga",
        "soccer_uefa_champs_league",
    ],
    "basketball": [
        "basketball_nba",
    ],
    "baseball": [
        "baseball_mlb",
    ],
}


def save_metrics(conn, model_version: str, brier: float, logloss: float, n_samples: int):
    with conn.cursor() as cur:
        cur.execute("""
            INSERT INTO ml_model_metrics (model_version, brier_score, log_loss, sample_size)
            VALUES (%s, %s, %s, %s)
        """, (model_version, float(brier), float(logloss), n_samples))
    conn.commit()


def train_group(conn, sport_group: str, sport_keys: list):
    X, y = build_dataset(conn, sports=sport_keys)

    if len(X) < 20:
        print(f"[{sport_group}] Not enough data: {len(X)} samples (need >= 20). Skipping.")
        return

    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=0.2, random_state=42, stratify=y if len(y.unique()) > 1 else None
    )

    model = LogisticRegression(max_iter=1000, random_state=42)
    model.fit(X_train, y_train)

    proba = model.predict_proba(X_test)
    lb = LabelBinarizer()
    y_bin = lb.fit_transform(y_test)
    if y_bin.shape[1] == 1:
        y_bin = np.hstack([1 - y_bin, y_bin])

    brier = np.mean([
        brier_score_loss(y_bin[:, i], proba[:, i])
        for i in range(proba.shape[1])
    ])
    logloss = log_loss(y_test, proba)

    model_version = f"lr_{sport_group}_{date.today().isoformat()}"

    MODELS_DIR.mkdir(parents=True, exist_ok=True)
    versioned_path = MODELS_DIR / f"model_{model_version}.pkl"
    latest_path = MODELS_DIR / f"model_{sport_group}_latest.pkl"

    bundle = {"model": model, "version": model_version, "features": FEATURE_COLS, "sport_group": sport_group}
    joblib.dump(bundle, versioned_path)
    joblib.dump(bundle, latest_path)

    save_metrics(conn, model_version, brier, logloss, len(X))

    print(f"[{sport_group}] Model trained: {model_version}")
    print(f"  Samples: {len(X)} train={len(X_train)} test={len(X_test)}")
    print(f"  Brier score: {brier:.4f}")
    print(f"  Log loss:    {logloss:.4f}")
    print(f"  Saved to:    {latest_path}")


def main():
    conn = get_connection()
    for sport_group, sport_keys in SPORT_GROUPS.items():
        train_group(conn, sport_group, sport_keys)
    conn.close()


if __name__ == "__main__":
    main()
