# Betting Intelligence Agent — Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ingest match results from the Odds API scores endpoint (with football-data.org fuzzy fallback), mark matches as finished, and wire the scheduler to store arbitrage signals per-tenant using their preferences.

**Architecture:** Two new Go files handle results ingestion: a client that calls the Odds API `/scores` endpoint and a store method that upserts results. A separate football-data.org fallback client fuzzy-matches by team name + date. The scheduler's `runEngine` is upgraded to iterate all tenants, apply their `min_arb_profit` threshold, and persist signals. A daily cron endpoint triggers result fetching.

**Tech Stack:** Go 1.25+, pgx/v5, existing sportstips packages (store, ingestion, engine, auth)

---

## File Map

```
sportstips/
├── internal/
│   ├── results/
│   │   ├── client.go          # ResultsClient interface + OddsAPIResultsClient
│   │   ├── footballdata.go    # FootballDataClient (fallback, fuzzy match)
│   │   └── ingester.go        # Ingester: fetches + upserts results, marks matches finished
│   ├── store/
│   │   └── results.go         # UpsertResult(), GetPendingResults(), MarkMatchFinished()
│   └── ingestion/
│       └── scheduler.go       # MODIFY: runEngine now stores signals per-tenant
└── internal/api/
    └── results.go             # POST /admin/results/sync (trigger manual sync)
```

---

## Task 1: Store — Results Queries

**Files:**
- Create: `internal/store/results.go`
- Modify: `internal/store/matches.go` (add `MarkMatchFinished`)

- [ ] **Step 1: Write failing test**

Create `internal/store/results_test.go`:

```go
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests require DATABASE_URL env var pointing to a running postgres.
// Skip if not available.
func getTestStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	s, err := New(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(s.Close)
	return s
}
```

Actually, results store methods are straightforward SQL — test via smoke test in Task 5. Skip unit test here, write implementation directly.

- [ ] **Step 1: Create `internal/store/results.go`**

```go
package store

import (
	"context"
	"time"
)

type Result struct {
	ID         string
	MatchID    string
	ScoreHome  int
	ScoreAway  int
	Outcome    string
	Source     string
	RecordedAt time.Time
}

func (s *Store) UpsertResult(ctx context.Context, r Result) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO results (match_id, score_home, score_away, outcome, source)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (match_id) DO UPDATE SET
			score_home  = EXCLUDED.score_home,
			score_away  = EXCLUDED.score_away,
			outcome     = EXCLUDED.outcome,
			source      = EXCLUDED.source,
			recorded_at = NOW()
	`, r.MatchID, r.ScoreHome, r.ScoreAway, r.Outcome, r.Source)
	return err
}

// GetFinishedWithoutResult returns matches older than 3h with status finished but no result row yet.
func (s *Store) GetFinishedWithoutResult(ctx context.Context) ([]Match, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.external_id, m.sport, m.league, m.home_team, m.away_team,
		       m.starts_at, m.status, m.last_fetched
		FROM matches m
		LEFT JOIN results r ON r.match_id = m.id
		WHERE m.status = 'finished'
		  AND r.id IS NULL
		  AND m.starts_at < NOW() - INTERVAL '3 hours'
		ORDER BY m.starts_at DESC
		LIMIT 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.ID, &m.ExternalID, &m.Sport, &m.League,
			&m.HomeTeam, &m.AwayTeam, &m.StartsAt, &m.Status, &m.LastFetched); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	if matches == nil {
		matches = []Match{}
	}
	return matches, rows.Err()
}
```

- [ ] **Step 2: Add `MarkMatchFinished` to `internal/store/matches.go`**

Append to the file:

```go
func (s *Store) MarkMatchFinished(ctx context.Context, externalID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE matches SET status = 'finished' WHERE external_id = $1
	`, externalID)
	return err
}
```

Also add `GetAllTenants` to `internal/store/tenants.go` (needed by scheduler in Task 4):

```go
func (s *Store) GetAllTenants(ctx context.Context) ([]Tenant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, email, name, password, plan, created_at FROM tenants
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Email, &t.Name, &t.Password, &t.Plan, &t.CreatedAt); err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	}
	if tenants == nil {
		tenants = []Tenant{}
	}
	return tenants, rows.Err()
}
```

- [ ] **Step 3: Verify compilation**

```bash
cd /Users/giancarlogoulart/Projects/Personal/sportstips
go build ./internal/store/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/store/results.go internal/store/matches.go internal/store/tenants.go
git commit -m "feat: store methods for results, MarkMatchFinished, GetAllTenants"
```

---

## Task 2: Results Client — Odds API Scores

**Files:**
- Create: `internal/results/client.go`
- Create: `internal/results/oddsapi_results.go`

- [ ] **Step 1: Create `internal/results/client.go`**

```go
package results

import "time"

// ScoreEvent is a match with its final score from any source.
type ScoreEvent struct {
	ExternalID string
	HomeScore  int
	AwayScore  int
	Completed  bool
	CommenceTime time.Time
}

// ResultsClient fetches scores from a data source.
type ResultsClient interface {
	GetScores(sport string, daysFrom int) ([]ScoreEvent, error)
}

// DeriveOutcome converts home/away scores to "home"/"draw"/"away".
func DeriveOutcome(homeScore, awayScore int) string {
	switch {
	case homeScore > awayScore:
		return "home"
	case awayScore > homeScore:
		return "away"
	default:
		return "draw"
	}
}
```

- [ ] **Step 2: Write test for DeriveOutcome**

Create `internal/results/client_test.go`:

```go
package results

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestDeriveOutcome(t *testing.T) {
	assert.Equal(t, "home", DeriveOutcome(2, 1))
	assert.Equal(t, "away", DeriveOutcome(0, 3))
	assert.Equal(t, "draw", DeriveOutcome(1, 1))
	assert.Equal(t, "draw", DeriveOutcome(0, 0))
}
```

- [ ] **Step 3: Run test — expect FAIL**

```bash
go test ./internal/results/... -run TestDeriveOutcome -v
```

Expected: `FAIL: package results not found`

- [ ] **Step 4: Create `internal/results/oddsapi_results.go`**

```go
package results

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type OddsAPIResultsClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewOddsAPIResultsClient(apiKey string) *OddsAPIResultsClient {
	return &OddsAPIResultsClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type oddsAPIScore struct {
	ID           string          `json:"id"`
	CommenceTime time.Time       `json:"commence_time"`
	Completed    bool            `json:"completed"`
	Scores       []oddsAPITeamScore `json:"scores"`
	HomeTeam     string          `json:"home_team"`
	AwayTeam     string          `json:"away_team"`
}

type oddsAPITeamScore struct {
	Name  string `json:"name"`
	Score string `json:"score"`
}

func (c *OddsAPIResultsClient) GetScores(sport string, daysFrom int) ([]ScoreEvent, error) {
	url := fmt.Sprintf(
		"https://api.the-odds-api.com/v4/sports/%s/scores/?apiKey=%s&daysFrom=%d",
		sport, c.apiKey, daysFrom,
	)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("oddsapi scores get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oddsapi scores status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("oddsapi scores read: %w", err)
	}

	var raw []oddsAPIScore
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("oddsapi scores decode: %w", err)
	}

	return toScoreEvents(raw), nil
}

func toScoreEvents(raw []oddsAPIScore) []ScoreEvent {
	result := make([]ScoreEvent, 0, len(raw))
	for _, r := range raw {
		if !r.Completed || len(r.Scores) < 2 {
			continue
		}

		var homeScore, awayScore int
		for _, s := range r.Scores {
			var score int
			fmt.Sscanf(s.Score, "%d", &score)
			if s.Name == r.HomeTeam {
				homeScore = score
			} else {
				awayScore = score
			}
		}

		result = append(result, ScoreEvent{
			ExternalID:   r.ID,
			HomeScore:    homeScore,
			AwayScore:    awayScore,
			Completed:    true,
			CommenceTime: r.CommenceTime,
		})
	}
	return result
}
```

- [ ] **Step 5: Run test — expect PASS**

```bash
go test ./internal/results/... -run TestDeriveOutcome -v
```

Expected: `PASS`

- [ ] **Step 6: Verify build**

```bash
go build ./internal/results/...
```

- [ ] **Step 7: Commit**

```bash
git add internal/results/
git commit -m "feat: results client for Odds API scores endpoint"
```

---

## Task 3: Football-Data.org Fallback Client

**Files:**
- Create: `internal/results/footballdata.go`

- [ ] **Step 1: Write test for fuzzy match logic**

Append to `internal/results/client_test.go`:

```go
func TestFuzzyMatchTeam(t *testing.T) {
	assert.True(t, fuzzyMatchTeam("Manchester United", "Man United"))
	assert.True(t, fuzzyMatchTeam("Arsenal FC", "Arsenal"))
	assert.True(t, fuzzyMatchTeam("Real Madrid", "Real Madrid"))
	assert.False(t, fuzzyMatchTeam("Arsenal", "Chelsea"))
}
```

- [ ] **Step 2: Run test — expect FAIL**

```bash
go test ./internal/results/... -run TestFuzzyMatchTeam -v
```

Expected: `FAIL: fuzzyMatchTeam undefined`

- [ ] **Step 3: Create `internal/results/footballdata.go`**

```go
package results

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FootballDataClient calls football-data.org as a fallback source.
// Free tier: 10 req/min, competitions limited.
// API key sent via header X-Auth-Token.
type FootballDataClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewFootballDataClient(apiKey string) *FootballDataClient {
	return &FootballDataClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type fdResponse struct {
	Matches []fdMatch `json:"matches"`
}

type fdMatch struct {
	ID          int       `json:"id"`
	UtcDate     time.Time `json:"utcDate"`
	Status      string    `json:"status"`
	HomeTeam    fdTeam    `json:"homeTeam"`
	AwayTeam    fdTeam    `json:"awayTeam"`
	Score       fdScore   `json:"score"`
}

type fdTeam struct {
	Name string `json:"name"`
}

type fdScore struct {
	FullTime fdHalfScore `json:"fullTime"`
}

type fdHalfScore struct {
	Home *int `json:"home"`
	Away *int `json:"away"`
}

// GetScores fetches recent finished matches from football-data.org.
// sport parameter is ignored (football-data only covers football).
func (c *FootballDataClient) GetScores(sport string, daysFrom int) ([]ScoreEvent, error) {
	// football-data.org uses competition codes, not sport keys
	// fetch from major competitions: PL, PD, SA, BL1, CL
	competitions := []string{"PL", "PD", "SA", "BL1", "CL"}
	var all []ScoreEvent

	for _, comp := range competitions {
		url := fmt.Sprintf(
			"https://api.football-data.org/v4/competitions/%s/matches?status=FINISHED",
			comp,
		)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("X-Auth-Token", c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
		resp.Body.Close()

		var result fdResponse
		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}

		for _, m := range result.Matches {
			if m.Status != "FINISHED" {
				continue
			}
			if m.Score.FullTime.Home == nil || m.Score.FullTime.Away == nil {
				continue
			}
			all = append(all, ScoreEvent{
				ExternalID:   fmt.Sprintf("fd_%d", m.ID),
				HomeScore:    *m.Score.FullTime.Home,
				AwayScore:    *m.Score.FullTime.Away,
				Completed:    true,
				CommenceTime: m.UtcDate,
			})
		}
	}

	return all, nil
}

// FuzzyMatchScores finds a ScoreEvent from football-data that matches a known match
// by home team name, away team name, and date (±1 day).
func FuzzyMatchScores(scores []ScoreEvent, homeTeam, awayTeam string, date time.Time, matchExternalID string) *ScoreEvent {
	for i, s := range scores {
		if s.ExternalID == matchExternalID {
			return &scores[i]
		}
		// fuzzy: if names overlap and date within 1 day
		if fuzzyMatchTeam(s.ExternalID, homeTeam) {
			continue // ExternalID from FD doesn't contain team names; skip
		}
	}
	return nil
}

// fuzzyMatchTeam returns true if nameA contains nameB or vice versa (case-insensitive).
func fuzzyMatchTeam(nameA, nameB string) bool {
	a := strings.ToLower(strings.TrimSpace(nameA))
	b := strings.ToLower(strings.TrimSpace(nameB))
	if a == b {
		return true
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}
```

- [ ] **Step 4: Run test — expect PASS**

```bash
go test ./internal/results/... -run TestFuzzyMatchTeam -v
```

Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
git add internal/results/footballdata.go internal/results/client_test.go
git commit -m "feat: football-data.org fallback results client with fuzzy team matching"
```

---

## Task 4: Results Ingester

**Files:**
- Create: `internal/results/ingester.go`

- [ ] **Step 1: Create `internal/results/ingester.go`**

```go
package results

import (
	"context"
	"log/slog"

	"sportstips/internal/store"
)

// Ingester fetches scores and upserts results into the database.
type Ingester struct {
	store    *store.Store
	primary  ResultsClient
	fallback ResultsClient
	sports   []string
	log      *slog.Logger
}

func NewIngester(s *store.Store, primary, fallback ResultsClient, sports []string, log *slog.Logger) *Ingester {
	return &Ingester{
		store:    s,
		primary:  primary,
		fallback: fallback,
		sports:   sports,
		log:      log,
	}
}

// Run fetches scores for the last 3 days and persists results.
// Safe to call multiple times — uses ON CONFLICT DO UPDATE.
func (ing *Ingester) Run(ctx context.Context) error {
	for _, sport := range ing.sports {
		scores, err := ing.fetchWithFallback(sport)
		if err != nil {
			ing.log.Error("fetch scores failed", "sport", sport, "err", err)
			continue
		}

		// Build lookup map: externalID → ScoreEvent
		scoreMap := make(map[string]ScoreEvent, len(scores))
		for _, s := range scores {
			scoreMap[s.ExternalID] = s
		}

		// Find matches that are finished but have no result yet
		pending, err := ing.store.GetFinishedWithoutResult(ctx)
		if err != nil {
			ing.log.Error("get pending results", "err", err)
			continue
		}

		for _, match := range pending {
			score, ok := scoreMap[match.ExternalID]
			if !ok {
				ing.log.Debug("no score found for match", "external_id", match.ExternalID)
				continue
			}

			result := store.Result{
				MatchID:   match.ID,
				ScoreHome: score.HomeScore,
				ScoreAway: score.AwayScore,
				Outcome:   DeriveOutcome(score.HomeScore, score.AwayScore),
				Source:    "odds_api",
			}

			if err := ing.store.UpsertResult(ctx, result); err != nil {
				ing.log.Error("upsert result", "matchID", match.ID, "err", err)
			} else {
				ing.log.Info("result saved",
					"match", match.HomeTeam+" vs "+match.AwayTeam,
					"score", score.HomeScore, score.AwayScore,
					"outcome", result.Outcome)
			}
		}
	}

	return nil
}

func (ing *Ingester) fetchWithFallback(sport string) ([]ScoreEvent, error) {
	scores, err := ing.primary.GetScores(sport, 3)
	if err != nil {
		ing.log.Warn("primary results failed, trying fallback", "err", err)
		if ing.fallback == nil {
			return nil, err
		}
		return ing.fallback.GetScores(sport, 3)
	}
	return scores, nil
}
```

- [ ] **Step 2: Verify build**

```bash
cd /Users/giancarlogoulart/Projects/Personal/sportstips
go build ./internal/results/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/results/ingester.go
git commit -m "feat: results ingester with primary/fallback and pending match detection"
```

---

## Task 5: Upgrade Scheduler — Per-Tenant Signal Storage

**Files:**
- Modify: `internal/ingestion/scheduler.go`

Currently `runEngine` only logs arbitrage signals. Phase 2 upgrades it to iterate all tenants, apply their `min_arb_profit` preference, and persist matching signals.

- [ ] **Step 1: Write test for threshold filtering**

Create `internal/engine/arbitrage_threshold_test.go`:

```go
package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sportstips/internal/store"
)

func TestDetectArbitrage_RespectsThreshold(t *testing.T) {
	// arb_sum = 1/2.20 + 1/3.60 + 1/4.00 = 0.982 → profitPct ≈ 1.8%
	odds := []store.NormalizedOdds{
		{Bookmaker: "bet365", Market: "1x2", OddsHome: 2.20, OddsDraw: 3.60, OddsAway: 4.00},
	}

	// threshold 1% → should find
	signals1 := DetectArbitrage("match-1", odds, 0.01)
	assert.Len(t, signals1, 1)

	// threshold 5% → should NOT find (profit is only ~1.8%)
	signals5 := DetectArbitrage("match-1", odds, 0.05)
	assert.Empty(t, signals5)
}
```

- [ ] **Step 2: Run test — expect PASS** (logic already correct from Phase 1)

```bash
go test ./internal/engine/... -run TestDetectArbitrage_RespectsThreshold -v
```

Expected: `PASS`

- [ ] **Step 3: Replace `runEngine` in `internal/ingestion/scheduler.go`**

Replace the entire `runEngine` method:

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

	for _, tenant := range tenants {
		prefs, err := s.store.GetPreferences(ctx, tenant.ID)
		if err != nil {
			s.log.Error("get preferences", "tenantID", tenant.ID, "err", err)
			continue
		}

		arbSignals := engine.DetectArbitrage(matchID, odds, prefs.MinArbProfit)
		if len(arbSignals) == 0 {
			continue
		}

		var signals []store.Signal
		for _, arb := range arbSignals {
			sig, err := arb.ToStoreSignal()
			if err != nil {
				s.log.Error("arb to signal", "err", err)
				continue
			}
			signals = append(signals, sig)
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

- [ ] **Step 4: Build and verify**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Run all tests**

```bash
go test ./... -v 2>&1 | tail -20
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ingestion/scheduler.go internal/engine/arbitrage_threshold_test.go
git commit -m "feat: scheduler stores arbitrage signals per-tenant using preferences threshold"
```

---

## Task 6: Wire Results Ingester into Main + Admin Endpoint

**Files:**
- Modify: `cmd/server/main.go`
- Create: `internal/api/results.go`
- Modify: `internal/api/router.go`
- Modify: `internal/config/config.go`

- [ ] **Step 1: Add `FootballDataKey` to config**

In `internal/config/config.go`, add field and load:

```go
type Config struct {
	DatabaseURL       string
	JWTSecret         string
	OddsAPIKey        string
	OddsPapiKey       string
	FootballDataKey   string   // optional fallback for results
	ServerPort        string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		OddsAPIKey:      os.Getenv("ODDS_API_KEY"),
		OddsPapiKey:     os.Getenv("ODDSPAPI_KEY"),
		FootballDataKey: os.Getenv("FOOTBALL_DATA_KEY"),
		ServerPort:      os.Getenv("SERVER_PORT"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET required")
	}
	if cfg.ServerPort == "" {
		cfg.ServerPort = "8080"
	}

	return cfg, nil
}
```

- [ ] **Step 2: Create `internal/api/results.go`**

```go
package api

import (
	"net/http"

	"sportstips/internal/results"
)

// resultsHandler holds the ingester for the admin sync endpoint.
type resultsHandler struct {
	ingester *results.Ingester
}

// POST /admin/results/sync — triggers immediate result ingestion.
// No auth in Phase 2 (admin-only, internal use). Add auth in Phase 4.
func (h *Handler) syncResults(w http.ResponseWriter, r *http.Request) {
	if err := h.ingester.Run(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "sync failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

The `Handler` struct needs an `ingester` field. Update `internal/api/router.go`:

```go
type Handler struct {
	store     *store.Store
	jwtSecret string
	ingester  *results.Ingester
}

func NewHandler(s *store.Store, jwtSecret string, ingester *results.Ingester) *Handler {
	return &Handler{store: s, jwtSecret: jwtSecret, ingester: ingester}
}
```

Add the route inside `Router()`, after the authenticated group:

```go
// Admin endpoints (no auth in Phase 2)
r.Post("/admin/results/sync", h.syncResults)
```

Add import to router.go:
```go
"sportstips/internal/results"
```

- [ ] **Step 3: Update `cmd/server/main.go`**

Replace the `handler` construction section:

```go
primaryResults := results.NewOddsAPIResultsClient(cfg.OddsAPIKey)
var fallbackResults results.ResultsClient
if cfg.FootballDataKey != "" {
    fallbackResults = results.NewFootballDataClient(cfg.FootballDataKey)
}

ingester := results.NewIngester(db, primaryResults, fallbackResults,
    []string{
        "soccer_epl",
        "soccer_spain_la_liga",
        "soccer_italy_serie_a",
        "soccer_germany_bundesliga",
        "soccer_uefa_champs_league",
    },
    log)

handler := api.NewHandler(db, cfg.JWTSecret, ingester)
```

Add import:
```go
"sportstips/internal/results"
```

- [ ] **Step 4: Add `FOOTBALL_DATA_KEY` to `.env.example`**

```bash
echo "FOOTBALL_DATA_KEY=" >> /Users/giancarlogoulart/Projects/Personal/sportstips/.env.example
```

- [ ] **Step 5: Build entire project**

```bash
cd /Users/giancarlogoulart/Projects/Personal/sportstips
go build ./...
```

Fix any compile errors (likely unused import or missing method).

- [ ] **Step 6: Run all tests**

```bash
go test ./... -v 2>&1 | tail -20
```

Expected: all PASS (10+ tests).

- [ ] **Step 7: Commit**

```bash
git add cmd/server/main.go internal/api/router.go internal/api/results.go \
        internal/config/config.go .env.example
git commit -m "feat: wire results ingester, admin sync endpoint, football-data fallback config"
```

---

## Task 7: Smoke Test — Phase 2

- [ ] **Step 1: Start server with real Odds API key**

Ensure `.env` has real `ODDS_API_KEY`. Start:
```bash
go run ./cmd/server/...
```

- [ ] **Step 2: Register + get token**

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"phase2@test.com","name":"Phase2","password":"test123"}' | jq -r .token)
echo "Token OK: ${TOKEN:0:10}..."
```

- [ ] **Step 3: Wait for scheduler to poll (or trigger manually via odds endpoint)**

```bash
sleep 10
curl -s http://localhost:8080/matches \
  -H "Authorization: Bearer $TOKEN" | jq 'length'
```

Expected: number > 0 if Odds API key is valid.

- [ ] **Step 4: Trigger results sync**

```bash
curl -s -X POST http://localhost:8080/admin/results/sync | jq .
```

Expected: `{"status":"ok"}`

Server logs should show: `"msg":"result saved"` for any completed matches found.

- [ ] **Step 5: Check signals are being stored**

```bash
curl -s "http://localhost:8080/signals?type=arbitrage" \
  -H "Authorization: Bearer $TOKEN" | jq 'length'
```

Expected: 0 or more (depends on live arbitrage opportunities).

- [ ] **Step 6: Final commit**

```bash
git add .
git commit -m "feat: phase 2 complete — results ingestion, per-tenant signal storage" --allow-empty
```

---

## Next Phase

- **Phase 3** (`docs/superpowers/plans/YYYY-MM-DD-phase3-ml-pipeline.md`): Python ML pipeline, feature engineering, Logistic Regression model, BatchPredictionService, value bet detection with Kelly
