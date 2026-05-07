# Betting Intelligence Agent — Design Spec

**Date:** 2026-05-05
**Status:** Approved

---

## Objective

Multi-tenant system that aggregates sports betting odds, identifies arbitrage opportunities, detects value bets using ML predictions, and provides a monitoring dashboard. No automatic betting — recommendations only.

---

## Decisions Log

| Decision | Choice | Reason |
|---|---|---|
| Backend | Go (monolito modular) | User dominates Go, performance on ingestion |
| ML | Python (batch worker) | Único ecossistema viável pra XGBoost/sklearn |
| ML communication | Interface Go → DB (batch) | Simpler MVP; interface permite migrar pra gRPC sem retrabalho |
| DB | PostgreSQL + RLS | Multi-tenancy via Row-Level Security |
| Cache | Sem Redis (MVP) | Não necessário na fase atual |
| Deploy | Hetzner VPS (CX22, ~€4/mês) | Custo fixo, controle total, ML batch sem limite de memória |
| Dashboard | React | User pode manter |
| Alerts | Telegram Bot | Simples, async |
| Esportes | Futebol primeiro | Basketball/baseball em fases futuras |
| Odds source | Odds API (primary) + OddsPapi (fallback) | |
| Polling | Adaptativo por estado do jogo | Respeitar Starter tier ~10k req/mês |
| Results | Odds API scores endpoint + fallback football-data.org | Mantém external_id consistente |

---

## Architecture

### Stack

| Camada | Tech |
|---|---|
| API + Ingestion + Engine | Go (monolito modular) |
| ML Pipeline | Python (batch worker) |
| DB | PostgreSQL |
| Deploy | Hetzner VPS + Docker Compose |
| Dashboard | React |
| Alertas | Telegram Bot |
| Odds primary | Odds API (Starter) |
| Odds fallback | OddsPapi |

### Project Structure

```
sportstips/
├── cmd/
│   └── server/main.go
├── internal/
│   ├── ingestion/       # polling, normalização, fallback entre fontes
│   ├── engine/          # arbitrage, value bet, kelly criterion
│   ├── predictions/     # interface PredictionService + batch impl (lê DB)
│   ├── alerts/          # telegram notifications
│   ├── auth/            # JWT, multi-tenant middleware
│   └── api/             # HTTP handlers
├── migrations/          # SQL migrations numeradas
├── docker-compose.yml
└── .env.example

ml/
├── features.py          # feature engineering
├── train.py             # treino diário/semanal
├── predict.py           # batch predictions → ml_predictions (PG)
├── results.py           # ingestion de resultados
└── monitor.py           # drift detection + métricas
```

### Data Flow

```
Odds API ──► ingestion ──► odds_raw (PG)
                        ──► odds_normalized (PG)
                               │
                    ┌──────────┴──────────┐
                    ▼                     ▼
              engine/arbitrage      ml/predict.py (cron)
              engine/value_bet ◄── ml_predictions (PG)
                    │
                    ▼
              signals (PG, tenant-scoped)
                    │
          ┌─────────┴─────────┐
          ▼                   ▼
    alerts/telegram        api/ ──► React Dashboard
```

---

## Database Schema

### Multi-tenancy Strategy

Row-Level Security no PostgreSQL. Cada query autentica via `SET LOCAL app.tenant_id = '<uuid>'`. Dados de mercado (odds, matches, predictions) são compartilhados — só `signals` e `tenant_preferences` são isolados por tenant.

### Tables

```sql
CREATE TABLE tenants (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email       TEXT UNIQUE NOT NULL,
    name        TEXT NOT NULL,
    plan        TEXT DEFAULT 'free',
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE matches (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id  TEXT UNIQUE NOT NULL,
    sport        TEXT NOT NULL DEFAULT 'soccer',
    league       TEXT NOT NULL,
    home_team    TEXT NOT NULL,
    away_team    TEXT NOT NULL,
    starts_at    TIMESTAMPTZ NOT NULL,
    status       TEXT DEFAULT 'upcoming', -- upcoming | live | finished
    last_fetched TIMESTAMPTZ,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE odds_raw (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id   UUID REFERENCES matches(id),
    source     TEXT NOT NULL,  -- 'odds_api' | 'oddspapi'
    payload    JSONB NOT NULL,
    fetched_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE odds_normalized (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id   UUID REFERENCES matches(id),
    bookmaker  TEXT NOT NULL,
    market     TEXT NOT NULL,  -- '1x2' | 'over_under_2.5' | 'asian_handicap'
    odds_home  NUMERIC(8,4),
    odds_draw  NUMERIC(8,4),
    odds_away  NUMERIC(8,4),
    timestamp  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE results (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id    UUID REFERENCES matches(id) UNIQUE,
    score_home  INT,
    score_away  INT,
    outcome     TEXT NOT NULL,  -- 'home' | 'draw' | 'away'
    source      TEXT NOT NULL,  -- 'odds_api' | 'football_data'
    recorded_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE ml_predictions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id      UUID REFERENCES matches(id),
    model_version TEXT NOT NULL,
    prob_home     NUMERIC(5,4),
    prob_draw     NUMERIC(5,4),
    prob_away     NUMERIC(5,4),
    predicted_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE ml_model_metrics (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model_version  TEXT NOT NULL,
    brier_score    NUMERIC(6,5),
    log_loss       NUMERIC(6,5),
    sample_size    INT,
    trained_at     TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE signals (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID REFERENCES tenants(id),
    match_id   UUID REFERENCES matches(id),
    type       TEXT NOT NULL,  -- 'arbitrage' | 'value_bet'
    market     TEXT NOT NULL,
    data       JSONB NOT NULL,
    confidence NUMERIC(5,4),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE tenant_preferences (
    tenant_id         UUID PRIMARY KEY REFERENCES tenants(id),
    min_arb_profit    NUMERIC(5,4) DEFAULT 0.01,
    min_value_edge    NUMERIC(5,4) DEFAULT 0.05,
    alert_telegram_id TEXT,
    alert_email       TEXT,
    bookmakers        TEXT[] DEFAULT '{}',  -- empty = todas as casas
    updated_at        TIMESTAMPTZ DEFAULT NOW()
);

-- RLS
ALTER TABLE signals ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON signals
    USING (tenant_id = current_setting('app.tenant_id')::UUID);
```

---

## Decision Engine

### Interface Go (permite migração batch → gRPC sem retrabalho)

```go
type PredictionService interface {
    Predict(ctx context.Context, matchID string) (Prediction, error)
}

// MVP: lê de ml_predictions no PG
type BatchPredictionService struct { db *sql.DB }

// Futuro: gRPC pra Python (troca só aqui)
type GRPCPredictionService struct { client pb.MLClient }
```

### Arbitrage

```
arb_sum = Σ(1/odds_i) para cada outcome
if arb_sum < 1.0:
    profit% = (1 - arb_sum) / arb_sum * 100
    stake_i = bankroll * (1/odds_i) / arb_sum

Threshold: tenant_preferences.min_arb_profit (default 1%)
Cross-bookmaker: engine compara melhor odd de cada outcome entre todas casas
Mercados: 1x2, over/under, asian handicap
```

### Value Bet

```
implied_prob = 1 / odds_bookmaker
model_prob   = ml_predictions.prob_{outcome}
edge         = model_prob - implied_prob

if edge > tenant_preferences.min_value_edge (default 5%):
    → value bet signal
```

### Kelly Criterion (Half-Kelly)

```
b = odds - 1
p = model_prob
q = 1 - p
kelly_full = (b*p - q) / b
kelly_half = kelly_full * 0.5   # reduz variância

recommended_stake_pct = kelly_half * 100
```

### Signal Payload

```json
{
  "type": "value_bet",
  "outcome": "home",
  "bookmaker": "bet365",
  "odds": 2.30,
  "implied_prob": 0.435,
  "model_prob": 0.55,
  "edge": 0.115,
  "kelly_full": 0.21,
  "kelly_half": 0.105,
  "recommended_stake_pct": 10.5
}
```

---

## ML Pipeline (Python Batch Worker)

### Features (futebol)

```python
features = [
    "implied_prob_home",        # 1/odds_home (média das casas)
    "implied_prob_draw",
    "implied_prob_away",
    "elo_home",
    "elo_away",
    "elo_diff",
    "home_win_rate_5",          # win rate últimos 5 jogos em casa
    "away_win_rate_5",
    "goal_diff_home_5",         # saldo de gols últimos 5
    "goal_diff_away_5",
    "days_since_last_match_home",
    "days_since_last_match_away",
    "league_encoded",
]
```

### Modelo

- **Fase 3 inicial:** Logistic Regression
- **Upgrade:** XGBoost / LightGBM

### Cron Schedule

```cron
0 3 * * *   docker compose run --rm ml python train.py    # treino diário 3am
0 4 * * *   docker compose run --rm ml python predict.py  # predictions pós-treino
0 */6 * * * docker compose run --rm ml python predict.py  # refresh a cada 6h
0 5 * * *   docker compose run --rm ml python results.py  # ingestion de resultados
```

### Model Drift

```python
# threshold: 15% degradação no Brier Score vs baseline
if brier_recent > brier_baseline * 1.15:
    send_alert("model drift detected")
    # não retreina automático — alerta pra revalidar features
```

### Results Ingestion

- Primary: Odds API scores endpoint (mantém `external_id` consistente, zero reconciliação)
- Fallback: football-data.org fuzzy match por `home_team + away_team + date + league`

---

## Ingestion Service + Adaptive Polling

### Budget Estimado (Starter ~10k req/mês)

```
~80 jogos/mês × estados:
  Fase futura  (>24h):  80 × 24 × 1 req  = 1.920
  Fase próxima (1-24h): 80 × 4  × 4 req  = 1.280
  Fase ao vivo (<1h):   80 × 2h × 30 req = 4.800
  Total estimado: ~8.000 req/mês ✅
```

### Intervalos

| Estado | Intervalo |
|---|---|
| `starts_at > 24h` | 60 min |
| `starts_at 1-24h` | 15 min |
| `starts_at < 1h` | 5 min |
| `status = live` | 2 min |
| `status = finished` | para |

### Fallback

Odds API falha → OddsPapi. Ambos falham → log erro, skip, retry no próximo tick.

---

## Auth + Multi-tenancy

- JWT com `tenant_id` no payload
- Middleware Go: extrai tenant_id → `SET LOCAL app.tenant_id`
- RLS PostgreSQL filtra `signals` automaticamente
- Sem OAuth no MVP — email/password + bcrypt

---

## API Endpoints

```
POST /auth/register
POST /auth/login

GET  /matches
GET  /matches/:id/odds
GET  /matches/:id/signals

GET  /signals
GET  /signals?type=arbitrage
GET  /signals?type=value_bet

GET  /predictions/:match_id

PATCH /preferences
```

---

## Alerts (Telegram)

```
⚡ VALUE BET — Premier League
Arsenal vs Chelsea
Bookmaker: Bet365 | Odds: 2.30 (home)
Edge: +11.5% | Kelly: 10.5% bankroll

🔄 ARBITRAGE — La Liga
Real Madrid vs Barcelona
Profit: 2.3% | Stakes: home 45% / draw 30% / away 25%
```

Trigger: novo signal inserido + edge > threshold do tenant.

---

## Deploy — Hetzner VPS

### Docker Compose

```yaml
services:
  api:
    build: .
    ports: ["8080:8080"]
    depends_on: [postgres]
    env_file: .env

  postgres:
    image: postgres:16
    volumes: [pgdata:/var/lib/postgresql/data]
    env_file: .env

volumes:
  pgdata:
```

ML workers rodam via cron no host (não no compose por padrão):
```bash
docker compose run --rm ml python train.py
docker compose run --rm ml python predict.py
```

### Setup Hetzner

1. `hcloud server create --name sportstips --type cx22 --image ubuntu-24.04`
2. Instala Docker + Docker Compose
3. Caddy como reverse proxy (SSL automático via Let's Encrypt)
4. Deploy via git pull + docker compose up -d
5. Domínio opcional — acesso via IP durante dev

---

## Backtesting (Fase 4)

```python
# Replay histórico pra validar ROI real do decision engine
# Para cada jogo com resultado conhecido:
#   1. Odds que existiam antes do jogo
#   2. Roda arb + value bet + kelly
#   3. Simula resultado
#   4. Calcula ROI acumulado, win rate, avg edge
```

---

## Execution Plan (Fases)

### Fase 1 — Core (sem ML)
1. PostgreSQL schema + migrations
2. Odds ingestion (Odds API + fallback OddsPapi)
3. Normalization layer
4. Arbitrage detection (cross-bookmaker, multi-market)
5. Auth + multi-tenancy (JWT + RLS)
6. API endpoints básicos

### Fase 2 — Dados Históricos
7. Results ingestion (Odds API scores + fallback)
8. Historical odds collection

### Fase 3 — ML
9. Feature engineering
10. Train pipeline (Logistic Regression → XGBoost)
11. Batch predictions → ml_predictions
12. Model drift monitoring
13. Value bet detection com ML + Kelly Criterion

### Fase 4 — Produto Completo
14. Backtesting framework
15. React dashboard
16. Telegram alerts
17. Tenant preferences UI

---

## Constraints

- Sem apostas automáticas — recomendações apenas
- Respeitar rate limits das APIs
- Log de todas predições e decisões
- Variáveis sensíveis via env vars, nunca no código
