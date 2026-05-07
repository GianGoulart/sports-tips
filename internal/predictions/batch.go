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
