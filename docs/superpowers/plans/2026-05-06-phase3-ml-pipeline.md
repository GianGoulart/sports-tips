# Betting Intelligence Agent — Phase 3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Python ML batch pipeline (feature engineering, Logistic Regression training, batch predictions), implement BatchPredictionService in Go, and add value bet detection using Kelly Criterion to the scheduler.

**Architecture:** Python batch workers run as Docker containers via cron: `train.py` builds features from historical odds+results and trains a model; `predict.py` writes probabilities to `ml_predictions` table. The Go `BatchPredictionService` reads from that table. The scheduler's engine is extended to detect value bets per-tenant using `predictions.PredictionService` + `engine.Kelly`. The `StubPredictionService` is replaced by `BatchPredictionService` in `main.go`.

**Tech Stack:** Python 3.11+, scikit-learn, pandas, psycopg2-binary, joblib; Go 1.25+ (existing packages); PostgreSQL (shared DB)

---

## File Map

```
sportstips/
├── ml/
│   ├── Dockerfile
│   ├── requirements.txt
│   ├── db.py              # shared DB connection helper
│   ├── features.py        # build feature matrix from DB
│   ├── train.py           # train model, save .pkl + metrics to DB
│   ├── predict.py         # batch predictions → ml_predictions table
│   └── monitor.py         # Brier score drift detection
├── internal/
│   ├── predictions/
│   │   └── batch.go       # BatchPredictionService reads ml_predictions from DB
│   └── engine/
│       ├── valuebet.go    # DetectValueBets(odds, prediction, minEdge) → []ValueBetSignal
│       └── valuebet_test.go
│   └── ingestion/
│       └── scheduler.go   # MODIFY: runEngine calls predictions + DetectValueBets
└── internal/api/
    └── predictions.go     # GET /predictions/:match_id
```

---

## Task 1: Python ML Setup — Dockerfile + requirements + DB helper

**Files:**
- Create: `ml/requirements.txt`
- Create: `ml/Dockerfile`
- Create: `ml/db.py`

- [ ] **Step 1: Create `ml/requirements.txt`**

```
scikit-learn==1.5.2
pandas==2.2.3
psycopg2-binary==2.9.9
joblib==1.4.2
python-dotenv==1.0.1
numpy==1.26.4
```

- [ ] **Step 2: Create `ml/Dockerfile`**

```dockerfile
FROM python:3.11-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

# No default CMD — each script is called explicitly via docker compose run
```

- [ ] **Step 3: Create `ml/db.py`**

```python
import os
import psycopg2
from dotenv import load_dotenv

load_dotenv()

def get_connection():
    """Return a psycopg2 connection from DATABASE_URL env var."""
    url = os.environ["DATABASE_URL"]
    return psycopg2.connect(url)
```

- [ ] **Step 4: Add ml service to docker-compose.yml**

The `ml` service already exists in docker-compose.yml but has no `command`. Verify it has:

```yaml
  ml:
    build:
      context: ./ml
    profiles: ["ml"]
    env_file: .env
```

If the `build` section only has `context: ./ml` — that's correct. No changes needed.

- [ ] **Step 5: Build ml image to verify**

```bash
cd /Users/giancarlogoulart/Projects/Personal/sportstips
docker compose build ml
```

Expected: build succeeds, no pip errors.

- [ ] **Step 6: Commit**

```bash
git add ml/
git commit -m "feat: python ml Dockerfile and requirements"
```

---

## Task 2: Feature Engineering

**Files:**
- Create: `ml/features.py`

- [ ] **Step 1: Create `ml/features.py`**

```python
"""
Feature engineering for match outcome prediction.

Features per match:
  implied_prob_home  - average implied probability from bookmaker odds (home win)
  implied_prob_draw  - average implied probability from bookmaker odds (draw)
  implied_prob_away  - average implied probability from bookmaker odds (away win)
  elo_home           - simple ELO rating for home team (computed from results history)
  elo_away           - simple ELO rating for away team
  elo_diff           - elo_home - elo_away
  home_win_rate_5    - home team win rate in last 5 home matches
  away_win_rate_5    - away team win rate in last 5 away matches
  goal_diff_home_5   - home team avg goal diff in last 5 home matches
  goal_diff_away_5   - away team avg goal diff in last 5 away matches
"""

import pandas as pd
import numpy as np
from db import get_connection

FEATURE_COLS = [
    "implied_prob_home",
    "implied_prob_draw",
    "implied_prob_away",
    "elo_diff",
    "home_win_rate_5",
    "away_win_rate_5",
    "goal_diff_home_5",
    "goal_diff_away_5",
]

TARGET_COL = "outcome_encoded"  # 0=home, 1=draw, 2=away


def fetch_training_data(conn) -> pd.DataFrame:
    """
    Join matches + results + odds_normalized to build raw training rows.
    Returns one row per match that has a result.
    """
    query = """
        SELECT
            m.id          AS match_id,
            m.home_team,
            m.away_team,
            m.starts_at,
            r.outcome,
            r.score_home,
            r.score_away,
            AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_home, 0) END) AS implied_prob_home,
            AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_draw, 0) END) AS implied_prob_draw,
            AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_away, 0) END) AS implied_prob_away
        FROM matches m
        JOIN results r ON r.match_id = m.id
        JOIN odds_normalized o ON o.match_id = m.id
        GROUP BY m.id, m.home_team, m.away_team, m.starts_at, r.outcome, r.score_home, r.score_away
        ORDER BY m.starts_at ASC
    """
    return pd.read_sql(query, conn)


def compute_elo(df: pd.DataFrame, k: int = 20) -> pd.DataFrame:
    """
    Compute ELO ratings in chronological order.
    Adds elo_home and elo_away columns. Modifies df in place.
    """
    elo = {}  # team -> rating, default 1500

    def get_elo(team):
        return elo.get(team, 1500.0)

    elo_home_list, elo_away_list = [], []

    for _, row in df.iterrows():
        h, a = row["home_team"], row["away_team"]
        eh, ea = get_elo(h), get_elo(a)
        elo_home_list.append(eh)
        elo_away_list.append(ea)

        # Expected scores
        exp_h = 1 / (1 + 10 ** ((ea - eh) / 400))
        exp_a = 1 - exp_h

        # Actual scores
        if row["outcome"] == "home":
            act_h, act_a = 1.0, 0.0
        elif row["outcome"] == "away":
            act_h, act_a = 0.0, 1.0
        else:
            act_h, act_a = 0.5, 0.5

        elo[h] = eh + k * (act_h - exp_h)
        elo[a] = ea + k * (act_a - exp_a)

    df["elo_home"] = elo_home_list
    df["elo_away"] = elo_away_list
    df["elo_diff"] = df["elo_home"] - df["elo_away"]
    return df


def compute_rolling_stats(df: pd.DataFrame, n: int = 5) -> pd.DataFrame:
    """
    Compute rolling win rates and goal diffs per team over last n matches.
    Adds home_win_rate_5, away_win_rate_5, goal_diff_home_5, goal_diff_away_5.
    """
    home_win_rates, away_win_rates = [], []
    home_gdiffs, away_gdiffs = [], []

    # Build per-team history up to (but not including) current match
    team_history = {}  # team -> list of (is_win, goal_diff) from home perspective

    for idx, row in df.iterrows():
        h, a = row["home_team"], row["away_team"]
        h_hist = team_history.get(h, [])
        a_hist = team_history.get(a, [])

        last_h = h_hist[-n:] if len(h_hist) >= n else h_hist
        last_a = a_hist[-n:] if len(a_hist) >= n else a_hist

        home_win_rates.append(np.mean([x[0] for x in last_h]) if last_h else 0.5)
        away_win_rates.append(np.mean([x[0] for x in last_a]) if last_a else 0.5)
        home_gdiffs.append(np.mean([x[1] for x in last_h]) if last_h else 0.0)
        away_gdiffs.append(np.mean([x[1] for x in last_a]) if last_a else 0.0)

        # Update history
        gd_h = row["score_home"] - row["score_away"]
        gd_a = -gd_h
        is_home_win = 1.0 if row["outcome"] == "home" else 0.0
        is_away_win = 1.0 if row["outcome"] == "away" else 0.0

        team_history.setdefault(h, []).append((is_home_win, gd_h))
        team_history.setdefault(a, []).append((is_away_win, gd_a))

    df["home_win_rate_5"] = home_win_rates
    df["away_win_rate_5"] = away_win_rates
    df["goal_diff_home_5"] = home_gdiffs
    df["goal_diff_away_5"] = away_gdiffs
    return df


def encode_outcome(df: pd.DataFrame) -> pd.DataFrame:
    """Encode outcome string → int: home=0, draw=1, away=2."""
    mapping = {"home": 0, "draw": 1, "away": 2}
    df[TARGET_COL] = df["outcome"].map(mapping)
    return df


def build_dataset(conn) -> tuple[pd.DataFrame, pd.Series]:
    """
    Full pipeline: fetch → ELO → rolling stats → encode → return X, y.
    """
    df = fetch_training_data(conn)
    if df.empty:
        return pd.DataFrame(columns=FEATURE_COLS), pd.Series(dtype=int)

    df = compute_elo(df)
    df = compute_rolling_stats(df)
    df = encode_outcome(df)

    df = df.dropna(subset=FEATURE_COLS + [TARGET_COL])

    X = df[FEATURE_COLS]
    y = df[TARGET_COL].astype(int)
    return X, y
```

- [ ] **Step 2: Verify Python syntax**

```bash
cd /Users/giancarlogoulart/Projects/Personal/sportstips
docker compose run --rm ml python -c "import features; print('features.py OK')"
```

Expected: `features.py OK`

- [ ] **Step 3: Commit**

```bash
git add ml/features.py
git commit -m "feat: ml feature engineering with ELO ratings and rolling stats"
```

---

## Task 3: Training Pipeline

**Files:**
- Create: `ml/train.py`

- [ ] **Step 1: Create `ml/train.py`**

```python
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


def save_metrics(conn, model_version: str, brier: float, logloss: float, n_samples: int):
    with conn.cursor() as cur:
        cur.execute("""
            INSERT INTO ml_model_metrics (model_version, brier_score, log_loss, sample_size)
            VALUES (%s, %s, %s, %s)
        """, (model_version, float(brier), float(logloss), n_samples))
    conn.commit()


def main():
    conn = get_connection()
    X, y = build_dataset(conn)

    if len(X) < 20:
        print(f"Not enough training data: {len(X)} samples (need >= 20). Exiting.")
        conn.close()
        sys.exit(0)

    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=0.2, random_state=42, stratify=y if len(y.unique()) > 1 else None
    )

    model = LogisticRegression(max_iter=1000, random_state=42)
    model.fit(X_train, y_train)

    # Brier score (multi-class): average over outcomes
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

    model_version = f"lr_{date.today().isoformat()}"

    MODELS_DIR.mkdir(parents=True, exist_ok=True)
    model_path = MODELS_DIR / f"model_{model_version}.pkl"
    joblib.dump({"model": model, "version": model_version, "features": FEATURE_COLS}, model_path)

    # Also write to stable path for predict.py to load
    latest_path = MODELS_DIR / "model_latest.pkl"
    joblib.dump({"model": model, "version": model_version, "features": FEATURE_COLS}, latest_path)

    save_metrics(conn, model_version, brier, logloss, len(X))
    conn.close()

    print(f"Model trained: {model_version}")
    print(f"  Samples: {len(X)} train={len(X_train)} test={len(X_test)}")
    print(f"  Brier score: {brier:.4f}")
    print(f"  Log loss:    {logloss:.4f}")
    print(f"  Saved to:    {model_path}")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Verify syntax**

```bash
docker compose run --rm ml python -c "import train; print('train.py OK')"
```

Expected: `train.py OK`

- [ ] **Step 3: Commit**

```bash
git add ml/train.py
git commit -m "feat: ml training pipeline with LogisticRegression and Brier score metrics"
```

---

## Task 4: Batch Prediction Service (Python)

**Files:**
- Create: `ml/predict.py`

- [ ] **Step 1: Create `ml/predict.py`**

```python
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
LATEST_MODEL = MODELS_DIR / "model_latest.pkl"


def fetch_upcoming_matches(conn) -> pd.DataFrame:
    """Return upcoming/live matches that need predictions."""
    query = """
        SELECT
            m.id AS match_id,
            m.home_team,
            m.away_team,
            m.starts_at,
            AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_home, 0) END) AS implied_prob_home,
            AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_draw, 0) END) AS implied_prob_draw,
            AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_away, 0) END) AS implied_prob_away
        FROM matches m
        LEFT JOIN odds_normalized o ON o.match_id = m.id
        WHERE m.status IN ('upcoming', 'live')
          AND m.starts_at < NOW() + INTERVAL '48 hours'
        GROUP BY m.id, m.home_team, m.away_team, m.starts_at
        HAVING AVG(CASE WHEN o.market = '1x2' THEN 1.0 / NULLIF(o.odds_home, 0) END) IS NOT NULL
    """
    return pd.read_sql(query, conn)


def build_prediction_features(upcoming: pd.DataFrame, history: pd.DataFrame) -> pd.DataFrame:
    """
    Compute ELO and rolling stats for upcoming matches using historical data as base.
    Returns upcoming with all FEATURE_COLS filled.
    """
    # Start from history (sorted chronologically) to build team state
    # then apply that state to upcoming matches
    combined = pd.concat([history, upcoming], ignore_index=True)
    combined = compute_elo(combined)
    combined = compute_rolling_stats(combined)

    # Return only the upcoming rows
    result = combined.tail(len(upcoming)).copy()
    return result


def insert_predictions(conn, predictions: list[dict]):
    with conn.cursor() as cur:
        for p in predictions:
            cur.execute("""
                INSERT INTO ml_predictions (match_id, model_version, prob_home, prob_draw, prob_away)
                VALUES (%s, %s, %s, %s, %s)
                ON CONFLICT DO NOTHING
            """, (p["match_id"], p["model_version"], p["prob_home"], p["prob_draw"], p["prob_away"]))
    conn.commit()


def main():
    if not LATEST_MODEL.exists():
        print("No trained model found. Run train.py first.")
        sys.exit(0)

    bundle = joblib.load(LATEST_MODEL)
    model = bundle["model"]
    model_version = bundle["version"]

    conn = get_connection()

    upcoming = fetch_upcoming_matches(conn)
    if upcoming.empty:
        print("No upcoming matches to predict.")
        conn.close()
        return

    # Build historical baseline for ELO/rolling stats (team history for ELO/rolling computation)
    from features import fetch_training_data as _fetch_history
    history = _fetch_history(conn)

    features_df = build_prediction_features(upcoming, history)

    # Fill missing feature columns with 0
    for col in FEATURE_COLS:
        if col not in features_df.columns:
            features_df[col] = 0.0
    features_df[FEATURE_COLS] = features_df[FEATURE_COLS].fillna(0.0)

    X = features_df[FEATURE_COLS]
    proba = model.predict_proba(X)
    classes = list(model.classes_)  # e.g. [0, 1, 2] = home, draw, away

    # Map class indices to probabilities
    idx_home = classes.index(0) if 0 in classes else 0
    idx_draw = classes.index(1) if 1 in classes else 1
    idx_away = classes.index(2) if 2 in classes else 2

    predictions = []
    for i, (_, row) in enumerate(upcoming.iterrows()):
        predictions.append({
            "match_id": row["match_id"],
            "model_version": model_version,
            "prob_home": float(proba[i][idx_home]),
            "prob_draw": float(proba[i][idx_draw]) if proba.shape[1] > 2 else 0.0,
            "prob_away": float(proba[i][idx_away]),
        })

    insert_predictions(conn, predictions)
    conn.close()

    print(f"Predictions written: {len(predictions)} matches (model={model_version})")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Verify syntax**

```bash
docker compose run --rm ml python -c "import predict; print('predict.py OK')"
```

Expected: `predict.py OK`

- [ ] **Step 3: Commit**

```bash
git add ml/predict.py
git commit -m "feat: batch prediction service writes to ml_predictions table"
```

---

## Task 5: Model Drift Monitor

**Files:**
- Create: `ml/monitor.py`

- [ ] **Step 1: Create `ml/monitor.py`**

```python
"""
Model drift monitor.
Compares recent Brier score against baseline.
Logs a warning if degradation exceeds 15%.
"""

import sys
from db import get_connection


def get_baseline_brier(conn) -> float | None:
    """Return the best (lowest) Brier score ever recorded."""
    with conn.cursor() as cur:
        cur.execute("""
            SELECT MIN(brier_score) FROM ml_model_metrics
        """)
        row = cur.fetchone()
        return float(row[0]) if row and row[0] is not None else None


def get_recent_brier(conn, days: int = 7) -> float | None:
    """Return the most recent Brier score within the last N days."""
    with conn.cursor() as cur:
        cur.execute("""
            SELECT brier_score FROM ml_model_metrics
            WHERE trained_at > NOW() - INTERVAL '%s days'
            ORDER BY trained_at DESC
            LIMIT 1
        """, (days,))
        row = cur.fetchone()
        return float(row[0]) if row and row[0] is not None else None


def main():
    conn = get_connection()
    baseline = get_baseline_brier(conn)
    recent = get_recent_brier(conn)
    conn.close()

    if baseline is None or recent is None:
        print("Insufficient metric history for drift check.")
        sys.exit(0)

    threshold = baseline * 1.15
    degradation_pct = ((recent - baseline) / baseline) * 100

    print(f"Brier baseline: {baseline:.4f}")
    print(f"Brier recent:   {recent:.4f}")
    print(f"Degradation:    {degradation_pct:+.1f}%")

    if recent > threshold:
        print(f"WARNING: Model drift detected! Brier {recent:.4f} > threshold {threshold:.4f}")
        print("Action: review features and retrain with fresh data.")
        sys.exit(1)  # non-zero exit so cron/alerting can detect
    else:
        print("Model drift: OK")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Verify syntax**

```bash
docker compose run --rm ml python -c "import monitor; print('monitor.py OK')"
```

Expected: `monitor.py OK`

- [ ] **Step 3: Commit**

```bash
git add ml/monitor.py
git commit -m "feat: model drift monitor with 15% Brier score threshold"
```

---

## Task 6: Go — BatchPredictionService

**Files:**
- Create: `internal/predictions/batch.go`

`BatchPredictionService` reads the latest prediction from `ml_predictions` for a given match.

- [ ] **Step 1: Write failing test**

Create `internal/predictions/batch_test.go`:

```go
package predictions_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

// BatchPredictionService is tested via integration (needs DB).
// This file ensures the type compiles and satisfies the interface.

func TestBatchPredictionService_ImplementsInterface(t *testing.T) {
	// Compile-time check: *BatchPredictionService satisfies PredictionService.
	// If this compiles, the interface is implemented.
	var _ PredictionService = (*BatchPredictionService)(nil)
	assert.True(t, true) // dummy assertion so test runner counts it
}
```

- [ ] **Step 2: Run test — expect FAIL**

```bash
cd /Users/giancarlogoulart/Projects/Personal/sportstips
go test ./internal/predictions/... -v 2>&1 | head -10
```

Expected: `FAIL: BatchPredictionService undefined`

- [ ] **Step 3: Create `internal/predictions/batch.go`**

```go
package predictions

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BatchPredictionService reads predictions from ml_predictions table.
// Python predict.py populates this table on a schedule.
type BatchPredictionService struct {
	pool *pgxpool.Pool
}

func NewBatchPredictionService(pool *pgxpool.Pool) *BatchPredictionService {
	return &BatchPredictionService{pool: pool}
}

func (s *BatchPredictionService) Predict(ctx context.Context, matchID string) (Prediction, error) {
	var p Prediction
	err := s.pool.QueryRow(ctx, `
		SELECT prob_home, prob_draw, prob_away, model_version
		FROM ml_predictions
		WHERE match_id = $1
		ORDER BY predicted_at DESC
		LIMIT 1
	`, matchID).Scan(&p.ProbHome, &p.ProbDraw, &p.ProbAway, &p.ModelVersion)
	if err != nil {
		return Prediction{}, fmt.Errorf("batch predict: %w", err)
	}
	return p, nil
}
```

- [ ] **Step 4: Update batch_test.go to use correct imports**

Replace `internal/predictions/batch_test.go` with:

```go
package predictions

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestBatchPredictionService_ImplementsInterface(t *testing.T) {
	var _ PredictionService = (*BatchPredictionService)(nil)
	assert.True(t, true)
}
```

- [ ] **Step 5: Run test — expect PASS**

```bash
go test ./internal/predictions/... -v
```

Expected: `PASS`

- [ ] **Step 6: Commit**

```bash
git add internal/predictions/batch.go internal/predictions/batch_test.go
git commit -m "feat: BatchPredictionService reads ml_predictions from DB"
```

---

## Task 7: Go — Value Bet Detection Engine

**Files:**
- Create: `internal/engine/valuebet.go`
- Create: `internal/engine/valuebet_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/engine/valuebet_test.go
package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sportstips/internal/predictions"
	"sportstips/internal/store"
)

func TestDetectValueBets_Found(t *testing.T) {
	// odds=2.30 → implied_prob=0.435
	// model says 0.55 → edge=0.115 > 0.05 threshold → value bet
	odds := []store.NormalizedOdds{
		{Bookmaker: "bet365", Market: "1x2", OddsHome: 2.30, OddsDraw: 3.50, OddsAway: 3.00},
	}
	pred := predictions.Prediction{
		ProbHome:     0.55,
		ProbDraw:     0.25,
		ProbAway:     0.20,
		ModelVersion: "lr_test",
	}

	signals := DetectValueBets("match-1", odds, pred, 0.05)
	assert.Len(t, signals, 1)
	assert.Equal(t, "value_bet", signals[0].Type)
	assert.Equal(t, "home", signals[0].Outcome)
	assert.InDelta(t, 0.115, signals[0].Edge, 0.001)
}

func TestDetectValueBets_NotFound(t *testing.T) {
	// model prob equals implied prob → no edge
	odds := []store.NormalizedOdds{
		{Bookmaker: "bet365", Market: "1x2", OddsHome: 2.00, OddsDraw: 3.50, OddsAway: 3.00},
	}
	pred := predictions.Prediction{
		ProbHome:     0.50, // exactly implied (1/2.00)
		ProbDraw:     0.286,
		ProbAway:     0.214,
		ModelVersion: "lr_test",
	}

	signals := DetectValueBets("match-1", odds, pred, 0.05)
	assert.Empty(t, signals)
}

func TestDetectValueBets_BelowThreshold(t *testing.T) {
	// small edge (2%) below threshold (5%) → not triggered
	odds := []store.NormalizedOdds{
		{Bookmaker: "bet365", Market: "1x2", OddsHome: 2.10, OddsDraw: 3.50, OddsAway: 3.00},
	}
	pred := predictions.Prediction{
		ProbHome:     0.50, // implied=0.476, edge=0.024 < 0.05
		ProbDraw:     0.28,
		ProbAway:     0.22,
		ModelVersion: "lr_test",
	}

	signals := DetectValueBets("match-1", odds, pred, 0.05)
	assert.Empty(t, signals)
}
```

- [ ] **Step 2: Run test — expect FAIL**

```bash
go test ./internal/engine/... -run TestDetectValueBets -v
```

Expected: `FAIL: DetectValueBets undefined`

- [ ] **Step 3: Create `internal/engine/valuebet.go`**

```go
package engine

import (
	"encoding/json"
	"fmt"

	"sportstips/internal/predictions"
	"sportstips/internal/store"
)

// ValueBetSignal represents a detected value bet opportunity.
type ValueBetSignal struct {
	MatchID      string
	Type         string
	Market       string
	Outcome      string  // "home" | "draw" | "away"
	Bookmaker    string
	Odds         float64
	ImpliedProb  float64
	ModelProb    float64
	Edge         float64
	KellyFull    float64
	KellyHalf    float64
	ModelVersion string
}

// DetectValueBets finds value bets for a match given current odds and ML prediction.
// Only checks 1x2 market. minEdge: minimum model_prob - implied_prob (e.g. 0.05 = 5%).
func DetectValueBets(
	matchID string,
	odds []store.NormalizedOdds,
	pred predictions.Prediction,
	minEdge float64,
) []ValueBetSignal {
	if pred.ModelVersion == "" {
		return nil
	}

	var signals []ValueBetSignal

	for _, o := range odds {
		if o.Market != "1x2" {
			continue
		}

		type candidate struct {
			outcome   string
			bookmaker string
			oddsVal   float64
			modelProb float64
		}

		candidates := []candidate{
			{"home", o.Bookmaker, o.OddsHome, pred.ProbHome},
			{"draw", o.Bookmaker, o.OddsDraw, pred.ProbDraw},
			{"away", o.Bookmaker, o.OddsAway, pred.ProbAway},
		}

		for _, c := range candidates {
			if c.oddsVal <= 1.0 || c.modelProb <= 0 {
				continue
			}
			k := Kelly(c.oddsVal, c.modelProb)
			if !k.IsValueBet || k.Edge < minEdge {
				continue
			}
			signals = append(signals, ValueBetSignal{
				MatchID:      matchID,
				Type:         "value_bet",
				Market:       "1x2",
				Outcome:      c.outcome,
				Bookmaker:    c.bookmaker,
				Odds:         c.oddsVal,
				ImpliedProb:  k.ImpliedProb,
				ModelProb:    c.modelProb,
				Edge:         k.Edge,
				KellyFull:    k.KellyFull,
				KellyHalf:    k.KellyHalf,
				ModelVersion: pred.ModelVersion,
			})
		}
	}

	return signals
}

// ToStoreSignal converts ValueBetSignal to store.Signal for persistence.
func (v ValueBetSignal) ToStoreSignal() (store.Signal, error) {
	data := map[string]any{
		"outcome":              v.Outcome,
		"bookmaker":            v.Bookmaker,
		"odds":                 v.Odds,
		"implied_prob":         v.ImpliedProb,
		"model_prob":           v.ModelProb,
		"edge":                 fmt.Sprintf("%.2f%%", v.Edge*100),
		"kelly_full":           v.KellyFull,
		"kelly_half":           v.KellyHalf,
		"recommended_stake_pct": v.KellyHalf * 100,
		"model_version":        v.ModelVersion,
	}
	b, err := json.Marshal(data)
	if err != nil {
		return store.Signal{}, err
	}
	return store.Signal{
		MatchID:    v.MatchID,
		Type:       "value_bet",
		Market:     v.Market,
		Data:       b,
		Confidence: v.Edge,
	}, nil
}
```

- [ ] **Step 4: Run test — expect PASS**

```bash
go test ./internal/engine/... -run TestDetectValueBets -v
```

Expected: all 3 tests PASS

- [ ] **Step 5: Run all tests**

```bash
go test ./... -v 2>&1 | tail -20
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/engine/valuebet.go internal/engine/valuebet_test.go
git commit -m "feat: value bet detection using ML predictions and Kelly criterion"
```

---

## Task 8: Wire — Scheduler Value Bets + BatchPredictionService

**Files:**
- Modify: `internal/ingestion/scheduler.go`
- Modify: `cmd/server/main.go`

The scheduler's `runEngine` currently stores only arbitrage signals. Extend it to also run value bet detection using `PredictionService`.

- [ ] **Step 1: Add PredictionService to Scheduler**

In `internal/ingestion/scheduler.go`, update `Scheduler` struct and constructor:

```go
type Scheduler struct {
	store       *store.Store
	primary     OddsClient
	fallback    OddsClient
	sports      []string
	log         *slog.Logger
	predictions predictions.PredictionService
}

func NewScheduler(s *store.Store, primary, fallback OddsClient, sports []string, log *slog.Logger, pred predictions.PredictionService) *Scheduler {
	return &Scheduler{
		store:       s,
		primary:     primary,
		fallback:    fallback,
		sports:      sports,
		log:         log,
		predictions: pred,
	}
}
```

Add import `"sportstips/internal/predictions"` to scheduler.go.

- [ ] **Step 2: Extend runEngine to detect value bets**

In `runEngine`, after arbitrage signal block, add value bet detection:

```go
// Value bet detection (requires ML prediction)
pred, err := s.predictions.Predict(ctx, matchID)
if err == nil {
    vbSignals := engine.DetectValueBets(matchID, odds, pred, prefs.MinValueEdge)
    for _, vb := range vbSignals {
        sig, err := vb.ToStoreSignal()
        if err != nil {
            s.log.Error("vb to signal", "err", err)
            continue
        }
        signals = append(signals, sig)
    }
}
// If prediction fails (no ML data yet), skip silently — not an error in Phase 3 startup
```

The full updated `runEngine` method:

```go
func (s *Scheduler) runEngine(ctx context.Context, matchID string) {
	odds, err := s.store.GetLatestOddsByMatch(ctx, matchID)
	if err != nil {
		s.log.Error("get odds for engine", "matchID", matchID, "err", err)
		return
	}

	tenants, err := s.store.GetAllTenants(ctx)
	if err != nil {
		s.log.Error("get tenants", "err", err)
		return
	}

	// Fetch ML prediction once per match (shared across tenants)
	pred, predErr := s.predictions.Predict(ctx, matchID)

	for _, tenant := range tenants {
		prefs, err := s.store.GetPreferences(ctx, tenant.ID)
		if err != nil {
			s.log.Error("get preferences", "tenantID", tenant.ID, "err", err)
			continue
		}

		var signals []store.Signal

		// Arbitrage signals
		arbSignals := engine.DetectArbitrage(matchID, odds, prefs.MinArbProfit)
		for _, arb := range arbSignals {
			sig, err := arb.ToStoreSignal()
			if err != nil {
				s.log.Error("arb to signal", "err", err)
				continue
			}
			signals = append(signals, sig)
		}

		// Value bet signals (only if ML prediction available)
		if predErr == nil {
			vbSignals := engine.DetectValueBets(matchID, odds, pred, prefs.MinValueEdge)
			for _, vb := range vbSignals {
				sig, err := vb.ToStoreSignal()
				if err != nil {
					s.log.Error("vb to signal", "err", err)
					continue
				}
				signals = append(signals, sig)
			}
		}

		if len(signals) == 0 {
			continue
		}

		if err := s.store.InsertSignals(ctx, tenant.ID, signals); err != nil {
			s.log.Error("insert signals", "tenantID", tenant.ID, "err", err)
			continue
		}

		s.log.Info("signals stored",
			"tenantID", tenant.ID,
			"matchID", matchID,
			"count", len(signals))
	}
}
```

- [ ] **Step 3: Update cmd/server/main.go**

Replace `StubPredictionService` with `BatchPredictionService`. Find and replace:

```go
// PredictionService wired in Phase 3 — stub satisfies interface for compilation
var _ predictions.PredictionService = &predictions.StubPredictionService{}
```

With:

```go
predSvc := predictions.NewBatchPredictionService(db.Pool())
```

Also update the scheduler construction to pass `predSvc`:

```go
scheduler := ingestion.NewScheduler(db, primaryClient, fallbackClient,
    []string{
        "soccer_epl",
        "soccer_spain_la_liga",
        "soccer_italy_serie_a",
        "soccer_germany_bundesliga",
        "soccer_uefa_champs_league",
    },
    log, predSvc)
```

- [ ] **Step 4: Expose Pool() on Store**

`BatchPredictionService` needs a `*pgxpool.Pool`. Add a `Pool()` accessor to `internal/store/db.go`:

```go
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}
```

- [ ] **Step 5: Build**

```bash
cd /Users/giancarlogoulart/Projects/Personal/sportstips
go build ./...
```

Fix ALL compile errors. Common issue: `NewScheduler` now takes 6 args — update any test files that construct a Scheduler.

- [ ] **Step 6: Run all tests**

```bash
go test ./... -v 2>&1 | tail -20
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ingestion/scheduler.go internal/predictions/batch.go \
        internal/store/db.go cmd/server/main.go
git commit -m "feat: wire BatchPredictionService and value bet detection into scheduler"
```

---

## Task 9: API — Predictions Endpoint

**Files:**
- Create: `internal/api/predictions.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Create `internal/api/predictions.go`**

```go
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"sportstips/internal/auth"
)

func (h *Handler) getPrediction(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	matchID := chi.URLParam(r, "match_id")
	pred, err := h.pred.Predict(r.Context(), matchID)
	if err != nil {
		writeError(w, http.StatusNotFound, "prediction not available")
		return
	}
	writeJSON(w, http.StatusOK, pred)
}
```

- [ ] **Step 2: Add pred field to Handler and update NewHandler**

In `internal/api/router.go`, add `pred predictions.PredictionService` to `Handler`:

```go
import (
    "encoding/json"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "sportstips/internal/auth"
    "sportstips/internal/predictions"
    "sportstips/internal/results"
    "sportstips/internal/store"
)

type Handler struct {
	store     *store.Store
	jwtSecret string
	ingester  *results.Ingester
	pred      predictions.PredictionService
}

func NewHandler(s *store.Store, jwtSecret string, ingester *results.Ingester, pred predictions.PredictionService) *Handler {
	return &Handler{store: s, jwtSecret: jwtSecret, ingester: ingester, pred: pred}
}
```

Add route inside authenticated group:
```go
r.Get("/predictions/{match_id}", h.getPrediction)
```

- [ ] **Step 3: Update cmd/server/main.go**

Update `api.NewHandler` call to pass `predSvc`:

```go
handler := api.NewHandler(db, cfg.JWTSecret, ingester, predSvc)
```

- [ ] **Step 4: Build**

```bash
go build ./...
```

- [ ] **Step 5: Run all tests**

```bash
go test ./... 2>&1 | tail -10
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/predictions.go internal/api/router.go cmd/server/main.go
git commit -m "feat: GET /predictions/:match_id endpoint"
```

---

## Task 10: End-to-End Smoke Test — Phase 3

- [ ] **Step 1: Run full ML cycle (requires real DB data from Phase 1+2)**

```bash
cd /Users/giancarlogoulart/Projects/Personal/sportstips

# Train (needs ≥20 matches with results)
docker compose run --rm ml python train.py

# Predict
docker compose run --rm ml python predict.py

# Monitor
docker compose run --rm ml python monitor.py
```

If not enough data: `"Not enough training data"` is expected. The Go server still works — predictions endpoint returns 404, scheduler skips value bets silently.

- [ ] **Step 2: Start server**

```bash
go run ./cmd/server/... &
SERVER_PID=$!
sleep 4
```

- [ ] **Step 3: Get token**

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"phase2smoke@test.com","password":"test123"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
```

- [ ] **Step 4: Check predictions endpoint**

```bash
# Get a match ID from /matches
MATCH_ID=$(curl -s http://localhost:8080/matches \
  -H "Authorization: Bearer $TOKEN" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

curl -s "http://localhost:8080/predictions/$MATCH_ID" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

Expected: prediction JSON or `{"error":"prediction not available"}` if no ML data yet — both are correct.

- [ ] **Step 5: Check value_bet signals**

```bash
curl -s "http://localhost:8080/signals?type=value_bet" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

Expected: `[]` initially (needs ML data to trigger).

- [ ] **Step 6: Stop server and commit**

```bash
kill $SERVER_PID 2>/dev/null || true
git add .
git commit -m "feat: phase 3 complete — ML pipeline, value bet detection, predictions endpoint" --allow-empty
```

---

## Cron Schedule (Hetzner deploy)

Add to crontab on Hetzner VPS after deploy:

```cron
0 3 * * *   cd /opt/sportstips && docker compose run --rm ml python train.py >> /var/log/sportstips-ml.log 2>&1
0 4 * * *   cd /opt/sportstips && docker compose run --rm ml python predict.py >> /var/log/sportstips-ml.log 2>&1
0 */6 * * * cd /opt/sportstips && docker compose run --rm ml python predict.py >> /var/log/sportstips-ml.log 2>&1
0 5 * * *   cd /opt/sportstips && docker compose run --rm ml python monitor.py >> /var/log/sportstips-ml.log 2>&1
```

---

## Next Phase

- **Phase 4** (`docs/superpowers/plans/YYYY-MM-DD-phase4-dashboard-alerts.md`): React dashboard, Telegram alerts, backtesting framework, tenant preferences UI
