package predictions

import (
	"context"
	"fmt"
)

type StubPredictionService struct{}

func (s *StubPredictionService) Predict(_ context.Context, matchID string) (Prediction, error) {
	return Prediction{}, fmt.Errorf("ml predictions not available (Phase 3): matchID=%s", matchID)
}
