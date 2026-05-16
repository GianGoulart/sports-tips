package ingestion

import (
	"context"
	"crypto/md5"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"sportstips/internal/alerts"
	"sportstips/internal/engine"
	"sportstips/internal/predictions"
	"sportstips/internal/store"
)

type Scheduler struct {
	store       *store.Store
	primary     OddsClient
	fallback    OddsClient
	sports      []string
	log         *slog.Logger
	predictions predictions.PredictionService
	alerter     alerts.Alerter

	mu       sync.Mutex
	oddsHash map[string]string // matchID → md5 of latest odds
}

func NewScheduler(s *store.Store, primary, fallback OddsClient, sports []string, log *slog.Logger, pred predictions.PredictionService, alerter alerts.Alerter) *Scheduler {
	return &Scheduler{
		store:       s,
		primary:     primary,
		fallback:    fallback,
		sports:      sports,
		log:         log,
		predictions: pred,
		alerter:     alerter,
		oddsHash:    make(map[string]string),
	}
}

// Run starts the polling loop. Blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	s.fetchAll(ctx)

	for {
		select {
		case <-ticker.C:
			s.fetchAll(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) fetchAll(ctx context.Context) {
	for _, sport := range s.sports {
		if !s.sportNeedsUpdate(ctx, sport) {
			continue
		}

		events, err := s.fetchWithFallback(sport)
		if err != nil {
			s.log.Error("fetch failed", "sport", sport, "err", err)
			continue
		}

		for _, event := range events {
			match := store.Match{
				ExternalID: event.ExternalID,
				Sport:      event.Sport,
				League:     event.League,
				HomeTeam:   event.HomeTeam,
				AwayTeam:   event.AwayTeam,
				StartsAt:   event.CommenceTime,
				Status:     inferStatus(event.CommenceTime),
			}

			if err := s.store.UpsertMatch(ctx, match); err != nil {
				s.log.Error("upsert match", "id", event.ExternalID, "err", err)
				continue
			}

			dbMatch, err := s.store.GetMatchByExternalID(ctx, event.ExternalID)
			if err != nil {
				s.log.Error("get match", "id", event.ExternalID, "err", err)
				continue
			}

			if !s.shouldFetch(dbMatch) {
				continue
			}

			if err := s.store.InsertOddsRaw(ctx, dbMatch.ID, "odds_api", event); err != nil {
				s.log.Error("insert raw", "err", err)
			}

			normalized := Normalize(dbMatch.ID, event)
			if err := s.store.InsertOddsNormalized(ctx, normalized); err != nil {
				s.log.Error("insert normalized", "err", err)
				continue
			}

			if s.oddsChanged(dbMatch.ID, event) {
				s.runEngine(ctx, dbMatch.ID)
			}
		}
	}
}

// sportNeedsUpdate returns true if any active match in the sport is due for a fetch.
// Skips the API call entirely when all matches were recently fetched.
func (s *Scheduler) sportNeedsUpdate(ctx context.Context, sport string) bool {
	matches, err := s.store.GetActiveMatchesBySport(ctx, sport)
	if err != nil {
		// Can't query DB — fetch anyway to be safe
		return true
	}
	// No known matches: fetch once to discover new ones
	if len(matches) == 0 {
		return true
	}
	for _, m := range matches {
		if s.shouldFetch(&m) {
			return true
		}
	}
	return false
}

// oddsChanged returns true if the odds for a match differ from the last seen hash.
// Prevents running the engine when bookmakers haven't updated their lines.
func (s *Scheduler) oddsChanged(matchID string, event RawEvent) bool {
	h := oddsFingerprint(event)
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.oddsHash[matchID]
	if !ok || prev != h {
		s.oddsHash[matchID] = h
		return true
	}
	return false
}

func oddsFingerprint(e RawEvent) string {
	h := md5.New()
	for _, bk := range e.Bookmakers {
		fmt.Fprintf(h, "%s", bk.Key)
		for _, m := range bk.Markets {
			fmt.Fprintf(h, "%s", m.Key)
			for _, o := range m.Outcomes {
				fmt.Fprintf(h, "%s%.4f", o.Name, o.Price)
			}
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

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

		// Send Telegram alerts if tenant has configured a chat ID
		if prefs.TelegramID != nil && *prefs.TelegramID != "" {
			for _, sig := range signals {
				msg := alerts.FormatSignal(sig.Type, sig.Market, matchID, sig.Data)
				if err := s.alerter.Send(*prefs.TelegramID, msg); err != nil {
					s.log.Warn("telegram alert failed", "tenantID", tenant.ID, "err", err)
				}
			}
		}
	}
}

func (s *Scheduler) fetchWithFallback(sport string) ([]RawEvent, error) {
	events, err := s.primary.GetOdds(sport)
	if err != nil {
		s.log.Warn("primary source failed, trying fallback", "err", err)
		if s.fallback == nil {
			return nil, fmt.Errorf("primary failed and no fallback configured: %w", err)
		}
		return s.fallback.GetOdds(sport)
	}
	return events, nil
}

func (s *Scheduler) shouldFetch(m *store.Match) bool {
	if m.LastFetched == nil {
		return true
	}
	return time.Since(*m.LastFetched) >= s.pollInterval(m)
}

func (s *Scheduler) pollInterval(m *store.Match) time.Duration {
	now := time.Now()
	if m.Status == "live" {
		return 2 * time.Minute
	}
	untilStart := m.StartsAt.Sub(now)
	switch {
	case untilStart < time.Hour:
		return 5 * time.Minute
	case untilStart < 24*time.Hour:
		return 15 * time.Minute
	default:
		return 60 * time.Minute
	}
}

func inferStatus(commenceTime time.Time) string {
	now := time.Now()
	if commenceTime.After(now) {
		return "upcoming"
	}
	if commenceTime.After(now.Add(-2 * time.Hour)) {
		return "live"
	}
	return "finished"
}
