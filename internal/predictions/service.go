package predictions

import "context"

type Prediction struct {
	ProbHome     float64
	ProbDraw     float64
	ProbAway     float64
	ModelVersion string
}

type PredictionService interface {
	Predict(ctx context.Context, matchID string) (Prediction, error)
}
