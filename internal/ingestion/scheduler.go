package ingestion

import (
	"context"
	"log/slog"
	"time"

	"sportstips/internal/engine"
	"sportstips/internal/store"
)

type Scheduler struct {
	store    *store.Store
	primary  OddsClient
	fallback OddsClient
	sports   []string
	log      *slog.Logger
}

func NewScheduler(s *store.Store, primary, fallback OddsClient, sports []string, log *slog.Logger) *Scheduler {
	return &Scheduler{
		store:    s,
		primary:  primary,
		fallback: fallback,
		sports:   sports,
		log:      log,
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

			s.runEngine(ctx, dbMatch.ID)
		}
	}
}

func (s *Scheduler) runEngine(ctx context.Context, matchID string) {
	odds, err := s.store.GetLatestOddsByMatch(ctx, matchID)
	if err != nil {
		s.log.Error("get odds for engine", "matchID", matchID, "err", err)
		return
	}

	arbSignals := engine.DetectArbitrage(matchID, odds, 0.01)
	var signals []store.Signal
	for _, arb := range arbSignals {
		sig, err := arb.ToStoreSignal()
		if err != nil {
			s.log.Error("arb to signal", "err", err)
			continue
		}
		signals = append(signals, sig)
	}

	if len(signals) == 0 {
		return
	}

	// Phase 1: log only. signals table requires tenant_id (NOT NULL),
	// storage happens in Phase 2 when tenant preference loop runs.
	markets := make([]string, len(signals))
	for i, sig := range signals {
		markets[i] = sig.Market
	}
	s.log.Info("arbitrage found", "matchID", matchID, "count", len(signals), "markets", markets)
}

func (s *Scheduler) fetchWithFallback(sport string) ([]RawEvent, error) {
	events, err := s.primary.GetOdds(sport)
	if err != nil {
		s.log.Warn("primary source failed, trying fallback", "err", err)
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
