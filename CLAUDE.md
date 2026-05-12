# Betting Intelligence Agent (with ML Pipeline)

## Objective

Aggregate sports betting odds, detect arbitrage + value bets, use ML to estimate true probabilities, provide a monitoring dashboard. No automatic betting.

---

## Current Status (2026-05-11)

### ✅ Implemented

**Phase 1 — Core** (complete)
- Odds ingestion: Odds API client + OddsPapi fallback (disabled — TLS SNI issue on their server)
- PostgreSQL schema: tenants, matches, odds_raw, odds_normalized, results, signals, ml_predictions, preferences (7 migrations)
- Odds normalization layer
- Arbitrage detection + stake calc + Kelly Criterion
- JWT auth + multi-tenancy (RLS on signals)
- REST API: auth, matches, odds, signals, results, predictions, preferences

**Phase 2 — Historical Data** (complete)
- Results ingestion endpoint
- `ml_predictions` table ready

**Phase 3 — ML Pipeline** (complete)
- `ml/features.py` — feature engineering from DB
- `ml/train.py` — Logistic Regression, saves model metrics to DB
- `ml/predict.py` — batch predictions → `ml_predictions` table
- `ml/monitor.py` — Brier score drift detection
- `internal/predictions/batch.go` — BatchPredictionService reads from DB
- `internal/engine/valuebet.go` — value bet detection with Kelly edge threshold
- `internal/alerts/` — Telegram alerts

**Tech Stack (actual)**
- API: Go 1.25, chi router, pgx/v5, golang-jwt
- ML: Python 3.11, scikit-learn, pandas, psycopg2, joblib
- DB: PostgreSQL 16
- Deploy: Railway

### 🚧 Not Done

**Phase 4 — Deployment + Dashboard**
- GitHub repo not created (no remote) — Railway auto-deploy blocked
- Railway services created but not connected:
  - `postgres` service: CRASHED (needs env vars fixed)
  - `api` service: created, no deployment, domain: `api-production-e453.up.railway.app`
  - `ml` service: created, no deployment
- `DATABASE_URL` reference var not set (blocked by missing GitHub repo)
- Dashboard (frontend) — not started
- Email alerts — not implemented
- Telegram bot token — not configured

### Next Steps (in order)

1. Create GitHub repo + push (`brew install gh && gh auth login && gh repo create sportstips --private --source=. --push`)
2. Wire `DATABASE_URL` reference from postgres → api and ml services
3. Fix postgres service crash (check env vars)
4. Connect Railway services to repo, set `rootDirectory=ml` on ml service
5. Build GitHub Actions CI pipeline (go test gate → railway deploy)
6. Configure Telegram bot token
7. Build dashboard

---

## APIs

- **Primary**: Odds API (`ODDS_API_KEY` set on Railway)
- **Secondary**: OddsPapi (disabled — TLS SNI failure on their server)
- **Optional**: OpticOdds

---

## Railway Infrastructure

- Project: `discerning-manifestation` (`b78c5ac1-4deb-41c5-b74a-b2c1fe6f33d5`)
- Environment: `production` (`e8839995-9519-404e-8ee2-1c75270a4d91`)
- Services:
  - `postgres` — `575f62a8-d8bc-4d4b-ba0c-34b8e28239f5`
  - `api` — `03ba04f0-4254-439a-b425-4ffa0fe92500`
  - `ml` — `37c425d3-2f59-494d-9115-fff6e32edf94`

---

## Architecture

```
Odds API → ingestion/scheduler.go (every 30s adaptive)
         → store: odds_raw + odds_normalized
         → engine: arbitrage detection → signals
         → predictions/batch.go (reads ml_predictions)
         → engine: value bet detection → signals
         → alerts: Telegram

ml/train.py (daily cron) → model.pkl + metrics in DB
ml/predict.py (batch)    → ml_predictions table
ml/monitor.py            → Brier score drift alert
```

---

## DB Schema (7 migrations)

| Table | Purpose |
|-------|---------|
| `tenants` | multi-tenancy |
| `matches` | match metadata |
| `odds_raw` | raw API responses |
| `odds_normalized` | cleaned odds |
| `results` | final outcomes (ML training) |
| `ml_predictions` | batch model output |
| `signals` | arb + value bet signals (RLS) |
| `preferences` | per-tenant thresholds |

---

## Constraints

- DO NOT place bets automatically — recommendations only
- Respect API rate limits
- Log all predictions and decisions

---

## Coding Guidelines

- Modular architecture
- ML pipeline separate from API layer
- All config via environment variables
- Per-sport ML models (baseball_mlb, basketball_nba each get own model)
- 2-outcome sports (e.g. MLB) skip draw probability
