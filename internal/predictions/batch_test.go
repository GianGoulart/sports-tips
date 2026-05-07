package predictions

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestBatchPredictionService_ImplementsInterface(t *testing.T) {
	var _ PredictionService = (*BatchPredictionService)(nil)
	assert.True(t, true)
}
