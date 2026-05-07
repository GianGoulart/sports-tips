# 🎯 Project: Betting Intelligence Agent (with ML Pipeline)

## Objective

Build a system that:

* Aggregates sports betting odds from multiple APIs
* Identifies arbitrage opportunities
* Detects value bets
* Uses Machine Learning to estimate true probabilities
* Provides a dashboard for monitoring (no automatic betting)

---

## APIs

### Primary

* Odds API

### Secondary

* OddsPapi

### (Optional)

* OpticOdds

---

## Core Architecture

### Modules

1. Odds Ingestion Service
2. Data Storage Layer
3. ML Pipeline
4. Decision Engine
5. Alerts System
6. Dashboard API

---

## 1. Odds Ingestion Service

* Fetch odds every 30 seconds
* Normalize data into schema:

```
event_id
sport
league
home_team
away_team
bookmaker
market
odds_home
odds_draw
odds_away
timestamp
```

---

## 2. Data Storage

### Tables

#### odds_raw

* store raw API responses

#### odds_normalized

* cleaned and structured odds

#### matches

* match metadata

#### results

* final outcomes (critical for ML)

---

## 3. ML Pipeline

### Objective

Predict probability of outcomes:

* home win
* draw
* away win

---

### 3.1 Feature Engineering

Generate features such as:

* implied probability from odds
* moving average of team performance
* ELO rating
* home vs away performance
* goal difference (last N games)

---

### 3.2 Training Dataset

Join:

* historical odds
* match results

Output format:

```
features → X
result → y
```

---

### 3.3 Model (initial version)

Start simple:

* Logistic Regression

Later upgrade to:

* Gradient Boosting (XGBoost / LightGBM)

---

### 3.4 Training Process

* Run daily or weekly
* Save model to disk:

```
/models/model_v1.pkl
```

---

### 3.5 Prediction Service

* Input: upcoming match + features
* Output:

```
{
  home_win_prob: 0.55,
  draw_prob: 0.25,
  away_win_prob: 0.20
}
```

---

## 4. Decision Engine

### 4.1 Arbitrage

Condition:

```
(1/odd1 + 1/odd2 + 1/odd3) < 1
```

Return:

* stake distribution
* profit %

---

### 4.2 Value Bet Detection

Steps:

1. Convert odds to implied probability:

```
implied_prob = 1 / odds
```

2. Compare with ML prediction:

```
if model_prob > implied_prob:
    value_bet = true
```

3. Add threshold:

```
if (model_prob - implied_prob) > 0.05:
    trigger
```

---

## 5. Alerts System

Trigger when:

* arbitrage detected
* value bet > threshold

Send via:

* Telegram bot
* email

---

## 6. Dashboard

Display:

* live odds
* arbitrage opportunities
* value bets
* model predictions

---

## Data Pipeline (End-to-End)

1. Fetch odds (API)
2. Store raw
3. Normalize data
4. Update database
5. Generate features
6. Run prediction model
7. Run decision engine:

   * arbitrage
   * value bet
8. Store signals
9. Trigger alerts
10. Update dashboard

---

## Tech Stack

* Backend: Python (preferred for ML)
* API: FastAPI
* DB: PostgreSQL
* Cache: Redis
* Queue: Celery

---

## Constraints

* DO NOT place bets automatically
* Only provide recommendations
* Respect API rate limits

---

## Coding Guidelines

* Modular architecture
* Separate ML pipeline from API layer
* Use environment variables
* Log all predictions and decisions

---

## Deliverables

* Odds ingestion service
* Database schema
* ML training pipeline
* Prediction API
* Arbitrage + value bet engine
* Basic dashboard

---

## Execution Plan (IMPORTANT)

### Phase 1 (No ML)

1. Odds ingestion
2. Database
3. Arbitrage detection

### Phase 2

4. Historical data collection
5. Results ingestion

### Phase 3 (ML)

6. Feature engineering
7. Train first model
8. Prediction service

### Phase 4

9. Value bet detection with ML
10. Alerts + dashboard

---

## First Tasks for Claude Code

1. Implement Odds API integration
2. Create PostgreSQL schema
3. Build odds normalization layer
4. Create script to store historical data
5. Implement basic arbitrage function

ONLY after that:
6. Start ML pipeline
