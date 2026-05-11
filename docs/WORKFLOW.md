# Betting Intelligence Agent — Workflow

## Overview

Sistema detecta automaticamente oportunidades de apostas (arbitrage e value bets) agregando odds de múltiplas casas, aplicando um modelo de ML para estimar probabilidades reais, e alertando via Telegram por tenant.

---

## Data Flow

```
Odds API
    │
    ▼
[Ingestion Scheduler]          ← polling adaptativo (2min–60min por estado do jogo)
    │
    ├── odds_raw (DB)          ← payload bruto preservado
    └── odds_normalized (DB)   ← normalizado: bookmaker / market / odds_home / draw / away
              │
    ┌─────────┴──────────┐
    ▼                    ▼
[Arbitrage Engine]   [ML Predictions]    ← Python batch (cron diário)
    │                    │
    └────────┬───────────┘
             ▼
    [Decision Engine]
         │       │
    arbitrage  value_bet
         │       │
         └───┬───┘
             ▼
         signals (DB, por tenant)
             │
    ┌────────┴────────┐
    ▼                 ▼
[Telegram Alert]   [REST API]  →  Dashboard
```

---

## Phase 1 — Odds Ingestion + Arbitrage

### Adaptive Polling

| Estado do jogo | Intervalo |
|---|---|
| `starts_at > 24h` | 60 min |
| `starts_at 1–24h` | 15 min |
| `starts_at < 1h` | 5 min |
| `live` | 2 min |
| `finished` | para |

**Função:** `internal/ingestion/scheduler.go` → `Scheduler.fetchAll()`

Ligas monitoradas:
- `soccer_epl` — Premier League
- `soccer_spain_la_liga` — La Liga
- `soccer_italy_serie_a` — Serie A
- `soccer_germany_bundesliga` — Bundesliga
- `soccer_uefa_champs_league` — Champions League

---

### Arbitrage Detection

**Condição:**
```
arb_sum = (1/odds_home) + (1/odds_draw) + (1/odds_away)
if arb_sum < 1.0 → arbitrage
profit% = (1 - arb_sum) / arb_sum
```

**Cross-bookmaker:** compara melhor odd de cada outcome entre todas casas antes de calcular.

**Mercados suportados:** `1x2`, `over_under_N`

**Função:** `internal/engine/arbitrage.go` → `DetectArbitrage(matchID, odds, minProfitPct)`

**Threshold:** configurável por tenant via `min_arb_profit` (default 1%).

**Stake distribution:**
```
stake_home = bankroll × (1/odds_home) / arb_sum
stake_draw = bankroll × (1/odds_draw) / arb_sum
stake_away = bankroll × (1/odds_away) / arb_sum
```

---

## Phase 2 — Results Ingestion + Per-Tenant Signals

### Results Ingestion

**Primary:** Odds API `/scores` endpoint — mantém `external_id` consistente, zero reconciliação.

**Fallback:** football-data.org com fuzzy match por `home_team + away_team + date`.

**Endpoint:** `POST /admin/results/sync`

**Função:** `internal/results/ingester.go` → `Ingester.Run()`

---

### Per-Tenant Signal Storage

Cada tenant tem preferências independentes:

| Preferência | Default | Efeito |
|---|---|---|
| `min_arb_profit` | 1% | threshold mínimo de lucro pro arbitrage |
| `min_value_edge` | 5% | threshold mínimo de edge pro value bet |
| `alert_telegram_id` | — | chat ID pra receber alertas |

**Função:** `internal/ingestion/scheduler.go` → `Scheduler.runEngine()`

---

## Phase 3 — ML Pipeline + Value Bet Detection

### Feature Engineering

| Feature | Descrição |
|---|---|
| `implied_prob_home/draw/away` | Probabilidade implícita média das odds das casas |
| `elo_home / elo_away` | Rating ELO por time (K=20, base 1500) |
| `elo_diff` | `elo_home - elo_away` |
| `home_win_rate_5` | Taxa de vitória últimos 5 jogos em casa |
| `away_win_rate_5` | Taxa de vitória últimos 5 jogos fora |
| `goal_diff_home_5` | Saldo de gols médio últimos 5 (casa) |
| `goal_diff_away_5` | Saldo de gols médio últimos 5 (fora) |

**Arquivo:** `ml/features.py` → `build_dataset(conn)`

---

### Model Training

**Modelo inicial:** Logistic Regression (scikit-learn)

**Métricas:** Brier Score + Log Loss (salvas em `ml_model_metrics`)

**Arquivo:** `ml/train.py`

```bash
docker compose run --rm ml python train.py
# Requer ≥20 partidas com resultados no DB
```

**Output:** `/app/models/model_latest.pkl` + registro em `ml_model_metrics`

---

### Batch Predictions

**Arquivo:** `ml/predict.py`

```bash
docker compose run --rm ml python predict.py
```

Gera probabilidades pra todas partidas upcoming (próximas 48h) com odds disponíveis. Salva em `ml_predictions`.

**Cron (produção):**
```cron
0 3 * * *   docker compose run --rm ml python train.py
0 4 * * *   docker compose run --rm ml python predict.py
0 */6 * * * docker compose run --rm ml python predict.py
0 5 * * *   docker compose run --rm ml python monitor.py
```

---

### Value Bet Detection

**Condição:**
```
implied_prob = 1 / odds_bookmaker
model_prob   = ml_predictions.prob_{outcome}
edge         = model_prob - implied_prob

if edge > min_value_edge → value bet
```

**Kelly Criterion (Half-Kelly):**
```
b = odds - 1
kelly_full = (b × model_prob - (1 - model_prob)) / b
kelly_half = kelly_full × 0.5   ← stake recomendado
```

**Função:** `internal/engine/valuebet.go` → `DetectValueBets(matchID, odds, pred, minEdge)`

**Função:** `internal/engine/kelly.go` → `Kelly(odds, modelProb)`

---

### Model Drift Monitor

Compara Brier Score recente (últimos 7 dias) com baseline histórico.

**Threshold:** degradação > 15% → warning no log + exit(1) pra cron detectar.

**Arquivo:** `ml/monitor.py`

---

## REST API Endpoints

### Auth
| Method | Path | Descrição |
|---|---|---|
| `POST` | `/auth/register` | Criar conta (retorna JWT) |
| `POST` | `/auth/login` | Login (retorna JWT) |

### Matches
| Method | Path | Descrição |
|---|---|---|
| `GET` | `/matches` | Partidas ativas (upcoming + live) |
| `GET` | `/matches/:id/odds` | Odds normalizadas por bookmaker |
| `GET` | `/matches/:id/signals` | Signals do tenant pra essa partida |

### Signals
| Method | Path | Descrição |
|---|---|---|
| `GET` | `/signals` | Todos os signals do tenant |
| `GET` | `/signals?type=arbitrage` | Só arbitrages |
| `GET` | `/signals?type=value_bet` | Só value bets |

### Predictions (ML)
| Method | Path | Descrição |
|---|---|---|
| `GET` | `/predictions/:match_id` | Probabilidades do modelo pra partida |

### Preferences
| Method | Path | Descrição |
|---|---|---|
| `PATCH` | `/preferences` | Atualizar thresholds e Telegram ID |

### Admin
| Method | Path | Descrição |
|---|---|---|
| `POST` | `/admin/results/sync` | Trigger manual de ingestion de resultados |

---

## Signal Payloads

### Arbitrage
```json
{
  "type": "arbitrage",
  "market": "1x2",
  "data": {
    "arb_sum": 0.982,
    "profit_pct": "1.83%",
    "home_bookmaker": "bet365",
    "home_odds": 2.20,
    "draw_bookmaker": "betway",
    "draw_odds": 3.60,
    "away_bookmaker": "unibet",
    "away_odds": 4.00,
    "stakes": {
      "home": 45.2,
      "draw": 27.8,
      "away": 27.0
    }
  }
}
```

### Value Bet
```json
{
  "type": "value_bet",
  "market": "1x2",
  "data": {
    "outcome": "home",
    "bookmaker": "bet365",
    "odds": 2.30,
    "implied_prob": 0.435,
    "model_prob": 0.55,
    "edge": "11.50%",
    "kelly_full": 0.204,
    "kelly_half": 0.102,
    "recommended_stake_pct": 10.2,
    "model_version": "lr_2026-05-07"
  }
}
```

---

## Telegram Alerts

Disparados automaticamente após `InsertSignals` se `alert_telegram_id` configurado.

**Configurar:**
```bash
curl -X PATCH http://localhost:8080/preferences \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"alert_telegram_id": "SEU_CHAT_ID"}'
```

**Formato arbitrage:**
```
🔄 ARBITRAGE — Arsenal vs Chelsea
Market: 1x2 | Profit: 1.83%
Home: bet365 @ 2.20
Draw: betway @ 3.60
Away: unibet @ 4.00
Stakes: home: 45.2% | draw: 27.8% | away: 27.0%
```

**Formato value bet:**
```
⚡ VALUE BET — Real Madrid vs Barcelona
Market: 1x2 | Outcome: home @ 2.30
Edge: 11.50% | Kelly: 10.2% bankroll
Model: 0.55 → Implied: 0.435
```

---

## Stack

| Camada | Tech |
|---|---|
| API + Engine | Go 1.25 (chi, pgx/v5, golang-jwt) |
| ML Pipeline | Python 3.11 (scikit-learn, pandas, psycopg2) |
| DB | PostgreSQL 16 (RLS por tenant) |
| Deploy | Docker Compose |
| Odds Source | The Odds API (Starter) |

---

## Constraints

- Sem apostas automáticas — sistema só recomenda
- Rate limit Odds API: ~8.000 req/mês estimado (Starter tier)
- ML predictions requerem ≥20 partidas com resultados para treinar
