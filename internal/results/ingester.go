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
func (ing *Ingester) Run(ctx context.Context) error {
	for _, sport := range ing.sports {
		scores, err := ing.fetchWithFallback(sport)
		if err != nil {
			ing.log.Error("fetch scores failed", "sport", sport, "err", err)
			continue
		}

		scoreMap := make(map[string]ScoreEvent, len(scores))
		for _, s := range scores {
			scoreMap[s.ExternalID] = s
		}

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
					"score_home", score.HomeScore,
					"score_away", score.AwayScore,
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
